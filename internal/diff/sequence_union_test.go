package diff

import (
	"encoding/json"
	"testing"

	"github.com/oxforge/diago/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fragA: alt over w,a with [ok: 2..2] [fail: 3..3] on four messages.
const fragA = `{"type":"sequence",
 "actors":[{"id":"u","label":"User"},{"id":"w","label":"Web"},{"id":"a","label":"Auth"}],
 "interactions":[
  {"from":"u","to":"w","label":"login"},
  {"from":"w","to":"a","label":"check"},
  {"from":"a","to":"w","label":"ok","style":"dashed"},
  {"from":"a","to":"w","label":"fail","style":"dashed"}],
 "fragments":[{"type":"alt","over":["w","a"],"sections":[{"label":"ok","start":2,"end":2},{"label":"fail","start":3,"end":3}]}]}`

func TestBuildUnionSequence_ContentAndStatuses(t *testing.T) {
	after := parseSeq(t, `{"type":"sequence",
 "actors":[{"id":"u","label":"User"},{"id":"w","label":"Web App"},{"id":"a","label":"Auth"},{"id":"l","label":"Log"}],
 "interactions":[
  {"from":"u","to":"w","label":"login"},
  {"from":"w","to":"a","label":"check"},
  {"from":"a","to":"w","label":"ok","style":"dashed"},
  {"from":"w","to":"l","label":"audit","style":"async"},
  {"from":"a","to":"w","label":"fail","style":"dashed"}],
 "fragments":[{"type":"alt","over":["w","a"],"sections":[{"label":"ok","start":2,"end":3},{"label":"fail","start":4,"end":4}]}],
 "activations": false}`)
	u := BuildUnionSequence(parseSeq(t, fragA), after)
	assert.Equal(t, []string{"u", "w", "a", "l"}, actorIDs(u.Diagram.Actors))
	assert.Equal(t, "Web App", u.Diagram.Actors[1].Label, "survivors carry after content")
	assert.Equal(t, map[string]Status{"u": Same, "w": Changed, "a": Same, "l": Added}, u.Actors)
	assert.Equal(t, []Status{Same, Same, Same, Added, Same}, u.Interactions)
	assert.Equal(t, []bool{false, false, false, false, false}, u.Removed)
	assert.False(t, u.Diagram.Activations, "activations flag from after")
	require.Len(t, u.Diagram.Fragments, 1)
	assert.Equal(t, []model.FragmentSection{{Label: "ok", Start: 2, End: 3}, {Label: "fail", Start: 4, End: 4}}, u.Diagram.Fragments[0].Sections)
	assert.Equal(t, FragmentStatus{Status: Same, Sections: []Status{Same, Same}}, u.Fragments[0])
	assert.Equal(t, []string{"l"}, u.Diff.AddedActors)
	assert.Equal(t, []string{"w"}, u.Diff.ChangedActors)
	assert.Equal(t, []int{3}, u.Diff.AddedMessages)
	assert.Equal(t, "Web", u.BeforeLabels["w"])
}

func TestBuildUnionSequence_RemovedMessageKeepsRowAndMask(t *testing.T) {
	after := parseSeq(t, `{"type":"sequence",
 "actors":[{"id":"u","label":"User"},{"id":"w","label":"Web"},{"id":"a","label":"Auth"}],
 "interactions":[
  {"from":"u","to":"w","label":"login"},
  {"from":"a","to":"w","label":"ok","style":"dashed"},
  {"from":"a","to":"w","label":"fail","style":"dashed"}],
 "fragments":[{"type":"alt","over":["w","a"],"sections":[{"label":"ok","start":1,"end":1},{"label":"fail","start":2,"end":2}]}]}`)
	u := BuildUnionSequence(parseSeq(t, fragA), after)
	assert.Equal(t, []Status{Same, Removed, Same, Same}, u.Interactions)
	assert.Equal(t, []bool{false, true, false, false}, u.Removed)
	assert.Equal(t, "check", u.Diagram.Interactions[1].Label, "removed content from before")
	assert.Equal(t, []model.FragmentSection{{Label: "ok", Start: 2, End: 2}, {Label: "fail", Start: 3, End: 3}}, u.Diagram.Fragments[0].Sections, "sections remapped through the alignment")
	assert.Equal(t, []int{1}, u.Diff.RemovedMessages)
}

func TestBuildUnionSequence_SectionAddedAndRemoved(t *testing.T) {
	after := parseSeq(t, `{"type":"sequence",
 "actors":[{"id":"u","label":"User"},{"id":"w","label":"Web"},{"id":"a","label":"Auth"}],
 "interactions":[
  {"from":"u","to":"w","label":"login"},
  {"from":"w","to":"a","label":"check"},
  {"from":"a","to":"w","label":"ok","style":"dashed"},
  {"from":"w","to":"u","label":"locked","style":"dashed"}],
 "fragments":[{"type":"alt","over":["w","a"],"sections":[{"label":"ok","start":2,"end":2},{"label":"locked","start":3,"end":3}]}]}`)
	u := BuildUnionSequence(parseSeq(t, fragA), after)
	// Union steps: login, check, ok, fail(removed), locked(added).
	assert.Equal(t, []Status{Same, Same, Same, Removed, Added}, u.Interactions)
	f := u.Diagram.Fragments[0]
	assert.Equal(t, []model.FragmentSection{{Label: "ok", Start: 2, End: 2}, {Label: "fail", Start: 3, End: 3}, {Label: "locked", Start: 4, End: 4}}, f.Sections, "sorted by union start")
	assert.Equal(t, FragmentStatus{Status: Changed, Sections: []Status{Same, Removed, Added}}, u.Fragments[0])
	assert.Equal(t, []SectionRef{{0, "locked", model.FragmentAlt}}, u.Diff.AddedSections)
	assert.Equal(t, []SectionRef{{0, "fail", model.FragmentAlt}}, u.Diff.RemovedSections)
	assert.Empty(t, u.Diff.AddedFragments)
	assert.Empty(t, u.Diff.RemovedFragments)
}

func TestBuildUnionSequence_RemovedSectionOverlappingKeptIsDroppedFromGeometry(t *testing.T) {
	// after moves "fail" endpoints (removed + added) and widens "ok" over
	// both rows; the before-only section "fail" remaps onto rows inside
	// "ok" and is dropped from geometry, kept in the legend.
	after := parseSeq(t, `{"type":"sequence",
 "actors":[{"id":"u","label":"User"},{"id":"w","label":"Web"},{"id":"a","label":"Auth"}],
 "interactions":[
  {"from":"u","to":"w","label":"login"},
  {"from":"w","to":"a","label":"check"},
  {"from":"a","to":"w","label":"ok","style":"dashed"},
  {"from":"a","to":"u","label":"fail","style":"dashed"}],
 "fragments":[{"type":"alt","over":["w","a"],"sections":[{"label":"ok","start":2,"end":3}]}]}`)
	u := BuildUnionSequence(parseSeq(t, fragA), after)
	// Union steps: login, check, ok, fail(removed, a->w), fail(added, a->u).
	assert.Equal(t, []Status{Same, Same, Same, Removed, Added}, u.Interactions)
	f := u.Diagram.Fragments[0]
	assert.Equal(t, []model.FragmentSection{{Label: "ok", Start: 2, End: 4}}, f.Sections, "the removed section overlapped ok and was dropped")
	assert.Equal(t, FragmentStatus{Status: Changed, Sections: []Status{Same}}, u.Fragments[0])
	assert.Equal(t, []SectionRef{{0, "fail", model.FragmentAlt}}, u.Diff.RemovedSections, "still in the legend")
}

func TestBuildUnionSequence_FragmentAddedAndRemoved(t *testing.T) {
	before := parseSeq(t, `{"type":"sequence",
 "actors":[{"id":"u","label":"User"},{"id":"w","label":"Web"}],
 "interactions":[{"from":"u","to":"w","label":"a"},{"from":"u","to":"w","label":"b"}],
 "fragments":[{"type":"loop","over":["u","w"],"sections":[{"label":"retry","start":0,"end":0}]}]}`)
	after := parseSeq(t, `{"type":"sequence",
 "actors":[{"id":"u","label":"User"},{"id":"w","label":"Web"}],
 "interactions":[{"from":"u","to":"w","label":"a"},{"from":"u","to":"w","label":"b"}],
 "fragments":[{"type":"opt","over":["u","w"],"sections":[{"label":"maybe","start":1,"end":1}]}]}`)
	u := BuildUnionSequence(before, after)
	require.Len(t, u.Diagram.Fragments, 2)
	assert.Equal(t, model.FragmentOpt, u.Diagram.Fragments[0].Type, "after's fragments first")
	assert.Equal(t, model.FragmentLoop, u.Diagram.Fragments[1].Type, "then removed before fragments")
	assert.Equal(t, FragmentStatus{Status: Added, Sections: []Status{Added}}, u.Fragments[0])
	assert.Equal(t, FragmentStatus{Status: Removed, Sections: []Status{Removed}}, u.Fragments[1])
	assert.Equal(t, []int{0}, u.Diff.AddedFragments)
	assert.Equal(t, []int{1}, u.Diff.RemovedFragments)
	assert.Empty(t, u.Diff.AddedSections, "a whole added fragment lists no sections")
	assert.Empty(t, u.Diff.RemovedSections)
}

func TestBuildUnionSequence_FragmentMatchesByWeakKeyWhenLabelsChange(t *testing.T) {
	after := parseSeq(t, `{"type":"sequence",
 "actors":[{"id":"u","label":"User"},{"id":"w","label":"Web"},{"id":"a","label":"Auth"}],
 "interactions":[
  {"from":"u","to":"w","label":"login"},
  {"from":"w","to":"a","label":"check"},
  {"from":"a","to":"w","label":"ok","style":"dashed"},
  {"from":"a","to":"w","label":"fail","style":"dashed"}],
 "fragments":[{"type":"alt","over":["a","w"],"sections":[{"label":"success","start":2,"end":2},{"label":"fail","start":3,"end":3}]}]}`)
	u := BuildUnionSequence(parseSeq(t, fragA), after)
	require.Len(t, u.Diagram.Fragments, 1, "matched on (type, sorted over) despite the label change")
	// The before-only section "ok" remaps to 2..2, overlaps the kept
	// "success" (2..2) and is dropped from geometry, kept in the legend.
	assert.Equal(t, FragmentStatus{Status: Changed, Sections: []Status{Added, Same}}, u.Fragments[0])
	assert.Equal(t, []model.FragmentSection{{Label: "success", Start: 2, End: 2}, {Label: "fail", Start: 3, End: 3}}, u.Diagram.Fragments[0].Sections)
	assert.Equal(t, []SectionRef{{0, "ok", model.FragmentAlt}}, u.Diff.RemovedSections)
}

func TestBuildUnionSequence_Deterministic(t *testing.T) {
	before, after := parseSeq(t, fragA), parseSeq(t, fragA)
	first, err := json.Marshal(BuildUnionSequence(before, after))
	require.NoError(t, err)
	for i := 0; i < 20; i++ {
		again, err := json.Marshal(BuildUnionSequence(before, after))
		require.NoError(t, err)
		require.Equal(t, string(first), string(again))
	}
}

func actorIDs(as []model.Actor) []string {
	out := make([]string, len(as))
	for i, a := range as {
		out[i] = a.ID
	}
	return out
}

// The union is presentation-free: a title on either side
// never reaches the diff render.
func TestBuildUnionSequence_DropsTitle(t *testing.T) {
	before := &model.SequenceDiagram{Title: "Before", Actors: []model.Actor{{ID: "a", Label: "A"}}}
	after := &model.SequenceDiagram{Title: "After", Actors: []model.Actor{{ID: "a", Label: "A"}}}
	u := BuildUnionSequence(before, after)
	assert.Equal(t, "", u.Diagram.Title)
}
