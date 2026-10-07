package pipeline

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/diff"
	"github.com/oxforge/diago/internal/theme"
)

func TestRenderDiff_SequenceRemovedMessageHasNoActivation(t *testing.T) {
	before := []byte(`{"type":"sequence","actors":[{"id":"u","label":"U"},{"id":"w","label":"W"}],
 "interactions":[{"from":"u","to":"w","label":"a"},{"from":"w","to":"u","label":"b","style":"dashed"},{"from":"u","to":"w","label":"c"}]}`)
	after := []byte(`{"type":"sequence","actors":[{"id":"u","label":"U"},{"id":"w","label":"W"}],
 "interactions":[{"from":"u","to":"w","label":"a"},{"from":"u","to":"w","label":"c"}]}`)
	in, err := parseDiffInputs(before, after)
	require.NoError(t, err)
	require.Equal(t, "sequence", in.kind)
	u := diff.BuildUnionSequence(in.seqBefore, in.seqAfter)
	th, _ := resolveTheme("default")
	ps := layoutSequenceDiff(u, th, false)
	require.Len(t, ps.Interactions, 3, "the removed message keeps its row")
	for _, a := range ps.Activations {
		assert.NotEqual(t, "u", a.ActorID, "the masked message must not activate u")
	}
}

func TestRenderDiff_StatusPaintKeepsOnlyFills(t *testing.T) {
	before := []byte(`{"type":"sequence","actors":[{"id":"u","label":"U"},{"id":"db","label":"DB","color":"red"}],
 "interactions":[{"from":"u","to":"db","label":"read"}]}`)
	after := []byte(`{"type":"sequence","actors":[{"id":"u","label":"U"},{"id":"db","label":"DB","color":"red"}],
 "interactions":[{"from":"u","to":"db","label":"read"},{"from":"db","to":"u","label":"rows","style":"dashed"}]}`)
	out, err := RenderDiff(context.Background(), before, after, DiffOptions{})
	require.NoError(t, err)
	s := string(out)
	th, err := resolveTheme("default")
	require.NoError(t, err)
	red := theme.DeriveColors(th.Colors["red"], th.Background, theme.ElementNode)
	assert.Contains(t, s, `<g id="actor-db" class="dimmed">`)
	assert.Contains(t, s, `fill="`+red.Fill+`"`, "the unchanged DB keeps its own fill")
	assert.NotContains(t, s, `stroke="`+red.Stroke+`"`, "and nothing takes its own outline color")
	assert.Contains(t, s, `url(#diago-arrow-added)`)
}

func TestRenderDiff_SVGChangeList(t *testing.T) {
	before := []byte(`{"type":"flow","nodes":[{"id":"a","label":"Gateway"},{"id":"b","label":"Store"},{"id":"c","label":"Mail"}],
 "edges":[{"id":"q","from":"a","to":"b"},{"id":"m","from":"a","to":"c"}]}`)
	after := []byte(`{"type":"flow","nodes":[{"id":"a","label":"Gateway"},{"id":"b","label":"Store","color":"red"},{"id":"d","label":"Metrics"}],
 "edges":[{"id":"q","from":"a","to":"b","style":"dashed"},{"id":"n","from":"a","to":"d"}]}`)
	out, err := RenderDiff(context.Background(), before, after, DiffOptions{})
	require.NoError(t, err)
	s := string(out)
	th, err := resolveTheme("default")
	require.NoError(t, err)
	i := strings.Index(s, `<g id="changes">`)
	require.GreaterOrEqual(t, i, 0)
	list := s[i:]
	for _, want := range []string{
		"added: node Metrics", "added: edge Gateway -&gt; Metrics",
		"removed: node Mail", "removed: edge Gateway -&gt; Mail",
		"changed: node Store: color none → red", "changed: edge Gateway -&gt; Store: style solid → dashed",
	} {
		assert.Contains(t, list, want)
	}
	assert.Contains(t, list, `fill="`+th.Diff.Removed+`"`)

	same, err := RenderDiff(context.Background(), before, before, DiffOptions{})
	require.NoError(t, err)
	assert.NotContains(t, string(same), `id="changes"`, "identical inputs list nothing")
}
