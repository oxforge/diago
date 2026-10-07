// Package place computes every vertex's cross-axis position (S7).
package place

import (
	"cmp"
	"context"
	"math"
	"slices"

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
// rail counts as on its column within the same tolerance (sideExits), and
// a node of a row as clear of a side column at the node clearance
// (blocking).
const realizedTolerance = 1e-6

// Place returns every vertex's cross-axis center (S7). Act 1 builds a
// balanced reference by weighted barycenter sweeps, each layer projected
// exactly onto its minimum separations. Act 2 aligns near-straight hops
// into rigid blocks and compacts them: once as the baseline, and again
// with the side-exit columns of diamonds forced where the port an exit
// ends on has not reached its column (short of a lone column, off a side
// rail), and a diamond's primary exit forced straight under it, or,
// without one, its lone one-hop bottom exit (the bottom clause, which
// gives way to the diamond's sides where they cannot stand together), the
// forcing decided again on each rerun's result until a round forces no
// new vertex and adds no room. A diamond with a back edge is forced in
// the second pass only, from the router's replay. A diamond whose row
// blocks its secondary's column gives the secondary room: every node of
// the row that blocks it keeps the node clearance beyond its port. A
// diamond whose room does not hold is denied its primary, and given it
// back when the denial does not clear the row, each restarting the rounds
// from the baseline, every diamond one round finds so in one restart.
// Then the level's parts close up toward its weighted center.
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

// realized reports whether forced exit f's first hop lands where its
// forcing puts it on x, within realizedTolerance.
func (p *placer) realized(x []float64, f forcedExit) bool {
	c := p.g.Chains[f.edge]
	delta := f.offset - p.offset(c[1], f.edge, ports.Tail)
	return math.Abs(x[c[0]]+delta-x[c[1]]) <= realizedTolerance
}

// pack moves the level's parts, the vertices chain hops join, rigidly
// toward the level's weighted center under the minimum separations between
// parts and the rooms between them (S7, *Packing*): act 1's reference
// drifts a part that shares no edge with the rest, and act 2 keeps the
// gap. Each part wants the shift that puts its weighted mean on the
// center, with the sum of its members' weights; parts are numbered by
// their first vertex in layer order, which solve's ties go by. A level
// with one part, or whose parts interleave (no one left-to-right order of
// them agrees with every layer and every room that holds), keeps x.
func (p *placer) pack(ctx context.Context, x []float64, rooms []room) {
	g := p.g
	part, n := p.parts()
	if n < 2 {
		return
	}
	var cons []sepCon
	succ := make([][]int, n)
	for _, layer := range g.Layers {
		for i := 1; i < len(layer); i++ {
			a, b := layer[i-1], layer[i]
			if part[a] == part[b] {
				continue
			}
			cons = append(cons, sepCon{left: part[a], right: part[b], gap: p.sep(a, b) - (x[b] - x[a])})
			succ[part[a]] = append(succ[part[a]], part[b])
		}
	}
	for _, r := range rooms {
		a, b, gap := p.space.roomSep(g, r)
		if part[a] == part[b] || x[b]-x[a] < gap-realizedTolerance {
			continue // a part moves rigidly; a room that gave way is no room
		}
		cons = append(cons, sepCon{left: part[a], right: part[b], gap: gap - (x[b] - x[a])})
		succ[part[a]] = append(succ[part[a]], part[b])
	}
	if !acyclic(succ) {
		layoutdbg.Decision(ctx, "parts_packed",
			"phase", "place", "module", "diago", "spec_ref", "S7",
			"parts", n, "interleaved", true, "moved", 0, "shift", 0.0)
		return
	}
	sum, weight := make([]float64, n), make([]float64, n)
	center, total := 0.0, 0.0
	for v := range g.Vertices {
		w := p.weight[v]
		sum[part[v]] += w * x[v]
		weight[part[v]] += w
		center += w * x[v]
		total += w
	}
	center /= total
	vars := make([]sepVar, n)
	for c := range vars {
		vars[c] = sepVar{desired: center - sum[c]/weight[c], weight: weight[c]}
	}
	shift := solve(vars, cons)
	moved, most := 0, 0.0
	for _, s := range shift {
		if math.Abs(s) > eps {
			moved++
			most = max(most, math.Abs(s))
		}
	}
	for v := range x {
		if s := shift[part[v]]; math.Abs(s) > eps {
			x[v] = clean(x[v] + s)
		}
	}
	layoutdbg.Decision(ctx, "parts_packed",
		"phase", "place", "module", "diago", "spec_ref", "S7",
		"parts", n, "interleaved", false, "moved", moved, "shift", most)
}

// parts returns each vertex's part, the hop-connected component it lies
// in, and the number of parts, numbered by their first vertex, layer by
// layer from the top and each layer from the left (S7, *Packing*).
func (p *placer) parts() ([]int, int) {
	g := p.g
	root := make([]int, len(g.Vertices))
	for v := range root {
		root[v] = v
	}
	find := func(v int) int {
		for root[v] != v {
			root[v] = root[root[v]]
			v = root[v]
		}
		return v
	}
	for _, h := range g.Hops() {
		root[find(h.Upper)] = find(h.Lower)
	}
	number := make([]int, len(g.Vertices))
	for v := range number {
		number[v] = -1
	}
	part := make([]int, len(g.Vertices))
	n := 0
	for _, layer := range g.Layers {
		for _, v := range layer {
			r := find(v)
			if number[r] < 0 {
				number[r] = n
				n++
			}
			part[v] = number[r]
		}
	}
	return part, n
}

// acyclic reports whether the order succ gives, each entry's successors,
// has no cycle.
func acyclic(succ [][]int) bool {
	indeg := make([]int, len(succ))
	for _, next := range succ {
		for _, c := range next {
			indeg[c]++
		}
	}
	var queue []int
	for c, d := range indeg {
		if d == 0 {
			queue = append(queue, c)
		}
	}
	for h := 0; h < len(queue); h++ {
		for _, c := range succ[queue[h]] {
			if indeg[c]--; indeg[c] == 0 {
				queue = append(queue, c)
			}
		}
	}
	return len(queue) == len(succ)
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

// primaryClause is where a diamond stands with S7's primary clause: held,
// the clause applies; denied, its row blocks its secondary's column, and
// the exit rule decides; restored, the denial did not clear the row, and
// the clause applies for good.
type primaryClause uint8

const (
	primaryHeld primaryClause = iota
	primaryDenied
	primaryRestored
)

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
		// The Reversed check matters now: a reversed edge's node ends are
		// counter-flow and get no link (the continue above), but its
		// terminal ends (S9) stay on their face and do get one, so
		// without this skip a reversed edge's terminal-to-real hop would
		// fall into the same exemption as a forward edge's.
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

// sep is the minimum distance between the centers of neighbors a and b,
// a left of b: the gap, or the room their side columns and loops need
// (S7).
func (p *placer) sep(a, b int) float64 {
	return p.space.sep(p.g, a, b)
}

// reference is act 1 (S7): each layer starts packed from 0; then 6 sweeps
// alternate downward and upward, 3 equilibrium rounds pull from both
// sides, and a final upward pass centers parents over their children.
func (p *placer) reference() []float64 {
	g := p.g
	x := make([]float64, len(g.Vertices))
	for _, layer := range g.Layers {
		cursor := 0.0
		for _, v := range layer {
			w := g.Vertices[v].W
			x[v] = cursor + w/2
			cursor += w + p.o.Gap
		}
	}
	for s := range sweeps {
		if s%2 == 0 {
			for l := 1; l < len(g.Layers); l++ {
				p.project(x, l, p.up)
			}
		} else {
			for l := len(g.Layers) - 2; l >= 0; l-- {
				p.project(x, l, p.down)
			}
		}
	}
	both := make([][]pull, len(g.Vertices))
	for v := range both {
		both[v] = slices.Concat(p.up[v], p.down[v])
	}
	for range equilibriumRounds {
		for l := 1; l < len(g.Layers); l++ {
			p.project(x, l, both)
		}
		for l := len(g.Layers) - 2; l >= 0; l-- {
			p.project(x, l, both)
		}
	}
	for l := len(g.Layers) - 2; l >= 0; l-- {
		p.project(x, l, p.down)
	}
	return x
}

// project moves layer l to the weighted isotonic fit of its targets: each
// vertex's target is the mean of x[n] + delta over its pulls in nbrs, and
// a vertex without pulls holds its place with a weight of 0.01. A vertex
// without neighbors on either side takes no part in the fit: it rides
// with the nearest vertex before it that has neighbors, or, with none
// before it, the nearest after it, at the minimum separations from it; a
// layer without such a vertex holds its place.
func (p *placer) project(x []float64, l int, nbrs [][]pull) {
	layer := p.g.Layers[l]
	if len(layer) == 0 {
		return
	}
	var z, w []float64
	var fitted []int // the indices in layer of the vertices the fit places
	off := make([]float64, len(layer))
	for i, v := range layer {
		if i > 0 {
			off[i] = off[i-1] + p.sep(layer[i-1], v)
		}
		if p.alone(v) {
			continue
		}
		target, weight := x[v], idleWeight
		if ns := nbrs[v]; len(ns) > 0 {
			sum := 0.0
			for _, n := range ns {
				sum += x[n.node] + n.delta
			}
			target, weight = sum/float64(len(ns)), p.weight[v]
		}
		z, w, fitted = append(z, target-off[i]), append(w, weight), append(fitted, i)
	}
	if len(fitted) == 0 {
		return
	}
	u := make([]float64, len(layer))
	for k, fit := range isotonic(z, w) {
		u[fitted[k]] = fit
	}
	// each vertex left out takes the fit of the one it rides with
	next := 0
	for i := range layer {
		if next < len(fitted) && fitted[next] == i {
			next++
			continue
		}
		u[i] = u[fitted[max(next-1, 0)]]
	}
	for i, v := range layer {
		x[v] = u[i] + off[i]
	}
}

// alone reports whether vertex v has no neighbor on either side: an
// isolated node (S3), or one whose only edges are self-loops.
func (p *placer) alone(v int) bool {
	return len(p.up[v]) == 0 && len(p.down[v]) == 0
}

// isotonic is the weighted nondecreasing fit of values
// (pool-adjacent-violators): a violating run pools at its weighted mean.
func isotonic(values, weights []float64) []float64 {
	type pool struct {
		sum, weight float64
		count       int
	}
	var pools []pool
	for i, v := range values {
		b := pool{v * weights[i], weights[i], 1}
		for len(pools) > 0 {
			prev := pools[len(pools)-1]
			if prev.sum/prev.weight <= b.sum/b.weight {
				break
			}
			pools = pools[:len(pools)-1]
			b = pool{prev.sum + b.sum, prev.weight + b.weight, prev.count + b.count}
		}
		pools = append(pools, b)
	}
	out := make([]float64, 0, len(values))
	for _, b := range pools {
		for range b.count {
			out = append(out, b.sum/b.weight)
		}
	}
	return out
}

// forcedExit is an exit whose first hop must put its next vertex under a
// side column, or, for a primary exit or a bottom clause's, under the
// bottom vertex: offset is the column's distance from the node's center,
// 0 under the bottom vertex, and round the recheck that forced it, 0 for
// the baseline.
type forcedExit struct {
	edge    int
	offset  float64
	round   int
	primary bool
	bottom  bool
}

// exitRound is one round of S7's side-exit forcing: the forcings kept and
// added, in edge order, and the rooms kept and added, the added ones after
// the kept ones; or the changes of diamonds' primary clauses, in vertex
// order, which restart the rounds once for all of them.
type exitRound struct {
	forced  []forcedExit
	rooms   []room
	changes []clauseChange
}

// room keeps node other, of diamond's row, the node clearance beyond the
// port the diamond's secondary exit edge ends on, at its next vertex next,
// on the side sign points to (S7): port is next's in-port offset for edge.
type room struct {
	edge, diamond, next, other int
	sign, port                 float64
}

// clauseChange denies diamond node its primary, or gives it back when
// restore is set, for the exit edge whose column its row blocks (S7).
type clauseChange struct {
	node, edge int
	restore    bool
}

// sideExits predicts, against the layout base, which exits of each pinned
// node the router draws from a side vertex, and returns kept, the
// forcings of earlier rounds, with the exits of this round added (S7): on
// each side vertex no forcing holds yet, the exit heading farthest out
// when the port its first hop ends on (its next vertex's center plus that
// vertex's in-port offset) has not reached the side column, W/2 + Reach
// from the center. A port at or beyond the column has reached it, since
// the router moves a lone column out over it where the row lets it; a
// side vertex that two or more exits leave from is a side rail, which the
// router never moves out, so there only a port on the column has. Every
// exit heads for that port. The vertices come from the exit rule
// (ports.Exits), and in the second pass from the router's replay
// (Options.Side): the recorded side vertex, the bottom for the others. A
// diamond with two exits of which exactly one leads on takes them by the
// primary clause instead: the primary on the bottom, forced straight
// under it while its port lies off the axis, and the other on the side
// its port lies of the primary's, judged only in a round after the
// primary's forcing. A secondary judged so whose port lies beyond its
// column where nodes of the row block the column (blocking) gets room,
// when its primary is realized: every one of them keeps the node
// clearance beyond the port, which act 2 holds from the next rerun on,
// among the rooms the round returns. In the first pass, a secondary still
// blocked so with its room kept (the room gave way, or another node now
// blocks), or one whose primary's forcing dropped, denies its diamond the
// primary; once the rounds
// settle with no denial and no new room, a denied diamond whose exit that
// was the secondary still leaves a side the row blocks the same way gets
// it back. The round returns every denial it finds, or, settled, every
// give-back, as its changes. The second pass gives room too, but denies
// and gives back nothing.
// A diamond without a primary whose vertices put a lone exit on its
// bottom vertex, one hop long, forces it straight under it, by the bottom
// clause, unless it was denied its primary or the clause gave way. A node with a single exit gets none: the
// router settles those from the realized coordinates; nor does a node with
// a counter-flow end in the first pass, before the router has attached
// its back edge at a side; in the second pass it takes no primary clause,
// and the side vertex its back edge holds forces nothing. Nor does a
// circle: its exits all leave its bottom vertex, a merged fan-out (S8).
func (p *placer) sideExits(base []float64, kept []forcedExit, keptRooms []room, round int) exitRound {
	g := p.g
	counter := make([]bool, len(g.Vertices))
	backSide := make([][2]bool, len(g.Vertices)) // per vertex, left and right: a counter-flow end the replay attached there
	leads := make([]bool, len(g.Vertices))       // per vertex: a forward chain leaves it
	exits := make([][]ports.Exit, len(g.Vertices))
	for e, chain := range g.Chains {
		if len(chain) < 2 {
			continue
		}
		if g.Reversed != nil && g.Reversed[e] {
			for which, v := range [2]int{chain[0], chain[len(chain)-1]} {
				if !p.pinned(v) {
					continue
				}
				counter[v] = true
				if p.o.Side == nil {
					continue
				}
				switch p.o.Side[e][which] {
				case ports.Left:
					backSide[v][0] = true
				case ports.Right:
					backSide[v][1] = true
				}
			}
			continue
		}
		leads[chain[0]] = true
		if p.pinned(chain[0]) {
			next := chain[1]
			// the port its first hop ends on (S8, Shape ports), its group's
			// anchor on a group (S9)
			port := base[next] + p.offset(next, e, ports.Tail)
			exits[chain[0]] = append(exits[chain[0]], ports.Exit{Edge: e, Toward: port})
		}
	}
	held := make([][2]bool, len(g.Vertices))  // per vertex, left and right: a side a kept forcing holds
	straight := make([]bool, len(g.Vertices)) // per vertex: a kept forcing holds its primary
	for _, f := range kept {
		v := g.Chains[f.edge][0]
		switch {
		case f.primary || f.bottom:
			straight[v] = true
		case f.offset > 0:
			held[v][1] = true
		default:
			held[v][0] = true
		}
	}
	band := max(p.o.Clearance, p.o.Span)
	forced, rooms := slices.Clone(kept), slices.Clone(keptRooms)
	roomed := map[int]bool{} // per edge: a kept room holds it
	for _, r := range keptRooms {
		roomed[r.edge] = true
	}
	var denials, restores []clauseChange // restores: denied diamonds whose denials did not clear their rows
	for v, ex := range exits {
		// a counter-flow end's side vertex is known in the second pass only
		if len(ex) < 2 || (counter[v] && p.o.Side == nil) || ports.Merged(g.Level.Nodes[g.Vertices[v].Node].Shape) {
			continue
		}
		cx, column := base[v], g.Vertices[v].W/2+p.o.Reach
		sides := ports.Exits(cx, band, ex)
		if p.o.Side != nil {
			for i := range ex {
				sides[i] = p.o.Side[ex[i].Edge][ports.Head] // the router's replay (S8)
			}
		}
		prim, sec := p.primary(ex, leads, sides), -1
		if counter[v] {
			prim = -1 // its back edge claims a side first: no main line
		}
		if prim >= 0 && p.clause[v] == primaryDenied {
			sec, prim = 1-prim, -1 // the exit that was the secondary
		}
		if prim >= 0 {
			sec = 1 - prim
			sides[prim], sides[sec] = ports.Bottom, ports.Left
			if ex[sec].Toward > ex[prim].Toward {
				sides[sec] = ports.Right
			}
			if !straight[v] && math.Abs(ex[prim].Toward-cx) > realizedTolerance {
				forced = append(forced, forcedExit{edge: ex[prim].Edge, round: round, primary: true})
				continue // its sides wait for the rerun that puts the primary straight
			}
		}
		// the bottom clause: no primary, nor one denied, and a lone exit on
		// the bottom vertex whose first hop ends at its node
		if b := lone(sides, ports.Bottom); prim < 0 && sec < 0 && b >= 0 && len(g.Chains[ex[b].Edge]) == 2 &&
			!straight[v] && !p.bottomless[v] && math.Abs(ex[b].Toward-cx) > realizedTolerance {
			forced = append(forced, forcedExit{edge: ex[b].Edge, round: round, bottom: true})
			continue // its sides wait for the rerun that puts it straight
		}
		for k, side := range [2]ports.Vertex{ports.Left, ports.Right} {
			if held[v][k] || backSide[v][k] {
				continue // a side a forcing holds, or the back edge's column
			}
			sign := -1.0
			if side == ports.Right {
				sign = 1
			}
			best, rail := -1, 0
			for i := range ex {
				if sides[i] != side {
					continue
				}
				rail++
				if best < 0 || sign*(ex[i].Toward-ex[best].Toward) > 0 {
					best = i
				}
			}
			if best < 0 {
				continue
			}
			beyond := sign * (ex[best].Toward - (cx + sign*column))
			reached := beyond >= 0
			if rail > 1 {
				reached = math.Abs(beyond) <= realizedTolerance
			}
			if reached && best == sec && rail == 1 {
				if blocking := p.blocking(base, v, sign, ex[best].Toward); len(blocking) > 0 {
					e := ex[sec].Edge
					clauseHeld := prim >= 0 && straight[v] && p.clause[v] == primaryHeld
					// a room only beside a realized primary: its next vertex
					// keeps the node gap from the secondary's
					realized := clauseHeld && math.Abs(ex[prim].Toward-cx) <= realizedTolerance
					switch {
					case realized && !roomed[e]:
						next := g.Chains[e][1]
						for _, o := range blocking {
							rooms = append(rooms, room{edge: e, diamond: v, next: next, other: o, sign: sign, port: p.offset(next, e, ports.Tail)})
						}
					case p.o.Side != nil: // the second pass denies nothing
					case clauseHeld:
						denials = append(denials, clauseChange{node: v, edge: e})
					case prim < 0:
						restores = append(restores, clauseChange{node: v, edge: e, restore: true})
					}
				}
			}
			if reached {
				continue
			}
			forced = append(forced, forcedExit{edge: ex[best].Edge, offset: sign * column, round: round})
		}
	}
	if len(denials) > 0 {
		return exitRound{changes: denials}
	}
	if len(restores) > 0 && len(forced) == len(kept) && len(rooms) == len(keptRooms) {
		return exitRound{changes: restores}
	}
	slices.SortFunc(forced, func(a, b forcedExit) int { return cmp.Compare(a.edge, b.edge) })
	return exitRound{forced: forced, rooms: rooms}
}

// lone is the index of the only exit sides puts on vertex at, or -1 when
// none or several do.
func lone(sides []ports.Vertex, at ports.Vertex) int {
	found := -1
	for i, side := range sides {
		if side != at {
			continue
		}
		if found >= 0 {
			return -1
		}
		found = i
	}
	return found
}

// blocking lists, in layer order, the nodes of pinned vertex v's row that
// keep the router from moving v's side column, on the side sign points
// to, out over x (S8, Side attachments): the column at x, or its stub from
// v's side vertex, would come within the node clearance of the node's box
// by more than realizedTolerance, its own farthest side column or loop leg
// included as reaches predicts it. In the first pass a pinned node's loops
// run on its right there, though the router puts a loop on the left when
// another end holds the right and the left is free (S8, Self-loops). A
// dummy does not block: a port gives way from its wire and takes the
// column along (S8, Ports).
func (p *placer) blocking(base []float64, v int, sign, x float64) []int {
	g := p.g
	vertex := base[v] + sign*g.Vertices[v].W/2
	lo, hi := math.Min(x, vertex), math.Max(x, vertex)
	var out []int
	for _, o := range g.Layers[g.Vertices[v].Layer] {
		if o == v || g.Dummy(o) {
			continue
		}
		half := g.Vertices[o].W / 2
		left := base[o] - half - p.space.out[o][0] - p.o.Clearance
		right := base[o] + half + p.space.out[o][1] + p.o.Clearance
		if lo < right-realizedTolerance && hi > left+realizedTolerance {
			out = append(out, o)
		}
	}
	return out
}

// primary is the index of a diamond's primary exit among its two exits ex
// (S7, Side-exit forcing), or -1: the one of the two whose edge ends at a
// node that leads on, when exactly one does. In the second pass the
// replay must record it on the bottom vertex and the other on a side;
// otherwise the diamond keeps its replayed vertices and has no primary.
func (p *placer) primary(ex []ports.Exit, leads []bool, sides []ports.Vertex) int {
	if len(ex) != 2 {
		return -1
	}
	on := func(i int) bool {
		c := p.g.Chains[ex[i].Edge]
		return leads[c[len(c)-1]]
	}
	if on(0) == on(1) {
		return -1
	}
	prim := 0
	if on(1) {
		prim = 1
	}
	if p.o.Side != nil && (sides[prim] != ports.Bottom || sides[1-prim] == ports.Bottom) {
		return -1
	}
	return prim
}

// withForced replaces each forced exit's first-hop link with a forced one
// that puts the next vertex under its column.
func (p *placer) withForced(forced []forcedExit) []link {
	skip := map[[3]int]bool{}
	for _, f := range forced {
		c := p.g.Chains[f.edge]
		skip[[3]int{f.edge, c[0], c[1]}] = true
	}
	var out []link
	for _, l := range p.links {
		if !skip[l.key()] {
			out = append(out, l)
		}
	}
	for _, f := range forced {
		c := p.g.Chains[f.edge]
		out = append(out, link{
			edge: f.edge, upper: c[0], lower: c[1],
			delta:  f.offset - p.offset(c[1], f.edge, ports.Tail),
			forced: true,
		})
	}
	return out
}
