package svg

import (
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/model"
	"github.com/oxforge/diago/internal/theme"
)

func renderElement(e Element) string {
	var sb strings.Builder
	e.Render(&sb)
	return sb.String()
}

// pathPoints returns every coordinate pair of a path's d attribute: the
// points on the outline and the curves' control points.
func pathPoints(t *testing.T, d string) []model.Point {
	t.Helper()
	var nums []float64
	for _, tok := range strings.Fields(d) {
		if v, err := strconv.ParseFloat(tok, 64); err == nil {
			nums = append(nums, v)
		}
	}
	require.Zero(t, len(nums)%2, "coordinates come in pairs: %s", d)
	pts := make([]model.Point, 0, len(nums)/2)
	for i := 0; i < len(nums); i += 2 {
		pts = append(pts, model.Point{X: nums[i], Y: nums[i+1]})
	}
	return pts
}

// viewBox parses a marker's viewBox.
func viewBox(t *testing.T, m Marker) (minX, minY, w, h float64) {
	t.Helper()
	f := strings.Fields(m.ViewBox)
	require.Len(t, f, 4)
	v := make([]float64, 4)
	for i := range f {
		var err error
		v[i], err = strconv.ParseFloat(f[i], 64)
		require.NoError(t, err)
	}
	return v[0], v[1], v[2], v[3]
}

func markerPath(t *testing.T, m Marker) Path {
	t.Helper()
	require.Len(t, m.Children, 1)
	p, ok := m.Children[0].(Path)
	require.True(t, ok, "the marker draws one path")
	return p
}

func TestCleanMarkers_Unchanged(t *testing.T) {
	assert.Equal(t, `<marker id="a" viewBox="0 0 8 8" refX="8" refY="4" markerWidth="8" markerHeight="8" orient="auto-start-reverse" markerUnits="userSpaceOnUse"><path d="M 0 0 L 8 4 L 0 8 Z" fill="#333"/></marker>`,
		renderElement(ArrowMarker("a", 8, "#333")))
	assert.Equal(t, `<marker id="o" viewBox="0 0 8 8" refX="8" refY="4" markerWidth="8" markerHeight="8" orient="auto-start-reverse" markerUnits="userSpaceOnUse"><path d="M 0 0 L 8 4 L 0 8" fill="none" stroke="#333" stroke-width="1.5"/></marker>`,
		renderElement(OpenArrowMarker("o", 8, "#333")))
	assert.Equal(t, `<marker id="t" viewBox="-1 -1 14 16" refX="0" refY="7" markerWidth="14" markerHeight="16" orient="auto-start-reverse" markerUnits="userSpaceOnUse"><path d="M 0 0 L 12 7 L 0 14 Z" fill="none" stroke="#333" stroke-width="1.5"/></marker>`,
		renderElement(TriangleMarker("t", "#333", 1.5)))
	assert.Equal(t, `<marker id="d" viewBox="-1 -1 16 14" refX="0" refY="6" markerWidth="16" markerHeight="14" orient="auto-start-reverse" markerUnits="userSpaceOnUse"><path d="M 0 6 L 7 0 L 14 6 L 7 12 Z" fill="#333" stroke="#333" stroke-width="1.5"/></marker>`,
		renderElement(DiamondMarker("d", "#333", 1.5, true)))
}

func TestPath_LineCapAndJoin(t *testing.T) {
	assert.Equal(t, `<path d="M 0 0 L 1 1" fill="none" stroke="#000" stroke-width="2"/>`,
		renderElement(Path{D: "M 0 0 L 1 1", Fill: "none", Stroke: "#000", StrokeWidth: 2}), "unset, no attributes")
	assert.Equal(t, `<path d="M 0 0 L 1 1" fill="none" stroke="#000" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>`,
		renderElement(Path{D: "M 0 0 L 1 1", Fill: "none", Stroke: "#000", StrokeWidth: 2, LineCap: "round", LineJoin: "round"}))
}

// sketchMarkerCase is one hand-drawn marker beside the clean one it
// replaces, with the stroke width its outline is drawn at and the x its
// tip must reach.
type sketchMarkerCase struct {
	name         string
	build        func(id string) Marker
	clean        Marker
	outlineWidth float64
	tipX         float64
}

func sketchMarkerCases() []sketchMarkerCase {
	return []sketchMarkerCase{
		{"arrow", func(id string) Marker { return sketchArrowMarker(id, 8, "#333") }, ArrowMarker("m", 8, "#333"), 1, 8},
		{"thick arrow", func(id string) Marker { return sketchArrowMarker(id, 20, "#333") }, ArrowMarker("m", 20, "#333"), 2.5, 20},
		{"open arrow", func(id string) Marker { return sketchOpenArrowMarker(id, 8, "#333") }, OpenArrowMarker("m", 8, "#333"), 1.5, 8},
		{"triangle", func(id string) Marker { return sketchTriangleMarker(id, "#333", 1.8) }, TriangleMarker("m", "#333", 1.8), 1.8, umlTriangleDepth},
		{"diamond", func(id string) Marker { return sketchDiamondMarker(id, "#333", 1.8, false) }, DiamondMarker("m", "#333", 1.8, false), 1.8, umlDiamondDepth},
		{"filled diamond", func(id string) Marker { return sketchDiamondMarker(id, "#333", 1.8, true) }, DiamondMarker("m", "#333", 1.8, true), 1.8, umlDiamondDepth},
	}
}

func TestSketchMarkers_HandDrawn(t *testing.T) {
	for _, c := range sketchMarkerCases() {
		t.Run(c.name, func(t *testing.T) {
			m := c.build("m")
			p := markerPath(t, m)
			clean := markerPath(t, c.clean)

			assert.Equal(t, renderElement(m), renderElement(c.build("m")), "deterministic")
			assert.NotEqual(t, p.D, markerPath(t, c.build("other")).D, "seeded by the marker id")
			assert.NotEqual(t, clean.D, p.D, "not the clean geometry")
			assert.Contains(t, p.D, "Q", "bowed sides")
			assert.Equal(t, "round", p.LineJoin)
			assert.InDelta(t, c.outlineWidth, p.StrokeWidth, 1e-9)
			assert.Equal(t, clean.Fill, p.Fill, "fill unchanged")
			if c.name == "open arrow" {
				assert.Equal(t, "round", p.LineCap, "two strokes with round ends")
				assert.Equal(t, 2, strings.Count(p.D, "M"), "two strokes")
			}

			// The reference point and the scale stay the clean marker's.
			assert.Equal(t, c.clean.RefX, m.RefX)
			assert.Equal(t, c.clean.RefY, m.RefY)
			assert.Equal(t, c.clean.Orient, m.Orient)
			assert.Equal(t, c.clean.MarkerUnits, m.MarkerUnits)
			_, _, w, h := viewBox(t, m)
			assert.Equal(t, w, m.Width, "viewBox at scale 1")
			assert.Equal(t, h, m.Height, "viewBox at scale 1")

			// Whatever the seed, the outline stays inside the viewBox and
			// its tip, rounded, lands exactly where the clean tip does.
			for seed := range 200 {
				m := c.build("seed-" + strconv.Itoa(seed))
				p := markerPath(t, m)
				minX, minY, w, h := viewBox(t, m)
				half := p.StrokeWidth / 2
				maxX := math.Inf(-1)
				for _, pt := range pathPoints(t, p.D) {
					require.GreaterOrEqual(t, pt.X-half, minX, "seed %d: not clipped left", seed)
					require.GreaterOrEqual(t, pt.Y-half, minY, "seed %d: not clipped above", seed)
					require.LessOrEqual(t, pt.X+half, minX+w, "seed %d: not clipped right", seed)
					require.LessOrEqual(t, pt.Y+half, minY+h, "seed %d: not clipped below", seed)
					maxX = math.Max(maxX, pt.X)
				}
				require.InDelta(t, c.tipX, maxX+half, 1e-9, "seed %d: the tip reaches the border, no further", seed)
			}
		})
	}
}

func TestSketchUMLMarkers_BaseMeetsTheWire(t *testing.T) {
	// The wire is shortened by the adornment's depth and starts at x = 0:
	// the triangle's base and the diamond's back corner sit on it.
	tri := markerPath(t, sketchTriangleMarker("t", "#333", 1.8))
	pts := pathPoints(t, tri.D)
	assert.Equal(t, 0.0, pts[0].X, "first base corner on the wire's start")
	dia := markerPath(t, sketchDiamondMarker("d", "#333", 1.8, true))
	pts = pathPoints(t, dia.D)
	assert.Equal(t, 0.0, pts[0].X, "back corner on the wire's start")
	assert.InDelta(t, umlDiamondHalf, pts[0].Y, 0.5, "back corner on the wire")
}

// markerBlocks returns every <marker> element of an SVG document.
func markerBlocks(svg string) []string {
	var out []string
	for {
		i := strings.Index(svg, "<marker ")
		if i < 0 {
			return out
		}
		j := strings.Index(svg[i:], "</marker>")
		out = append(out, svg[i:i+j])
		svg = svg[i+j:]
	}
}

func sketchTheme() theme.Theme {
	th := theme.DefaultTheme()
	th.Style = "sketch"
	return th
}

func TestRender_SketchMarkersEverywhere(t *testing.T) {
	flow := twoNodeGraph()
	flow.Edges = append(flow.Edges,
		model.PositionedEdge{ID: "thick", From: "A", To: "B", Style: model.EdgeThick, Color: "red", Points: []model.Point{{X: 140, Y: 104}, {X: 140, Y: 176}}})
	flowOpts := &RenderOptions{
		Class:       func(kind, id string) string { return map[string]string{"edge:thick": "added"}[kind+":"+id] },
		MarkerFills: map[string]string{"added": "#0a0"},
	}
	classOpts := &RenderOptions{
		Class:       func(kind, id string) string { return map[string]string{"edge:r2": "changed"}[kind+":"+id] },
		MarkerFills: map[string]string{"changed": "#f80"},
	}
	cases := []struct {
		name   string
		render func(th theme.Theme) string
		want   []string // marker ids the render must emit
	}{
		{"flow", func(th theme.Theme) string { return RenderWithOptions(flow, th, flowOpts) },
			[]string{"diago-arrow", "diago-arrow-thick", "diago-arrow-thick-e03131", "diago-arrow-thick-added"}},
		{"class", func(th theme.Theme) string { return RenderWithOptions(classGraph(), th, classOpts) },
			[]string{"diago-uml-triangle", "diago-uml-diamond", "diago-uml-diamond-filled", "diago-uml-diamond-filled-changed"}},
		{"sequence", func(th theme.Theme) string {
			return RenderSequenceWithOptions(diffSequence(), th, seqStatusOpts(th, map[string]string{"interaction:0": "changed", "interaction:1": "added"}))
		}, []string{"diago-arrow", "diago-arrow-open", "diago-arrow-changed", "diago-arrow-open-added"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sketch := c.render(sketchTheme())
			for _, id := range c.want {
				assert.Contains(t, sketch, `<marker id="`+id+`"`)
			}
			blocks := markerBlocks(sketch)
			require.NotEmpty(t, blocks)
			for _, b := range blocks {
				assert.Contains(t, b, `stroke-linejoin="round"`, "hand-drawn: %s", b[:min(len(b), 40)])
			}
			clean := c.render(theme.DefaultTheme())
			for _, b := range markerBlocks(clean) {
				assert.NotContains(t, b, "stroke-linejoin", "a clean theme keeps its markers")
			}
		})
	}
}

func TestRenderClass_SketchLegend(t *testing.T) {
	g := classGraph()
	g.Legend = []model.LegendEntry{{Kind: "composition", Label: "composition"}, {Kind: "dependency", Label: "depends on"}, {Kind: "inheritance", Label: "is-a"}, {Kind: "aggregation", Label: "aggregation"}}
	out := Render(g, sketchTheme())
	legend := out[strings.Index(out, `<g id="legend">`):]
	legend = legend[:strings.Index(legend, "</g>")]
	assert.NotContains(t, legend, "<polygon", "samples drawn by hand")
	assert.NotContains(t, legend, "<line", "wires drawn by hand")
	assert.Equal(t, 3, strings.Count(legend, `stroke-linejoin="round"`), "one hand-drawn adornment per adorned row")
	assert.Contains(t, legend, `marker-end="url(#diago-arrow)"`, "the dependency keeps its arrowhead")

	// The sample is the marker's own geometry, mirrored: its tip on the
	// left at the sample's start.
	tri := markerPath(t, sketchTriangleMarker(umlMarkerID(model.RelationInheritance, ""), "#333", 1.5))
	minX := math.Inf(1)
	for _, pt := range pathPoints(t, tri.D) {
		minX = math.Min(minX, umlTriangleDepth-pt.X)
	}
	assert.InDelta(t, 0.75, minX, 1e-9, "the mirrored tip, inset by half the outline")

	assert.Equal(t, out, Render(g, sketchTheme()), "deterministic")
}
