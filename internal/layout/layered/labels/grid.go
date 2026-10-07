package labels

import (
	"math"
	"math/bits"
)

// grid buckets boxes by the cells of a uniform grid their closed extents
// touch, so a spot's cost visits only the boxes near it. A box with a
// nonzero term in a spot's cost meets the spot, so it shares a cell with
// it: skipping the others leaves every sum as it was, and every sum keeps
// its order, since near lists the boxes in the order they were added.
// Placements do not depend on the grid (S10).
type grid struct {
	x0, y0 float64
	sx, sy float64 // a cell's extents, zero along an axis of one cell
	nx, ny int
	cells  [][]int  // per cell, row by row, the boxes touching it, ascending
	marks  []uint64 // one bit per box, set while near gathers the boxes, clear between calls
	buf    []int
}

// maxCells bounds a grid's cells along each axis.
const maxCells = 256

// newGrid covers bounds with cells of the given side, at most maxCells
// along each axis, wider when bounds would need more; a box beyond bounds
// falls into the cells at their border.
func newGrid(bounds Rect, side float64) *grid {
	axis := func(extent float64) (float64, int) {
		if !(side > 0) || !(extent > 0) {
			return 0, 1
		}
		n := math.Ceil(extent / side)
		if n > maxCells {
			return extent / maxCells, maxCells
		}
		return side, int(n)
	}
	g := &grid{x0: bounds.X, y0: bounds.Y}
	g.sx, g.nx = axis(bounds.W)
	g.sy, g.ny = axis(bounds.H)
	g.cells = make([][]int, g.nx*g.ny)
	return g
}

// index is the cell along one axis that coordinate v falls in, clamped
// to the grid: it never decreases as v grows.
func index(v, v0, s float64, n int) int {
	if s == 0 {
		return 0
	}
	f := math.Floor((v - v0) / s)
	switch {
	case !(f > 0): // NaN included
		return 0
	case f >= float64(n):
		return n - 1
	}
	return int(f)
}

// span is the range of cells the closed box x0..x1, y0..y1 touches.
func (g *grid) span(x0, y0, x1, y1 float64) (i0, i1, j0, j1 int) {
	return index(x0, g.x0, g.sx, g.nx), index(x1, g.x0, g.sx, g.nx), index(y0, g.y0, g.sy, g.ny), index(y1, g.y0, g.sy, g.ny)
}

// add files box r under id, which must exceed every id added before.
func (g *grid) add(id int, r Rect) {
	for id>>6 >= len(g.marks) {
		g.marks = append(g.marks, 0)
	}
	i0, i1, j0, j1 := g.span(r.X, r.Y, r.X+r.W, r.Y+r.H)
	for j := j0; j <= j1; j++ {
		for i := i0; i <= i1; i++ {
			g.cells[j*g.nx+i] = append(g.cells[j*g.nx+i], id)
		}
	}
}

// near lists, ascending and each once, the boxes sharing a cell with the
// closed box x0..x1, y0..y1: every box that meets it among them. The list
// is the grid's own and holds until the next call.
func (g *grid) near(x0, y0, x1, y1 float64) []int {
	i0, i1, j0, j1 := g.span(x0, y0, x1, y1)
	if i0 == i1 && j0 == j1 {
		return g.cells[j0*g.nx+i0]
	}
	lo, hi := len(g.marks), -1
	for j := j0; j <= j1; j++ {
		for _, cell := range g.cells[j*g.nx+i0 : j*g.nx+i1+1] {
			for _, id := range cell {
				w := id >> 6
				g.marks[w] |= 1 << (id & 63)
				lo, hi = min(lo, w), max(hi, w)
			}
		}
	}
	g.buf = g.buf[:0]
	for w := lo; w <= hi; w++ {
		for m := g.marks[w]; m != 0; m &= m - 1 {
			g.buf = append(g.buf, w<<6|bits.TrailingZeros64(m))
		}
		g.marks[w] = 0
	}
	return g.buf
}

// touching lists the boxes sharing a cell with the closed box x0..x1,
// y0..y1, in no order and some more than once, for a caller whose result
// depends on neither. The list is the grid's own and holds until the next
// call.
func (g *grid) touching(x0, y0, x1, y1 float64) []int {
	i0, i1, j0, j1 := g.span(x0, y0, x1, y1)
	if i0 == i1 && j0 == j1 {
		return g.cells[j0*g.nx+i0]
	}
	g.buf = g.buf[:0]
	for j := j0; j <= j1; j++ {
		for _, cell := range g.cells[j*g.nx+i0 : j*g.nx+i1+1] {
			g.buf = append(g.buf, cell...)
		}
	}
	return g.buf
}

// nearBox is near for box r.
func (g *grid) nearBox(r Rect) []int { return g.near(r.X, r.Y, r.X+r.W, r.Y+r.H) }
