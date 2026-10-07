package svg

import (
	"bytes"
	"testing"

	"github.com/oxforge/diago/internal/model"
	"github.com/oxforge/diago/internal/theme"
	"github.com/stretchr/testify/assert"
)

// testStyle returns a default node style for tests.
func testStyle() theme.NodeStyle {
	return theme.DefaultTheme().Node
}

// testNode builds a PositionedNode centered at (100, 100) with given shape.
func testNode(id string, shape model.Shape) model.PositionedNode {
	return model.PositionedNode{
		ID:     id,
		Label:  "Test Label",
		Shape:  shape,
		X:      100,
		Y:      100,
		Width:  120,
		Height: 48,
	}
}

func renderGroupToString(g SVGGroup) string {
	var buf bytes.Buffer
	g.Render(&buf)
	return buf.String()
}

var allShapes = []struct {
	name     string
	shape    model.Shape
	expected []string
}{
	{
		name:     "rect",
		shape:    model.ShapeRect,
		expected: []string{"<rect", "Test Label"},
	},
	{
		name:     "rounded",
		shape:    model.ShapeRounded,
		expected: []string{"<rect", `rx="`, "Test Label"},
	},
	{
		name:     "circle",
		shape:    model.ShapeCircle,
		expected: []string{"<circle", "Test Label"},
	},
	{
		name:     "diamond",
		shape:    model.ShapeDiamond,
		expected: []string{"<polygon", "Test Label"},
	},
	{
		name:     "cylinder",
		shape:    model.ShapeCylinder,
		expected: []string{"<path", "<ellipse", "Test Label"},
	},
	{
		name:     "hexagon",
		shape:    model.ShapeHexagon,
		expected: []string{"<polygon", "Test Label"},
	},
	{
		name:     "parallelogram",
		shape:    model.ShapeParallelogram,
		expected: []string{"<polygon", "Test Label"},
	},
}

func TestEachShapeRender(t *testing.T) {
	style := testStyle()
	for _, tc := range allShapes {
		t.Run(tc.name, func(t *testing.T) {
			node := testNode("n1", tc.shape)
			grp := RenderNode(node, style, false)
			out := renderGroupToString(grp)
			assert.NotEmpty(t, out, "output should not be empty")
			for _, want := range tc.expected {
				assert.Contains(t, out, want, "shape %s: expected %q in output", tc.name, want)
			}
			assert.Contains(t, out, `id="node-n1"`, "group should have correct id")
		})
	}
}

func TestRenderRectAttributes(t *testing.T) {
	style := testStyle()
	node := testNode("r1", model.ShapeRect)
	grp := RenderRect(node, style, false)
	out := renderGroupToString(grp)
	// Node is 120x48 centered at (100,100) — top-left should be at (40, 76).
	assert.Contains(t, out, `x="40"`)
	assert.Contains(t, out, `y="76"`)
	assert.Contains(t, out, `width="120"`)
	assert.Contains(t, out, `height="48"`)
}

func TestRenderRoundedHasRadius(t *testing.T) {
	style := testStyle()
	style.CornerRadius = 8
	node := testNode("r2", model.ShapeRounded)
	grp := RenderRounded(node, style, false)
	out := renderGroupToString(grp)
	assert.Contains(t, out, `rx="8"`)
}

func TestRenderCircleCenteredCorrectly(t *testing.T) {
	style := testStyle()
	node := testNode("c1", model.ShapeCircle)
	grp := RenderCircle(node, style, false)
	out := renderGroupToString(grp)
	assert.Contains(t, out, `cx="100"`)
	assert.Contains(t, out, `cy="100"`)
}

func TestRenderDiamondFourPoints(t *testing.T) {
	style := testStyle()
	node := testNode("d1", model.ShapeDiamond)
	grp := RenderDiamond(node, style, false)
	out := renderGroupToString(grp)
	// Diamond has 4 points: top (100,76), right (160,100), bottom (100,124), left (40,100)
	assert.Contains(t, out, `<polygon`)
}

func TestRenderCylinderHasEllipse(t *testing.T) {
	style := testStyle()
	node := testNode("cy1", model.ShapeCylinder)
	grp := RenderCylinder(node, style, false)
	out := renderGroupToString(grp)
	assert.Contains(t, out, `<ellipse`)
	assert.Contains(t, out, `<path`)
}

func TestRenderNodeDispatch(t *testing.T) {
	style := testStyle()
	for _, shape := range []model.Shape{
		model.ShapeRect, model.ShapeRounded, model.ShapeCircle,
		model.ShapeDiamond, model.ShapeCylinder, model.ShapeHexagon,
		model.ShapeParallelogram,
	} {
		node := testNode("x", shape)
		grp := RenderNode(node, style, false)
		out := renderGroupToString(grp)
		assert.NotEmpty(t, out, "RenderNode should produce output for shape %v", shape)
	}
}

func TestRenderParallelogramWithinBBox(t *testing.T) {
	// C2.2: the drawn outline must stay inside the node box (C0).
	// testNode: 120x48 centered at (100,100) → bbox [40,160]x[76,124],
	// slant = 0.3*48 = 14.4.
	style := testStyle()
	node := testNode("p1", model.ShapeParallelogram)
	grp := RenderParallelogram(node, style, false)
	poly, ok := grp.Children[0].(Polygon)
	if !ok {
		t.Fatalf("expected Polygon child, got %T", grp.Children[0])
	}
	if len(poly.Points) != 4 {
		t.Fatalf("expected 4 points, got %d", len(poly.Points))
	}
	for i, p := range poly.Points {
		if p.X < 40-1e-9 || p.X > 160+1e-9 || p.Y < 76-1e-9 || p.Y > 124+1e-9 {
			t.Errorf("point %d (%g, %g) outside bbox [40,160]x[76,124]", i, p.X, p.Y)
		}
	}
	// Right lean: top-left inset by the slant, bottom-left flush with bbox.
	if got, want := poly.Points[0].X, 40+node.Height*0.3; mathAbs(got-want) > 1e-9 {
		t.Errorf("top-left X = %g, want %g", got, want)
	}
	if got := poly.Points[3].X; mathAbs(got-40) > 1e-9 {
		t.Errorf("bottom-left X = %g, want 40", got)
	}
}

func mathAbs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// TestNodeLabel_DrawsTheMeasuredLines: a node draws its line list, of any
// length, instead of its raw label: the layered engine (S1) keeps a
// one-entry list when a label's runs of spaces collapse, and the label as
// written would be wider than the box it measured.
func TestNodeLabel_DrawsTheMeasuredLines(t *testing.T) {
	n := testNode("a", model.ShapeRect)
	n.Label = "a    b"
	n.Lines = []string{"a b"}
	var buf bytes.Buffer
	nodeLabel(n, testStyle()).Render(&buf)
	assert.Contains(t, buf.String(), ">a b</text>")
	assert.NotContains(t, buf.String(), "a    b")

	n.Lines = nil
	buf.Reset()
	nodeLabel(n, testStyle()).Render(&buf)
	assert.Contains(t, buf.String(), ">a    b</text>", "no list: the label as written")
}
