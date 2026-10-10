package svg

import (
	"strings"
	"testing"

	"github.com/oxforge/diago/internal/model"
	"github.com/stretchr/testify/assert"
)

func TestSketchPolylineDeterministic(t *testing.T) {
	pts := []model.Point{{X: 0, Y: 0}, {X: 100, Y: 0}, {X: 100, Y: 100}}
	path1 := sketchPolyline(pts, "a-b", nil)
	path2 := sketchPolyline(pts, "a-b", nil)
	assert.Equal(t, path1, path2)
}

func TestSketchPolylineDiffersByEdgeID(t *testing.T) {
	pts := []model.Point{{X: 0, Y: 0}, {X: 100, Y: 0}}
	path1 := sketchPolyline(pts, "a-b", nil)
	path2 := sketchPolyline(pts, "c-d", nil)
	assert.NotEqual(t, path1, path2)
}

func TestSketchPolylineSubdivides(t *testing.T) {
	pts := []model.Point{{X: 0, Y: 0}, {X: 100, Y: 0}}
	path := sketchPolyline(pts, "test", nil)
	// 100px segment should be subdivided into multiple L commands.
	assert.Contains(t, path, "L")
	assert.Contains(t, path, "M")
}

func TestSketchPolylineShortSegmentUsesLine(t *testing.T) {
	pts := []model.Point{{X: 0, Y: 0}, {X: 0.5, Y: 0}}
	path := sketchPolyline(pts, "short", nil)
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
	// sketchWobble is 0.75, half of 1.5, for subtle hand-drawn
	// deviations.
	assert.Equal(t, 0.75, sketchWobble)
}

func TestSketchPolyline_HopsOverItsCrossings(t *testing.T) {
	pts := []model.Point{{X: 0, Y: 0}, {X: 100, Y: 0}}
	plain := sketchPolyline(pts, "a-b", nil)
	hopped := sketchPolyline(pts, "a-b", []model.Crossing{{SegmentIndex: 0, T: 0.5}})
	assert.NotContains(t, plain, " A")
	assert.Equal(t, 1, strings.Count(hopped, " A5,5 0 0 1 55,0"), hopped)
}
