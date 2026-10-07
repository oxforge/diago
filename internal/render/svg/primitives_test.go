package svg

import (
	"bytes"
	"strings"
	"testing"

	"github.com/oxforge/diago/internal/model"
	"github.com/stretchr/testify/assert"
)

func renderToString(e Element) string {
	var buf bytes.Buffer
	e.Render(&buf)
	return buf.String()
}

func TestRectRender(t *testing.T) {
	r := Rect{
		X: 10, Y: 20, Width: 100, Height: 50,
		Fill: "#ffffff", Stroke: "#000000", StrokeWidth: 1.5,
	}
	out := renderToString(r)
	assert.Contains(t, out, `<rect`)
	assert.Contains(t, out, `x="10"`)
	assert.Contains(t, out, `y="20"`)
	assert.Contains(t, out, `width="100"`)
	assert.Contains(t, out, `height="50"`)
	assert.Contains(t, out, `fill="#ffffff"`)
	assert.Contains(t, out, `stroke="#000000"`)
	assert.Contains(t, out, `stroke-width="1.5"`)
}

func TestRectRenderWithRadius(t *testing.T) {
	r := Rect{X: 0, Y: 0, Width: 50, Height: 30, Rx: 6}
	out := renderToString(r)
	assert.Contains(t, out, `rx="6"`)
	assert.Contains(t, out, `ry="6"`)
}

func TestRectRenderWithDash(t *testing.T) {
	r := Rect{X: 0, Y: 0, Width: 50, Height: 30, StrokeDash: "4,2"}
	out := renderToString(r)
	assert.Contains(t, out, `stroke-dasharray="4,2"`)
}

func TestTextRender(t *testing.T) {
	txt := Text{
		X: 50, Y: 25, Content: "Hello",
		FontFamily: "Inter", FontSize: "14", FontWeight: "500",
		Fill: "#333333", Anchor: "middle", DominantBaseline: "middle",
	}
	out := renderToString(txt)
	assert.Contains(t, out, `<text`)
	assert.Contains(t, out, `x="50"`)
	assert.Contains(t, out, `y="25"`)
	assert.Contains(t, out, `>Hello</text>`)
	assert.Contains(t, out, `font-family="Inter"`)
	assert.Contains(t, out, `text-anchor="middle"`)
}

func TestTextRenderEscaping(t *testing.T) {
	txt := Text{X: 0, Y: 0, Content: "<b>a & b</b>"}
	out := renderToString(txt)
	assert.NotContains(t, out, "<b>")
	assert.Contains(t, out, "&lt;b&gt;")
	assert.Contains(t, out, "&amp;")
}

func TestCircleRender(t *testing.T) {
	c := Circle{CX: 50, CY: 50, R: 30, Fill: "#eee", Stroke: "#333", StrokeWidth: 1}
	out := renderToString(c)
	assert.Contains(t, out, `<circle`)
	assert.Contains(t, out, `cx="50"`)
	assert.Contains(t, out, `cy="50"`)
	assert.Contains(t, out, `r="30"`)
}

func TestPolylineRender(t *testing.T) {
	pl := Polyline{
		Points:    []model.Point{{X: 0, Y: 0}, {X: 10, Y: 10}, {X: 20, Y: 0}},
		Stroke:    "#333",
		MarkerEnd: "url(#arrow)",
	}
	out := renderToString(pl)
	assert.Contains(t, out, `<polyline`)
	assert.Contains(t, out, `fill="none"`)
	assert.Contains(t, out, `marker-end="url(#arrow)"`)
	assert.Contains(t, out, `0,0`)
}

func TestSVGGroupRender(t *testing.T) {
	g := SVGGroup{
		ID: "test-group",
		Children: []Element{
			Rect{X: 0, Y: 0, Width: 10, Height: 10},
		},
	}
	out := renderToString(g)
	assert.Contains(t, out, `<g id="test-group">`)
	assert.Contains(t, out, `<rect`)
	assert.Contains(t, out, `</g>`)
}

func TestDefsRender(t *testing.T) {
	d := Defs{Children: []Element{
		Marker{ID: "arrow", ViewBox: "0 0 10 10", RefX: 10, RefY: 5, Width: 10, Height: 10, Orient: "auto"},
	}}
	out := renderToString(d)
	assert.Contains(t, out, `<defs>`)
	assert.Contains(t, out, `<marker id="arrow"`)
	assert.Contains(t, out, `</defs>`)
}

func TestSVGDocRender(t *testing.T) {
	doc := SVGDoc{
		Width: 200, Height: 100,
		ViewBox:  "0 0 200 100",
		FontFace: "@font-face { font-family: 'Inter'; }",
		Defs: []Element{
			Marker{ID: "arr", ViewBox: "0 0 8 8", RefX: 8, RefY: 4, Width: 8, Height: 8},
		},
		Children: []Element{
			Rect{X: 0, Y: 0, Width: 200, Height: 100, Fill: "#fff"},
		},
	}
	out := doc.Render()
	assert.True(t, strings.HasPrefix(out, `<?xml`), "should start with XML declaration")
	assert.Contains(t, out, `xmlns="http://www.w3.org/2000/svg"`)
	assert.Contains(t, out, `<style>@font-face`)
	assert.Contains(t, out, `<defs>`)
	assert.Contains(t, out, `<marker id="arr"`)
	assert.Contains(t, out, `<rect`)
	assert.True(t, strings.HasSuffix(out, `</svg>`), "should end with </svg>")
}
