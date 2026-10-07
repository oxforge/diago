package ports

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/oxforge/diago/internal/model"
)

// TestMerged pins S8's merged fan-out: a circle's exits merge on its
// bottom vertex, a diamond's do not (C8.5 forbids it a shared vertex), and
// a shape with faces, never pinned, reports false too.
func TestMerged(t *testing.T) {
	assert.True(t, Merged(model.ShapeCircle))
	for _, s := range []model.Shape{model.ShapeDiamond, model.ShapeRect, model.ShapeRounded, model.ShapeHexagon} {
		assert.False(t, Merged(s), "%s", s)
	}
}

func TestExits(t *testing.T) {
	const cx, band = 100.0, 16.0
	tests := []struct {
		name    string
		towards []float64
		want    []Vertex
	}{
		{"a lone exit takes the bottom", []float64{20}, []Vertex{Bottom}},
		{"two flanking exits take the sides", []float64{50, 150}, []Vertex{Left, Right}},
		{"two to the left: the farther takes the left", []float64{20, 70}, []Vertex{Left, Bottom}},
		{"one under the node, one to the right", []float64{105, 160}, []Vertex{Bottom, Right}},
		{"two inside the band still part", []float64{90, 103}, []Vertex{Left, Bottom}},
		{"two at equal distances: the first by heading is the farther", []float64{108, 92}, []Vertex{Bottom, Left}},
		{"two heading at the center: the first edge takes the right", []float64{100, 100}, []Vertex{Right, Bottom}},
		{"three by heading", []float64{140, 60, 100}, []Vertex{Right, Left, Bottom}},
		{"three to the left still take all three", []float64{10, 40, 70}, []Vertex{Left, Bottom, Right}},
		{"three tie by edge order", []float64{100, 100, 100}, []Vertex{Left, Bottom, Right}},
		{"four share by heading", []float64{50, 80, 105, 160}, []Vertex{Left, Left, Bottom, Right}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exits := make([]Exit, len(tt.towards))
			for i, x := range tt.towards {
				exits[i] = Exit{Edge: i, Toward: x}
			}
			assert.Equal(t, tt.want, Exits(cx, band, exits))
		})
	}
}

func TestFarther(t *testing.T) {
	const cx = 100.0
	tests := []struct {
		name    string
		towards []float64
		want    int
	}{
		{"the one heading farther from the center", []float64{20, 70}, 0},
		{"on opposite sides too", []float64{50, 180}, 1},
		{"at equal distances, the first by heading", []float64{108, 92}, 1},
		{"both at the center, the first edge", []float64{100, 100}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exits := []Exit{{Edge: 0, Toward: tt.towards[0]}, {Edge: 1, Toward: tt.towards[1]}}
			assert.Equal(t, tt.want, Farther(cx, exits))
		})
	}
}
