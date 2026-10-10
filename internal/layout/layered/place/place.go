// Package place computes every vertex's cross-axis position (S7).
package place

import (
	"context"
	"math"

	"github.com/oxforge/diago/internal/layout/layered/lgraph"
	"github.com/oxforge/diago/internal/layout/layered/ports"
	"github.com/oxforge/diago/internal/layoutdbg"
	"github.com/oxforge/diago/internal/model"
)

// Options carries S7's values from the Config (S14).
type Options struct {
	Gap         float64           // clear space between neighbors in a layer (the node gap)
	TerminalGap float64           // the distance between the centers of two terminals of a row (S9): the terminal gap (S14)
	Span        float64           // the straighten span
	Reach       float64           // how far a side column sits outside a vertex
	Clearance   float64           // the node clearance; with Span, the band an exit heading must leave
	InLaneGap   float64           // the step between nested self-loop legs (S8), and between two facing side columns or loop legs
	Dir         model.Direction   // the resolved output direction, for the face spans
	Text        bool              // the text profile, where no shape is pinned
	Side        [][2]ports.Vertex // per edge and chain end, the side the router attached it at (S8), a side face or a pinned node's side vertex, ports.Bottom for none; nil for none
}

// Act 1's schedule and weights (S7).
const (
	sweeps            = 6
	equilibriumRounds = 3
	dummyWeight       = 8.0
	idleWeight        = 0.01
)

// realizedTolerance is how close a forced exit's residual must land to
// zero, on the final coordinates, to count as realized rather than
// dropped. It sits above the package's eps (1e-9): compact's clean()
// rounds to a billionth of a unit, and a realized forcing can carry a
// residue up to just under that grid (measured as high as 9.96e-10) that
// is rounding noise, not misalignment, while a genuinely rejected forcing
// measures orders of magnitude higher (from 0.046 up). A port on a side
// rail counts as on its column within the same tolerance (judgeSide), and
// a node of a row as clear of a side column at the node clearance
// (blocking).
const realizedTolerance = 1e-6

// Place returns every vertex's cross-axis center (S7): act 1's reference
// (reference), act 2's alignment once as the baseline (align), then the
// rounds of side-exit forcing (sideExits), each rerunning act 2 on the
// same reference, until a round forces no new vertex and adds no room. A
// round that denies a diamond its primary or gives one back restarts the
// rounds from the baseline. Last, the level's parts close up toward its
// weighted center (pack), and in the text profile every box moves onto
// the cell grid (cells).
func Place(ctx context.Context, g *lgraph.Graph, o Options) []float64 {
	if len(g.Vertices) == 0 {
		return nil
	}
	p := newPlacer(g, o)
	in := alignInput{g: g, space: p.space, span: o.Span, ref: p.reference(), links: p.links, weight: p.weight}
	baseline := align(in)
	x := baseline
	var forced, gaveWay []forcedExit
	var rooms []room
	restarts := 0
	// every round keeps the vertices already forced and the rooms already
	// kept and adds at least one of either, a diamond forces at most one
	// exit per vertex, and a secondary gets its room at most once; a
	// restart denies at least one diamond its primary or gives at least one
	// back, each at most once per diamond; so the loop ends
	for round := 0; ; round++ {
		if ctx.Err() != nil {
			break // out of time: the caller stops the layout
		}
		r := p.sideExits(x, forced, rooms, round)
		if len(r.changes) > 0 {
			for _, c := range r.changes {
				decision := "primary_denied"
				p.clause[c.node] = primaryDenied
				if c.restore {
					decision = "primary_restored"
					p.clause[c.node] = primaryRestored
				}
				layoutdbg.Decision(ctx, decision,
					"phase", "place", "module", "diago", "spec_ref", "S7",
					"node", g.Vertices[c.node].ID, "edge", g.Level.Edges[c.edge].ID, "round", round)
			}
			forced, rooms, in.links, in.rooms, x, round = nil, nil, p.links, nil, baseline, -1
			restarts++
			continue
		}
		if len(r.forced) == len(forced) && len(r.rooms) == len(rooms) {
			break
		}
		for _, rm := range r.rooms[len(rooms):] {
			layoutdbg.Decision(ctx, "secondary_room",
				"phase", "place", "module", "diago", "spec_ref", "S7",
				"node", g.Vertices[rm.diamond].ID, "edge", g.Level.Edges[rm.edge].ID, "blocking", g.Vertices[rm.other].ID, "round", round)
		}
		forced, rooms = r.forced, r.rooms
		in.links, in.rooms = p.withForced(forced), rooms
		x = align(in)
		// a bottom clause the rerun left unrealized gives way to its
		// diamond's sides, for good, and the rerun runs again without it
		// and without the side forcings judged beside it, which the next
		// round judges again
		since := map[int]int{} // per diamond that gives way: its bottom clause's round
		for _, f := range forced {
			if f.bottom && !p.realized(x, f) {
				v := g.Chains[f.edge][0]
				p.bottomless[v] = true
				since[v] = f.round
				gaveWay = append(gaveWay, f)
			}
		}
		kept := forced[:0:0]
		for _, f := range forced {
			if r, ok := since[g.Chains[f.edge][0]]; !ok || (!f.bottom && f.round < r) {
				kept = append(kept, f)
			}
		}
		if len(kept) < len(forced) {
			forced = kept
			in.links = p.withForced(forced)
			x = align(in)
		}
	}
	p.pack(ctx, x, rooms)
	if o.Text {
		cells(g, x)
	}
	dropped := len(gaveWay)
	for _, f := range gaveWay {
		layoutdbg.Decision(ctx, "side_exit_dropped",
			"phase", "place", "module", "diago", "spec_ref", "S7",
			"edge", g.Level.Edges[f.edge].ID, "offset", f.offset, "round", f.round, "primary", f.primary, "bottom", f.bottom)
	}
	for _, f := range forced {
		if p.realized(x, f) {
			layoutdbg.Decision(ctx, "side_exit_forced",
				"phase", "place", "module", "diago", "spec_ref", "S7",
				"edge", g.Level.Edges[f.edge].ID, "offset", f.offset, "round", f.round, "primary", f.primary, "bottom", f.bottom)
		} else {
			dropped++
			layoutdbg.Decision(ctx, "side_exit_dropped",
				"phase", "place", "module", "diago", "spec_ref", "S7",
				"edge", g.Level.Edges[f.edge].ID, "offset", f.offset, "round", f.round, "primary", f.primary, "bottom", f.bottom)
		}
	}
	layoutdbg.Decision(ctx, "placed",
		"phase", "place", "module", "diago", "spec_ref", "S7",
		"vertices", len(g.Vertices), "links", len(p.links), "forced", len(forced)-dropped, "dropped", dropped, "restarts", restarts)
	return x
}

// cells puts every vertex of a text layout on the character grid (S7):
// its box's low edge moves to the nearest cell boundary (a half cell
// up). Every extent is a whole number of cells, so the high edge lands on
// one too, and since every minimum separation between two low edges is a
// whole number of cells as well (the left box's extent plus the gap), none
// is lost.
func cells(g *lgraph.Graph, x []float64) {
	for v := range x {
		half := g.Vertices[v].W / 2
		x[v] = math.Floor(x[v]-half+0.5) + half
	}
}

// pull is a neighbor's pull on a vertex: the vertex wants x[node] + delta.
type pull struct {
	node  int
	delta float64
}

type placer struct {
	g        *lgraph.Graph
	o        Options
	ranks    [][2]ports.Rank
	up, down [][]pull // each vertex's neighbors in the layer above and below
	links    []link
	weight   []float64
	space    spacing
	clause   []primaryClause // per vertex: whether a diamond's primary clause holds (S7)
	// per vertex: a diamond whose bottom clause gave way to its sides (S7)
	bottomless []bool
}

func newPlacer(g *lgraph.Graph, o Options) *placer {
	p := &placer{
		g: g, o: o, ranks: ports.Ranks(g, ports.Off(o.Side)),
		up: make([][]pull, len(g.Vertices)), down: make([][]pull, len(g.Vertices)),
		clause: make([]primaryClause, len(g.Vertices)), bottomless: make([]bool, len(g.Vertices)),
	}
	for e, chain := range g.Chains {
		for i := 0; i+1 < len(chain); i++ {
			upper, lower := chain[i], chain[i+1]
			delta := p.offset(upper, e, ports.Head) - p.offset(lower, e, ports.Tail)
			intoUpper, intoLower := i == 0 && p.counterFlow(e, upper), i+2 == len(chain) && p.counterFlow(e, lower)
			// a hop into a counter-flow end on a node with faces is no
			// neighbor of that node in act 1: it does not pull the node,
			// while the hop's other vertex keeps its pull toward it (S7)
			if !intoLower || p.pinned(lower) {
				p.up[lower] = append(p.up[lower], pull{upper, delta})
			}
			if !intoUpper || p.pinned(upper) {
				p.down[upper] = append(p.down[upper], pull{lower, -delta})
			}
			if intoUpper || intoLower {
				continue // the router side-attaches this end: no port to align on
			}
			p.links = append(p.links, link{
				edge: e, upper: upper, lower: lower, delta: delta,
				exempt: p.slack(upper) && p.slack(lower),
			})
		}
	}
	p.exempt()
	p.space = spacing{gap: o.Gap, terminal: o.TerminalGap, clearance: o.Clearance, inLane: o.InLaneGap, text: o.Text, out: p.reaches()}
	p.weight = make([]float64, len(g.Vertices))
	for v := range g.Vertices {
		p.weight[v] = dummyWeight
		if !p.slack(v) {
			p.weight[v] = 1 + float64(len(p.up[v])+len(p.down[v]))
		}
	}
	return p
}

// offset is the port offset of edge e's end at vertex v: on the out-face
// for the chain's head, on the in-face for its tail. A dummy has none.
func (p *placer) offset(v, e, end int) float64 {
	n := p.g.Vertices[v].Node
	if n < 0 {
		return 0
	}
	nd := p.g.Level.Nodes[n]
	in, out := ports.Faces(nd.Shape, nd.W, nd.H, p.o.Dir, p.o.Text)
	if end == ports.Head {
		return p.ranks[e][ports.Head].Offset(out)
	}
	return p.ranks[e][ports.Tail].Offset(in)
}

func (p *placer) pinned(v int) bool {
	n := p.g.Vertices[v].Node
	return n >= 0 && ports.Pinned(p.g.Level.Nodes[n].Shape, p.o.Text)
}

// counterFlow reports whether edge e's end at vertex v is a reversed
// edge's end on a node, which the router attaches at a side (S8). A group
// or a terminal keeps it on its face (S9).
func (p *placer) counterFlow(e, v int) bool {
	return p.g.Reversed != nil && p.g.Reversed[e] && !p.g.Dummy(v) && !p.faced(v)
}

// faced reports whether vertex v is a group or a terminal, whose ends stay
// on its faces (S9).
func (p *placer) faced(v int) bool {
	n := p.g.Vertices[v].Node
	return n >= 0 && (p.g.Level.Nodes[n].Group || p.g.Level.Nodes[n].Terminal)
}

// slack reports whether vertex v is a dummy or a terminal: a point a wire
// passes, which place pulls as a dummy (S7, S9).
func (p *placer) slack(v int) bool {
	return p.g.Dummy(v) || terminal(p.g, v)
}

// exempt marks the links that are never a deliberate detour (S7): a
// chain's terminal hop to a real end that is not pinned, and an only child
// under an only parent, neither pinned, the parent a pinned node's branch
// or not. Hops between dummies or terminals are exempt from the start;
// reversed edges keep the span rule.
func (p *placer) exempt() {
	for i := range p.links {
		l := &p.links[i]
		// A reversed edge's node ends are counter-flow and get no link
		// (newPlacer), but its terminal ends (S9) stay on their face and
		// do get one, so without this skip a reversed edge's
		// terminal-to-real hop would fall into the same exemption as a
		// forward edge's.
		if l.exempt || (p.g.Reversed != nil && p.g.Reversed[l.edge]) {
			continue
		}
		upperDummy, lowerDummy := p.slack(l.upper), p.slack(l.lower)
		if upperDummy != lowerDummy {
			end := l.upper
			if upperDummy {
				end = l.lower
			}
			l.exempt = !p.pinned(end)
			continue
		}
		if p.pinned(l.upper) || p.pinned(l.lower) {
			continue
		}
		l.exempt = len(p.down[l.upper]) == 1 && len(p.up[l.lower]) == 1
	}
}

// reaches returns, per vertex, how far outside its left and right side
// its farthest side column or self-loop leg runs (S8), 0 for none: in the
// second pass each side column the router attached it at, the column Reach
// out on that side; a node with faces nests its self-loops on its right,
// the outermost leg farthest out; and each loop of a pinned node runs the
// column Reach out beside the side vertex it leaves, which the router picks
// from the side vertices the attachments hold.
func (p *placer) reaches() [][2]float64 {
	g, o := p.g, p.o
	out := make([][2]float64, len(g.Vertices))
	held := make([][2]bool, len(g.Vertices))
	column := ports.Reach(o.Reach, o.Text)
	for e, sides := range o.Side {
		chain := g.Chains[e]
		for which, side := range sides {
			if side == ports.Bottom || len(chain) < 2 {
				continue
			}
			v, k := chain[0], 0
			if which == ports.Tail {
				v = chain[len(chain)-1]
			}
			if side == ports.Right {
				k = 1
			}
			out[v][k] = max(out[v][k], column)
			held[v][k] = true
		}
	}
	for v, n := range g.Level.SelfLoops() {
		if n == 0 {
			continue
		}
		if !ports.Pinned(g.Level.Nodes[v].Shape, o.Text) {
			out[v][1] = ports.LoopOut(o.Reach, o.InLaneGap, 0, n, o.Text)
			continue
		}
		// each loop takes a side vertex as the router picks it, from the
		// sides the attachments and the loops before it hold
		for range n {
			k := 0
			if ports.LoopSide(held[v][0], held[v][1]) == ports.Right {
				k = 1
			}
			out[v][k] = max(out[v][k], o.Reach)
			held[v][k] = true
		}
	}
	return out
}

// alone reports whether vertex v has no neighbor on either side: an
// isolated node (S3), or one whose only edges are self-loops.
func (p *placer) alone(v int) bool {
	return len(p.up[v]) == 0 && len(p.down[v]) == 0
}
