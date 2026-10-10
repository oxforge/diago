package contract

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/oxforge/diago/internal/model"
)

// has reports whether vs holds a violation of rule about subject.
func has(vs []Violation, rule, subject string) bool {
	for _, v := range vs {
		if v.Rule == rule && v.Subject == subject {
			return true
		}
	}
	return false
}

func TestC2_EdgesPresent(t *testing.T) {
	pg := chain()
	vs := Check(pg, Options{Limits: ScreenLimits(), Direction: model.Down, EdgeIDs: []string{"a->b#0", "a->c#0"}})
	assert.Equal(t, []Violation{{Rule: "C2.1", Subject: "a->c#0", Detail: "spec edge is missing from the layout"}}, vs)

	pg.Edges[0].Points = pg.Edges[0].Points[:1]
	assert.True(t, has(check(pg), "C2.1", "a->b#0"), "a one-point route is not drawn")
}

func TestC2_Attachment(t *testing.T) {
	tests := []struct {
		name   string
		last   model.Point
		target string
		want   bool
	}{
		{"on the top side", model.Point{X: 60, Y: 120}, "b", false},
		{"1.5 px off, within tolerance", model.Point{X: 60, Y: 118.5}, "b", false},
		{"3 px off", model.Point{X: 60, Y: 117}, "b", true},
		{"unknown node", model.Point{X: 60, Y: 120}, "z", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pg := chain()
			pg.Edges[0].To = tt.target
			pg.Edges[0].Points[1] = tt.last
			assert.Equal(t, tt.want, has(check(pg), "C2.2", "a->b#0"))
		})
	}
}

func TestOutlineDist(t *testing.T) {
	cyl := shaped("n", model.ShapeCylinder, 60, 50, 80, 60)
	tests := []struct {
		name string
		n    model.PositionedNode
		p    model.Point
		want float64
	}{
		{"rect side", rect("n", 60, 40), model.Point{X: 60, Y: 20}, 0},
		{"rect, 3 px out", rect("n", 60, 40), model.Point{X: 60, Y: 17}, 3},
		{"circle cardinal point", shaped("n", model.ShapeCircle, 60, 40, 40, 40), model.Point{X: 80, Y: 40}, 0},
		{"diamond vertex", shaped("n", model.ShapeDiamond, 60, 43, 80, 46), model.Point{X: 60, Y: 20}, 0},
		{"diamond edge midpoint", shaped("n", model.ShapeDiamond, 60, 43, 80, 46), model.Point{X: 80, Y: 31.5}, 0},
		{"hexagon flat top", shaped("n", model.ShapeHexagon, 60, 40, 80, 40), model.Point{X: 70, Y: 20}, 0},
		{"parallelogram top-left vertex", shaped("n", model.ShapeParallelogram, 60, 40, 80, 40), model.Point{X: 32, Y: 20}, 0},
		{"cylinder top arc, off center", cyl, model.Point{X: 90, Y: 23.6565}, 0},
		{"cylinder bounding box top, off center", cyl, model.Point{X: 90, Y: 20}, 3.6565},
		{"cylinder side line", cyl, model.Point{X: 100, Y: 50}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.InDelta(t, tt.want, outlineDist(tt.n, tt.p), 0.001)
		})
	}
}

func TestC3_Orthogonal(t *testing.T) {
	pg := chain()
	pg.Edges[0].Points[1] = model.Point{X: 70, Y: 120}
	assert.True(t, has(check(pg), "C3", "a->b#0"))
	assert.False(t, has(check(chain()), "C3", "a->b#0"))
}

func TestC4_NodeInteriors(t *testing.T) {
	t.Run("through a foreign node", func(t *testing.T) {
		pg := layout(120, 280, []model.PositionedNode{rect("a", 60, 40), rect("m", 60, 140), rect("b", 60, 240)},
			wire("a", "b", 60, 60, 60, 220))
		assert.True(t, has(check(pg), "C4", "a->b#0"))
	})
	t.Run("back through its own target", func(t *testing.T) {
		// Down through b, then up into b's bottom side.
		pg := layout(120, 200, []model.PositionedNode{rect("a", 60, 40), rect("b", 60, 140)},
			wire("a", "b", 60, 60, 60, 180, 80, 180, 80, 160))
		assert.True(t, has(check(pg), "C4", "a->b#0"))
	})
	t.Run("terminal segment from a cylinder arc", func(t *testing.T) {
		// The bottom-arc port lies inside the cylinder's box; its stub may
		// cross the box corner on the way out.
		pg := layout(150, 200, []model.PositionedNode{shaped("a", model.ShapeCylinder, 60, 50, 80, 60), rect("b", 90, 160)},
			wire("a", "b", 90, 76.3435, 90, 140))
		vs := check(pg)
		assert.False(t, has(vs, "C4", "a->b#0"))
		assert.False(t, has(vs, "C2.2", "a->b#0"))
	})
}

func TestC5_NodeClearance(t *testing.T) {
	for _, tt := range []struct {
		name string
		mx   float64 // m's center; its left side is mx-40, the wire runs at x=60
		want bool
	}{
		{"10 px from a foreign node", 110, true},
		{"13 px from a foreign node", 113, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pg := layout(180, 280, []model.PositionedNode{rect("a", 60, 40), rect("m", tt.mx, 140), rect("b", 60, 240)},
				wire("a", "b", 60, 60, 60, 220))
			assert.Equal(t, tt.want, has(check(pg), "C5", "a->b#0"))
		})
	}
}

func TestC6_GroupClearance(t *testing.T) {
	scene := func(gx float64) *model.PositionedGraph {
		pg := layout(220, 280,
			[]model.PositionedNode{rect("a", 60, 40), rect("c", gx+50, 140), rect("b", 60, 240)},
			wire("a", "b", 60, 60, 60, 220),
			wire("a", "c", 60, 60, 60, 140, gx+10, 140))
		pg.Groups = []model.PositionedGroup{{ID: "g", X: gx, Y: 110, Width: 100, Height: 60, Contains: []string{"c"}, Depth: 1}}
		return pg
	}
	assert.True(t, has(check(scene(70)), "C6.1", "a->b#0"), "10 px from the group border")
	assert.False(t, has(check(scene(73)), "C6.1", "a->b#0"), "13 px from the group border")
	assert.False(t, has(check(scene(70)), "C6.1", "a->c#0"), "an edge into a member crosses the border")
}

// besideSides is a layout whose group g spans (100, 100) to (300, 220)
// and holds c; the edge from a outside it into c runs through xy.
func besideSides(xy ...float64) *model.PositionedGraph {
	pg := layout(400, 300, []model.PositionedNode{rect("a", 200, 20), rect("c", 200, 160)}, wire("a", "c", xy...))
	pg.Groups = []model.PositionedGroup{{ID: "g", X: 100, Y: 100, Width: 200, Height: 120, Contains: []string{"c"}, Depth: 1}}
	return pg
}

func TestC6_2_NoWireAlongAGroupsSide(t *testing.T) {
	// an edge into a member crosses g's border freely (C6.1) but never
	// runs along one of its sides: a segment parallel to a side keeps the
	// 8 px track gap from it where their extents overlap (C6.2)
	for _, tc := range []struct {
		name string
		xy   []float64
		want bool
	}{
		{"on the left side", []float64{100, 120, 100, 200}, true},
		{"7 px inside the left side", []float64{107, 120, 107, 200}, true},
		{"8 px inside the left side", []float64{108, 120, 108, 200}, false},
		{"5 px outside the left side", []float64{95, 60, 95, 200}, true},
		{"on the right side", []float64{300, 120, 300, 200}, true},
		{"on the top side", []float64{150, 100, 250, 100}, true},
		{"on the bottom side", []float64{150, 220, 250, 220}, true},
		{"7 px inside the bottom side", []float64{150, 213, 250, 213}, true},
		{"8 px inside the bottom side", []float64{150, 212, 250, 212}, false},
		{"across the top side", []float64{200, 40, 200, 140}, false},
		{"on the left side's line, above the box", []float64{100, 20, 100, 100}, false},
		{"on the top side's line, left of the box", []float64{20, 100, 100, 100}, false},
	} {
		assert.Equal(t, tc.want, has(check(besideSides(tc.xy...)), "C6.2", "a->c#0"), tc.name)
	}

	pg := besideSides(200, 40, 200, 140)
	pg.Nodes = append(pg.Nodes, rect("b", 40, 260))
	pg.Edges = append(pg.Edges, wire("a", "b", 100, 20, 100, 260))
	assert.True(t, has(check(pg), "C6.2", "a->b#0"), "an edge with no end inside is bound too")
}

func TestC6_2_TextReadsTheCellsAWireAndAFrameAreDrawnIn(t *testing.T) {
	// g, on whole cells from (96, 96) to (304, 224), draws its frame in
	// columns 12..37 and rows 6..13; a wire on a cell boundary is drawn in
	// the cell right of it or below it (C14.2), so a run 16 px above the
	// bottom side is drawn on the frame's bottom row
	text := Options{Limits: TextLimits(model.Down), Direction: model.Down, Text: true}
	scene := func(xy ...float64) *model.PositionedGraph {
		pg := besideSides(xy...)
		g := &pg.Groups[0]
		g.X, g.Y, g.Width, g.Height = 96, 96, 208, 128
		return pg
	}
	for _, tc := range []struct {
		name string
		xy   []float64
		want bool
	}{
		{"on the left side, column 12", []float64{100, 120, 100, 200}, true},
		{"one column inside the left side", []float64{108, 120, 108, 200}, true},
		{"two columns inside the left side", []float64{116, 120, 116, 200}, false},
		{"one column outside the left side", []float64{92, 60, 92, 200}, true},
		{"on the right side, column 37", []float64{296, 120, 296, 200}, true},
		{"two columns inside the right side", []float64{284, 120, 284, 200}, false},
		{"on the top side, row 6", []float64{150, 104, 250, 104}, true},
		{"one row inside the top side", []float64{150, 120, 250, 120}, false},
		{"one row outside the top side", []float64{150, 88, 250, 88}, false},
		{"16 px above the bottom side, drawn on row 13", []float64{150, 208, 250, 208}, true},
		{"one row inside the bottom side", []float64{150, 200, 250, 200}, false},
		{"across the top side", []float64{200, 40, 200, 140}, false},
	} {
		assert.Equal(t, tc.want, has(Check(scene(tc.xy...), text), "C6.2", "a->c#0"), tc.name)
	}
	assert.False(t, has(check(scene(150, 208, 250, 208)), "C6.2", "a->c#0"), "on screen the run keeps 16 px")
}

func TestC7_Stubs(t *testing.T) {
	tests := []struct {
		name  string
		by    float64 // b's center y
		route []float64
		want  bool
	}{
		{"L route, 30 px stubs", 140, []float64{60, 60, 60, 90, 40, 90, 40, 120}, false},
		{"15 px first stub", 140, []float64{60, 60, 60, 75, 40, 75, 40, 120}, true},
		{"first segment runs along the bottom side", 140, []float64{60, 60, 90, 60, 90, 120}, true},
		{"20 px straight wire", 100, []float64{60, 60, 60, 80}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pg := layout(120, 180, []model.PositionedNode{rect("a", 60, 40), rect("b", 60, tt.by)}, wire("a", "b", tt.route...))
			assert.Equal(t, tt.want, has(check(pg), "C7", "a->b#0"))
		})
	}
}

// TestC7_EachShapesSides pins C7's side of an endpoint per shape: the
// first segment leaves s, 120 x 60 at (200,200), from the endpoint 30 px
// along a direction, and must leave that side perpendicularly outward. A
// hexagon has flat top and bottom faces (|dx| <= 30) and angled sides
// facing left and right; a parallelogram (slant 18) slanted left and right
// sides; a cylinder (cap 10.8) side lines facing left and right and top
// and bottom arcs; a diamond and a circle (60 x 60) the side of the
// vertex.
func TestC7_EachShapesSides(t *testing.T) {
	up, down, left, right := [2]float64{0, -1}, [2]float64{0, 1}, [2]float64{-1, 0}, [2]float64{1, 0}
	for _, tc := range []struct {
		name  string
		shape model.Shape
		at    [2]float64
		dir   [2]float64
		want  bool
	}{
		{"hexagon: up from its top face", model.ShapeHexagon, [2]float64{200, 170}, up, false},
		{"hexagon: right along its top face", model.ShapeHexagon, [2]float64{200, 170}, right, true},
		{"hexagon: right from its angled side", model.ShapeHexagon, [2]float64{245, 185}, right, false},
		{"hexagon: up from its angled side", model.ShapeHexagon, [2]float64{245, 185}, up, true},
		{"hexagon: left from its left vertex", model.ShapeHexagon, [2]float64{140, 200}, left, false},
		{"hexagon: down from its left vertex", model.ShapeHexagon, [2]float64{140, 200}, down, true},
		{"parallelogram: up from its top side", model.ShapeParallelogram, [2]float64{200, 170}, up, false},
		{"parallelogram: left along its top side", model.ShapeParallelogram, [2]float64{200, 170}, left, true},
		{"parallelogram: right from its slanted right side", model.ShapeParallelogram, [2]float64{251, 200}, right, false},
		{"parallelogram: down from its slanted right side", model.ShapeParallelogram, [2]float64{251, 200}, down, true},
		{"parallelogram: left from its slanted left side", model.ShapeParallelogram, [2]float64{149, 200}, left, false},
		{"parallelogram: up from its slanted left side", model.ShapeParallelogram, [2]float64{149, 200}, up, true},
		{"cylinder: right from its right side line", model.ShapeCylinder, [2]float64{260, 200}, right, false},
		{"cylinder: up along its right side line", model.ShapeCylinder, [2]float64{260, 200}, up, true},
		{"cylinder: up from its top arc", model.ShapeCylinder, [2]float64{200, 170}, up, false},
		{"cylinder: right along its top arc", model.ShapeCylinder, [2]float64{200, 170}, right, true},
		{"cylinder: down from its bottom arc", model.ShapeCylinder, [2]float64{200, 230}, down, false},
		{"cylinder: left along its bottom arc", model.ShapeCylinder, [2]float64{200, 230}, left, true},
		{"diamond: left from its left vertex", model.ShapeDiamond, [2]float64{140, 200}, left, false},
		{"diamond: up from its left vertex", model.ShapeDiamond, [2]float64{140, 200}, up, true},
		{"diamond: down from its bottom vertex", model.ShapeDiamond, [2]float64{200, 230}, down, false},
		{"diamond: right from its bottom vertex", model.ShapeDiamond, [2]float64{200, 230}, right, true},
		{"circle: right from its right vertex", model.ShapeCircle, [2]float64{230, 200}, right, false},
		{"circle: down from its right vertex", model.ShapeCircle, [2]float64{230, 200}, down, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := 120.0
			if tc.shape == model.ShapeCircle {
				w = 60
			}
			x, y := tc.at[0]+30*tc.dir[0], tc.at[1]+30*tc.dir[1]
			// on to b's top side, which the last segment leaves upward
			pg := layout(600, 600, []model.PositionedNode{shaped("s", tc.shape, 200, 200, w, 60), rect("b", 500, 500)},
				wire("s", "b", tc.at[0], tc.at[1], x, y, 500, y, 500, 480))
			assert.Equal(t, tc.want, has(check(pg), "C7", "s->b#0"), "%v", filter(check(pg), "C7"))
		})
	}
}

func TestC10_SelfRetrace(t *testing.T) {
	for _, tt := range []struct {
		name  string
		route []float64
	}{
		{"backtrack", []float64{60, 60, 60, 100, 60, 90, 60, 120}},
		{"revisited waypoint", []float64{60, 60, 60, 90, 80, 90, 80, 100, 60, 100, 60, 90, 60, 120}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pg := chain()
			pg.Edges[0] = wire("a", "b", tt.route...)
			assert.True(t, has(check(pg), "C10", "a->b#0"))
		})
	}
	assert.False(t, has(check(chain()), "C10", "a->b#0"))
}
