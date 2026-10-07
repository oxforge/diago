package diff

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/oxforge/diago/internal/model"
)

func TestStampLabels(t *testing.T) {
	u := BuildUnion(parse(t, specA), parse(t, `{"type":"flow","direction":"DOWN",
	 "nodes":[{"id":"d","label":"D"},{"id":"a","label":"A!"},{"id":"b","label":"B"}],
	 "edges":[{"from":"a","to":"b","label":"now"},{"from":"b","to":"d"}],
	 "groups":[{"id":"g","label":"G2","contains":["b"]}]}`))
	g := StampLabels(u)
	labels := map[string]string{}
	for _, n := range g.Nodes {
		labels[n.ID] = n.Label
	}
	assert.Equal(t, "+ D", labels["d"])
	assert.Equal(t, "~ A!", labels["a"])
	assert.Equal(t, "B", labels["b"])
	assert.Equal(t, "- C", labels["c"])
	assert.Equal(t, "~ G2", g.Groups[0].Label)
	assert.Equal(t, "now", g.Edges[0].Label, "edge labels are never stamped")
	assert.Equal(t, "A!", u.Graph.Nodes[1].Label, "the union itself is untouched")
}

func TestFlowLegend_OrderAndWording(t *testing.T) {
	u := BuildUnion(parse(t, specA), parse(t, `{"type":"flow","direction":"DOWN",
	 "nodes":[{"id":"d","label":"D"},{"id":"a","label":"A!"},{"id":"b","label":"B"}],
	 "edges":[{"from":"a","to":"b","label":"now"},{"from":"b","to":"d"}],
	 "groups":[{"id":"g","label":"G2","contains":["b"]}]}`))
	assert.Equal(t, []string{
		"added: node D",
		"added: edge B -> D",
		"removed: node C",
		"removed: edge B -> C",
		`changed: node A!: label "A" → "A!"`,
		`changed: edge A! -> B: label none → "now"`,
		`changed: group G2: label "G" → "G2"`,
	}, FlowLegend(u))
}

func TestFlowLegend_EmptyWhenIdentical(t *testing.T) {
	assert.Empty(t, FlowLegend(BuildUnion(parse(t, specA), parse(t, specA))))
}

func TestStampLabels_MemberMarks(t *testing.T) {
	u := BuildUnion(parseClass(t, `{"type":"class","classes":[
	 {"id":"a","label":"A","attributes":[{"visibility":"-","text":"keep: int"},{"visibility":"-","text":"gone: int"},{"visibility":"-","text":"edit: int"}]},
	 {"id":"b","label":"B","attributes":[{"visibility":"-","text":"same: int"}]}]}`),
		parseClass(t, `{"type":"class","classes":[
	 {"id":"a","label":"A","attributes":[{"visibility":"-","text":"keep: int"},{"visibility":"+","text":"edit: int"},{"visibility":"-","text":"fresh: int"}]},
	 {"id":"b","label":"B","attributes":[{"visibility":"-","text":"same: int"}]}]}`))
	g := StampLabels(u)
	byID := map[string]model.Node{}
	for _, n := range g.Nodes {
		byID[n.ID] = n
	}
	var marks []string
	var texts []string
	for _, mm := range byID["a"].Members.Attributes {
		marks = append(marks, mm.Mark)
		texts = append(texts, mm.Text)
	}
	assert.Equal(t, []string{"keep: int", "gone: int", "edit: int", "fresh: int"}, texts)
	assert.Equal(t, []string{"  ", "- ", "~ ", "+ "}, marks)
	for _, mm := range byID["b"].Members.Attributes {
		assert.Equal(t, "  ", mm.Mark, "a class without a union still gets the blank column")
	}
	assert.Empty(t, u.Graph.Nodes[0].Members.Attributes[0].Mark, "the union itself is untouched")
}

func TestFlowLegend_SwappedRelationNamedInSpecOrientation(t *testing.T) {
	before := parseClass(t, `{"type":"class","classes":[{"id":"dog","label":"Dog"},{"id":"animal","label":"Animal"}],
	 "relations":[{"id":"isa","from":"dog","to":"animal","kind":"inheritance"}]}`)
	after := parseClass(t, `{"type":"class","classes":[{"id":"dog","label":"Dog"},{"id":"animal","label":"Animal"}]}`)
	assert.Equal(t, []string{"removed: relation Dog -> Animal"}, FlowLegend(BuildUnion(before, after)))
}

func TestFlowLegend_LabelsWithNewlineAndQuote(t *testing.T) {
	before := parse(t, `{"type":"flow","nodes":[{"id":"a","label":"Two\nLines"},{"id":"b","label":"B"}],"edges":[]}`)
	after := parse(t, `{"type":"flow","nodes":[{"id":"a","label":"Two\nLines","color":"red"},{"id":"b","label":"Say \"hi\""}],"edges":[]}`)
	got := FlowLegend(BuildUnion(before, after))
	assert.Equal(t, []string{
		`changed: node Two Lines: color none → red`,
		`changed: node Say "hi": label "B" → "Say \"hi\""`,
	}, got)
	for _, l := range got {
		assert.NotContains(t, l, "\n", "one change, one line")
	}
}
