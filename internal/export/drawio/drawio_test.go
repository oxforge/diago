package drawio

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/oxforge/diago/internal/model"
	"github.com/oxforge/diago/internal/theme"
	"github.com/stretchr/testify/assert"
)

func TestExport_SingleNode(t *testing.T) {
	pg := &model.PositionedGraph{
		Nodes: []model.PositionedNode{
			{ID: "a", Label: "Node A", Shape: model.ShapeRect, X: 100, Y: 50, Width: 120, Height: 60},
		},
	}
	data := Export(pg, theme.DefaultTheme())
	s := string(data)

	if !strings.Contains(s, `<?xml version="1.0" encoding="UTF-8"?>`) {
		t.Error("missing XML declaration")
	}
	if !strings.Contains(s, `<mxfile>`) {
		t.Error("missing mxfile element")
	}
	if !strings.Contains(s, `id="a"`) {
		t.Error("missing node ID")
	}
	if !strings.Contains(s, `value="Node A"`) {
		t.Error("missing node label")
	}
	if !strings.Contains(s, `vertex="1"`) {
		t.Error("missing vertex attribute")
	}
	// Center-based to top-left: x=100-60=40, y=50-30=20
	if !strings.Contains(s, `x="40"`) || !strings.Contains(s, `y="20"`) {
		t.Errorf("incorrect geometry, expected x=40 y=20, got:\n%s", s)
	}
	if !strings.Contains(s, `width="120"`) || !strings.Contains(s, `height="60"`) {
		t.Error("incorrect dimensions")
	}
}

func TestExport_TwoNodesWithEdge(t *testing.T) {
	pg := &model.PositionedGraph{
		Nodes: []model.PositionedNode{
			{ID: "a", Label: "A", Shape: model.ShapeRect, X: 100, Y: 50, Width: 80, Height: 40},
			{ID: "b", Label: "B", Shape: model.ShapeRect, X: 100, Y: 150, Width: 80, Height: 40},
		},
		Edges: []model.PositionedEdge{
			{From: "a", To: "b", Label: "connects", Style: model.EdgeSolid, Direction: model.EdgeForward,
				Points: []model.Point{{X: 100, Y: 70}, {X: 100, Y: 130}}},
		},
	}
	data := Export(pg, theme.DefaultTheme())
	s := string(data)

	if !strings.Contains(s, `source="a"`) {
		t.Error("missing edge source")
	}
	if !strings.Contains(s, `target="b"`) {
		t.Error("missing edge target")
	}
	if !strings.Contains(s, `edge="1"`) {
		t.Error("missing edge attribute")
	}
	if !strings.Contains(s, `value="connects"`) {
		t.Error("missing edge label")
	}
	// Only 2 points = endpoints, no intermediate waypoints expected.
	if strings.Contains(s, `<Array`) {
		t.Error("should not have Array element for edge with only 2 points")
	}
}

func TestExport_AllShapeTypes(t *testing.T) {
	tests := []struct {
		shape model.Shape
		want  string
	}{
		{model.ShapeRect, "rounded=0;whiteSpace=wrap;html=1;"},
		{model.ShapeRounded, "rounded=1;whiteSpace=wrap;html=1;"},
		{model.ShapeCircle, "ellipse;whiteSpace=wrap;html=1;"},
		{model.ShapeDiamond, "rhombus;whiteSpace=wrap;html=1;"},
		{model.ShapeCylinder, "shape=cylinder3;whiteSpace=wrap;html=1;"},
		{model.ShapeHexagon, "shape=hexagon;perimeter=hexagonPerimeter2;whiteSpace=wrap;html=1;"},
		{model.ShapeParallelogram, "shape=parallelogram;perimeter=parallelogramPerimeter;whiteSpace=wrap;html=1;"},
	}

	for _, tt := range tests {
		t.Run(tt.shape.String(), func(t *testing.T) {
			pg := &model.PositionedGraph{
				Nodes: []model.PositionedNode{
					{ID: "n", Label: "N", Shape: tt.shape, X: 50, Y: 50, Width: 80, Height: 40},
				},
			}
			data := Export(pg, theme.DefaultTheme())
			if !strings.Contains(string(data), tt.want) {
				t.Errorf("shape %s: expected style %q in output", tt.shape, tt.want)
			}
		})
	}
}

func TestExport_EdgeWithWaypoints(t *testing.T) {
	pg := &model.PositionedGraph{
		Nodes: []model.PositionedNode{
			{ID: "a", Label: "A", Shape: model.ShapeRect, X: 50, Y: 50, Width: 80, Height: 40},
			{ID: "b", Label: "B", Shape: model.ShapeRect, X: 250, Y: 150, Width: 80, Height: 40},
		},
		Edges: []model.PositionedEdge{
			{From: "a", To: "b", Style: model.EdgeSolid, Direction: model.EdgeForward,
				Points: []model.Point{
					{X: 90, Y: 50},   // start
					{X: 150, Y: 50},  // intermediate
					{X: 150, Y: 150}, // intermediate
					{X: 210, Y: 150}, // end
				}},
		},
	}
	data := Export(pg, theme.DefaultTheme())
	s := string(data)

	if !strings.Contains(s, "<Array") {
		t.Error("missing Array element for waypoints")
	}
	if !strings.Contains(s, `x="150"`) {
		t.Error("missing intermediate waypoint x=150")
	}
	// Should have exactly 2 intermediate points.
	count := strings.Count(s, "<mxPoint")
	if count != 2 {
		t.Errorf("expected 2 intermediate mxPoints, got %d", count)
	}
}

func TestExport_GroupsWithChildren(t *testing.T) {
	pg := &model.PositionedGraph{
		Nodes: []model.PositionedNode{
			{ID: "api", Label: "API", Shape: model.ShapeRect, X: 120, Y: 80, Width: 80, Height: 40},
			{ID: "db", Label: "DB", Shape: model.ShapeCylinder, X: 120, Y: 160, Width: 80, Height: 40},
		},
		Groups: []model.PositionedGroup{
			{ID: "backend", Label: "Backend", X: 50, Y: 30, Width: 200, Height: 200,
				Contains: []string{"api", "db"}, Depth: 1},
		},
	}
	data := Export(pg, theme.DefaultTheme())
	s := string(data)

	// Group should be a container.
	if !strings.Contains(s, `id="backend"`) {
		t.Error("missing group ID")
	}
	if !strings.Contains(s, "swimlane;") {
		t.Error("missing swimlane style")
	}
	if !strings.Contains(s, "container=0") && !strings.Contains(s, "collapsible=0") {
		t.Error("missing collapsible=0 style")
	}

	// Nodes should have parent=backend.
	// Check that both nodes reference the group as parent.
	if strings.Count(s, `parent="backend"`) != 2 {
		t.Errorf("expected 2 cells with parent=backend, got %d", strings.Count(s, `parent="backend"`))
	}
}

func TestExport_NestedGroups(t *testing.T) {
	pg := &model.PositionedGraph{
		Nodes: []model.PositionedNode{
			{ID: "n1", Label: "N1", Shape: model.ShapeRect, X: 120, Y: 100, Width: 60, Height: 30},
		},
		Groups: []model.PositionedGroup{
			{ID: "outer", Label: "Outer", X: 10, Y: 10, Width: 300, Height: 300,
				Contains: []string{}, Children: []string{"inner"}, Depth: 1},
			{ID: "inner", Label: "Inner", X: 50, Y: 50, Width: 200, Height: 200,
				Contains: []string{"n1"}, Depth: 2},
		},
	}
	data := Export(pg, theme.DefaultTheme())
	s := string(data)

	// inner group should have parent=outer.
	if !strings.Contains(s, `id="inner"`) {
		t.Error("missing inner group")
	}
	// inner's parent should be outer.
	// Parse to verify structure.
	var file mxFile
	if err := xml.Unmarshal(data, &file); err != nil {
		t.Fatalf("failed to parse XML: %v", err)
	}

	cellMap := make(map[string]mxCell)
	for _, c := range file.Diagram.GraphModel.Root.Cells {
		cellMap[c.ID] = c
	}

	inner := cellMap["inner"]
	if inner.Parent != "outer" {
		t.Errorf("inner group parent = %q, want %q", inner.Parent, "outer")
	}

	n1 := cellMap["n1"]
	if n1.Parent != "inner" {
		t.Errorf("node n1 parent = %q, want %q", n1.Parent, "inner")
	}

	// inner group position should be relative to outer.
	if inner.Geometry == nil {
		t.Fatal("inner group has no geometry")
	}
	// inner absolute X=50, outer absolute X=10 → relative X=40
	if inner.Geometry.X != 40 {
		t.Errorf("inner group relative X = %v, want 40", inner.Geometry.X)
	}
	if inner.Geometry.Y != 40 {
		t.Errorf("inner group relative Y = %v, want 40", inner.Geometry.Y)
	}

	// n1 position should be relative to inner group.
	if n1.Geometry == nil {
		t.Fatal("n1 has no geometry")
	}
	// n1 center (120,100), size (60,30) → top-left (90, 85)
	// inner group at (50, 50) → relative (40, 35)
	if n1.Geometry.X != 40 {
		t.Errorf("n1 relative X = %v, want 40", n1.Geometry.X)
	}
	if n1.Geometry.Y != 35 {
		t.Errorf("n1 relative Y = %v, want 35", n1.Geometry.Y)
	}
}

func TestExport_EdgeStyles(t *testing.T) {
	tests := []struct {
		style model.EdgeStyle
		want  string
	}{
		{model.EdgeSolid, "endArrow=block;"},
		{model.EdgeDashed, "dashed=1;"},
		{model.EdgeDotted, "dashPattern=1 3;"},
		{model.EdgeThick, "strokeWidth=3;"},
	}

	for _, tt := range tests {
		t.Run(tt.style.String(), func(t *testing.T) {
			pg := &model.PositionedGraph{
				Nodes: []model.PositionedNode{
					{ID: "a", Label: "A", Shape: model.ShapeRect, X: 50, Y: 50, Width: 80, Height: 40},
					{ID: "b", Label: "B", Shape: model.ShapeRect, X: 200, Y: 50, Width: 80, Height: 40},
				},
				Edges: []model.PositionedEdge{
					{From: "a", To: "b", Style: tt.style, Direction: model.EdgeForward,
						Points: []model.Point{{X: 90, Y: 50}, {X: 160, Y: 50}}},
				},
			}
			data := Export(pg, theme.DefaultTheme())
			if !strings.Contains(string(data), tt.want) {
				t.Errorf("edge style %s: expected %q in output", tt.style, tt.want)
			}
		})
	}
}

func TestExport_EdgeDirections(t *testing.T) {
	tests := []struct {
		dir  model.EdgeDirection
		want string
	}{
		{model.EdgeForward, "endArrow=block;endFill=1;startArrow=none;"},
		{model.EdgeBackward, "startArrow=block;startFill=1;endArrow=none;"},
		{model.EdgeBoth, "startArrow=block;startFill=1;endArrow=block;endFill=1;"},
	}

	for _, tt := range tests {
		t.Run(tt.dir.String(), func(t *testing.T) {
			pg := &model.PositionedGraph{
				Nodes: []model.PositionedNode{
					{ID: "a", Label: "A", Shape: model.ShapeRect, X: 50, Y: 50, Width: 80, Height: 40},
					{ID: "b", Label: "B", Shape: model.ShapeRect, X: 200, Y: 50, Width: 80, Height: 40},
				},
				Edges: []model.PositionedEdge{
					{From: "a", To: "b", Style: model.EdgeSolid, Direction: tt.dir,
						Points: []model.Point{{X: 90, Y: 50}, {X: 160, Y: 50}}},
				},
			}
			data := Export(pg, theme.DefaultTheme())
			if !strings.Contains(string(data), tt.want) {
				t.Errorf("edge direction %s: expected %q in output", tt.dir, tt.want)
			}
		})
	}
}

func TestExport_MultiLineLabel(t *testing.T) {
	pg := &model.PositionedGraph{
		Nodes: []model.PositionedNode{
			{ID: "n", Label: "Line 1\nLine 2", Shape: model.ShapeRect, X: 50, Y: 50, Width: 80, Height: 40},
		},
	}
	data := Export(pg, theme.DefaultTheme())

	// XML round-trip should preserve the newline in the label.
	var file mxFile
	if err := xml.Unmarshal(data, &file); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}
	for _, c := range file.Diagram.GraphModel.Root.Cells {
		if c.ID == "n" {
			if c.Value != "Line 1\nLine 2" {
				t.Errorf("label = %q, want %q", c.Value, "Line 1\nLine 2")
			}
			return
		}
	}
	t.Error("node 'n' not found")
}

func TestExport_ValidXML(t *testing.T) {
	pg := &model.PositionedGraph{
		Nodes: []model.PositionedNode{
			{ID: "a", Label: "A", Shape: model.ShapeRect, X: 50, Y: 50, Width: 80, Height: 40},
			{ID: "b", Label: "B", Shape: model.ShapeRounded, X: 200, Y: 50, Width: 80, Height: 40},
		},
		Edges: []model.PositionedEdge{
			{From: "a", To: "b", Label: "edge", Style: model.EdgeSolid, Direction: model.EdgeForward,
				Points: []model.Point{{X: 90, Y: 50}, {X: 160, Y: 50}}},
		},
		Groups: []model.PositionedGroup{
			{ID: "g1", Label: "Group", X: 10, Y: 10, Width: 300, Height: 100,
				Contains: []string{"a", "b"}, Depth: 1},
		},
	}
	data := Export(pg, theme.DefaultTheme())

	// Strip XML header for unmarshal.
	var file mxFile
	if err := xml.Unmarshal(data, &file); err != nil {
		t.Fatalf("output is not valid XML: %v\n%s", err, string(data))
	}

	if file.Diagram.Name != "Page-1" {
		t.Errorf("diagram name = %q, want %q", file.Diagram.Name, "Page-1")
	}

	// 2 root cells + 1 group + 2 nodes + 1 edge + 1 group title = 7
	if len(file.Diagram.GraphModel.Root.Cells) != 7 {
		t.Errorf("expected 7 cells, got %d", len(file.Diagram.GraphModel.Root.Cells))
	}
}

func TestExport_EmptyGraph(t *testing.T) {
	pg := &model.PositionedGraph{}
	data := Export(pg, theme.DefaultTheme())

	// Should produce valid XML with just the 2 root cells.
	var file mxFile
	if err := xml.Unmarshal(data, &file); err != nil {
		t.Fatalf("output is not valid XML: %v", err)
	}
	if len(file.Diagram.GraphModel.Root.Cells) != 2 {
		t.Errorf("expected 2 root cells, got %d", len(file.Diagram.GraphModel.Root.Cells))
	}
}

func TestExport_EdgeExitEntryPoints(t *testing.T) {
	pg := &model.PositionedGraph{
		Nodes: []model.PositionedNode{
			// Node A centered at (100,50), size 80x40 → top-left (60,30), bottom-right (140,70)
			{ID: "a", Label: "A", Shape: model.ShapeRect, X: 100, Y: 50, Width: 80, Height: 40},
			// Node B centered at (100,200), size 80x40
			{ID: "b", Label: "B", Shape: model.ShapeRect, X: 100, Y: 200, Width: 80, Height: 40},
		},
		Edges: []model.PositionedEdge{
			{From: "a", To: "b", Style: model.EdgeSolid, Direction: model.EdgeForward,
				// Exit from bottom center of A (100,70), enter top center of B (100,180)
				Points: []model.Point{{X: 100, Y: 70}, {X: 100, Y: 180}}},
		},
	}
	data := Export(pg, theme.DefaultTheme())
	s := string(data)

	// exitX=0.50 (center), exitY=1.00 (bottom)
	if !strings.Contains(s, "exitX=0.50") || !strings.Contains(s, "exitY=1.00") {
		t.Errorf("expected exit from bottom center, got:\n%s", s)
	}
	// entryX=0.50 (center), entryY=0.00 (top)
	if !strings.Contains(s, "entryX=0.50") || !strings.Contains(s, "entryY=0.00") {
		t.Errorf("expected entry at top center, got:\n%s", s)
	}
}

func TestExport_OrthogonalEdges(t *testing.T) {
	pg := &model.PositionedGraph{
		Nodes: []model.PositionedNode{
			{ID: "a", Label: "A", Shape: model.ShapeRect, X: 50, Y: 50, Width: 80, Height: 40},
			{ID: "b", Label: "B", Shape: model.ShapeRect, X: 200, Y: 150, Width: 80, Height: 40},
		},
		Edges: []model.PositionedEdge{
			{From: "a", To: "b", Style: model.EdgeSolid, Direction: model.EdgeForward,
				Points: []model.Point{{X: 50, Y: 70}, {X: 50, Y: 150}, {X: 160, Y: 150}}},
		},
	}
	data := Export(pg, theme.DefaultTheme())
	s := string(data)

	if !strings.Contains(s, "edgeStyle=orthogonalEdgeStyle;") {
		t.Error("diago is orthogonal-only: every edge should produce orthogonalEdgeStyle in draw.io")
	}
}

func TestExport_GroupsVisibleAsSwimlane(t *testing.T) {
	pg := &model.PositionedGraph{
		Nodes: []model.PositionedNode{
			{ID: "a", Label: "A", Shape: model.ShapeRect, X: 100, Y: 80, Width: 80, Height: 40},
		},
		Groups: []model.PositionedGroup{
			{ID: "g", Label: "My Group", X: 50, Y: 30, Width: 200, Height: 150,
				Contains: []string{"a"}, Depth: 1},
		},
	}
	data := Export(pg, theme.DefaultTheme())
	s := string(data)

	if !strings.Contains(s, "swimlane;") {
		t.Error("groups should use swimlane style to be visible in draw.io")
	}
	if !strings.Contains(s, `value="My Group"`) {
		t.Error("group label should be present")
	}
}

// TestExport_GroupTitleAtItsOffset pins a group title where the layout
// placed it: LabelOffset from the group's left side and centered 16 px
// below its top, or 8 px in when the layout left the title to the
// renderer.
func TestExport_GroupTitleAtItsOffset(t *testing.T) {
	export := func(offset float64) string {
		pg := &model.PositionedGraph{
			Groups: []model.PositionedGroup{
				{ID: "g", Label: "My Group", X: 50, Y: 30, Width: 200, Height: 150, Depth: 1,
					LabelWidth: 60, LabelHeight: 15, LabelOffset: offset},
			},
		}
		return string(Export(pg, theme.DefaultTheme()))
	}
	assert.Contains(t, export(40), `<mxGeometry x="90" y="38.5" width="60" height="15" as="geometry">`, "the placed title")
	assert.Contains(t, export(0), `<mxGeometry x="58" y="38.5" width="60" height="15" as="geometry">`, "the renderer's default slot")
}

// TestExport_GroupTitleInFront pins the title layer (C13): a group's title
// is a text cell on the page after every edge, left-aligned, on a
// background in its group's fill that gaps an edge under it, and the
// swimlane keeps its box without a value.
func TestExport_GroupTitleInFront(t *testing.T) {
	pg := &model.PositionedGraph{
		Nodes: []model.PositionedNode{
			{ID: "a", Label: "A", Shape: model.ShapeRect, X: 100, Y: 10, Width: 80, Height: 20},
			{ID: "b", Label: "B", Shape: model.ShapeRect, X: 100, Y: 100, Width: 80, Height: 40},
		},
		Edges: []model.PositionedEdge{{ID: "a->b#0", From: "a", To: "b", Points: []model.Point{{X: 100, Y: 20}, {X: 100, Y: 80}}}},
		Groups: []model.PositionedGroup{
			{ID: "g", Label: "My Group", X: 50, Y: 30, Width: 200, Height: 150, Depth: 1, Contains: []string{"b"},
				LabelWidth: 60, LabelHeight: 15, LabelOffset: 40, LabelBlocked: true},
		},
	}
	s := string(Export(pg, theme.DefaultTheme()))
	title := strings.Index(s, `<mxCell id="title_0" value="My Group" style="text;`)
	if assert.GreaterOrEqual(t, title, 0, "the title is its own text cell") {
		assert.Greater(t, title, strings.Index(s, `id="edge_0"`), "after the edge")
		cell := s[title:]
		cell = cell[:strings.Index(cell, "</mxCell>")]
		assert.Contains(t, cell, "align=left;")
		assert.Contains(t, cell, "labelBackgroundColor=default;", "the swimlane header's own fill")
		assert.Contains(t, cell, `parent="1"`, "on the page")
	}
	assert.Contains(t, s, `<mxCell id="g" style="swimlane;`, "the swimlane has no value")

	pg.Groups[0].Color = "blue"
	dc := theme.DeriveColors(theme.DefaultTheme().Colors["blue"], theme.DefaultTheme().Background, theme.ElementGroup)
	assert.Contains(t, string(Export(pg, theme.DefaultTheme())), "fontColor="+dc.Text+";labelBackgroundColor="+dc.Fill+";", "an accent group's colors")
}

func TestExport_SpecialCharsInLabel(t *testing.T) {
	pg := &model.PositionedGraph{
		Nodes: []model.PositionedNode{
			{ID: "n", Label: `A <b>&</b> "C"`, Shape: model.ShapeRect, X: 50, Y: 50, Width: 80, Height: 40},
		},
	}
	data := Export(pg, theme.DefaultTheme())

	// XML should escape special characters properly.
	var file mxFile
	if err := xml.Unmarshal(data, &file); err != nil {
		t.Fatalf("output is not valid XML: %v", err)
	}

	// After round-trip, label should be preserved.
	for _, c := range file.Diagram.GraphModel.Root.Cells {
		if c.ID == "n" {
			if c.Value != `A <b>&</b> "C"` {
				t.Errorf("label = %q, want %q", c.Value, `A <b>&</b> "C"`)
			}
			return
		}
	}
	t.Error("node 'n' not found in output")
}

func TestEdgeStyle_None(t *testing.T) {
	s := edgeStyle(model.EdgeSolid, model.EdgeNone)
	assert.Contains(t, s, "startArrow=none;endArrow=none;")
	assert.NotContains(t, s, "block")
}

func TestEdgeStyle_AlwaysOrthogonal(t *testing.T) {
	for _, s := range []model.EdgeStyle{model.EdgeSolid, model.EdgeDashed, model.EdgeDotted, model.EdgeThick} {
		got := edgeStyle(s, model.EdgeForward)
		if !strings.HasPrefix(got, "edgeStyle=orthogonalEdgeStyle;") {
			t.Errorf("edgeStyle(%v) = %q, want the orthogonal prefix", s, got)
		}
	}
}
