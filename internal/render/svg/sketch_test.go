package svg

import (
	"math"
	"strings"
	"testing"

	"github.com/oxforge/diago/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// assertOnEllipse asserts that p lies at angle a on the ellipse around
// (cx, cy) with radii rx and ry, its y axis flipped when up, both radii
// moved by one offset of at most sketchWobble.
func assertOnEllipse(t *testing.T, p model.Point, cx, cy, rx, ry, a float64, up bool) {
	t.Helper()
	c, s := math.Cos(a), math.Sin(a)
	if up {
		s = -s
	}
	o := (p.Y-cy)/s - ry // read the offset off the larger component
	if math.Abs(c) >= math.Abs(s) {
		o = (p.X-cx)/c - rx
	}
	assert.LessOrEqual(t, math.Abs(o), sketchWobble+1e-3, "%v at angle %.3f", p, a)
	assert.InDelta(t, cx+(rx+o)*c, p.X, 1e-3, "%v at angle %.3f", p, a)
	assert.InDelta(t, cy+(ry+o)*s, p.Y, 1e-3, "%v at angle %.3f", p, a)
}

// TestSketchEllipse_StaysOnItsEllipse pins the hand-drawn ellipse: a
// closed path round from the right end, one point per 15 px of its
// circumference and at least 24, each on the ellipse within the wobble
// and the last on it exactly; one id draws one path.
func TestSketchEllipse_StaysOnItsEllipse(t *testing.T) {
	cx, cy, rx, ry := 200.0, 50.0, 150.0, 40.0
	d := sketchEllipse(cx, cy, rx, ry, "db-cap")
	assert.Equal(t, d, sketchEllipse(cx, cy, rx, ry, "db-cap"))
	assert.NotEqual(t, d, sketchEllipse(cx, cy, rx, ry, "other-cap"))
	require.True(t, strings.HasPrefix(d, "M ") && strings.HasSuffix(d, " Z"), d)
	pts := pathPoints(t, d)
	n := len(pts) - 1
	assert.Equal(t, 44, n, "a 648 px circumference")
	for i, p := range pts {
		assertOnEllipse(t, p, cx, cy, rx, ry, 2*math.Pi*float64(i)/float64(n), false)
	}
	assert.Equal(t, model.Point{X: cx + rx, Y: cy}, pts[n], "it closes on its right end")
	assert.Equal(t, 24, len(pathPoints(t, sketchEllipse(cx, cy, 30, 10, "db-cap")))-1, "a small ellipse takes 24 points")
}

// TestSketchCylinder_DrawsItsBody pins the hand-drawn cylinder body: from
// its top left corner down the left side, along the bottom arc, bulging
// down by ry, to the bottom right corner, up the right side, and along the
// top arc, bulging up by ry, back to the top left corner: every corner
// exact, the sides and the arcs' 16 points each within the wobble; one id
// draws one path.
func TestSketchCylinder_DrawsItsBody(t *testing.T) {
	cx, hw, topY, botY, ry := 100.0, 60.0, 30.0, 130.0, 12.0
	d := sketchCylinder(cx, hw, topY, botY, ry, "db-body")
	assert.Equal(t, d, sketchCylinder(cx, hw, topY, botY, ry, "db-body"))
	assert.NotEqual(t, d, sketchCylinder(cx, hw, topY, botY, ry, "other-body"))
	require.True(t, strings.HasPrefix(d, "M ") && strings.HasSuffix(d, " Z"), d)
	pts := pathPoints(t, d)
	corner := func(from int, c model.Point) int {
		t.Helper()
		for i := from; i < len(pts); i++ {
			if pts[i] == c {
				return i
			}
		}
		require.Failf(t, "no corner", "%v after point %d of %v", c, from, pts)
		return 0
	}
	tl := model.Point{X: cx - hw, Y: topY}
	require.Equal(t, tl, pts[0], "it starts on the top left corner")
	bl := corner(1, model.Point{X: cx - hw, Y: botY})
	br := corner(bl+1, model.Point{X: cx + hw, Y: botY})
	tr := corner(br+1, model.Point{X: cx + hw, Y: topY})
	require.Equal(t, tl, pts[len(pts)-1], "it closes on the top left corner")
	for _, side := range []struct {
		x     float64
		along []model.Point
	}{{cx - hw, pts[1:bl]}, {cx + hw, pts[br+1 : tr]}} {
		require.NotEmpty(t, side.along)
		for _, p := range side.along {
			assert.InDelta(t, side.x, p.X, sketchWobble+1e-3, "%v on the side at x %g", p, side.x)
			assert.True(t, topY < p.Y && p.Y < botY, "%v between the arcs", p)
		}
	}
	require.Equal(t, 16, br-bl, "the bottom arc's points")
	require.Equal(t, 16, len(pts)-1-tr, "the top arc's points")
	for i := 1; i <= 16; i++ {
		a := math.Pi * float64(i) / 16
		assertOnEllipse(t, pts[bl+i], cx, botY, hw, ry, math.Pi-a, false)
		assertOnEllipse(t, pts[tr+i], cx, topY, hw, ry, a, true)
	}
}
