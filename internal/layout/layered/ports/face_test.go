package ports

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/model"
)

func TestPinned(t *testing.T) {
	assert.True(t, Pinned(model.ShapeDiamond, false))
	assert.True(t, Pinned(model.ShapeCircle, false))
	assert.False(t, Pinned(model.ShapeHexagon, false))
	assert.False(t, Pinned(model.ShapeRect, false))
	assert.False(t, Pinned(model.ShapeDiamond, true), "every shape is a box in text")
}

func TestSpan_At(t *testing.T) {
	s := Span{Lo: -30, Hi: 30}
	assert.Equal(t, 0.0, s.At(0, 1))
	assert.Equal(t, -10.0, s.At(0, 2))
	assert.Equal(t, 10.0, s.At(1, 2))
	assert.Equal(t, 0.0, Span{}.At(2, 5), "a zero span pins every port to the center")
}

func TestSpan_AtOnCells(t *testing.T) {
	even := Span{Lo: -5, Hi: 5, Cells: true} // ten cells
	assert.Equal(t, 0.5, even.At(0, 1), "a lone port takes the cell right of the center line")
	assert.Equal(t, 0.0, Span{Lo: -3.5, Hi: 3.5, Cells: true}.At(0, 1), "or the center cell")
	wide := Span{Lo: -12, Hi: 12, Cells: true}
	assert.Equal(t, []float64{-4.5, 4.5}, []float64{wide.At(0, 2), wide.At(1, 2)},
		"two ports a third of the face apart on the lines between cells take the cells farther out")
	assert.Equal(t, []float64{-6.5, 0.5, 6.5}, []float64{wide.At(0, 3), wide.At(1, 3), wide.At(2, 3)},
		"the middle port of three on the middle line takes the cell after it")
	odd := Span{Lo: -5.5, Hi: 5.5, Cells: true} // eleven cells
	assert.Equal(t, []float64{-2, 2}, []float64{odd.At(0, 2), odd.At(1, 2)},
		"two ports on lines of an odd face, symmetric about its middle cell")

	// every port of a face of w cells holding n ports, n up to w - 2, sits
	// on a cell center, on its own cell inside the two corner cells, the
	// mirror of its partner about the face's middle
	for w := 3; w <= 30; w++ {
		s := Span{Lo: -float64(w) / 2, Hi: float64(w) / 2, Cells: true}
		for n := 1; n <= w-2; n++ {
			prev := 0.0
			for k := range n {
				c := s.At(k, n) - s.Lo - 0.5
				require.Equal(t, math.Floor(c), c, "w=%d n=%d k=%d: on a cell center", w, n, k)
				require.GreaterOrEqual(t, c, 1.0, "w=%d n=%d k=%d: inside the corner", w, n, k)
				require.LessOrEqual(t, c, float64(w-2), "w=%d n=%d k=%d: inside the corner", w, n, k)
				if k > 0 {
					require.Greater(t, c, prev, "w=%d n=%d k=%d: its own cell, in order", w, n, k)
				}
				prev = c
				if k != n-1-k {
					require.Equal(t, 0.0, s.At(k, n)+s.At(n-1-k, n),
						"w=%d n=%d k=%d: symmetric about the middle", w, n, k)
				}
			}
		}
	}
}

func TestFaces(t *testing.T) {
	const w, h = 120.0, 60.0
	full := Span{Lo: -60, Hi: 60}
	cells := Span{Lo: -60, Hi: 60, Cells: true}
	tests := []struct {
		name    string
		shape   model.Shape
		dir     model.Direction
		text    bool
		in, out Span
	}{
		{"rect", model.ShapeRect, model.Down, false, full, full},
		{"diamond", model.ShapeDiamond, model.Down, false, Span{}, Span{}},
		{"circle", model.ShapeCircle, model.Right, false, Span{}, Span{}},
		{"hexagon flat faces", model.ShapeHexagon, model.Up, false, Span{Lo: -30, Hi: 30}, Span{Lo: -30, Hi: 30}},
		{"hexagon angled sides", model.ShapeHexagon, model.Right, false, full, full},
		{"parallelogram down", model.ShapeParallelogram, model.Down, false, Span{Lo: -42, Hi: 60}, Span{Lo: -60, Hi: 42}},
		{"parallelogram up", model.ShapeParallelogram, model.Up, false, Span{Lo: -60, Hi: 42}, Span{Lo: -42, Hi: 60}},
		{"parallelogram slanted sides", model.ShapeParallelogram, model.Left, false, full, full},
		{"cylinder caps", model.ShapeCylinder, model.Down, false, full, full},
		{"cylinder side lines", model.ShapeCylinder, model.Right, false, Span{Lo: -38.4, Hi: 38.4}, Span{Lo: -38.4, Hi: 38.4}},
		{"text diamond", model.ShapeDiamond, model.Down, true, cells, cells},
		{"text hexagon", model.ShapeHexagon, model.Down, true, cells, cells},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in, out := Faces(tt.shape, w, h, tt.dir, tt.text)
			assert.InDelta(t, tt.in.Lo, in.Lo, 1e-9)
			assert.InDelta(t, tt.in.Hi, in.Hi, 1e-9)
			assert.InDelta(t, tt.out.Lo, out.Lo, 1e-9)
			assert.InDelta(t, tt.out.Hi, out.Hi, 1e-9)
		})
	}
}

// TestRoom_GivesEveryFaceItsSpan pins Room as Faces turned round: a node
// of every shape with faces, as wide as Room says, has usable in- and
// out-face spans of exactly the span asked for, in every direction; a
// pinned shape needs no room.
func TestRoom_GivesEveryFaceItsSpan(t *testing.T) {
	const span, h = 124.0, 46
	for _, s := range []model.Shape{model.ShapeRect, model.ShapeRounded, model.ShapeHexagon, model.ShapeParallelogram, model.ShapeCylinder} {
		for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
			in, out := Faces(s, Room(s, span, h, dir), h, dir, false)
			assert.InDelta(t, span, in.Hi-in.Lo, 1e-9, "%s %s: the in-face", s, dir)
			assert.InDelta(t, span, out.Hi-out.Lo, 1e-9, "%s %s: the out-face", s, dir)
		}
	}
	assert.Zero(t, Room(model.ShapeDiamond, span, h, model.Down))
	assert.Zero(t, Room(model.ShapeCircle, span, h, model.Right))
}

// TestSides pins the usable spans of the side faces, as offsets along the
// flow axis: the whole side, but where the drawn outline's side there is
// not the box's side (C7), which S8's self-loops and S9's flat edges keep
// to: a cylinder's side lines between its caps under DOWN and UP, and,
// where the side faces turn into the drawn top and bottom under RIGHT and
// LEFT, a hexagon's flat faces and a parallelogram's shortened edges.
func TestSides(t *testing.T) {
	const w, h = 40.0, 120.0 // on its side under RIGHT and LEFT: drawn 120 wide and 40 tall, slant 12
	full := Span{Lo: -60, Hi: 60}
	cells := Span{Lo: -60, Hi: 60, Cells: true}
	for _, tt := range []struct {
		name        string
		shape       model.Shape
		dir         model.Direction
		text        bool
		left, right Span
	}{
		{"rect", model.ShapeRect, model.Down, false, full, full},
		{"diamond", model.ShapeDiamond, model.Down, false, Span{}, Span{}},
		{"circle", model.ShapeCircle, model.Right, false, Span{}, Span{}},
		{"cylinder side lines", model.ShapeCylinder, model.Down, false, Span{Lo: -38.4, Hi: 38.4}, Span{Lo: -38.4, Hi: 38.4}},
		{"cylinder side lines up", model.ShapeCylinder, model.Up, false, Span{Lo: -38.4, Hi: 38.4}, Span{Lo: -38.4, Hi: 38.4}},
		{"cylinder caps", model.ShapeCylinder, model.Right, false, full, full},
		{"hexagon angled sides", model.ShapeHexagon, model.Down, false, full, full},
		{"hexagon flat faces", model.ShapeHexagon, model.Left, false, Span{Lo: -30, Hi: 30}, Span{Lo: -30, Hi: 30}},
		{"parallelogram slanted sides", model.ShapeParallelogram, model.Up, false, full, full},
		{"parallelogram right", model.ShapeParallelogram, model.Right, false, Span{Lo: -48, Hi: 60}, Span{Lo: -60, Hi: 48}},
		{"parallelogram left", model.ShapeParallelogram, model.Left, false, Span{Lo: -60, Hi: 48}, Span{Lo: -48, Hi: 60}},
		{"text diamond", model.ShapeDiamond, model.Down, true, cells, cells},
		{"text cylinder", model.ShapeCylinder, model.Down, true, cells, cells},
	} {
		t.Run(tt.name, func(t *testing.T) {
			left, right := Sides(tt.shape, w, h, tt.dir, tt.text)
			assert.InDelta(t, tt.left.Lo, left.Lo, 1e-9)
			assert.InDelta(t, tt.left.Hi, left.Hi, 1e-9)
			assert.Equal(t, tt.left.Cells, left.Cells)
			assert.InDelta(t, tt.right.Lo, right.Lo, 1e-9)
			assert.InDelta(t, tt.right.Hi, right.Hi, 1e-9)
			assert.Equal(t, tt.right.Cells, right.Cells)
		})
	}
}

// TestSides_OnTheDrawnOutline pins each side span to the drawn outline:
// across it, a point Inset inside the box lies where the outline's side
// is the box's side (C7), so a run leaving it across the flow is
// perpendicular to the drawn shape. Under RIGHT a parallelogram's top
// edge lies on the box (no inset) from the slant on, and a hexagon's flat
// face within its middle half.
func TestSides_OnTheDrawnOutline(t *testing.T) {
	left, _ := Sides(model.ShapeParallelogram, 40, 120, model.Right, false)
	for _, off := range []float64{left.Lo, 0, left.Hi} {
		assert.Zero(t, Inset(model.ShapeParallelogram, 40, 120, model.Right, false, FaceLeft, off), "offset %v", off)
	}
	flat, _ := Sides(model.ShapeHexagon, 40, 120, model.Right, false)
	assert.Zero(t, Inset(model.ShapeHexagon, 40, 120, model.Right, false, FaceLeft, flat.Hi))
	assert.Positive(t, Inset(model.ShapeHexagon, 40, 120, model.Right, false, FaceLeft, flat.Hi+1), "beyond it, the angled side")
}
