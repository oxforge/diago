package diff

import (
	"testing"

	"github.com/oxforge/diago/internal/model"
	"github.com/oxforge/diago/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func parseSeq(t *testing.T, spec string) *model.SequenceDiagram {
	t.Helper()
	d, err := schema.ParseSequence([]byte(spec))
	require.NoError(t, err)
	return d
}

const seqA = `{"type":"sequence",
 "actors":[{"id":"u","label":"User"},{"id":"w","label":"Web"},{"id":"a","label":"Auth"}],
 "interactions":[
  {"from":"u","to":"w","label":"login"},
  {"from":"w","to":"a","label":"check"},
  {"from":"a","to":"w","label":"ok","style":"dashed"},
  {"from":"w","to":"u","label":"welcome","style":"dashed"}]}`

func steps(al SeqAlignment) [][3]any {
	out := make([][3]any, len(al.Steps))
	for i, s := range al.Steps {
		out[i] = [3]any{s.Status, s.BeforeIdx, s.AfterIdx}
	}
	return out
}

func TestAlignSequences_IdenticalIsAllSame(t *testing.T) {
	al := AlignSequences(parseSeq(t, seqA), parseSeq(t, seqA))
	assert.Equal(t, [][3]any{{Same, 0, 0}, {Same, 1, 1}, {Same, 2, 2}, {Same, 3, 3}}, steps(al))
	assert.Equal(t, []ActorStatus{{"u", Same}, {"w", Same}, {"a", Same}}, al.Actors)
}

func TestAlignSequences_LabelEditIsChanged(t *testing.T) {
	after := parseSeq(t, `{"type":"sequence",
 "actors":[{"id":"u","label":"User"},{"id":"w","label":"Web"},{"id":"a","label":"Auth"}],
 "interactions":[
  {"from":"u","to":"w","label":"sign in"},
  {"from":"w","to":"a","label":"check"},
  {"from":"a","to":"w","label":"ok","style":"dashed"},
  {"from":"w","to":"u","label":"welcome","style":"dashed"}]}`)
	al := AlignSequences(parseSeq(t, seqA), after)
	assert.Equal(t, [][3]any{{Changed, 0, 0}, {Same, 1, 1}, {Same, 2, 2}, {Same, 3, 3}}, steps(al))
}

func TestAlignSequences_EndpointMoveIsRemovedThenAdded(t *testing.T) {
	after := parseSeq(t, `{"type":"sequence",
 "actors":[{"id":"u","label":"User"},{"id":"w","label":"Web"},{"id":"a","label":"Auth"}],
 "interactions":[
  {"from":"u","to":"w","label":"login"},
  {"from":"u","to":"a","label":"check"},
  {"from":"a","to":"w","label":"ok","style":"dashed"},
  {"from":"w","to":"u","label":"welcome","style":"dashed"}]}`)
	al := AlignSequences(parseSeq(t, seqA), after)
	assert.Equal(t, [][3]any{{Same, 0, 0}, {Removed, 1, -1}, {Added, -1, 1}, {Same, 2, 2}, {Same, 3, 3}}, steps(al), "removed first in the gap")
}

func TestAlignSequences_InsertionKeepsNeighboursSame(t *testing.T) {
	after := parseSeq(t, `{"type":"sequence",
 "actors":[{"id":"u","label":"User"},{"id":"w","label":"Web"},{"id":"a","label":"Auth"}],
 "interactions":[
  {"from":"u","to":"w","label":"login"},
  {"from":"w","to":"a","label":"check"},
  {"from":"w","to":"w","label":"audit"},
  {"from":"a","to":"w","label":"ok","style":"dashed"},
  {"from":"w","to":"u","label":"welcome","style":"dashed"}]}`)
	al := AlignSequences(parseSeq(t, seqA), after)
	assert.Equal(t, [][3]any{{Same, 0, 0}, {Same, 1, 1}, {Added, -1, 2}, {Same, 2, 3}, {Same, 3, 4}}, steps(al))
}

func TestAlignSequences_TiePrefersEarliestBefore(t *testing.T) {
	before := parseSeq(t, `{"type":"sequence","actors":[{"id":"u","label":"U"},{"id":"w","label":"W"}],
 "interactions":[{"from":"u","to":"w","label":"ping"},{"from":"u","to":"w","label":"ping"}]}`)
	after := parseSeq(t, `{"type":"sequence","actors":[{"id":"u","label":"U"},{"id":"w","label":"W"}],
 "interactions":[{"from":"u","to":"w","label":"ping"}]}`)
	al := AlignSequences(before, after)
	assert.Equal(t, [][3]any{{Same, 0, 0}, {Removed, 1, -1}}, steps(al))
}

func TestAlignSequences_StyleChangeIsRemovedPlusAddedThenWeakPaired(t *testing.T) {
	// Same endpoints, different style: no strong match, weak pass pairs them as changed.
	before := parseSeq(t, `{"type":"sequence","actors":[{"id":"u","label":"U"},{"id":"w","label":"W"}],
 "interactions":[{"from":"u","to":"w","label":"ping"}]}`)
	after := parseSeq(t, `{"type":"sequence","actors":[{"id":"u","label":"U"},{"id":"w","label":"W"}],
 "interactions":[{"from":"u","to":"w","label":"ping","style":"async"}]}`)
	assert.Equal(t, [][3]any{{Changed, 0, 0}}, steps(AlignSequences(before, after)))
}

func TestAlignSequences_ColorChangeIsChanged(t *testing.T) {
	before := parseSeq(t, `{"type":"sequence","actors":[{"id":"u","label":"U"},{"id":"w","label":"W"}],
 "interactions":[{"from":"u","to":"w","label":"ping"}]}`)
	after := parseSeq(t, `{"type":"sequence","actors":[{"id":"u","label":"U"},{"id":"w","label":"W"}],
 "interactions":[{"from":"u","to":"w","label":"ping","color":"red"}]}`)
	assert.Equal(t, [][3]any{{Changed, 0, 0}}, steps(AlignSequences(before, after)))
}

func TestAlignSequences_RemovedActorSplicedBesideLeftNeighbour(t *testing.T) {
	before := parseSeq(t, `{"type":"sequence",
 "actors":[{"id":"x","label":"X"},{"id":"u","label":"User"},{"id":"m","label":"Mail"},{"id":"n","label":"Notify"},{"id":"w","label":"Web"}],
 "interactions":[{"from":"u","to":"w","label":"login"}]}`)
	after := parseSeq(t, `{"type":"sequence",
 "actors":[{"id":"w","label":"Web"},{"id":"u","label":"User!"},{"id":"z","label":"Z"}],
 "interactions":[{"from":"u","to":"w","label":"login"}]}`)
	al := AlignSequences(before, after)
	assert.Equal(t, []ActorStatus{
		{"x", Removed}, // no surviving left neighbour: front
		{"w", Same},
		{"u", Changed}, {"m", Removed}, {"n", Removed}, // consecutive removed keep before order after u
		{"z", Added},
	}, al.Actors)
}

// TestAlignSequences_Deterministic guards the rule that an LCS tie prefers
// the earliest before index, so a double run is identical: calling
// AlignSequences twice on the same inputs must produce byte-for-byte equal
// results, across several fixtures already used above.
func TestAlignSequences_Deterministic(t *testing.T) {
	labelEditAfter := parseSeq(t, `{"type":"sequence",
 "actors":[{"id":"u","label":"User"},{"id":"w","label":"Web"},{"id":"a","label":"Auth"}],
 "interactions":[
  {"from":"u","to":"w","label":"sign in"},
  {"from":"w","to":"a","label":"check"},
  {"from":"a","to":"w","label":"ok","style":"dashed"},
  {"from":"w","to":"u","label":"welcome","style":"dashed"}]}`)
	endpointMoveAfter := parseSeq(t, `{"type":"sequence",
 "actors":[{"id":"u","label":"User"},{"id":"w","label":"Web"},{"id":"a","label":"Auth"}],
 "interactions":[
  {"from":"u","to":"w","label":"login"},
  {"from":"u","to":"a","label":"check"},
  {"from":"a","to":"w","label":"ok","style":"dashed"},
  {"from":"w","to":"u","label":"welcome","style":"dashed"}]}`)
	actorSpliceBefore := parseSeq(t, `{"type":"sequence",
 "actors":[{"id":"x","label":"X"},{"id":"u","label":"User"},{"id":"m","label":"Mail"},{"id":"n","label":"Notify"},{"id":"w","label":"Web"}],
 "interactions":[{"from":"u","to":"w","label":"login"}]}`)
	actorSpliceAfter := parseSeq(t, `{"type":"sequence",
 "actors":[{"id":"w","label":"Web"},{"id":"u","label":"User!"},{"id":"z","label":"Z"}],
 "interactions":[{"from":"u","to":"w","label":"login"}]}`)

	cases := []struct {
		name          string
		before, after *model.SequenceDiagram
	}{
		{"label edit", parseSeq(t, seqA), labelEditAfter},
		{"endpoint move", parseSeq(t, seqA), endpointMoveAfter},
		{"actor splice", actorSpliceBefore, actorSpliceAfter},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			first := AlignSequences(c.before, c.after)
			second := AlignSequences(c.before, c.after)
			assert.Equal(t, first.Steps, second.Steps)
			assert.Equal(t, first.Actors, second.Actors)
		})
	}
}

const gapActors = `"actors":[{"id":"a","label":"A"},{"id":"b","label":"B"},{"id":"c","label":"C"}]`

func TestAlignSequences_AGapKeepsAfterOrder(t *testing.T) {
	before := parseSeq(t, `{"type":"sequence",`+gapActors+`,"interactions":[
 {"from":"a","to":"b","label":"start"},{"from":"a","to":"b","label":"old call"},{"from":"b","to":"a","label":"end"}]}`)
	after := parseSeq(t, `{"type":"sequence",`+gapActors+`,"interactions":[
 {"from":"a","to":"b","label":"start"},{"from":"a","to":"c","label":"first new"},{"from":"a","to":"b","label":"second new"},{"from":"b","to":"a","label":"end"}]}`)
	assert.Equal(t, [][3]any{{Same, 0, 0}, {Added, -1, 1}, {Changed, 1, 2}, {Same, 2, 3}}, steps(AlignSequences(before, after)))
}

// Pairs inside a gap never cross: of the two pairs on from|to, which cross,
// one is made, and both sides keep their order.
func TestAlignSequences_GapPairsNeverCross(t *testing.T) {
	before := parseSeq(t, `{"type":"sequence",`+gapActors+`,"interactions":[
 {"from":"a","to":"b","label":"x"},{"from":"a","to":"c","label":"y"}]}`)
	after := parseSeq(t, `{"type":"sequence",`+gapActors+`,"interactions":[
 {"from":"a","to":"c","label":"y2"},{"from":"a","to":"b","label":"x2"}]}`)
	var bi, ai []int
	changed := 0
	for _, s := range AlignSequences(before, after).Steps {
		if s.Status == Changed {
			changed++
		}
		if s.BeforeIdx >= 0 {
			bi = append(bi, s.BeforeIdx)
		}
		if s.AfterIdx >= 0 {
			ai = append(ai, s.AfterIdx)
		}
	}
	assert.Equal(t, 1, changed, "one pair")
	assert.IsIncreasing(t, bi)
	assert.IsIncreasing(t, ai)
}

// A gap with one side empty emits the other side's messages in order, and a
// pair at either end of a gap keeps the leftovers on their own side of it.
func TestAlignSequences_GapEdges(t *testing.T) {
	seq := func(msgs string) *model.SequenceDiagram {
		return parseSeq(t, `{"type":"sequence",`+gapActors+`,"interactions":[
 {"from":"a","to":"b","label":"start"},`+msgs+`{"from":"b","to":"a","label":"end"}]}`)
	}
	two := `{"from":"a","to":"b","label":"x"},{"from":"a","to":"c","label":"y"},`
	for _, tc := range []struct {
		name          string
		before, after string
		want          [][3]any
	}{
		{"after side only", "", two, [][3]any{{Same, 0, 0}, {Added, -1, 1}, {Added, -1, 2}, {Same, 1, 3}}},
		{"before side only", two, "", [][3]any{{Same, 0, 0}, {Removed, 1, -1}, {Removed, 2, -1}, {Same, 3, 1}}},
		{"pair first", `{"from":"a","to":"b","label":"old"},`, `{"from":"a","to":"b","label":"new"},{"from":"a","to":"c","label":"extra"},`,
			[][3]any{{Same, 0, 0}, {Changed, 1, 1}, {Added, -1, 2}, {Same, 2, 3}}},
		{"pair last", `{"from":"a","to":"c","label":"gone"},{"from":"a","to":"b","label":"old"},`, `{"from":"a","to":"b","label":"new"},`,
			[][3]any{{Same, 0, 0}, {Removed, 1, -1}, {Changed, 2, 1}, {Same, 3, 2}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, steps(AlignSequences(seq(tc.before), seq(tc.after))))
		})
	}
}
