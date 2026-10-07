package diff

import (
	"testing"

	"github.com/oxforge/diago/internal/model"
	"github.com/oxforge/diago/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func parse(t *testing.T, spec string) *model.Graph {
	t.Helper()
	g, err := schema.ParseFlow([]byte(spec))
	require.NoError(t, err)
	return g
}

func parseClass(t *testing.T, spec string) *model.Graph {
	t.Helper()
	g, err := schema.ParseClass([]byte(spec))
	require.NoError(t, err)
	return g
}

const specA = `{"type":"flow","direction":"DOWN",
 "nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"},{"id":"c","label":"C","shape":"cylinder"}],
 "edges":[{"from":"a","to":"b"},{"id":"link","from":"b","to":"c","label":"x"}],
 "groups":[{"id":"g","label":"G","contains":["b","c"]}]}`

func TestDiffGraphs_AddedRemovedChanged(t *testing.T) {
	before := parse(t, specA)
	after := parse(t, `{"type":"flow","direction":"DOWN",
	 "nodes":[{"id":"a","label":"A!"},{"id":"b","label":"B"},{"id":"d","label":"D"}],
	 "edges":[{"from":"a","to":"b","label":"now"},{"from":"b","to":"d"}],
	 "groups":[{"id":"g","label":"G2","contains":["b"]}]}`)
	d := DiffGraphs(before, after)
	assert.Equal(t, []string{"d"}, d.AddedNodes)
	assert.Equal(t, []string{"c"}, d.RemovedNodes)
	assert.Equal(t, []string{"a"}, d.ChangedNodes, "label change")
	assert.Equal(t, []string{"b->d#0"}, d.AddedEdges)
	assert.Equal(t, []string{"link"}, d.RemovedEdges)
	assert.Equal(t, []string{"a->b#0"}, d.ChangedEdges, "label change under the same id")
	assert.Equal(t, []string{"g"}, d.ChangedGroups, "label change; membership shows on members")
	assert.Empty(t, d.AddedGroups)
	assert.Empty(t, d.RemovedGroups)
}

func TestDiffGraphs_ParentChangeIsNodeChange(t *testing.T) {
	before := parse(t, specA)
	after := parse(t, `{"type":"flow","direction":"DOWN",
	 "nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"},{"id":"c","label":"C","shape":"cylinder"}],
	 "edges":[{"from":"a","to":"b"},{"id":"link","from":"b","to":"c","label":"x"}],
	 "groups":[{"id":"g","label":"G","contains":["c"]}]}`)
	d := DiffGraphs(before, after)
	assert.Equal(t, []string{"b"}, d.ChangedNodes, "b left the group")
	assert.Empty(t, d.ChangedGroups)
}

func TestDiffGraphs_MovedEdgeIsRemovedPlusAdded(t *testing.T) {
	before := parse(t, specA)
	after := parse(t, `{"type":"flow","direction":"DOWN",
	 "nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"},{"id":"c","label":"C","shape":"cylinder"}],
	 "edges":[{"from":"a","to":"b"},{"id":"link","from":"a","to":"c","label":"x"}],
	 "groups":[{"id":"g","label":"G","contains":["b","c"]}]}`)
	d := DiffGraphs(before, after)
	assert.Equal(t, []string{"link"}, d.AddedEdges)
	assert.Equal(t, []string{"link"}, d.RemovedEdges)
	assert.Empty(t, d.ChangedEdges)
}

func TestDiffGraphs_EdgeStyleDirectionColorChange(t *testing.T) {
	before := parse(t, `{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":[{"from":"a","to":"b"},{"from":"a","to":"b"},{"from":"b","to":"a"}]}`)
	after := parse(t, `{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":[{"from":"a","to":"b","style":"dashed"},{"from":"a","to":"b","direction":"both"},{"from":"b","to":"a","color":"red"}]}`)
	d := DiffGraphs(before, after)
	assert.Equal(t, []string{"a->b#0", "a->b#1", "b->a#0"}, d.ChangedEdges)
}

func TestDiffGraphs_EdgeFlatChange(t *testing.T) {
	before := parse(t, `{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":[{"from":"a","to":"b"}]}`)
	after := parse(t, `{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":[{"from":"a","to":"b","flat":true}]}`)
	d := DiffGraphs(before, after)
	assert.Equal(t, []string{"a->b#0"}, d.ChangedEdges, "toggling flat alone marks the edge changed")
	assert.Empty(t, d.ChangedNodes)
	assert.Empty(t, d.AddedEdges)
	assert.Empty(t, d.RemovedEdges)
}

func TestDiffGraphs_IdenticalIsEmpty(t *testing.T) {
	d := DiffGraphs(parse(t, specA), parse(t, specA))
	assert.True(t, d.Empty())
}

func TestDiffGraphs_NodeShapeChange(t *testing.T) {
	before := parse(t, `{"type":"flow","nodes":[{"id":"a","label":"A","shape":"rect"}],"edges":[]}`)
	after := parse(t, `{"type":"flow","nodes":[{"id":"a","label":"A","shape":"cylinder"}],"edges":[]}`)
	d := DiffGraphs(before, after)
	assert.Equal(t, []string{"a"}, d.ChangedNodes)
}

func TestDiffGraphs_NodeColorChange(t *testing.T) {
	before := parse(t, `{"type":"flow","nodes":[{"id":"a","label":"A"}],"edges":[]}`)
	after := parse(t, `{"type":"flow","nodes":[{"id":"a","label":"A","color":"red"}],"edges":[]}`)
	d := DiffGraphs(before, after)
	assert.Equal(t, []string{"a"}, d.ChangedNodes)
}

func TestDiffGraphs_GroupColorChange(t *testing.T) {
	before := parse(t, `{"type":"flow","nodes":[{"id":"a","label":"A"}],"edges":[],"groups":[{"id":"g","label":"G","contains":["a"]}]}`)
	after := parse(t, `{"type":"flow","nodes":[{"id":"a","label":"A"}],"edges":[],"groups":[{"id":"g","label":"G","color":"blue","contains":["a"]}]}`)
	d := DiffGraphs(before, after)
	assert.Equal(t, []string{"g"}, d.ChangedGroups)
	assert.Empty(t, d.ChangedNodes, "group color change does not touch its members")
}

func TestDiffGraphs_ClassMemberChangeMarksNode(t *testing.T) {
	before := parseClass(t, `{"type":"class","classes":[
	 {"id":"a","label":"A","methods":[{"visibility":"+","text":"go()"}]},
	 {"id":"b","label":"B"}]}`)
	after := parseClass(t, `{"type":"class","classes":[
	 {"id":"a","label":"A","methods":[{"visibility":"+","text":"go()"},{"visibility":"+","text":"stop()"}]},
	 {"id":"b","label":"B"}]}`)
	d := DiffGraphs(before, after)
	assert.Equal(t, []string{"a"}, d.ChangedNodes)
	assert.Empty(t, d.AddedNodes)
	assert.Empty(t, d.RemovedNodes)
}

func TestDiffGraphs_RelationKindAndCardinality(t *testing.T) {
	base := `{"type":"class","classes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],
	 "relations":[{"from":"a","to":"b","kind":"association","to_card":"1"}]}`
	kind := DiffGraphs(parseClass(t, base), parseClass(t, `{"type":"class","classes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],
	 "relations":[{"from":"a","to":"b","kind":"dependency","to_card":"1"}]}`))
	assert.Equal(t, []string{"a->b#0"}, kind.ChangedEdges)
	card := DiffGraphs(parseClass(t, base), parseClass(t, `{"type":"class","classes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],
	 "relations":[{"from":"a","to":"b","kind":"association","to_card":"*"}]}`))
	assert.Equal(t, []string{"a->b#0"}, card.ChangedEdges)
}

func TestDiffGraphs_MembersOnOneSide(t *testing.T) {
	plain := &model.Graph{Nodes: []model.Node{{ID: "a", Label: "A"}}}
	withMembers := &model.Graph{Nodes: []model.Node{{ID: "a", Label: "A", Members: &model.Members{}}}}
	assert.Equal(t, []string{"a"}, DiffGraphs(plain, withMembers).ChangedNodes)
	assert.Equal(t, []string{"a"}, DiffGraphs(withMembers, plain).ChangedNodes)
	assert.Empty(t, DiffGraphs(withMembers, withMembers).ChangedNodes)
}
