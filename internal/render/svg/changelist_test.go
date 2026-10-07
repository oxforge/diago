package svg

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/font"
	"github.com/oxforge/diago/internal/model"
	"github.com/oxforge/diago/internal/theme"
)

var svgSizeRe = regexp.MustCompile(`<svg [^>]*width="([0-9.]+)" height="([0-9.]+)"`)

// svgSize reads the document's width and height.
func svgSize(t *testing.T, out string) (w, h float64) {
	t.Helper()
	m := svgSizeRe.FindStringSubmatch(out)
	require.NotNil(t, m)
	w, _ = strconv.ParseFloat(m[1], 64)
	h, _ = strconv.ParseFloat(m[2], 64)
	return w, h
}

// changesGroup returns the change list's markup.
func changesGroup(t *testing.T, out string) string {
	t.Helper()
	i := strings.Index(out, `<g id="changes">`)
	require.GreaterOrEqual(t, i, 0, "no change list")
	rest := out[i:]
	return rest[:strings.Index(rest, "</g>")]
}

func sampleLines(th theme.Theme) []ChangeLine {
	return []ChangeLine{
		{Color: th.Diff.Added, Text: "added: node B"},
		{Color: th.Diff.Changed, Wire: true, Text: "changed: edge A -> B: style solid → dashed"},
		{Color: th.Diff.Removed, Dashed: true, Wire: true, Text: "removed: edge A -> C"},
	}
}

func TestChangeList_Rows(t *testing.T) {
	th := theme.DefaultTheme()
	out := RenderWithOptions(diffGraph(), th, &RenderOptions{Changes: sampleLines(th)})
	g := changesGroup(t, out)
	assert.Equal(t, 1, strings.Count(g, "<rect"), "one box sample, for the node")
	assert.Equal(t, 2, strings.Count(g, "<line"), "two wire samples, for the edges")
	assert.Contains(t, g, `stroke="`+th.Diff.Added+`"`)
	assert.Contains(t, g, `fill="`+th.Diff.Changed+`"`, "the text is in the status color")
	assert.Contains(t, g, `stroke-dasharray="4,3"`, "the removed sample is dashed")
	assert.Equal(t, 1, strings.Count(g, "stroke-dasharray"), "only the removed one")
	assert.NotContains(t, g, "class=", "the list carries no status class")
	assert.Contains(t, g, "changed: edge A -&gt; B: style solid → dashed")
}

func TestChangeList_EscapesText(t *testing.T) {
	th := theme.DefaultTheme()
	out := RenderWithOptions(diffGraph(), th, &RenderOptions{Changes: []ChangeLine{
		{Color: th.Diff.Changed, Text: `changed: node A: label "x<y&z" → none`},
	}})
	assert.Contains(t, changesGroup(t, out), "changed: node A: label &quot;x&lt;y&amp;z&quot; → none")
}

func TestChangeList_GrowsCanvas(t *testing.T) {
	th := theme.DefaultTheme()
	g := diffGraph()
	plainW, plainH := svgSize(t, RenderWithOptions(g, th, &RenderOptions{}))
	lines := sampleLines(th)
	long := "changed: edge API Gateway -> Postgres: label none → \"a very long label that runs well past the diagram's width\""
	lines = append(lines, ChangeLine{Color: th.Diff.Changed, Wire: true, Text: long})
	w, h := svgSize(t, RenderWithOptions(g, th, &RenderOptions{Changes: lines}))
	assert.InDelta(t, plainH+2*legendPad+4*legendRow, h, 1e-6, "one row per line plus padding")
	tw, _ := font.MeasureText(long, th.Edge.LabelFont.Size, th.Edge.LabelFont.Family)
	assert.GreaterOrEqual(t, w, legendLabelX+tw, "the canvas fits the longest line")
	assert.Greater(t, w, plainW)
}

func TestChangeList_NoneWithoutLines(t *testing.T) {
	th := theme.DefaultTheme()
	g := diffGraph()
	out := RenderWithOptions(g, th, &RenderOptions{})
	assert.NotContains(t, out, `id="changes"`)
	w, h := svgSize(t, out)
	assert.Equal(t, g.Width, w)
	assert.Equal(t, g.Height, h)
	assert.Equal(t, Render(g, th), RenderWithOptions(g, th, nil), "a nil RenderOptions stays byte-identical")
}

func TestChangeList_UnderClassLegend(t *testing.T) {
	th := theme.DefaultTheme()
	g := diffGraph()
	g.Legend = []model.LegendEntry{{Kind: "inheritance", Label: "inheritance"}, {Kind: "composition", Label: "composition"}}
	out := RenderWithOptions(g, th, &RenderOptions{Changes: sampleLines(th)})
	require.Less(t, strings.Index(out, `<g id="legend">`), strings.Index(out, `<g id="changes">`))
	_, legendH := legendSize(g.Legend, th)
	firstY := g.Height + legendH + legendPad + legendRow/2
	assert.Contains(t, changesGroup(t, out), `y="`+ff(firstY)+`"`, "the first row starts below the legend")
}

func TestChangeList_SketchSamplesByHand(t *testing.T) {
	th, err := theme.Load("sketch")
	require.NoError(t, err)
	g := changesGroup(t, RenderWithOptions(diffGraph(), th, &RenderOptions{Changes: sampleLines(th)}))
	assert.Equal(t, 3, strings.Count(g, "<path"), "every sample is drawn by hand")
	assert.NotContains(t, g, "filter=", "no distortion filter: it costs a full-canvas pass each")
}

func TestChangeList_Sequence(t *testing.T) {
	th := theme.DefaultTheme()
	s := diffSequence()
	_, plainH := svgSize(t, RenderSequenceWithOptions(s, th, &RenderOptions{}))
	out := RenderSequenceWithOptions(s, th, &RenderOptions{Changes: sampleLines(th)})
	_, h := svgSize(t, out)
	assert.InDelta(t, plainH+2*legendPad+3*legendRow, h, 1e-6)
	assert.Contains(t, changesGroup(t, out), "added: node B")
}

func TestChangeList_ValueArrowFollowsTheFont(t *testing.T) {
	sketch, err := theme.Load("sketch")
	require.NoError(t, err)
	g := changesGroup(t, RenderWithOptions(diffGraph(), sketch, &RenderOptions{Changes: sampleLines(sketch)}))
	assert.Contains(t, g, "style solid -&gt; dashed", "the sketch font has no arrow glyph")
	assert.NotContains(t, g, "→")

	th := theme.DefaultTheme()
	g = changesGroup(t, RenderWithOptions(diffGraph(), th, &RenderOptions{Changes: sampleLines(th)}))
	assert.Contains(t, g, "style solid → dashed")

	// The width is measured on the text as drawn.
	lines := sampleLines(sketch)
	w, _ := changeListSize(lines, sketch)
	drawn, _ := font.MeasureText("changed: edge A -> B: style solid -> dashed", sketch.Edge.LabelFont.Size, sketch.Edge.LabelFont.Family)
	assert.InDelta(t, legendLabelX+drawn+legendRightPad, w, 1e-6)
}
