package diff

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDiffGraphs_NodeFields(t *testing.T) {
	for name, tc := range map[string]struct {
		before, after string
		want          []FieldChange
	}{
		"label": {`{"id":"a","label":"A"}`, `{"id":"a","label":"A!"}`,
			[]FieldChange{{"label", `"A"`, `"A!"`}}},
		"shape": {`{"id":"a","label":"A","shape":"rect"}`, `{"id":"a","label":"A","shape":"cylinder"}`,
			[]FieldChange{{"shape", "rect", "cylinder"}}},
		"color": {`{"id":"a","label":"A"}`, `{"id":"a","label":"A","color":"red"}`,
			[]FieldChange{{"color", "none", "red"}}},
		"in spec order": {`{"id":"a","label":"A"}`, `{"id":"a","label":"B","shape":"diamond","color":"#ff0000"}`,
			[]FieldChange{{"label", `"A"`, `"B"`}, {"shape", "rect", "diamond"}, {"color", "none", "#ff0000"}}},
	} {
		t.Run(name, func(t *testing.T) {
			d := DiffGraphs(parse(t, `{"type":"flow","nodes":[`+tc.before+`],"edges":[]}`),
				parse(t, `{"type":"flow","nodes":[`+tc.after+`],"edges":[]}`))
			assert.Equal(t, tc.want, d.NodeFields["a"])
		})
	}
}

func TestDiffGraphs_NodeGroupField(t *testing.T) {
	before := parse(t, specA) // b and c in group G
	after := parse(t, `{"type":"flow","direction":"DOWN",
	 "nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"},{"id":"c","label":"C","shape":"cylinder"}],
	 "edges":[{"from":"a","to":"b"},{"id":"link","from":"b","to":"c","label":"x"}],
	 "groups":[{"id":"g","label":"G","contains":["c"]},{"id":"h","label":"H","contains":["a"]}]}`)
	d := DiffGraphs(before, after)
	assert.Equal(t, []FieldChange{{"group", `"G"`, "none"}}, d.NodeFields["b"], "left its group")
	assert.Equal(t, []FieldChange{{"group", "none", `"H"`}}, d.NodeFields["a"], "joined a group")
}

func TestDiffGraphs_EdgeFields(t *testing.T) {
	before := parse(t, `{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":[
	 {"from":"a","to":"b"},{"from":"a","to":"b"},{"from":"b","to":"a"},{"id":"f","from":"b","to":"a"},{"id":"l","from":"a","to":"b"}]}`)
	after := parse(t, `{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":[
	 {"from":"a","to":"b","style":"dashed"},{"from":"a","to":"b","direction":"both"},{"from":"b","to":"a","color":"red"},
	 {"id":"f","from":"b","to":"a","flat":true},{"id":"l","from":"a","to":"b","label":"read/write","style":"dotted"}]}`)
	d := DiffGraphs(before, after)
	assert.Equal(t, []FieldChange{{"style", "solid", "dashed"}}, d.EdgeFields["a->b#0"])
	assert.Equal(t, []FieldChange{{"direction", "forward", "both"}}, d.EdgeFields["a->b#1"])
	assert.Equal(t, []FieldChange{{"color", "none", "red"}}, d.EdgeFields["b->a#0"])
	assert.Equal(t, []FieldChange{{"flat", "false", "true"}}, d.EdgeFields["f"])
	assert.Equal(t, []FieldChange{{"label", "none", `"read/write"`}, {"style", "solid", "dotted"}}, d.EdgeFields["l"])
}

func TestDiffGraphs_RelationFields(t *testing.T) {
	classes := `"classes":[{"id":"a","label":"A"},{"id":"b","label":"B"}]`
	d := DiffGraphs(
		parseClass(t, `{"type":"class",`+classes+`,"relations":[{"id":"r","from":"a","to":"b","kind":"association","to_card":"1"},{"id":"s","from":"a","to":"b","from_card":"1"}]}`),
		parseClass(t, `{"type":"class",`+classes+`,"relations":[{"id":"r","from":"a","to":"b","kind":"dependency","to_card":"1"},{"id":"s","from":"a","to":"b","from_card":"0..1","to_card":"*"}]}`))
	assert.Equal(t, []FieldChange{{"kind", "association", "dependency"}, {"directed", "false", "true"}}, d.EdgeFields["r"],
		"a dependency is directed by default, and the list says so")
	assert.Equal(t, []FieldChange{{"from_card", `"1"`, `"0..1"`}, {"to_card", "none", `"*"`}}, d.EdgeFields["s"])
}

func TestDiffGraphs_ClassFields(t *testing.T) {
	d := DiffGraphs(
		parseClass(t, `{"type":"class","classes":[{"id":"a","label":"A","type_params":["T"]},{"id":"m","label":"M","attributes":[{"text":"x: int"}]}]}`),
		parseClass(t, `{"type":"class","classes":[{"id":"a","label":"A","stereotype":"entity","type_params":["T","K"]},{"id":"m","label":"M","attributes":[{"text":"x: int"},{"text":"y: int"}]}]}`))
	assert.Equal(t, []FieldChange{{"stereotype", "none", `"entity"`}, {"type_params", "T", "T, K"}}, d.NodeFields["a"])
	assert.Contains(t, d.ChangedNodes, "m")
	assert.Empty(t, d.NodeFields["m"], "a members-only change has no fields: the member lines name it")
}

func TestDiffGraphs_GroupFields(t *testing.T) {
	d := DiffGraphs(
		parse(t, `{"type":"flow","nodes":[{"id":"a","label":"A"}],"edges":[],"groups":[{"id":"g","label":"G","contains":["a"]}]}`),
		parse(t, `{"type":"flow","nodes":[{"id":"a","label":"A"}],"edges":[],"groups":[{"id":"g","label":"Core","color":"blue","contains":["a"]}]}`))
	assert.Equal(t, []FieldChange{{"label", `"G"`, `"Core"`}, {"color", "none", "blue"}}, d.GroupFields["g"])
}

func TestDiffGraphs_NoFieldsWhenIdentical(t *testing.T) {
	d := DiffGraphs(parse(t, specA), parse(t, specA))
	assert.Empty(t, d.NodeFields)
	assert.Empty(t, d.EdgeFields)
	assert.Empty(t, d.GroupFields)
}

func TestBuildUnionSequence_ActorFields(t *testing.T) {
	u := BuildUnionSequence(
		parseSeq(t, `{"type":"sequence","actors":[{"id":"u","label":"User"},{"id":"w","label":"Web"}],"interactions":[{"from":"u","to":"w","label":"a"}]}`),
		parseSeq(t, `{"type":"sequence","actors":[{"id":"u","label":"Person"},{"id":"w","label":"Web","color":"red"}],"interactions":[{"from":"u","to":"w","label":"a"}]}`))
	assert.Equal(t, []FieldChange{{"label", `"User"`, `"Person"`}}, u.Diff.ActorFields["u"])
	assert.Equal(t, []FieldChange{{"color", "none", "red"}}, u.Diff.ActorFields["w"])
}

func TestMessageFields(t *testing.T) {
	seq := parseSeq(t, `{"type":"sequence","actors":[{"id":"u","label":"U"},{"id":"w","label":"W"}],"interactions":[
	 {"from":"u","to":"w","label":"a"},{"from":"u","to":"w","label":"b","style":"async","color":"blue"}]}`)
	assert.Equal(t, []FieldChange{{"label", `"a"`, `"b"`}, {"style", "solid", "async"}, {"color", "none", "blue"}},
		messageFields(seq.Interactions[0], seq.Interactions[1]))
	assert.Empty(t, messageFields(seq.Interactions[0], seq.Interactions[0]))
}

func TestDiffGraphs_RelationCardsInSpecOrientation(t *testing.T) {
	classes := `"classes":[{"id":"a","label":"A"},{"id":"d","label":"D"}]`
	d := DiffGraphs(
		parseClass(t, `{"type":"class",`+classes+`,"relations":[{"id":"r","from":"d","to":"a","kind":"inheritance","to_card":"0..1"}]}`),
		parseClass(t, `{"type":"class",`+classes+`,"relations":[{"id":"r","from":"d","to":"a","kind":"inheritance","to_card":"*"}]}`))
	assert.Equal(t, []FieldChange{{"to_card", `"0..1"`, `"*"`}}, d.EdgeFields["r"], "a swapped kind keeps the spec's to_card")

	d = DiffGraphs(
		parseClass(t, `{"type":"class",`+classes+`,"relations":[{"id":"r","from":"a","to":"d","kind":"association","from_card":"1","to_card":"*"}]}`),
		parseClass(t, `{"type":"class",`+classes+`,"relations":[{"id":"r","from":"d","to":"a","kind":"inheritance","from_card":"1","to_card":"*"}]}`))
	assert.Equal(t, []FieldChange{{"kind", "association", "inheritance"}}, d.EdgeFields["r"], "same spec cards, only the kind differs")
}

func TestQuotedValue(t *testing.T) {
	assert.Equal(t, `"Say \"hi\""`, quotedValue(`Say "hi"`))
	assert.Equal(t, `"a\nb"`, quotedValue("a\nb"))
	assert.Equal(t, `"Café →"`, quotedValue("Café →"))
	assert.Equal(t, "none", quotedValue(""))
}
