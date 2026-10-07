package pipeline

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/diff"
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
