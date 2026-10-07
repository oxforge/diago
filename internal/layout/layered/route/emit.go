package route

import (
	"slices"

	"github.com/oxforge/diago/internal/layout/layered/ports"
	"github.com/oxforge/diago/internal/model"
)

// emit turns every wire into its polyline, from the edge's From node to
// its To node: a reversed edge's chain runs from To to From, so its
// points are reversed.
func (r *router) emit() [][]model.Point {
	g := r.g
	out := make([][]model.Point, len(g.Level.Edges))
	for e, chain := range g.Chains {
		if len(chain) < 2 {
			continue
		}
		var pts []model.Point
		head, tail := chain[0], chain[len(chain)-1]
		w := r.wires[e]
		if a := r.attach[e][ports.Head]; a != nil {
			pts = append(pts, r.sidePoint(head, a.side), model.Point{X: w.stops[0], Y: r.midY(head)})
		} else {
			_, top, _, h := r.box(head)
			x0 := w.stops[0]
			pts = append(pts, model.Point{X: x0, Y: top + h - r.inset(head, ports.FaceOut, x0-r.x[head])})
		}
		for i := 0; i+1 < len(w.stops); i++ {
			c := w.start + i
			lane, ok := r.lanes[[2]int{e, c}]
			if !ok {
				continue
			}
			y := r.laneY(c, lane)
			pts = append(pts, model.Point{X: w.stops[i], Y: y}, model.Point{X: w.stops[i+1], Y: y})
		}
		last := w.stops[len(w.stops)-1]
		if a := r.attach[e][ports.Tail]; a != nil {
			pts = append(pts, model.Point{X: last, Y: r.midY(tail)}, r.sidePoint(tail, a.side))
		} else {
			_, top, _, _ := r.box(tail)
			pts = append(pts, model.Point{X: last, Y: top + r.inset(tail, ports.FaceIn, last-r.x[tail])})
		}
		pts = Simplify(pts)
		if r.reversed(e) {
			slices.Reverse(pts)
		}
		out[e] = pts
	}
	for v, es := range r.loops {
		if r.pinned(v) {
			continue
		}
		for j, e := range es {
			out[e] = r.faceLoop(v, j)
		}
	}
	for _, lj := range r.loopAt {
		out[lj.edge] = r.vertexLoop(lj)
	}
	return out
}

// midY is node v's mid-height, where its side vertices and side
// attachments sit.
func (r *router) midY(v int) float64 {
	_, top, _, h := r.box(v)
	return top + h/2
}

// sidePoint is where a side attachment meets node v: its side vertex, or
// its side face's outline at mid-height.
func (r *router) sidePoint(v int, side ports.Vertex) model.Point {
	face := ports.FaceLeft
	if side == ports.Right {
		face = ports.FaceRight
	}
	x := r.vertexX(v, side) - sign(side)*r.inset(v, face, 0)
	return model.Point{X: x, Y: r.midY(v)}
}

// faceLoop is self-loop j (from 0, in edge order) on a node with faces
// (S8): the node's loops nest on its right face, the first outermost. Loop
// j leaves the face h/4 + j LaneGap down, runs ports.LoopOut outside it and
// returns 3h/4 - j LaneGap down, so its ports lie between the outer loops'
// ones, in the middle half of the face; loopRoom made the node long
// enough that the innermost loop's ports keep LaneGap apart.
func (r *router) faceLoop(v, j int) []model.Point {
	left, top, w, h := r.box(v)
	right := left + w
	leg := right + ports.LoopOut(r.o.Reach, r.o.InLaneGap, j, len(r.loops[v]), r.o.Text)
	in := float64(j) * r.o.LaneGap // how far inward of a lone loop's ports
	y1, y2 := top+h/4+in, top+3*h/4-in
	return []model.Point{
		{X: right - r.inset(v, ports.FaceRight, in-h/4), Y: y1},
		{X: leg, Y: y1},
		{X: leg, Y: y2},
		{X: right - r.inset(v, ports.FaceRight, h/4-in), Y: y2},
	}
}

// vertexLoop is a self-loop on a diamond or circle (S8): it leaves its
// side vertex, rises along the column Reach outside it into its lane in
// the channel above the node, and runs across to enter the in-vertex.
func (r *router) vertexLoop(lj loopJog) []model.Point {
	v := lj.node
	_, top, _, _ := r.box(v)
	y := r.laneY(r.g.Vertices[v].Layer-1, lj.lane)
	cx := r.x[v]
	return []model.Point{
		{X: r.vertexX(v, lj.side), Y: r.midY(v)},
		{X: lj.sx, Y: r.midY(v)},
		{X: lj.sx, Y: y},
		{X: cx, Y: y},
		{X: cx, Y: top},
	}
}

// Simplify drops repeated points and the middle point of three collinear
// ones.
func Simplify(pts []model.Point) []model.Point {
	var out []model.Point
	for _, p := range pts {
		if n := len(out); n > 0 && out[n-1] == p {
			continue
		}
		if n := len(out); n >= 2 {
			a, b := out[n-2], out[n-1]
			if (a.X == b.X && b.X == p.X) || (a.Y == b.Y && b.Y == p.Y) {
				out[n-1] = p
				continue
			}
		}
		out = append(out, p)
	}
	return out
}
