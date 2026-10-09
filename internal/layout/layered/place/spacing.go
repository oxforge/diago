package place

import (
	"math"

	"github.com/oxforge/diago/internal/layout/layered/lgraph"
)

// sep is the minimum distance between the centers of neighbors a and b,
// a left of b: the gap, or the room their side columns and loops need
// (S7).
func (p *placer) sep(a, b int) float64 {
	return p.space.sep(p.g, a, b)
}

// room keeps node other, of diamond's row, the node clearance beyond the
// port the diamond's secondary exit edge ends on, at its next vertex next,
// on the side sign points to (S7): port is next's in-port offset for edge.
type room struct {
	edge, diamond, next, other int
	sign, port                 float64
}

// roomSep is room r as a separation: the vertex it keeps on the left, the
// one on the right, and the least distance between their centers. The
// other node keeps the node clearance beyond the next vertex's port,
// counted from its box plus its own farthest side column or loop leg on
// its side facing the port (S7).
func (s spacing) roomSep(g *lgraph.Graph, r room) (left, right int, gap float64) {
	o := r.other
	half := g.Vertices[o].W / 2
	if r.sign > 0 {
		return r.next, o, r.port + half + s.out[o][0] + s.clearance
	}
	return o, r.next, -r.port + half + s.out[o][1] + s.clearance
}

// sep is the minimum distance between the centers of neighbors a and b,
// a left of b.
func (in alignInput) sep(a, b int) float64 {
	return in.space.sep(in.g, a, b)
}

// spacing holds what S7's minimum separations take from the Options and
// from the room each vertex needs beside it.
type spacing struct {
	gap       float64      // the node gap
	terminal  float64      // the distance between the centers of two terminals (S9)
	clearance float64      // kept beyond a side column or a loop leg
	inLane    float64      // kept between two facing side columns or loop legs
	text      bool         // the text profile, where every gap is whole cells
	out       [][2]float64 // per vertex: how far outside its left and right side its farthest side column or loop leg runs, 0 for none
}

// sep is the minimum distance between the centers of neighbors a and b of
// graph g, a left of b: the gap between their boxes, or more where a side
// column or a loop leg runs beside one of them: its distance out plus the
// clearance, and when a's right side and b's left side both hold one,
// both distances out plus the in-lane gap between them, in the text
// profile each rounded up to whole cells (S7). Two terminals, mere wire
// columns, keep only the terminal gap between their centers (S9, S14),
// whatever their widths: every terminal is a dummy column, DummyWidth
// wide (nested.go).
func (s spacing) sep(g *lgraph.Graph, a, b int) float64 {
	if terminal(g, a) && terminal(g, b) {
		return s.terminal
	}
	half := (g.Vertices[a].W + g.Vertices[b].W) / 2
	need := s.gap
	right, left := s.out[a][1], s.out[b][0]
	if right > 0 {
		need = max(need, s.cells(right+s.clearance))
	}
	if left > 0 {
		need = max(need, s.cells(left+s.clearance))
	}
	if right > 0 && left > 0 {
		need = max(need, s.cells(right+left+s.inLane))
	}
	return half + need
}

// cells rounds room up to whole cells in the text profile, where every
// separation is a whole number of cells.
func (s spacing) cells(room float64) float64 {
	if s.text {
		return math.Ceil(room)
	}
	return room
}

// terminal reports whether vertex v is a terminal (S9).
func terminal(g *lgraph.Graph, v int) bool {
	n := g.Vertices[v].Node
	return n >= 0 && g.Level.Nodes[n].Terminal
}
