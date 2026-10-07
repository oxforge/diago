// Package route draws every edge of a placed level (S8) in the engine's
// top-to-bottom frame: ports on the node faces, side attachments, one x
// stop per layer, jogs packed into lanes in the channels between layers,
// and self-loops. Crossings finds where the finished routes cross, once
// the frame (S11) has put them in the output frame.
package route

import (
	"context"
	"math"

	"github.com/oxforge/diago/internal/layout/layered/lgraph"
	"github.com/oxforge/diago/internal/layout/layered/ports"
	"github.com/oxforge/diago/internal/layoutdbg"
	"github.com/oxforge/diago/internal/model"
)

// Options carries S8's values from the Config (S14).
type Options struct {
	Clearance float64           // the node clearance: a side column keeps it from the other vertices of its row
	Span      float64           // the straighten span: a jog this short is closed by moving a stop, not drawn
	Reach     float64           // a side column's and the innermost self-loop's distance outside its node
	BaseGap   float64           // a channel's height without lanes
	LaneGap   float64           // between two lanes of one channel
	InLaneGap float64           // between two jogs that share a lane, and between nested self-loops
	Snap      float64           // stops this close count as one x
	Dir       model.Direction   // the output direction, for the face spans and the drawn outlines
	Text      bool              // the text profile, where every shape is a box
	Side      [][2]ports.Vertex // replay: exactly the ends with Left or Right attach at that side, a side face or a pinned node's side vertex; nil decides afresh
	Fans      [][]int           // per congruence (S12, Rule C): its source's edges into the members, whose spread runs pair up by rank from both ends of the row, each pair on one lane unless it cannot agree (S8)
}

// Result is a routed level in the engine's frame.
type Result struct {
	RowTop []float64         // per layer: the top of its row, where every node's box starts
	RowH   []float64         // per layer: the tallest node's flow extent
	Lanes  []int             // per channel (between layers c and c + 1): its lane count
	Edges  [][]model.Point   // per level edge: its route from its From node to its To node
	Side   [][2]ports.Vertex // per edge and chain end (ports.Head, ports.Tail): the side it attaches at, a side face or a pinned node's side vertex, else ports.Bottom
}

// attachment is a chain end that leaves or enters its node at a side: a
// diamond's or circle's side vertex, or a side face.
type attachment struct {
	side    ports.Vertex // ports.Left or ports.Right
	vx      float64      // the column the wire runs along beside the node
	rect    bool         // on a non-pinned node's side face
	aim     float64      // the x its column runs over when that is farther out than Reach
	partner bool         // aims over its one-hop partner end's column, which attached first
}

type router struct {
	g        *lgraph.Graph
	x        []float64
	o        Options
	ranks    [][2]ports.Rank
	loops    [][]int // per node: its self-loops, in edge order
	port     [][2]float64
	attach   [][2]*attachment
	taken    [][2]int // per node: the edge (plus one) holding its left and right side, 0 when free
	wires    []wire   // per edge; empty for a self-loop
	claims   [][]claim
	loopAt   []loopJog // self-loops on pinned nodes, which jog in the channel above their node
	loopWide [][2]bool // per node: whether a pinned self-loop reaches Reach out on its left/right
	lanes    laneMap
	rowTop   []float64
	rowH     []float64
	nLanes   []int
}

// Route draws every edge of g, whose vertices sit at the cross-axis
// centers x (S7), and returns the routes and the rows (S8).
func Route(ctx context.Context, g *lgraph.Graph, x []float64, o Options) *Result {
	r := &router{g: g, x: x, o: o}
	r.init()
	r.assignPorts(ctx)
	r.loopSides()
	r.attachRects()
	r.columns()
	r.buildWires()
	r.deconflict(ctx)
	r.straighten()
	r.slideEnds()
	r.settle()
	r.packLanes()
	r.rows()
	res := &Result{RowTop: r.rowTop, RowH: r.rowH, Lanes: r.nLanes, Edges: r.emit(), Side: make([][2]ports.Vertex, len(g.Level.Edges))}
	for e := range g.Level.Edges {
		for end := range 2 {
			if a := r.attach[e][end]; a != nil {
				res.Side[e][end] = a.side
				layoutdbg.Decision(ctx, "side_attached",
					"phase", "route", "module", "diago", "spec_ref", "S8",
					"edge", g.Level.Edges[e].ID, "end", end, "side", sideName(a.side), "face", a.rect)
			}
		}
	}
	layoutdbg.Decision(ctx, "routed",
		"phase", "route", "module", "diago", "spec_ref", "S8",
		"edges", len(g.Level.Edges), "channels", len(r.nLanes), "lanes", sum(r.nLanes), "replay", o.Side != nil)
	return res
}

func sideName(v ports.Vertex) string {
	if v == ports.Left {
		return "left"
	}
	return "right"
}

func sum(xs []int) int {
	n := 0
	for _, v := range xs {
		n += v
	}
	return n
}

func (r *router) init() {
	g := r.g
	n := len(g.Level.Edges)
	r.ranks = ports.Ranks(g, ports.Off(r.o.Side))
	r.loops = make([][]int, len(g.Level.Nodes))
	for e := range g.Level.Edges {
		if g.Level.SelfLoop(e) {
			v := g.Level.Edges[e].From
			r.loops[v] = append(r.loops[v], e)
		}
	}
	r.port = make([][2]float64, n)
	r.attach = make([][2]*attachment, n)
	r.taken = make([][2]int, len(g.Level.Nodes))
	r.wires = make([]wire, n)
	r.claims = make([][]claim, max(0, len(g.Layers)-1))
	r.loopWide = make([][2]bool, len(g.Level.Nodes))
}

// node returns level node v's record: its shape and frame extents.
func (r *router) node(v int) lgraph.Node { return r.g.Level.Nodes[v] }

// pinned reports whether vertex v is a diamond or a circle outside the
// text profile (C8).
func (r *router) pinned(v int) bool {
	return !r.g.Dummy(v) && ports.Pinned(r.node(v).Shape, r.o.Text)
}

func (r *router) reversed(e int) bool { return r.g.Reversed != nil && r.g.Reversed[e] }

// faces returns node v's in- and out-face spans (S8, Shape ports).
func (r *router) faces(v int) (in, out ports.Span) {
	n := r.node(v)
	return ports.Faces(n.Shape, n.W, n.H, r.o.Dir, r.o.Text)
}

// inset is how far inside node v's box its drawn outline lies on face f at
// offset off from the face's middle.
func (r *router) inset(v int, f ports.Face, off float64) float64 {
	n := r.node(v)
	return ports.Inset(n.Shape, n.W, n.H, r.o.Dir, r.o.Text, f, off)
}

// rows sets each layer's row: its height is its tallest node, and a
// channel of BaseGap plus a LaneGap per lane separates it from the next.
// A channel beside a row of terminals that holds no lane closes (S9): its
// wires run straight on across the group's padding.
func (r *router) rows() {
	g := r.g
	r.rowH = make([]float64, len(g.Layers))
	for _, v := range g.Vertices {
		if v.Node >= 0 {
			r.rowH[v.Layer] = math.Max(r.rowH[v.Layer], g.Level.Nodes[v.Node].H)
		}
	}
	r.rowTop = make([]float64, len(g.Layers))
	for l := 1; l < len(g.Layers); l++ {
		gap := r.o.BaseGap + float64(r.nLanes[l-1])*r.o.LaneGap
		if r.nLanes[l-1] == 0 && (r.terminals(l-1) || r.terminals(l)) {
			gap = 0
		}
		r.rowTop[l] = r.rowTop[l-1] + r.rowH[l-1] + gap
	}
}

// terminals reports whether layer l holds terminals only (S9).
func (r *router) terminals(l int) bool {
	for _, v := range r.g.Layers[l] {
		if n := r.g.Vertices[v].Node; n < 0 || !r.g.Level.Nodes[n].Terminal {
			return false
		}
	}
	return len(r.g.Layers[l]) > 0
}

// laneY is the height of lane k of channel c, whose lanes are centered in
// it: a channel's first lane sits (BaseGap + LaneGap) / 2 below its row.
// Channel -1 lies above layer 0 and stacks its lanes upward.
func (r *router) laneY(c, k int) float64 {
	pad := (r.o.BaseGap + r.o.LaneGap) / 2
	if c < 0 {
		return -pad - float64(k)*r.o.LaneGap
	}
	total := r.nLanes[c]
	height := r.o.BaseGap + float64(total)*r.o.LaneGap
	return r.rowTop[c] + r.rowH[c] + (height-float64(total-1)*r.o.LaneGap)/2 + float64(k)*r.o.LaneGap
}

// box returns node v's box: its left, top, width and height. Every node of
// a row starts at the row's top (S4, S8).
func (r *router) box(v int) (left, top, w, h float64) {
	n := r.node(v)
	return r.x[v] - n.W/2, r.rowTop[r.g.Vertices[v].Layer], n.W, n.H
}
