package excalidraw

import (
	"encoding/json"
	"testing"

	"github.com/oxforge/diago/internal/model"
	"github.com/oxforge/diago/internal/theme"
)

func TestSingleNode(t *testing.T) {
	pg := &model.PositionedGraph{
		Nodes: []model.PositionedNode{
			{ID: "a", Label: "Node A", Shape: model.ShapeRect, X: 100, Y: 50, Width: 120, Height: 60},
		},
	}

	data, err := Export(pg, theme.DefaultTheme())
	if err != nil {
		t.Fatal(err)
	}

	doc := parseDoc(t, data)
	if doc.Type != "excalidraw" || doc.Version != 2 {
		t.Fatalf("unexpected type/version: %s/%d", doc.Type, doc.Version)
	}

	// Should have exactly 2 elements: shape + text.
	if len(doc.Elements) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(doc.Elements))
	}

	shape := findElement(doc.Elements, "a")
	if shape == nil {
		t.Fatal("shape element not found")
	}
	if shape.Type != "rectangle" {
		t.Errorf("expected rectangle, got %s", shape.Type)
	}
	// Center (100,50) with size (120,60) -> top-left (40, 20).
	if shape.X != 40 || shape.Y != 20 {
		t.Errorf("expected x=40 y=20, got x=%v y=%v", shape.X, shape.Y)
	}
	if shape.Width != 120 || shape.Height != 60 {
		t.Errorf("expected 120x60, got %vx%v", shape.Width, shape.Height)
	}

	text := findElement(doc.Elements, "text_a")
	if text == nil {
		t.Fatal("text element not found")
	}
	if text.Type != "text" {
		t.Errorf("expected text, got %s", text.Type)
	}
	if text.Text != "Node A" {
		t.Errorf("expected label 'Node A', got %q", text.Text)
	}
	if text.ContainerID == nil || *text.ContainerID != "a" {
		t.Error("text containerId should reference shape")
	}

	// Shape should have text in boundElements.
	if len(shape.BoundElements) == 0 {
		t.Fatal("shape should have boundElements")
	}
	found := false
	for _, b := range shape.BoundElements {
		if b.ID == "text_a" && b.Type == "text" {
			found = true
		}
	}
	if !found {
		t.Error("shape boundElements missing text_a")
	}
}

func TestTwoNodesWithEdge(t *testing.T) {
	pg := &model.PositionedGraph{
		Nodes: []model.PositionedNode{
			{ID: "a", Label: "A", Shape: model.ShapeRect, X: 100, Y: 50, Width: 80, Height: 40},
			{ID: "b", Label: "B", Shape: model.ShapeRect, X: 100, Y: 200, Width: 80, Height: 40},
		},
		Edges: []model.PositionedEdge{
			{
				From: "a", To: "b", Style: model.EdgeSolid, Direction: model.EdgeForward,
				Points: []model.Point{{X: 100, Y: 70}, {X: 100, Y: 180}},
			},
		},
	}

	data, err := Export(pg, theme.DefaultTheme())
	if err != nil {
		t.Fatal(err)
	}

	doc := parseDoc(t, data)

	// 2 shapes + 2 texts + 1 arrow = 5.
	if len(doc.Elements) != 5 {
		t.Fatalf("expected 5 elements, got %d", len(doc.Elements))
	}

	arrow := findElement(doc.Elements, "edge_0")
	if arrow == nil {
		t.Fatal("arrow not found")
	}
	if arrow.Type != "arrow" {
		t.Errorf("expected arrow, got %s", arrow.Type)
	}

	// Arrow x,y should be first point.
	if arrow.X != 100 || arrow.Y != 70 {
		t.Errorf("expected arrow at 100,70 got %v,%v", arrow.X, arrow.Y)
	}

	// Points should be relative.
	if len(arrow.Points) != 2 {
		t.Fatalf("expected 2 points, got %d", len(arrow.Points))
	}
	if arrow.Points[0][0] != 0 || arrow.Points[0][1] != 0 {
		t.Error("first point should be [0,0]")
	}
	if arrow.Points[1][0] != 0 || arrow.Points[1][1] != 110 {
		t.Errorf("second point should be [0,110], got %v", arrow.Points[1])
	}

	// Check bindings.
	if arrow.StartBinding == nil || arrow.StartBinding.ElementID != "a" {
		t.Error("startBinding should reference 'a'")
	}
	if arrow.EndBinding == nil || arrow.EndBinding.ElementID != "b" {
		t.Error("endBinding should reference 'b'")
	}

	// Check arrowheads for forward direction.
	if arrow.StartArrowhead != nil {
		t.Error("forward edge should have nil startArrowhead")
	}
	if arrow.EndArrowhead == nil || *arrow.EndArrowhead != "arrow" {
		t.Error("forward edge should have 'arrow' endArrowhead")
	}

	// Shapes should have arrow in boundElements.
	shapeA := findElement(doc.Elements, "a")
	if !hasBound(shapeA.BoundElements, "edge_0", "arrow") {
		t.Error("shape 'a' should have edge_0 in boundElements")
	}
	shapeB := findElement(doc.Elements, "b")
	if !hasBound(shapeB.BoundElements, "edge_0", "arrow") {
		t.Error("shape 'b' should have edge_0 in boundElements")
	}
}

func TestDiamondShape(t *testing.T) {
	pg := &model.PositionedGraph{
		Nodes: []model.PositionedNode{
			{ID: "d", Label: "Decision", Shape: model.ShapeDiamond, X: 50, Y: 50, Width: 80, Height: 80},
		},
	}

	data, err := Export(pg, theme.DefaultTheme())
	if err != nil {
		t.Fatal(err)
	}

	doc := parseDoc(t, data)
	shape := findElement(doc.Elements, "d")
	if shape.Type != "diamond" {
		t.Errorf("expected diamond, got %s", shape.Type)
	}
}

func TestCircleShape(t *testing.T) {
	pg := &model.PositionedGraph{
		Nodes: []model.PositionedNode{
			{ID: "c", Label: "Circle", Shape: model.ShapeCircle, X: 50, Y: 50, Width: 60, Height: 60},
		},
	}

	data, err := Export(pg, theme.DefaultTheme())
	if err != nil {
		t.Fatal(err)
	}

	doc := parseDoc(t, data)
	shape := findElement(doc.Elements, "c")
	if shape.Type != "ellipse" {
		t.Errorf("expected ellipse, got %s", shape.Type)
	}
}

func TestLossyShapes(t *testing.T) {
	for _, s := range []model.Shape{model.ShapeCylinder, model.ShapeHexagon, model.ShapeParallelogram} {
		pg := &model.PositionedGraph{
			Nodes: []model.PositionedNode{
				{ID: "n", Label: "N", Shape: s, X: 50, Y: 50, Width: 80, Height: 40},
			},
		}
		data, err := Export(pg, theme.DefaultTheme())
		if err != nil {
			t.Fatal(err)
		}
		doc := parseDoc(t, data)
		shape := findElement(doc.Elements, "n")
		if shape.Type != "rectangle" {
			t.Errorf("shape %s: expected rectangle, got %s", s, shape.Type)
		}
	}
}

func TestRoundedShape(t *testing.T) {
	pg := &model.PositionedGraph{
		Nodes: []model.PositionedNode{
			{ID: "r", Label: "Rounded", Shape: model.ShapeRounded, X: 50, Y: 50, Width: 80, Height: 40},
		},
	}

	data, err := Export(pg, theme.DefaultTheme())
	if err != nil {
		t.Fatal(err)
	}

	doc := parseDoc(t, data)
	shape := findElement(doc.Elements, "r")
	if shape.Type != "rectangle" {
		t.Errorf("expected rectangle, got %s", shape.Type)
	}
	if shape.Roundness == nil || shape.Roundness.Type != 3 {
		t.Error("rounded shape should have roundness type 3")
	}
}

func TestEdgeWithLabel(t *testing.T) {
	lp := model.Point{X: 100, Y: 135}
	pg := &model.PositionedGraph{
		Nodes: []model.PositionedNode{
			{ID: "a", Label: "A", Shape: model.ShapeRect, X: 100, Y: 50, Width: 80, Height: 40},
			{ID: "b", Label: "B", Shape: model.ShapeRect, X: 100, Y: 200, Width: 80, Height: 40},
		},
		Edges: []model.PositionedEdge{
			{
				From: "a", To: "b", Label: "connects",
				Style: model.EdgeSolid, Direction: model.EdgeForward,
				Points:      []model.Point{{X: 100, Y: 70}, {X: 100, Y: 180}},
				LabelPos:    &lp,
				LabelWidth:  60,
				LabelHeight: 20,
			},
		},
	}

	data, err := Export(pg, theme.DefaultTheme())
	if err != nil {
		t.Fatal(err)
	}

	doc := parseDoc(t, data)

	labelText := findElement(doc.Elements, "edgelabel_0")
	if labelText == nil {
		t.Fatal("edge label text not found")
	}
	if labelText.Text != "connects" {
		t.Errorf("expected 'connects', got %q", labelText.Text)
	}
	if labelText.ContainerID == nil || *labelText.ContainerID != "edge_0" {
		t.Error("label should have containerId = edge_0")
	}

	arrow := findElement(doc.Elements, "edge_0")
	if !hasBound(arrow.BoundElements, "edgelabel_0", "text") {
		t.Error("arrow should have label in boundElements")
	}
}

func TestGroups(t *testing.T) {
	pg := &model.PositionedGraph{
		Nodes: []model.PositionedNode{
			{ID: "a", Label: "A", Shape: model.ShapeRect, X: 50, Y: 50, Width: 60, Height: 40},
			{ID: "b", Label: "B", Shape: model.ShapeRect, X: 150, Y: 50, Width: 60, Height: 40},
		},
		Groups: []model.PositionedGroup{
			{ID: "g1", Label: "Group 1", X: 10, Y: 10, Width: 200, Height: 100,
				Contains: []string{"a", "b"}, Depth: 1},
		},
	}

	data, err := Export(pg, theme.DefaultTheme())
	if err != nil {
		t.Fatal(err)
	}

	doc := parseDoc(t, data)

	shapeA := findElement(doc.Elements, "a")
	if len(shapeA.GroupIDs) != 1 || shapeA.GroupIDs[0] != "g1" {
		t.Errorf("expected groupIds [g1], got %v", shapeA.GroupIDs)
	}
	textA := findElement(doc.Elements, "text_a")
	if len(textA.GroupIDs) != 1 || textA.GroupIDs[0] != "g1" {
		t.Errorf("text should share groupIds, got %v", textA.GroupIDs)
	}

	// Group label should exist.
	grpLabel := findElement(doc.Elements, "grplabel_g1")
	if grpLabel == nil {
		t.Fatal("group label element not found")
	}
	if grpLabel.Text != "Group 1" {
		t.Errorf("expected 'Group 1', got %q", grpLabel.Text)
	}
}

// TestGroupTitleAtItsOffset pins a group label where the layout placed
// the title: LabelOffset from the group's left side, centered 16 px below
// its top, and above the group when the layout left it to the renderer.
func TestGroupTitleAtItsOffset(t *testing.T) {
	label := func(offset float64) *testElement {
		pg := &model.PositionedGraph{
			Groups: []model.PositionedGroup{
				{ID: "g1", Label: "Group 1", X: 10, Y: 10, Width: 200, Height: 100, Depth: 1,
					LabelWidth: 50, LabelHeight: 15, LabelOffset: offset},
			},
		}
		data, err := Export(pg, theme.DefaultTheme())
		if err != nil {
			t.Fatal(err)
		}
		return findElement(parseDoc(t, data).Elements, "grplabel_g1")
	}
	if l := label(120); l == nil || l.X != 130 || l.Y != 16 || l.Width != 50 {
		t.Errorf("placed title: want x=130 y=16 width=50, got %+v", l)
	}
	if l := label(0); l == nil || l.X != 10 || l.Y != -10 || l.Width != 200 {
		t.Errorf("default: want x=10 y=-10 width=200, got %+v", l)
	}
}

// TestGroupTitleInFront pins the title layer (C13): a group's label
// follows every arrow, right after a backing rectangle in the canvas
// background, 4 px wider than the title on each side, that gaps an arrow
// under it; a title without its measured extents gets no backing.
func TestGroupTitleInFront(t *testing.T) {
	pg := &model.PositionedGraph{
		Nodes: []model.PositionedNode{
			{ID: "a", Label: "A", Shape: model.ShapeRect, X: 100, Y: 10, Width: 60, Height: 20},
			{ID: "b", Label: "B", Shape: model.ShapeRect, X: 100, Y: 90, Width: 60, Height: 40},
		},
		Edges: []model.PositionedEdge{{ID: "a->b#0", From: "a", To: "b", Label: "go",
			Points: []model.Point{{X: 100, Y: 20}, {X: 100, Y: 70}}}},
		Groups: []model.PositionedGroup{
			{ID: "g1", Label: "Group 1", X: 10, Y: 10, Width: 200, Height: 140, Contains: []string{"b"}, Depth: 1,
				LabelWidth: 50, LabelHeight: 15, LabelOffset: 80, LabelBlocked: true},
		},
	}
	data, err := Export(pg, theme.DefaultTheme())
	if err != nil {
		t.Fatal(err)
	}
	els := parseDoc(t, data).Elements
	index := func(id string) int {
		for i, el := range els {
			if el.ID == id {
				return i
			}
		}
		return -1
	}
	backing, label := index("grpbacking_g1"), index("grplabel_g1")
	if backing < 0 || label != backing+1 {
		t.Fatalf("want the backing right before the label, got %d and %d", backing, label)
	}
	for _, id := range []string{"edge_0", "edgelabel_0"} {
		if i := index(id); i < 0 || i > backing {
			t.Errorf("%s at %d, want before the backing at %d", id, i, backing)
		}
	}
	b, l := els[backing], els[label]
	if b.Type != "rectangle" || b.BackgroundColor != "#ffffff" || b.StrokeWidth != 0 || b.Roughness != 0 {
		t.Errorf("backing: want a white rectangle without stroke or roughness, got %+v", b)
	}
	if b.X != l.X-4 || b.Y != l.Y || b.Width != 58 || b.Height != 20 {
		t.Errorf("backing: want x=%g y=%g width=58 height=20, got %+v", l.X-4, l.Y, b)
	}
	if len(b.GroupIDs) != 1 || b.GroupIDs[0] != "g1" {
		t.Errorf("backing: want the label's groupIds [g1], got %v", b.GroupIDs)
	}

	pg.Groups[0].LabelWidth = 0
	data, err = Export(pg, theme.DefaultTheme())
	if err != nil {
		t.Fatal(err)
	}
	if findElement(parseDoc(t, data).Elements, "grpbacking_g1") != nil {
		t.Error("a title without extents gets no backing")
	}
}

func TestNestedGroups(t *testing.T) {
	pg := &model.PositionedGraph{
		Nodes: []model.PositionedNode{
			{ID: "a", Label: "A", Shape: model.ShapeRect, X: 50, Y: 50, Width: 60, Height: 40},
		},
		Groups: []model.PositionedGroup{
			{ID: "outer", Label: "Outer", X: 0, Y: 0, Width: 300, Height: 200,
				Contains: []string{}, Children: []string{"inner"}, Depth: 1},
			{ID: "inner", Label: "Inner", X: 10, Y: 10, Width: 200, Height: 100,
				Contains: []string{"a"}, Depth: 2},
		},
	}

	data, err := Export(pg, theme.DefaultTheme())
	if err != nil {
		t.Fatal(err)
	}

	doc := parseDoc(t, data)

	shapeA := findElement(doc.Elements, "a")
	// Node 'a' is in 'inner' which is child of 'outer'.
	// groupIds should be [inner, outer].
	if len(shapeA.GroupIDs) != 2 {
		t.Fatalf("expected 2 groupIds, got %d: %v", len(shapeA.GroupIDs), shapeA.GroupIDs)
	}
	if shapeA.GroupIDs[0] != "inner" || shapeA.GroupIDs[1] != "outer" {
		t.Errorf("expected [inner, outer], got %v", shapeA.GroupIDs)
	}
}

func TestEdgeDirectionBackward(t *testing.T) {
	pg := &model.PositionedGraph{
		Nodes: []model.PositionedNode{
			{ID: "a", Label: "A", Shape: model.ShapeRect, X: 50, Y: 50, Width: 60, Height: 40},
			{ID: "b", Label: "B", Shape: model.ShapeRect, X: 150, Y: 50, Width: 60, Height: 40},
		},
		Edges: []model.PositionedEdge{
			{From: "a", To: "b", Direction: model.EdgeBackward,
				Points: []model.Point{{X: 80, Y: 50}, {X: 120, Y: 50}}},
		},
	}

	data, err := Export(pg, theme.DefaultTheme())
	if err != nil {
		t.Fatal(err)
	}

	doc := parseDoc(t, data)
	arrow := findElement(doc.Elements, "edge_0")
	if arrow.StartArrowhead == nil || *arrow.StartArrowhead != "arrow" {
		t.Error("backward should have start arrowhead")
	}
	if arrow.EndArrowhead != nil {
		t.Error("backward should have nil end arrowhead")
	}
}

func TestEdgeDirectionBoth(t *testing.T) {
	pg := &model.PositionedGraph{
		Nodes: []model.PositionedNode{
			{ID: "a", Label: "A", Shape: model.ShapeRect, X: 50, Y: 50, Width: 60, Height: 40},
			{ID: "b", Label: "B", Shape: model.ShapeRect, X: 150, Y: 50, Width: 60, Height: 40},
		},
		Edges: []model.PositionedEdge{
			{From: "a", To: "b", Direction: model.EdgeBoth,
				Points: []model.Point{{X: 80, Y: 50}, {X: 120, Y: 50}}},
		},
	}

	data, err := Export(pg, theme.DefaultTheme())
	if err != nil {
		t.Fatal(err)
	}

	doc := parseDoc(t, data)
	arrow := findElement(doc.Elements, "edge_0")
	if arrow.StartArrowhead == nil || *arrow.StartArrowhead != "arrow" {
		t.Error("both should have start arrowhead")
	}
	if arrow.EndArrowhead == nil || *arrow.EndArrowhead != "arrow" {
		t.Error("both should have end arrowhead")
	}
}

func TestEdgeStyles(t *testing.T) {
	tests := []struct {
		style      model.EdgeStyle
		wantStroke string
		wantWidth  float64
	}{
		{model.EdgeSolid, "solid", 2},
		{model.EdgeDashed, "dashed", 2},
		{model.EdgeDotted, "dotted", 2},
		{model.EdgeThick, "solid", 4},
	}
	for _, tt := range tests {
		pg := &model.PositionedGraph{
			Nodes: []model.PositionedNode{
				{ID: "a", Label: "A", Shape: model.ShapeRect, X: 50, Y: 50, Width: 60, Height: 40},
				{ID: "b", Label: "B", Shape: model.ShapeRect, X: 150, Y: 50, Width: 60, Height: 40},
			},
			Edges: []model.PositionedEdge{
				{From: "a", To: "b", Style: tt.style, Direction: model.EdgeForward,
					Points: []model.Point{{X: 80, Y: 50}, {X: 120, Y: 50}}},
			},
		}
		data, err := Export(pg, theme.DefaultTheme())
		if err != nil {
			t.Fatal(err)
		}
		doc := parseDoc(t, data)
		arrow := findElement(doc.Elements, "edge_0")
		if arrow.StrokeStyle != tt.wantStroke {
			t.Errorf("style %s: expected strokeStyle %q, got %q", tt.style, tt.wantStroke, arrow.StrokeStyle)
		}
		if arrow.StrokeWidth != tt.wantWidth {
			t.Errorf("style %s: expected strokeWidth %v, got %v", tt.style, tt.wantWidth, arrow.StrokeWidth)
		}
	}
}

func TestOutputIsValidJSON(t *testing.T) {
	pg := &model.PositionedGraph{
		Nodes: []model.PositionedNode{
			{ID: "a", Label: "A", Shape: model.ShapeRect, X: 100, Y: 50, Width: 80, Height: 40},
			{ID: "b", Label: "B", Shape: model.ShapeCircle, X: 200, Y: 150, Width: 60, Height: 60},
		},
		Edges: []model.PositionedEdge{
			{From: "a", To: "b", Label: "link", Style: model.EdgeDashed, Direction: model.EdgeForward,
				Points:   []model.Point{{X: 100, Y: 70}, {X: 150, Y: 110}, {X: 200, Y: 120}},
				LabelPos: &model.Point{X: 150, Y: 95}, LabelWidth: 30, LabelHeight: 16},
		},
		Groups: []model.PositionedGroup{
			{ID: "g", Label: "G", X: 30, Y: 10, Width: 250, Height: 200, Contains: []string{"a", "b"}, Depth: 1},
		},
	}

	data, err := Export(pg, theme.DefaultTheme())
	if err != nil {
		t.Fatal(err)
	}

	// Verify it parses back as valid JSON.
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	if raw["type"] != "excalidraw" {
		t.Errorf("expected type excalidraw, got %v", raw["type"])
	}
}

// --- Helpers ---

type testDoc struct {
	Type     string        `json:"type"`
	Version  int           `json:"version"`
	Elements []testElement `json:"elements"`
}

type testElement struct {
	Type            string      `json:"type"`
	ID              string      `json:"id"`
	X               float64     `json:"x"`
	Y               float64     `json:"y"`
	Width           float64     `json:"width"`
	Height          float64     `json:"height"`
	Text            string      `json:"text"`
	ContainerID     *string     `json:"containerId"`
	BoundElements   []boundRef  `json:"boundElements"`
	GroupIDs        []string    `json:"groupIds"`
	Roundness       *roundness  `json:"roundness"`
	Points          [][]float64 `json:"points"`
	StartBinding    *binding    `json:"startBinding"`
	EndBinding      *binding    `json:"endBinding"`
	StartArrowhead  *string     `json:"startArrowhead"`
	EndArrowhead    *string     `json:"endArrowhead"`
	StrokeStyle     string      `json:"strokeStyle"`
	StrokeWidth     float64     `json:"strokeWidth"`
	Roughness       int         `json:"roughness"`
	BackgroundColor string      `json:"backgroundColor"`
}

func parseDoc(t *testing.T, data []byte) testDoc {
	t.Helper()
	var doc testDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("failed to parse excalidraw JSON: %v", err)
	}
	return doc
}

func findElement(elements []testElement, id string) *testElement {
	for i := range elements {
		if elements[i].ID == id {
			return &elements[i]
		}
	}
	return nil
}

func hasBound(refs []boundRef, id, typ string) bool {
	for _, r := range refs {
		if r.ID == id && r.Type == typ {
			return true
		}
	}
	return false
}
