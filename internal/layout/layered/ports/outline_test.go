package ports

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/oxforge/diago/internal/model"
)

func TestInset(t *testing.T) {
	cap := 0.18 * 40
	for _, tt := range []struct {
		name  string
		shape model.Shape
		dir   model.Direction
		face  Face
		off   float64
		want  float64
	}{
		{"a rect is its box", model.ShapeRect, model.Down, FaceIn, 30, 0},
		{"cylinder top at the middle", model.ShapeCylinder, model.Down, FaceIn, 0, 0},
		{"cylinder top halfway out", model.ShapeCylinder, model.Down, FaceIn, 20, cap * (1 - math.Sqrt(0.75))},
		{"cylinder bottom at the side", model.ShapeCylinder, model.Up, FaceOut, -40, cap},
		{"cylinder side lines", model.ShapeCylinder, model.Down, FaceLeft, 10, 0},
		{"parallelogram left side at mid-height", model.ShapeParallelogram, model.Down, FaceLeft, 0, 6},
		{"parallelogram left side at the top", model.ShapeParallelogram, model.Down, FaceLeft, -20, 12},
		{"parallelogram right side at the top", model.ShapeParallelogram, model.Down, FaceRight, -20, 0},
		{"UP mirrors the sides", model.ShapeParallelogram, model.Up, FaceLeft, -20, 0},
		{"parallelogram top edge", model.ShapeParallelogram, model.Down, FaceIn, 10, 0},
		{"hexagon flat face", model.ShapeHexagon, model.Down, FaceIn, 10, 0},
		{"hexagon beyond its flat face", model.ShapeHexagon, model.Down, FaceOut, 30, 10},
		{"hexagon angled side", model.ShapeHexagon, model.Down, FaceRight, 10, 10},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.InDelta(t, tt.want, Inset(tt.shape, 80, 40, tt.dir, false, tt.face, tt.off), 1e-9)
		})
	}
}

// Under RIGHT and LEFT the engine's in- and out-faces become the drawn
// shape's left and right sides: w, the cross extent, is its height.
func TestInset_Sideways(t *testing.T) {
	// a parallelogram drawn 80 wide and 40 tall: slant 12
	assert.InDelta(t, 6, Inset(model.ShapeParallelogram, 40, 80, model.Right, false, FaceIn, 0), 1e-9)
	assert.InDelta(t, 12, Inset(model.ShapeParallelogram, 40, 80, model.Right, false, FaceIn, -20), 1e-9)
	assert.InDelta(t, 0, Inset(model.ShapeParallelogram, 40, 80, model.Left, false, FaceIn, -20), 1e-9, "LEFT's in-face is the right side")
	assert.InDelta(t, 10, Inset(model.ShapeHexagon, 40, 80, model.Right, false, FaceIn, 10), 1e-9)
	assert.InDelta(t, 0, Inset(model.ShapeCylinder, 40, 80, model.Right, false, FaceIn, 10), 1e-9, "the side lines")
}

func TestInset_TextIsABox(t *testing.T) {
	assert.Zero(t, Inset(model.ShapeCylinder, 8, 3, model.Down, true, FaceIn, 3))
}
