package svg

import (
	"testing"

	"github.com/oxforge/diago/internal/model"
	"github.com/stretchr/testify/assert"
)

func TestSketchPolylineDeterministic(t *testing.T) {
	pts := []model.Point{{X: 0, Y: 0}, {X: 100, Y: 0}, {X: 100, Y: 100}}
	path1 := sketchPolyline(pts, "a-b")
	path2 := sketchPolyline(pts, "a-b")
	assert.Equal(t, path1, path2)
}

func TestSketchPolylineDiffersByEdgeID(t *testing.T) {
	pts := []model.Point{{X: 0, Y: 0}, {X: 100, Y: 0}}
	path1 := sketchPolyline(pts, "a-b")
	path2 := sketchPolyline(pts, "c-d")
	assert.NotEqual(t, path1, path2)
}

func TestSketchPolylineSubdivides(t *testing.T) {
	pts := []model.Point{{X: 0, Y: 0}, {X: 100, Y: 0}}
	path := sketchPolyline(pts, "test")
	// 100px segment should be subdivided into multiple L commands.
	assert.Contains(t, path, "L")
	assert.Contains(t, path, "M")
}

func TestSketchPolylineShortSegmentUsesLine(t *testing.T) {
	pts := []model.Point{{X: 0, Y: 0}, {X: 0.5, Y: 0}}
	path := sketchPolyline(pts, "short")
	assert.Contains(t, path, "L")
	assert.NotContains(t, path, "Q")
}

func TestSketchRectDeterministic(t *testing.T) {
	r1 := sketchRect(10, 20, 100, 60, 6, "node-a")
	r2 := sketchRect(10, 20, 100, 60, 6, "node-a")
	assert.Equal(t, r1, r2)
}

func TestSketchRectDiffersByID(t *testing.T) {
	r1 := sketchRect(10, 20, 100, 60, 6, "node-a")
	r2 := sketchRect(10, 20, 100, 60, 6, "node-b")
	assert.NotEqual(t, r1, r2)
}

func TestSketchRectContainsArcs(t *testing.T) {
	r := sketchRect(0, 0, 100, 60, 6, "test")
	assert.Contains(t, r, "A")
	assert.Contains(t, r, "Z")
}

func TestSketchPolygonDeterministic(t *testing.T) {
	pts := []model.Point{{X: 0, Y: 0}, {X: 50, Y: -25}, {X: 50, Y: 25}}
	r1 := sketchPolygon(pts, "diamond-a")
	r2 := sketchPolygon(pts, "diamond-a")
	assert.Equal(t, r1, r2)
	assert.Contains(t, r1, "Z")
}

func TestSketchCircleDeterministic(t *testing.T) {
	r1 := sketchCircle(50, 50, 30, "circle-a")
	r2 := sketchCircle(50, 50, 30, "circle-a")
	assert.Equal(t, r1, r2)
	assert.Contains(t, r1, "Z")
}

func TestSketchWobbleIsHalf(t *testing.T) {
	// sketchWobble should be 0.75 (halved from original 1.5)
	// to produce subtler hand-drawn deviations.
	assert.Equal(t, 0.75, sketchWobble)
}

func TestSeedRNGDeterministic(t *testing.T) {
	r1 := seedRNG("test-id")
	r2 := seedRNG("test-id")
	// Draw several values and compare.
	for i := 0; i < 10; i++ {
		assert.Equal(t, r1.Float64(), r2.Float64())
	}
}
