package svg

import (
	"strings"
	"testing"

	"github.com/oxforge/diago/internal/model"
	"github.com/oxforge/diago/internal/theme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func titledGraph(title string) *model.PositionedGraph {
	return &model.PositionedGraph{
		Width: 120, Height: 80, Title: title,
		Nodes: []model.PositionedNode{{ID: "a", Label: "A", Lines: []string{"A"}, X: 20, Y: 20, Width: 80, Height: 40}},
	}
}

// A title adds a band of (font size + 3 + 16) above the
// content, the content is translated down by the band, and the canvas
// widens to the measured title when that is wider than the graph.
func TestRender_TitleBand(t *testing.T) {
	th, err := theme.Load("default")
	require.NoError(t, err)
	band := th.Node.Font.Size + 3 + 16

	plain := Render(titledGraph(""), th)
	assert.NotContains(t, plain, "translate(0,")
	assert.Contains(t, plain, `height="80"`)

	got := Render(titledGraph("Hello"), th)
	assert.Contains(t, got, ">Hello</text>")
	assert.Contains(t, got, `font-weight="600"`)
	assert.Contains(t, got, `transform="translate(0,`+ff(band)+`)"`)
	assert.Contains(t, got, `height="`+ff(80+band)+`"`)
	assert.Contains(t, got, `width="120"`, "a short title does not widen the canvas")

	long := Render(titledGraph(strings.Repeat("wide ", 12)), th)
	assert.NotContains(t, long, `width="120"`, "a long title widens the canvas")
}

func TestRenderSequence_TitleBand(t *testing.T) {
	th, err := theme.Load("default")
	require.NoError(t, err)
	band := th.Actor.Font.Size + 3 + 16
	ps := &model.PositionedSequence{Title: "Login", Width: 200, Height: 100,
		Actors: []model.PositionedActor{{ID: "a", Label: "A", BoxX: 10, BoxY: 10, BoxW: 60, BoxH: 30, LineX: 40, LineTop: 40, LineBottom: 90}}}
	got := RenderSequence(ps, th)
	assert.Contains(t, got, ">Login</text>")
	assert.Contains(t, got, `transform="translate(0,`+ff(band)+`)"`)
	assert.Contains(t, got, `height="`+ff(100+band)+`"`)
	ps.Title = ""
	assert.NotContains(t, RenderSequence(ps, th), "translate(0,")
}
