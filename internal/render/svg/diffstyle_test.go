package svg

import (
	"strings"
	"testing"

	"github.com/oxforge/diago/internal/model"
	"github.com/oxforge/diago/internal/theme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func diffGraph() *model.PositionedGraph {
	g := twoNodeGraph()
	g.Edges[0].ID = "A->B#0"
	g.Nodes = append(g.Nodes, model.PositionedNode{ID: "C", Label: "Gone", Shape: model.ShapeDiamond, X: 300, Y: 80, Width: 100, Height: 60})
	g.Edges = append(g.Edges, model.PositionedEdge{ID: "A->C#0", From: "A", To: "C", Style: model.EdgeThick,
		Points: []model.Point{{X: 160, Y: 80}, {X: 250, Y: 80}}})
	g.Groups = []model.PositionedGroup{{ID: "grp", Label: "Grp", X: 20, Y: 20, Width: 360, Height: 260, Depth: 1}}
	return g
}

func statusOpts(th theme.Theme, classes map[string]string) *RenderOptions {
	return &RenderOptions{
		Class:       func(kind, id string) string { return classes[kind+":"+id] },
		Styles:      DiffStyles(th),
		MarkerFills: map[string]string{"added": th.Diff.Added, "changed": th.Diff.Changed},
	}
}

func TestRenderWithOptions_NilIsRender(t *testing.T) {
	th := theme.DefaultTheme()
	g := twoNodeGraph()
	assert.Equal(t, Render(g, th), RenderWithOptions(g, th, nil))
	assert.NotContains(t, Render(g, th), "class=")
}

func TestRenderWithOptions_ClassesAndStyle(t *testing.T) {
	th := theme.DefaultTheme()
	out := RenderWithOptions(diffGraph(), th, statusOpts(th, map[string]string{
		"node:A": "dimmed", "node:B": "added", "node:C": "removed",
		"edge:A->B#0": "changed", "edge:A->C#0": "removed", "group:grp": "changed",
	}))

	assert.Contains(t, out, `<g id="node-B" class="added">`)
	assert.Contains(t, out, `<g id="node-C" class="removed">`)
	assert.Contains(t, out, `<g id="edge-A-B" class="changed">`)
	assert.Contains(t, out, `<g id="edge-label-A-B" class="changed">`, "hoisted label carries the edge class")
	assert.Contains(t, out, `<g id="group-grp" class="changed">`)
	assert.Contains(t, out, `<g id="group-title-grp" class="changed">`, "the title carries its group's class")
	assert.Contains(t, out, ".added rect.backing, .changed rect.backing { stroke: none; }", "a title's backing takes no status stroke")
	assert.Contains(t, out, ".dimmed { opacity: 0.5; }")
	assert.Contains(t, out, ".added rect, .added polygon, .added circle, .added ellipse, .added path")
	assert.NotContains(t, out, ".added rect {", "selectors never target bare rect alone")
	assert.Contains(t, out, "stroke: "+th.Diff.Added)
	assert.Less(t, strings.Index(out, "@font-face"), strings.Index(out, ".dimmed"), "diff style follows the font faces")
}

func TestRenderWithOptions_StatusMarkers(t *testing.T) {
	th := theme.DefaultTheme()
	g := diffGraph()
	g.Edges[0].Color = "red" // user color must lose to status for the marker
	out := RenderWithOptions(g, th, statusOpts(th, map[string]string{
		"edge:A->B#0": "added", "edge:A->C#0": "removed",
	}))

	assert.Contains(t, out, `id="diago-arrow-added"`)
	assert.Contains(t, out, `marker-end="url(#diago-arrow-added)"`, "added edge uses the status marker despite its user color")
	assert.Contains(t, out, `marker-end="url(#diago-arrow-thick)"`, "removed thick edge keeps the base thick marker")
	assert.NotContains(t, out, `id="diago-arrow-changed"`, "unused status markers are not emitted")
}

func TestDiffStyles_DarkPalette(t *testing.T) {
	dark, err := theme.Load("dark")
	require.NoError(t, err)
	assert.Contains(t, DiffStyles(dark), dark.Diff.Added)
	assert.NotContains(t, DiffStyles(dark), theme.DefaultTheme().Diff.Added)
}

func TestClassDiffStyles(t *testing.T) {
	th := theme.DefaultTheme()
	out := ClassDiffStyles(th)
	assert.Contains(t, out, DiffStyles(th), "class diff styles extend the flow diff styles")
	assert.Contains(t, out, "text.member.added { fill: "+th.Diff.Added+"; font-weight: 600; }")
	assert.Contains(t, out, "text.member.changed { fill: "+th.Diff.Changed+"; font-weight: 600; }")
	assert.Contains(t, out, "text.member.removed { opacity: 0.35; text-decoration: line-through; }")
}
