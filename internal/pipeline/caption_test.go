package pipeline

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captionPairs are a before and an after spec per diagram type, the after
// adding one element.
var captionPairs = map[string][2]string{
	"flow": {
		`{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":[{"id":"ab","from":"a","to":"b"}]}`,
		`{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"},{"id":"c","label":"C"}],"edges":[{"id":"ab","from":"a","to":"b"},{"id":"bc","from":"b","to":"c"}]}`,
	},
	"sequence": {
		`{"type":"sequence","actors":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"interactions":[{"from":"a","to":"b","label":"ping"}]}`,
		`{"type":"sequence","actors":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"interactions":[{"from":"a","to":"b","label":"ping"},{"from":"b","to":"a","label":"pong"}]}`,
	},
	"class": {
		`{"type":"class","classes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"relations":[{"id":"r","from":"a","to":"b"}]}`,
		`{"type":"class","classes":[{"id":"a","label":"A"},{"id":"b","label":"B"},{"id":"c","label":"C"}],"relations":[{"id":"r","from":"a","to":"b"},{"id":"s","from":"b","to":"c"}]}`,
	},
}

func TestDiffCaption(t *testing.T) {
	assert.Equal(t, "v1.json → v2.json", DiffCaption("v1.json", "v2.json"))
}

func TestRenderDiff_CaptionLeadsTheTextFooter(t *testing.T) {
	for kind, pair := range captionPairs {
		t.Run(kind, func(t *testing.T) {
			out, err := RenderDiff(context.Background(), []byte(pair[0]), []byte(pair[1]),
				DiffOptions{Format: "text", Caption: DiffCaption("v1.json", "v2.json")})
			require.NoError(t, err)
			assert.Contains(t, string(out), "\n\nv1.json → v2.json\nadded: ")
		})
	}
}

func TestRenderDiff_CaptionAloneWhenNothingChanged(t *testing.T) {
	flow := []byte(captionPairs["flow"][0])
	out, err := RenderDiff(context.Background(), flow, flow, DiffOptions{Format: "text", Caption: "v1.json → v1.json"})
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(string(out), "\n\nv1.json → v1.json\n"), string(out))

	svg, err := RenderDiff(context.Background(), flow, flow, DiffOptions{Caption: "v1.json → v1.json"})
	require.NoError(t, err)
	assert.Contains(t, string(svg), `<g id="changes">`)
	assert.Contains(t, string(svg), ">v1.json → v1.json</text>")
}

func TestRenderDiff_NoCaptionNoLine(t *testing.T) {
	flow := []byte(captionPairs["flow"][0])
	out, err := RenderDiff(context.Background(), flow, flow, DiffOptions{Format: "text"})
	require.NoError(t, err)
	assert.NotContains(t, string(out), "→", "identical specs and no caption: no footer at all")
	svg, err := RenderDiff(context.Background(), flow, flow, DiffOptions{})
	require.NoError(t, err)
	assert.NotContains(t, string(svg), `id="changes"`)
}
