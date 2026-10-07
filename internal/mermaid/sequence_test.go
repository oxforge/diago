package mermaid

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/oxforge/diago/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func parseSeq(t *testing.T, body string) (schema.SequenceSpec, []Report) {
	t.Helper()
	res, err := Parse([]byte("sequenceDiagram\n" + body))
	require.NoError(t, err)
	require.Equal(t, KindSequence, res.Kind)
	requireValid(t, res.Kind, res.JSON)
	var spec schema.SequenceSpec
	require.NoError(t, json.Unmarshal(res.JSON, &spec))
	return spec, res.Reports
}

func actor(id, label string) schema.ActorSpec { return schema.ActorSpec{ID: id, Label: label} }
func msg(from, to, label, style string) schema.InteractionSpec {
	return schema.InteractionSpec{From: from, To: to, Label: label, Style: style}
}
func section(label string, start, end int) schema.FragmentSectionSpec {
	return schema.FragmentSectionSpec{Label: label, Start: start, End: end}
}

func TestSequence_Participants(t *testing.T) {
	// participants, actor, as-labels, implicit declaration
	spec, reports := parseSeq(t, "participant W as Web App\nactor U as User\nU->>W: hi\nW-->>X: seen")
	assert.Equal(t, "sequence", spec.Type)
	assert.Equal(t, []schema.ActorSpec{actor("W", "Web App"), actor("U", "User"), actor("X", "X")}, spec.Actors)
	assert.Equal(t, []schema.InteractionSpec{msg("U", "W", "hi", ""), msg("W", "X", "seen", "dashed")}, spec.Interactions)
	assert.Equal(t, []Report{{3, `actor "U" rendered as participant`}}, reports)
	// duplicate explicit participant
	pe := errOf(t, "sequenceDiagram\nparticipant A\nparticipant A")
	assert.Equal(t, 3, pe.Line)
	assert.Equal(t, `duplicate participant "A"`, pe.Message)
}

func TestSequence_ArrowsAndActivations(t *testing.T) {
	// arrow kinds and activation shorthand; -) async arrow without dashes
	spec, reports := parseSeq(t, "A->>+B: q\nB--)C: fyi\nB-->>-A: r\nA-)B: ping")
	assert.Equal(t, []schema.InteractionSpec{msg("A", "B", "q", ""), msg("B", "C", "fyi", "async"),
		msg("B", "A", "r", "dashed"), msg("A", "B", "ping", "async")}, spec.Interactions)
	assert.Equal(t, []Report{
		{2, "activation mark ignored, activations are engine-computed"},
		{4, "activation mark ignored, activations are engine-computed"},
	}, reports)

	// explicit activate/deactivate lines are reported, not emitted
	spec, reports = parseSeq(t, "A->>B: q\nactivate B\nB-->>A: r\ndeactivate B")
	assert.Len(t, spec.Interactions, 2)
	assert.Equal(t, []Report{
		{3, `"activate B" ignored, activations are engine-computed`},
		{5, `"deactivate B" ignored, activations are engine-computed`},
	}, reports)
	assert.Equal(t, `deactivate "A" must follow a message from "A"`, errOf(t, "sequenceDiagram\ndeactivate A").Message)
	assert.Equal(t, `activate "A" must follow a message to "A"`, errOf(t, "sequenceDiagram\nactivate A").Message)

	// sender/receiver ids containing "-" must not be mistaken for a lost-message arrow
	spec, reports = parseSeq(t, "web-app->>db: q\nsvc-xyz-->>web-app: r")
	assert.Equal(t, []schema.ActorSpec{actor("web-app", "web-app"), actor("db", "db"), actor("svc-xyz", "svc-xyz")}, spec.Actors)
	assert.Equal(t, []schema.InteractionSpec{msg("web-app", "db", "q", ""), msg("svc-xyz", "web-app", "r", "dashed")}, spec.Interactions)
	assert.Empty(t, reports)
}

func TestSequence_NotesFold(t *testing.T) {
	// notes: over one, over two, left of, right of: all fold onto the
	// nearest preceding message in the same block
	spec, reports := parseSeq(t, "A->>B: x\nNote over A: one\nNote over A,B: both\nNote left of A: l\nNote right of B: r")
	assert.Len(t, spec.Interactions, 1)
	assert.Equal(t, "x (note: one) (note: both) (note: l) (note: r)", spec.Interactions[0].Label)
	assert.Equal(t, []Report{
		{3, `note folded into "x"`},
		{4, `note folded into "x (note: one)"`},
		{5, `note folded into "x (note: one) (note: both)"`},
		{6, `note folded into "x (note: one) (note: both) (note: l)"`},
	}, reports)
	// a note before any message in its block is dropped
	_, reports = parseSeq(t, "Note over A: early\nA->>B: x\nloop l\n  Note over A: first\n  A->>B: y\nend")
	assert.Equal(t, []Report{
		{2, "note dropped, no preceding message in its block"},
		{5, "note dropped, no preceding message in its block"},
	}, reports)
	// inside a nested branch the note folds onto that branch's last message
	spec, _ = parseSeq(t, "A->>B: outer\nalt a\n  A->>B: inner\n  Note over B: n\nelse b\n  Note over B: m\nend")
	assert.Equal(t, "outer", spec.Interactions[0].Label)
	assert.Equal(t, "inner (note: n)", spec.Interactions[1].Label)
	// a note on an unlabeled message
	spec, _ = parseSeq(t, "A->>B:\nNote over A: n")
	assert.Equal(t, "(note: n)", spec.Interactions[0].Label)
}

func TestSequence_Blocks(t *testing.T) {
	// loop, opt, nested alt/else
	spec, reports := parseSeq(t, strings.Join([]string{
		"loop every 30s", "  A->>B: poll", "  alt fresh", "    B-->>A: data", "  else stale", "    B-->>A: 304", "  end", "end",
	}, "\n"))
	assert.Empty(t, reports)
	assert.Equal(t, []schema.FragmentSpec{
		{Type: "loop", Over: []string{"A", "B"}, Sections: []schema.FragmentSectionSpec{section("every 30s", 0, 2)}},
		{Type: "alt", Over: []string{"A", "B"}, Sections: []schema.FragmentSectionSpec{section("fresh", 1, 1), section("stale", 2, 2)}},
	}, spec.Fragments)

	// par/and and break (extensions), opt, over restricted to the block's actors
	spec, _ = parseSeq(t, "A->>B: x\nC->>D: y\npar first\n  A->>B: p\nand second\n  A->>B: q\nend\nbreak on error\n  B->>A: e\nend\nopt maybe\n  C->>D: z\nend")
	assert.Equal(t, []schema.FragmentSpec{
		{Type: "par", Over: []string{"A", "B"}, Sections: []schema.FragmentSectionSpec{section("first", 2, 2), section("second", 3, 3)}},
		{Type: "break", Over: []string{"A", "B"}, Sections: []schema.FragmentSectionSpec{section("on error", 4, 4)}},
		{Type: "opt", Over: []string{"C", "D"}, Sections: []schema.FragmentSectionSpec{section("maybe", 5, 5)}},
	}, spec.Fragments)

	// empty branches and empty blocks are dropped with a report
	spec, reports = parseSeq(t, "alt a\n  A->>B: x\nelse b\nend\nloop nothing\nend")
	assert.Equal(t, []schema.FragmentSpec{
		{Type: "alt", Over: []string{"A", "B"}, Sections: []schema.FragmentSectionSpec{section("a", 0, 0)}},
	}, spec.Fragments)
	assert.Equal(t, []Report{
		{4, `empty "else" branch dropped`},
		{6, `empty "loop" block dropped`},
	}, reports)

	// a block keyword needs whitespace or end of line after it, so a
	// participant named "opt" starting a message line stays a participant
	spec, reports = parseSeq(t, "opt->>B: x\nloop\n  B-->>opt: y\nend")
	assert.Equal(t, []schema.InteractionSpec{msg("opt", "B", "x", ""), msg("B", "opt", "y", "dashed")}, spec.Interactions)
	assert.Equal(t, []schema.FragmentSpec{
		{Type: "loop", Over: []string{"opt", "B"}, Sections: []schema.FragmentSectionSpec{section("", 1, 1)}},
	}, spec.Fragments)
	assert.Empty(t, reports)
}

func TestSequence_Frontmatter(t *testing.T) {
	res, err := Parse([]byte("---\ntitle: Flow\n---\nsequenceDiagram\nA->>B: x"))
	require.NoError(t, err)
	var spec schema.SequenceSpec
	require.NoError(t, json.Unmarshal(res.JSON, &spec))
	assert.Equal(t, "Flow", spec.Title)
}

func TestSequence_Errors(t *testing.T) {
	// rejections name the construct and line
	assert.Contains(t, errOf(t, "sequenceDiagram\nautonumber\nA->>B: y").Message, `"autonumber" is not supported by diago's sequence subset`)
	assert.Contains(t, errOf(t, "sequenceDiagram\ncritical x\nA->>B: y\nend").Message, `"critical"`)
	pe := errOf(t, "sequenceDiagram\nA-xB: gone")
	assert.Equal(t, 2, pe.Line)
	assert.Equal(t, "lost-message arrows (-x) are not supported", pe.Message)
	pe = errOf(t, "sequenceDiagram\nA--xB: gone")
	assert.Equal(t, 2, pe.Line)
	assert.Equal(t, "lost-message arrows (-x) are not supported", pe.Message)
	assert.Equal(t, `"else" outside an "alt" block`, errOf(t, "sequenceDiagram\nelse lonely").Message)
	assert.Equal(t, `"and" outside a "par" block`, errOf(t, "sequenceDiagram\nand lonely").Message)
	assert.Equal(t, `"end" without an open block`, errOf(t, "sequenceDiagram\nend").Message)
	pe = errOf(t, "sequenceDiagram\nloop x\nA->>B: y\n")
	assert.Equal(t, `unclosed "loop" block`, pe.Message)
	assert.Equal(t, 2, pe.Line)
	pe = errOf(t, "sequenceDiagram\nwhat is this")
	assert.Equal(t, `expected a message, note, participant, block, or end near "what is this"`, pe.Message)
	// a source that starts with a message has no header and is not Mermaid
	assert.Equal(t, "not a Mermaid source", errOf(t, "A->>B: x").Message)
}
