package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/diff"
	"github.com/oxforge/diago/internal/font"
	"github.com/oxforge/diago/internal/layoutdbg"
	"github.com/oxforge/diago/internal/model"
	"github.com/oxforge/diago/internal/schema"
	"github.com/oxforge/diago/internal/theme"
)

const loopSpec = `{"type":"flow","direction":"DOWN","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],
"edges":[{"from":"a","to":"b","label":"go"},{"from":"b","to":"b"}]}`

// TestLayoutGraphForFormat_DrawsTheSelfLoop: the layered engine draws a
// self-loop and emits a carrier (C18), screen and text.
func TestLayoutGraphForFormat_DrawsTheSelfLoop(t *testing.T) {
	for _, format := range []string{"svg", "text"} {
		pg, err := LayoutGraphForFormat(context.Background(), []byte(loopSpec), schema.ParseFlow, format, nil)
		require.NoError(t, err, format)
		assert.NotNil(t, pg.LayoutHints, format)
		assert.GreaterOrEqual(t, len(pg.Edges[1].Points), 4, "%s: the self-loop is drawn", format)
	}
}

// TestLayered_AnchorsOnAPrevious: a previous spec, laid out by the
// layered engine, anchors a render of a changed spec (C18, S13), and a
// render anchored on its own SVG reproduces it (C18.1).
func TestLayered_AnchorsOnAPrevious(t *testing.T) {
	var buf bytes.Buffer
	ctx := layoutdbg.NewContext(context.Background(), slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	previous, err := ParsePrevious(ctx, []byte(loopSpec), "flow", "", "")
	require.NoError(t, err)
	require.NotNil(t, previous)

	changed := `{"type":"flow","direction":"DOWN","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"},{"id":"c","label":"C"}],
"edges":[{"from":"a","to":"b","label":"go"},{"from":"b","to":"b"},{"from":"a","to":"c"}]}`
	buf.Reset()
	_, err = RenderWithOptions(ctx, []byte(changed), Options{Previous: previous})
	require.NoError(t, err)
	var applied map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var rec map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &rec))
		if rec["decision"] == "previous_applied" {
			applied = rec
		}
	}
	require.NotNil(t, applied, "the layered engine reads the carrier")
	assert.Equal(t, "S13", applied["spec_ref"])
	assert.Equal(t, []any{"a", "b"}, applied["known"])
	assert.Equal(t, []any{"c"}, applied["new"])

	svg, err := RenderWithOptions(ctx, []byte(loopSpec), Options{})
	require.NoError(t, err)
	own, err := ParsePrevious(ctx, svg, "flow", "", "")
	require.NoError(t, err)
	again, err := RenderWithOptions(ctx, []byte(loopSpec), Options{Previous: own})
	require.NoError(t, err)
	assert.Equal(t, string(svg), string(again))
}

// TestLayoutFlowDiff_LayeredAnchorsTheUnion: the union takes after's
// node order (c before b), which a fresh layout would follow; anchored on
// before's layout, it keeps b left of c (C18).
func TestLayoutFlowDiff_LayeredAnchorsTheUnion(t *testing.T) {
	ctx := context.Background()
	before, err := schema.ParseFlow([]byte(`{"type":"flow","direction":"DOWN","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"},{"id":"c","label":"C"}],
"edges":[{"from":"a","to":"b"},{"from":"a","to":"c"}]}`))
	require.NoError(t, err)
	after, err := schema.ParseFlow([]byte(`{"type":"flow","direction":"DOWN","nodes":[{"id":"a","label":"A"},{"id":"c","label":"C"},{"id":"b","label":"B"},{"id":"d","label":"D"}],
"edges":[{"from":"a","to":"c"},{"from":"a","to":"b"},{"from":"c","to":"d"}]}`))
	require.NoError(t, err)
	th, err := theme.Load("default")
	require.NoError(t, err)
	u := diff.BuildUnion(before, after)
	fresh, err := layoutWith(ctx, u.Graph, th, false, nil)
	require.NoError(t, err)
	x := func(pg *model.PositionedGraph, id string) float64 {
		for _, n := range pg.Nodes {
			if n.ID == id {
				return n.X
			}
		}
		require.Failf(t, "no node", "%s", id)
		return 0
	}
	require.Less(t, x(fresh, "c"), x(fresh, "b"), "fresh, the union follows after's order")
	for _, text := range []bool{false, true} {
		pg, _, err := layoutFlowDiff(ctx, u, before, th, text)
		require.NoError(t, err)
		assert.Less(t, x(pg, "b"), x(pg, "c"), "text=%v", text)
	}
}

func TestRenderDiff_Layered(t *testing.T) {
	after := strings.Replace(loopSpec, `"label":"go"`, `"label":"went"`, 1)
	ctx := context.Background()
	for _, format := range []string{"svg", "text"} {
		out, err := RenderDiff(ctx, []byte(loopSpec), []byte(after), DiffOptions{Format: format})
		require.NoError(t, err, format)
		assert.NotEmpty(t, out, format)
	}
}

// TestLayered_MeasuresTitlesInTheGroupFont: the layered engine's screen
// Config takes the theme's group label font, the one the renderer draws
// titles in (S9, S14).
func TestLayered_MeasuresTitlesInTheGroupFont(t *testing.T) {
	g, err := schema.ParseFlow([]byte(`{"type":"flow","direction":"DOWN","nodes":[{"id":"a","label":"A"}],"groups":[{"id":"g","label":"Payments cluster","contains":["a"]}]}`))
	require.NoError(t, err)
	th, err := theme.Load("default")
	require.NoError(t, err)
	th.Group.LabelFont.Size = 20
	pg, err := layoutWith(context.Background(), *g, th, false, nil)
	require.NoError(t, err)
	w, h := font.MeasureText("Payments cluster", 20, th.Group.LabelFont.Family)
	assert.Equal(t, [2]float64{w, h}, [2]float64{pg.Groups[0].LabelWidth, pg.Groups[0].LabelHeight})
}
