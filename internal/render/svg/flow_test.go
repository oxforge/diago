package svg

import (
	"encoding/xml"
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/oxforge/diago/internal/font"
	"github.com/oxforge/diago/internal/model"
	"github.com/oxforge/diago/internal/theme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func twoNodeGraph() *model.PositionedGraph {
	return &model.PositionedGraph{
		Width:  400,
		Height: 300,
		Nodes: []model.PositionedNode{
			{ID: "A", Label: "Service A", Shape: model.ShapeRect, X: 100, Y: 80, Width: 120, Height: 48},
			{ID: "B", Label: "Service B", Shape: model.ShapeRect, X: 100, Y: 200, Width: 120, Height: 48},
		},
		Edges: []model.PositionedEdge{
			{From: "A", To: "B", Label: "call", Style: model.EdgeSolid,
				Points: []model.Point{{X: 100, Y: 104}, {X: 100, Y: 176}}},
		},
	}
}

func TestRenderBasicGraph(t *testing.T) {
	th := theme.DefaultTheme()
	out := Render(twoNodeGraph(), th)

	assert.Contains(t, out, `xmlns="http://www.w3.org/2000/svg"`)
	assert.Contains(t, out, "Service A")
	assert.Contains(t, out, "Service B")
	assert.Contains(t, out, "<polyline")
	assert.Contains(t, out, "@font-face")
	assert.Contains(t, out, "<style>")
}

func TestRenderDeterministic(t *testing.T) {
	th := theme.DefaultTheme()
	g := twoNodeGraph()
	out1 := Render(g, th)
	out2 := Render(g, th)
	assert.Equal(t, out1, out2, "same input should produce identical SVG bytes")
}

func TestRenderEdgeStyleDashed(t *testing.T) {
	th := theme.DefaultTheme()
	g := &model.PositionedGraph{
		Width: 300, Height: 200,
		Nodes: []model.PositionedNode{
			{ID: "A", Label: "A", Shape: model.ShapeRect, X: 80, Y: 60, Width: 80, Height: 40},
			{ID: "B", Label: "B", Shape: model.ShapeRect, X: 80, Y: 160, Width: 80, Height: 40},
		},
		Edges: []model.PositionedEdge{
			{From: "A", To: "B", Style: model.EdgeDashed,
				Points: []model.Point{{X: 80, Y: 80}, {X: 80, Y: 140}}},
		},
	}
	out := Render(g, th)
	assert.Contains(t, out, "stroke-dasharray", "dashed edge should have stroke-dasharray")
}

func TestRenderEdgeStyleDotted(t *testing.T) {
	th := theme.DefaultTheme()
	g := &model.PositionedGraph{
		Width: 300, Height: 200,
		Nodes: []model.PositionedNode{
			{ID: "A", Label: "A", Shape: model.ShapeRect, X: 80, Y: 60, Width: 80, Height: 40},
			{ID: "B", Label: "B", Shape: model.ShapeRect, X: 80, Y: 160, Width: 80, Height: 40},
		},
		Edges: []model.PositionedEdge{
			{From: "A", To: "B", Style: model.EdgeDotted,
				Points: []model.Point{{X: 80, Y: 80}, {X: 80, Y: 140}}},
		},
	}
	out := Render(g, th)
	assert.Contains(t, out, "stroke-dasharray", "dotted edge should have stroke-dasharray")
}

func TestRenderEdgeStyleThick(t *testing.T) {
	th := theme.DefaultTheme()
	g := &model.PositionedGraph{
		Width: 300, Height: 200,
		Nodes: []model.PositionedNode{
			{ID: "A", Label: "A", Shape: model.ShapeRect, X: 80, Y: 60, Width: 80, Height: 40},
			{ID: "B", Label: "B", Shape: model.ShapeRect, X: 80, Y: 160, Width: 80, Height: 40},
		},
		Edges: []model.PositionedEdge{
			{From: "A", To: "B", Style: model.EdgeThick,
				Points: []model.Point{{X: 80, Y: 80}, {X: 80, Y: 140}}},
		},
	}
	out := Render(g, th)
	// Thick edge uses a larger stroke-width; default is 1.5, thick is 2.5x = 3.75
	assert.Contains(t, out, "stroke-width", "thick edge should have stroke-width attribute")
}

func TestRenderAllShapes(t *testing.T) {
	th := theme.DefaultTheme()
	shapes := []model.Shape{
		model.ShapeRect, model.ShapeRounded, model.ShapeCircle,
		model.ShapeDiamond, model.ShapeCylinder, model.ShapeHexagon,
		model.ShapeParallelogram,
	}
	for i, shape := range shapes {
		g := &model.PositionedGraph{
			Width: 200, Height: 150,
			Nodes: []model.PositionedNode{
				{ID: "n1", Label: "Node", Shape: shape,
					X: float64(100 + i*10), Y: 75, Width: 100, Height: 48},
			},
		}
		assert.NotPanics(t, func() {
			out := Render(g, th)
			assert.NotEmpty(t, out)
		})
	}
}

func TestRenderWithGroups(t *testing.T) {
	th := theme.DefaultTheme()
	g := &model.PositionedGraph{
		Width: 400, Height: 300,
		Nodes: []model.PositionedNode{
			{ID: "A", Label: "A", Shape: model.ShapeRect, X: 100, Y: 80, Width: 80, Height: 40},
		},
		Groups: []model.PositionedGroup{
			{ID: "g1", Label: "Backend", X: 60, Y: 50, Width: 180, Height: 100},
		},
	}
	out := Render(g, th)
	assert.Contains(t, out, "Backend", "group label should appear in output")
	assert.Contains(t, out, `id="group-g1"`, "group should have correct id")
}

func TestRenderEdgeLabelPositioned(t *testing.T) {
	th := theme.DefaultTheme()
	out := Render(twoNodeGraph(), th)
	assert.Contains(t, out, "call", "edge label should appear in SVG")
}

// TestEdgeLabelsCentered verifies that edge labels are centered on the edge midpoint.
func TestEdgeLabelsCentered(t *testing.T) {
	th := theme.DefaultTheme()
	g := twoNodeGraph() // has edge A→B with label "call", points (100,104)→(100,176)
	out := Render(g, th)

	// Parse the SVG to find edge groups.
	edgeGroups := extractEdgeGroups(t, out)
	require.NotEmpty(t, edgeGroups, "should have at least one edge group")

	for _, eg := range edgeGroups {
		if eg.label == "" {
			continue
		}

		// Verify: label X matches the polyline midpoint X.
		midX := (eg.polylinePoints[0].X + eg.polylinePoints[len(eg.polylinePoints)-1].X) / 2
		midY := (eg.polylinePoints[0].Y + eg.polylinePoints[len(eg.polylinePoints)-1].Y) / 2
		assert.InDelta(t, midX, eg.labelX, 1.0,
			"edge %s→%s: label X should be centered on edge midpoint", eg.from, eg.to)
		assert.InDelta(t, midY, eg.labelY, 1.0,
			"edge %s→%s: label Y should be on edge midpoint", eg.from, eg.to)
	}
}

// TestEdgeLabelsCenteredGoldenBasic runs the full pipeline on flow-basic.json
// and verifies edge labels are centered on the edge midpoint.
func TestEdgeLabelsCenteredGoldenBasic(t *testing.T) {
	th := theme.DefaultTheme()
	g := &model.PositionedGraph{
		Width: 300, Height: 400,
		Nodes: []model.PositionedNode{
			{ID: "client", Label: "Client App", Shape: model.ShapeRounded, X: 150, Y: 60, Width: 140, Height: 48},
			{ID: "api", Label: "API Gateway", Shape: model.ShapeRect, X: 150, Y: 200, Width: 140, Height: 48},
			{ID: "db", Label: "PostgreSQL", Shape: model.ShapeCylinder, X: 150, Y: 340, Width: 140, Height: 60},
		},
		Edges: []model.PositionedEdge{
			{From: "client", To: "api", Label: "HTTPS", Style: model.EdgeSolid,
				Points: []model.Point{{X: 150, Y: 84}, {X: 150, Y: 176}}},
			{From: "api", To: "db", Label: "query", Style: model.EdgeSolid,
				Points: []model.Point{{X: 150, Y: 224}, {X: 150, Y: 310}}},
		},
	}
	out := Render(g, th)
	edgeGroups := extractEdgeGroups(t, out)

	for _, eg := range edgeGroups {
		if eg.label == "" {
			continue
		}
		midX := (eg.polylinePoints[0].X + eg.polylinePoints[len(eg.polylinePoints)-1].X) / 2
		midY := (eg.polylinePoints[0].Y + eg.polylinePoints[len(eg.polylinePoints)-1].Y) / 2

		assert.InDelta(t, midX, eg.labelX, 1.0,
			"label %q: X should match edge midpoint", eg.label)
		assert.InDelta(t, midY, eg.labelY, 1.0,
			"label %q: Y should match edge midpoint", eg.label)
	}
}

// edgeGroupInfo holds parsed information about an edge <g> element.
type edgeGroupInfo struct {
	from, to          string
	label             string
	labelX, labelY    float64
	polylinePoints    []model.Point
	hasBackgroundRect bool
	bgRectBeforeText  bool
	bgRectX, bgRectY  float64
	bgRectW, bgRectH  float64
	bgRectFill        string
}

// extractEdgeGroups parses the SVG XML and extracts edge group information.
func extractEdgeGroups(t *testing.T, svgStr string) []edgeGroupInfo {
	t.Helper()

	var results []edgeGroupInfo

	decoder := xml.NewDecoder(strings.NewReader(svgStr))
	var inEdgeGroup bool
	var current edgeGroupInfo
	var foundText bool

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}

		switch el := tok.(type) {
		case xml.StartElement:
			attrs := attrMap(el.Attr)

			if el.Name.Local == "g" {
				id := attrs["id"]
				if strings.HasPrefix(id, "edge-") {
					inEdgeGroup = true
					parts := strings.SplitN(strings.TrimPrefix(id, "edge-"), "-", 2)
					current = edgeGroupInfo{}
					if len(parts) == 2 {
						current.from = parts[0]
						current.to = parts[1]
					}
					foundText = false
				}
			}

			if !inEdgeGroup {
				continue
			}

			if el.Name.Local == "polyline" {
				current.polylinePoints = parsePolylinePoints(attrs["points"])
			}

			if el.Name.Local == "rect" && !foundText {
				// A rect before any text in this group is a background rect.
				current.hasBackgroundRect = true
				current.bgRectBeforeText = true
				current.bgRectX = parseFloat(attrs["x"])
				current.bgRectY = parseFloat(attrs["y"])
				current.bgRectW = parseFloat(attrs["width"])
				current.bgRectH = parseFloat(attrs["height"])
				current.bgRectFill = attrs["fill"]
			}

			if el.Name.Local == "text" {
				foundText = true
				current.labelX = parseFloat(attrs["x"])
				current.labelY = parseFloat(attrs["y"])
			}

		case xml.CharData:
			if inEdgeGroup && foundText {
				current.label = strings.TrimSpace(string(el))
			}

		case xml.EndElement:
			if el.Name.Local == "g" && inEdgeGroup {
				results = append(results, current)
				inEdgeGroup = false
			}
		}
	}

	return results
}

func TestRenderSketchHasDistortionFilter(t *testing.T) {
	th := theme.DefaultTheme()
	th.Style = "sketch"
	out := Render(twoNodeGraph(), th)

	// Filter definition should be in defs.
	assert.Contains(t, out, `<filter id="sketch-distortion"`)
	assert.Contains(t, out, `<feTurbulence`)
	assert.Contains(t, out, `<feDisplacementMap`)

	// Shape paths should have the filter applied.
	assert.Contains(t, out, `filter="url(#sketch-distortion)"`)

	// Text elements should NOT have the filter.
	// Split on "<text " and check none contain "filter="
	textParts := strings.Split(out, "<text ")
	require.Greater(t, len(textParts), 1, "should have at least one text element")
	for i := 1; i < len(textParts); i++ {
		endIdx := strings.Index(textParts[i], ">")
		if endIdx > 0 {
			attrs := textParts[i][:endIdx]
			assert.NotContains(t, attrs, "filter=",
				"text element should not have filter attribute")
		}
	}
}

func TestRenderCleanNoDistortionFilter(t *testing.T) {
	th := theme.DefaultTheme()
	th.Style = "clean"
	out := Render(twoNodeGraph(), th)
	assert.NotContains(t, out, "sketch-distortion",
		"clean style should not include sketch distortion filter")
}

func attrMap(attrs []xml.Attr) map[string]string {
	m := make(map[string]string, len(attrs))
	for _, a := range attrs {
		m[a.Name.Local] = a.Value
	}
	return m
}

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func parsePolylinePoints(s string) []model.Point {
	var pts []model.Point
	pairs := strings.Fields(s)
	for _, pair := range pairs {
		parts := strings.Split(pair, ",")
		if len(parts) != 2 {
			continue
		}
		x, _ := strconv.ParseFloat(parts[0], 64)
		y, _ := strconv.ParseFloat(parts[1], 64)
		pts = append(pts, model.Point{X: x, Y: y})
	}
	return pts
}

func TestEdgeLabelPositionZeroLengthSegment(t *testing.T) {
	pts := []model.Point{
		{X: 100, Y: 100},
		{X: 100, Y: 100},
		{X: 100, Y: 200},
	}
	x, y, _ := edgeLabelPosition(pts)
	assert.False(t, math.IsNaN(x), "x should not be NaN")
	assert.False(t, math.IsNaN(y), "y should not be NaN")
}

func TestRender_NodeWithColor(t *testing.T) {
	graph := &model.PositionedGraph{
		Nodes: []model.PositionedNode{
			{ID: "a", Label: "A", Shape: model.ShapeRect, X: 100, Y: 50, Width: 80, Height: 40, Color: "red"},
		},
		Width: 200, Height: 100,
	}
	th := theme.DefaultTheme()
	svg := Render(graph, th)
	// Should NOT use the default node fill
	if strings.Contains(svg, fmt.Sprintf(`fill="%s"`, th.Node.Fill)) {
		t.Error("colored node should not use default theme fill")
	}
}

func TestRender_EdgeWithColor(t *testing.T) {
	graph := &model.PositionedGraph{
		Width: 300, Height: 200,
		Nodes: []model.PositionedNode{
			{ID: "a", Label: "A", Shape: model.ShapeRect, X: 80, Y: 60, Width: 80, Height: 40},
			{ID: "b", Label: "B", Shape: model.ShapeRect, X: 80, Y: 160, Width: 80, Height: 40},
		},
		Edges: []model.PositionedEdge{
			{From: "a", To: "b", Style: model.EdgeSolid, Color: "#e03131",
				Points: []model.Point{{X: 80, Y: 80}, {X: 80, Y: 140}}},
		},
	}
	th := theme.DefaultTheme()
	svg := Render(graph, th)
	// Should have a custom arrow marker for the colored edge
	assert.Contains(t, svg, "diago-arrow-e03131", "should create custom marker for colored edge")
}

func TestRender_GroupWithColor(t *testing.T) {
	graph := &model.PositionedGraph{
		Width: 400, Height: 300,
		Nodes: []model.PositionedNode{
			{ID: "a", Label: "A", Shape: model.ShapeRect, X: 100, Y: 80, Width: 80, Height: 40},
		},
		Groups: []model.PositionedGroup{
			{ID: "g1", Label: "Backend", X: 60, Y: 50, Width: 180, Height: 100, Color: "blue"},
		},
	}
	th := theme.DefaultTheme()
	svg := Render(graph, th)
	// Should NOT use the default group fill
	if strings.Contains(svg, fmt.Sprintf(`fill="%s"`, th.Group.Fill)) {
		t.Error("colored group should not use default theme fill")
	}
}

// TestRender_GroupTitleAtItsOffset pins a group title where the layout
// placed it: LabelOffset from the group's left side and 16 px below its
// top, or 8 px in when the layout left the title to the renderer.
func TestRender_GroupTitleAtItsOffset(t *testing.T) {
	graph := func(offset float64) *model.PositionedGraph {
		return &model.PositionedGraph{
			Width: 400, Height: 300,
			Groups: []model.PositionedGroup{
				{ID: "g1", Label: "Backend", X: 60, Y: 50, Width: 180, Height: 100, LabelOffset: offset},
			},
		}
	}
	th := theme.DefaultTheme()
	assert.Contains(t, Render(graph(0), th), `<text x="68" y="66"`, "the renderer's default slot")
	assert.Contains(t, Render(graph(100), th), `<text x="160" y="66"`, "the placed title")
}

// TestRender_GroupTitleInFront pins the title layer (C13): a group's title
// is drawn after every edge and node, before the edge labels, on a
// backing in its group's fill, 4 px wider than the drawn text on each
// side, so a wire under it shows a gap and a wire beside it does not.
func TestRender_GroupTitleInFront(t *testing.T) {
	th := theme.DefaultTheme()
	// A layout measures the title at the theme's size read as points
	// (the layered engine's size stage, S1); the
	// renderer draws it at that many px, three quarters as wide.
	measured, _ := font.MeasureText("Backend", th.Group.LabelFont.Size, th.Group.LabelFont.Family)
	drawn, _ := font.MeasureText("Backend", th.Group.LabelFont.Size*72/96, th.Group.LabelFont.Family)
	require.Greater(t, measured-drawn, 8.0, "the backing's pad would not hide the difference")
	g := &model.PositionedGraph{
		Width: 400, Height: 300,
		Nodes: []model.PositionedNode{
			{ID: "A", Label: "A", Shape: model.ShapeRect, X: 100, Y: 20, Width: 80, Height: 30},
			{ID: "B", Label: "B", Shape: model.ShapeRect, X: 100, Y: 120, Width: 80, Height: 40},
		},
		Edges: []model.PositionedEdge{{ID: "A->B#0", From: "A", To: "B", Label: "call",
			Points: []model.Point{{X: 100, Y: 35}, {X: 100, Y: 100}}, LabelPos: &model.Point{X: 130, Y: 80}, LabelWidth: 24, LabelHeight: 14}},
		Groups: []model.PositionedGroup{
			{ID: "g1", Label: "Backend", X: 60, Y: 50, Width: 180, Height: 130, Depth: 1, LabelWidth: measured, LabelHeight: 14, LabelBlocked: true},
		},
	}
	out := Render(g, th)
	title := strings.Index(out, `<g id="group-title-g1">`)
	require.GreaterOrEqual(t, title, 0, "the title is its own element")
	assert.Greater(t, title, strings.Index(out, `<g id="group-g1">`), "after its box")
	assert.Greater(t, title, strings.Index(out, `id="edge-A-B"`), "after the edge")
	assert.Greater(t, title, strings.Index(out, `id="node-B"`), "after the nodes")
	assert.Less(t, title, strings.Index(out, `>call</text>`), "before the edge labels")
	backing := fmt.Sprintf(`<rect class="backing" x="64" y="59" width="%s" height="14" fill="%s"/>`, ff(drawn+8), adjustGroupFill(th.Group.Fill, 1))
	assert.Contains(t, out[title:], backing, "the backing, then the text")
	assert.Less(t, strings.Index(out, backing), strings.Index(out, `>Backend</text>`))

	g.Groups[0].LabelWidth = 0
	assert.NotContains(t, Render(g, th), `class="backing"`, "no backing without the title's extents")
	assert.Contains(t, Render(g, th), `>Backend</text>`)
}

func TestEdgeElementID(t *testing.T) {
	cases := []struct{ id, from, to, want string }{
		{"a->b#0", "a", "b", "edge-a-b"},
		{"a->b#1", "a", "b", "edge-a-b-1"},
		{"retry", "a", "b", "edge-retry"},
		{"", "a", "b", "edge-a-b"}, // no id assigned
	}
	for _, tc := range cases {
		got := edgeElementID(model.PositionedEdge{ID: tc.id, From: tc.from, To: tc.to})
		assert.Equal(t, tc.want, got, "id %q", tc.id)
	}
}

func TestRender_EdgeElementIDsUnique(t *testing.T) {
	g := twoNodeGraph()
	g.Edges = append(g.Edges, model.PositionedEdge{ID: "A->B#1", From: "A", To: "B",
		Points: []model.Point{{X: 110, Y: 104}, {X: 110, Y: 176}}})
	g.Edges[0].ID = "A->B#0"
	out := Render(g, theme.DefaultTheme())
	assert.Contains(t, out, `id="edge-A-B"`)
	assert.Contains(t, out, `id="edge-A-B-1"`)
}
