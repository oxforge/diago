package flat

import (
	"math"
	"slices"

	"github.com/oxforge/diago/internal/layout/layered/ports"
	"github.com/oxforge/diago/internal/model"
)

// scene is the composed layout one flat edge is routed on, from its From
// node A to its To node B.
type scene struct {
	pg    *model.PositionedGraph
	o     Options
	id    string         // the flat edge's id
	a, b  int            // A and B, indices into pg.Nodes
	index map[string]int // node id to index, for lookups only
	kind  kind           // the edge's kind by C0, from A's and B's boxes
	free  []bool         // per group: whether it holds neither end, so that C6.1 binds it
	wires []int          // the other edges drawn so far, in edge order
	exits []exit         // A's forward exits drawn so far, when A is a diamond or a circle (C8.5)
}

// kind is an edge's kind by C0 in the engine's frame, downstream +y.
type kind int

const (
	lateral kind = iota // the two boxes overlap on the flow axis, by more than kindSlack
	forward             // the target's box lies wholly downstream of the source's
	back                // wholly upstream
)

// exit is a forward edge leaving a diamond or a circle, and the vertex it
// leaves from.
type exit struct {
	edge string
	at   ports.Face
}

// bounds is an axis-aligned box in the engine's frame.
type bounds struct{ l, t, r, b float64 }

// grow is r grown by d.X on the left and on the right, and by d.Y above
// and below.
func (r bounds) grow(d XY) bounds { return bounds{r.l - d.X, r.t - d.Y, r.r + d.X, r.b + d.Y} }

// newScene reads pg around its edge e, or says why the edge cannot be
// routed at all.
func newScene(pg *model.PositionedGraph, e int, o Options) (*scene, string) {
	if pg == nil || e < 0 || e >= len(pg.Edges) {
		return nil, "no such edge"
	}
	fe := pg.Edges[e]
	s := &scene{pg: pg, o: o, id: fe.ID, index: make(map[string]int, len(pg.Nodes))}
	for i, n := range pg.Nodes {
		if _, ok := s.index[n.ID]; !ok {
			s.index[n.ID] = i
		}
	}
	a, okA := s.index[fe.From]
	b, okB := s.index[fe.To]
	switch {
	case !okA || !okB:
		return nil, "an unknown node"
	case a == b:
		return nil, "a self-loop"
	}
	s.a, s.b = a, b
	s.kind = kindOf(s.box(a), s.box(b))
	s.free = holdsNeither(pg, fe.From, fe.To)
	for j, w := range pg.Edges {
		if j != e && len(w.Points) >= 2 {
			s.wires = append(s.wires, j)
		}
	}
	if s.pinned(a) {
		for _, j := range s.wires {
			w := pg.Edges[j]
			t, ok := s.index[w.To]
			if w.From != fe.From || !ok || t == a || kindOf(s.box(a), s.box(t)) != forward {
				continue
			}
			s.exits = append(s.exits, exit{edge: w.ID, at: s.vertex(a, w.Points[0])})
		}
	}
	return s, ""
}

// kindSlack is C0's tolerance on distance floors, which the kinds take
// (C0): two boxes that overlap on the flow axis by up to it lie wholly
// downstream or upstream of each other, as the contract has them, so that
// C8.2, C8.4 and C8.5 bind the edge where the contract applies them. In
// the text profile, where it counts cells, two boxes overlap by nothing
// or by at least half a cell, so it reads them as the contract does.
const kindSlack = 0.05

// kindOf is the kind (C0) of an edge from a node with box f to one with
// box t.
func kindOf(f, t bounds) kind {
	switch {
	case t.t >= f.b-kindSlack:
		return forward
	case t.b <= f.t+kindSlack:
		return back
	}
	return lateral
}

// holdsNeither reports, per group of pg, whether it holds neither of the
// nodes from and to, directly or through a group inside it (C0,
// Membership).
func holdsNeither(pg *model.PositionedGraph, from, to string) []bool {
	byID := make(map[string]int, len(pg.Groups))
	for i, g := range pg.Groups {
		byID[g.ID] = i
	}
	var holds func(g, depth int) bool
	holds = func(g, depth int) bool {
		gr := pg.Groups[g]
		if slices.Contains(gr.Contains, from) || slices.Contains(gr.Contains, to) {
			return true
		}
		if depth > len(pg.Groups) {
			return false // a malformed child cycle
		}
		for _, ch := range gr.Children {
			if c, ok := byID[ch]; ok && holds(c, depth+1) {
				return true
			}
		}
		return false
	}
	free := make([]bool, len(pg.Groups))
	for g := range pg.Groups {
		free[g] = !holds(g, 0)
	}
	return free
}

// box is node n's box.
func (s *scene) box(n int) bounds {
	nd := s.pg.Nodes[n]
	return bounds{nd.X - nd.Width/2, nd.Y - nd.Height/2, nd.X + nd.Width/2, nd.Y + nd.Height/2}
}

// pinned reports whether node n is a diamond or a circle outside the text
// profile, which attaches at its four vertices only (C8).
func (s *scene) pinned(n int) bool { return ports.Pinned(s.pg.Nodes[n].Shape, s.o.Text) }

// radii are how far pinned node n's side vertices and its top and bottom
// vertices lie from its center: a diamond's box sides, a circle's radius,
// min(W, H) / 2 (C8).
func (s *scene) radii(n int) (rx, ry float64) {
	nd := s.pg.Nodes[n]
	if nd.Shape == model.ShapeCircle {
		r := math.Min(nd.Width, nd.Height) / 2
		return r, r
	}
	return nd.Width / 2, nd.Height / 2
}

// vertex is the vertex of pinned node n nearest p, as the face it lies on.
func (s *scene) vertex(n int, p model.Point) ports.Face {
	nd := s.pg.Nodes[n]
	rx, ry := s.radii(n)
	best, d := ports.FaceIn, math.Inf(1)
	for _, v := range []struct {
		f    ports.Face
		x, y float64
	}{{ports.FaceIn, nd.X, nd.Y - ry}, {ports.FaceOut, nd.X, nd.Y + ry}, {ports.FaceLeft, nd.X - rx, nd.Y}, {ports.FaceRight, nd.X + rx, nd.Y}} {
		if dv := math.Hypot(p.X-v.x, p.Y-v.y); dv < d {
			best, d = v.f, dv
		}
	}
	return best
}

// sideRange is the stretch of node n's side face f, as ys, where a side
// port may lie (S9, Flat edges): a pinned node's side vertex; else the
// middle nine tenths of the face's usable span, in the text profile the
// cells inside the box's corners.
func (s *scene) sideRange(n int, f ports.Face) (lo, hi float64) {
	nd := s.pg.Nodes[n]
	left, right := ports.Sides(nd.Shape, nd.Width, nd.Height, s.o.Dir, s.o.Text)
	if f == ports.FaceRight {
		left = right
	}
	return s.inside(nd.Y, left)
}

// faceRange is the stretch of node n's top (ports.FaceIn) or bottom
// (ports.FaceOut) face, as xs, where a column may end: a pinned node's
// vertex; else as sideRange.
func (s *scene) faceRange(n int, f ports.Face) (lo, hi float64) {
	nd := s.pg.Nodes[n]
	in, out := ports.Faces(nd.Shape, nd.Width, nd.Height, s.o.Dir, s.o.Text)
	if f == ports.FaceOut {
		in = out
	}
	return s.inside(nd.X, in)
}

// inside is the stretch of span sp, centered at c, where a port may lie:
// the span's middle nine tenths (as S8's ports slide, *Stops*), in the
// text profile the whole cells inside its two corner cells; a pinned
// node's zero span is its center.
func (s *scene) inside(c float64, sp ports.Span) (lo, hi float64) {
	switch {
	case sp.Hi-sp.Lo <= noise:
		return c, c
	case s.o.Text:
		return c + sp.Lo + 1, c + sp.Hi - 1
	}
	m := (sp.Hi - sp.Lo) / 20
	return c + sp.Lo + m, c + sp.Hi - m
}

// sidePoint is the end at y on node n's side face f, on its drawn outline
// (C2.2): a pinned node's side vertex, whatever y.
func (s *scene) sidePoint(n int, f ports.Face, y float64) model.Point {
	nd := s.pg.Nodes[n]
	if s.pinned(n) {
		rx, _ := s.radii(n)
		if f == ports.FaceLeft {
			rx = -rx
		}
		return model.Point{X: nd.X + rx, Y: nd.Y}
	}
	in := ports.Inset(nd.Shape, nd.Width, nd.Height, s.o.Dir, s.o.Text, f, y-nd.Y)
	if f == ports.FaceLeft {
		return model.Point{X: nd.X - nd.Width/2 + in, Y: y}
	}
	return model.Point{X: nd.X + nd.Width/2 - in, Y: y}
}

// facePoint is the end at x on node n's top (ports.FaceIn) or bottom
// (ports.FaceOut) face, on its drawn outline (C2.2): a pinned node's top
// or bottom vertex, whatever x.
func (s *scene) facePoint(n int, f ports.Face, x float64) model.Point {
	nd := s.pg.Nodes[n]
	if s.pinned(n) {
		_, ry := s.radii(n)
		if f == ports.FaceIn {
			ry = -ry
		}
		return model.Point{X: nd.X, Y: nd.Y + ry}
	}
	in := ports.Inset(nd.Shape, nd.Width, nd.Height, s.o.Dir, s.o.Text, f, x-nd.X)
	if f == ports.FaceIn {
		return model.Point{X: x, Y: nd.Y - nd.Height/2 + in}
	}
	return model.Point{X: x, Y: nd.Y + nd.Height/2 - in}
}
