package diff

import (
	"encoding/json"
	"testing"

	"github.com/oxforge/diago/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ids(ns []model.Node) []string {
	out := make([]string, len(ns))
	for i, n := range ns {
		out[i] = n.ID
	}
	return out
}

func TestBuildUnion_OrderAndStatuses(t *testing.T) {
	before := parse(t, specA)
	after := parse(t, `{"type":"flow","direction":"RIGHT",
	 "nodes":[{"id":"d","label":"D"},{"id":"a","label":"A!"},{"id":"b","label":"B"}],
	 "edges":[{"from":"a","to":"b","label":"now"},{"from":"b","to":"d"}],
	 "groups":[{"id":"g","label":"G","contains":["b"]}]}`)
	u := BuildUnion(before, after)
	assert.Equal(t, []string{"d", "a", "b", "c"}, ids(u.Graph.Nodes), "after order, then removed in before order")
	assert.Equal(t, model.Right, u.Graph.Direction, "presentation from after")
	assert.Equal(t, map[string]Status{"d": Added, "a": Changed, "b": Same, "c": Removed}, u.Nodes)
	assert.Equal(t, map[string]Status{"a->b#0": Changed, "b->d#0": Added, "link": Removed}, u.Edges)
	assert.Equal(t, map[string]Status{"g": Same}, u.Groups)
	assert.Equal(t, "C", u.BeforeLabels["c"])
	assert.Equal(t, "A", u.BeforeLabels["a"], "before labels are kept for every before element")
	g := u.Graph.Groups[0]
	assert.ElementsMatch(t, []string{"b", "c"}, g.Contains, "removed c stays in its before group")
}

func TestBuildUnion_KeepsEdgeFlat(t *testing.T) {
	// BuildUnion copies model.Edge whole (edges = append(edges, e)), so Flat
	// rides along with every other edge field without dedicated wiring.
	before := parse(t, `{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"},{"id":"c","label":"C"}],
	 "edges":[{"id":"e1","from":"a","to":"b","flat":true},{"id":"e2","from":"b","to":"c"}]}`)
	after := parse(t, `{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],
	 "edges":[{"id":"e1","from":"a","to":"b","flat":true}]}`)
	u := BuildUnion(before, after)
	byID := map[string]model.Edge{}
	for _, e := range u.Graph.Edges {
		byID[e.ID] = e
	}
	require.True(t, byID["e1"].Flat, "surviving flat edge keeps it")
	require.False(t, byID["e2"].Flat, "removed ordinary edge stays ordinary")
}

// TestBuildUnion_RemovedFlatEdgeUnderItsNewIDKeepsFlat: a removed flat
// edge whose id the after spec reuses for an ordinary edge enters the
// union as <id>__removed, flat as the before spec had it, beside the
// reused id, ordinary as the after spec has it; so the union graph routes
// the removed one flat (S9, Flat edges) and ranks the new one.
func TestBuildUnion_RemovedFlatEdgeUnderItsNewIDKeepsFlat(t *testing.T) {
	before := parse(t, `{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"},{"id":"c","label":"C"}],
	 "edges":[{"id":"e","from":"a","to":"b","flat":true}]}`)
	after := parse(t, `{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"},{"id":"c","label":"C"}],
	 "edges":[{"id":"e","from":"a","to":"c"}]}`)
	u := BuildUnion(before, after)
	byID := map[string]model.Edge{}
	for _, e := range u.Graph.Edges {
		byID[e.ID] = e
	}
	require.Contains(t, byID, "e__removed")
	assert.Equal(t, Removed, u.Edges["e__removed"])
	assert.True(t, byID["e__removed"].Flat, "the removed flat edge keeps flat")
	assert.Equal(t, "b", byID["e__removed"].To)
	assert.False(t, byID["e"].Flat, "the reused id is the after spec's ordinary edge")
}

func TestBuildUnion_RemovedEdgeIdCollision(t *testing.T) {
	before := parse(t, `{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"},{"id":"c","label":"C"}],"edges":[{"id":"e","from":"a","to":"b"}]}`)
	after := parse(t, `{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"},{"id":"c","label":"C"}],"edges":[{"id":"e","from":"a","to":"c"}]}`)
	u := BuildUnion(before, after)
	var got []string
	for _, e := range u.Graph.Edges {
		got = append(got, e.ID)
	}
	assert.Equal(t, []string{"e", "e__removed"}, got)
	assert.Equal(t, Added, u.Edges["e"])
	assert.Equal(t, Removed, u.Edges["e__removed"])
	assert.Equal(t, "b", u.Graph.Edges[1].To, "both geometries survive under distinct ids")
}

func TestBuildUnion_RemovedSubtreeKeepsStructure(t *testing.T) {
	before := parse(t, `{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"x","label":"X"},{"id":"y","label":"Y"}],
	 "edges":[{"from":"a","to":"x"},{"from":"x","to":"y"}],
	 "groups":[{"id":"outer","label":"O","contains":["a","inner"]},{"id":"inner","label":"I","contains":["x","y"]}]}`)
	after := parse(t, `{"type":"flow","nodes":[{"id":"a","label":"A"}],"edges":[],
	 "groups":[{"id":"outer","label":"O","contains":["a"]}]}`)
	u := BuildUnion(before, after)
	require.Len(t, u.Graph.Groups, 2)
	outer, inner := u.Graph.Groups[0], u.Graph.Groups[1]
	assert.Equal(t, "outer", outer.ID)
	assert.Equal(t, []string{"inner"}, outer.Children, "removed child group re-attached under its before parent")
	assert.ElementsMatch(t, []string{"x", "y"}, inner.Contains)
	assert.Equal(t, 2, inner.Depth)
	assert.Equal(t, Removed, u.Groups["inner"])
	assert.Equal(t, []string{"inner"}, u.Diff.RemovedGroups)
	assert.Equal(t, []string{"x", "y"}, u.Diff.RemovedNodes)
}

func TestBuildUnion_DepthOverflowHoistsToTop(t *testing.T) {
	// before: p(1) > g(2) > h(3); after moves p under a new top group q, so
	// re-attaching g under p would put h at depth 4. g is hoisted to the top.
	before := parse(t, `{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":[],
	 "groups":[{"id":"p","label":"P","contains":["g"]},{"id":"g","label":"G","contains":["h"]},{"id":"h","label":"H","contains":["a","b"]}]}`)
	after := parse(t, `{"type":"flow","nodes":[{"id":"a","label":"A"}],"edges":[],
	 "groups":[{"id":"q","label":"Q","contains":["p"]},{"id":"p","label":"P","contains":["a"]}]}`)
	u := BuildUnion(before, after)
	depth := map[string]int{}
	for _, g := range u.Graph.Groups {
		depth[g.ID] = g.Depth
	}
	assert.Equal(t, 1, depth["g"], "hoisted")
	assert.Equal(t, 2, depth["h"])
	assert.LessOrEqual(t, depth["h"], 3)
}

func TestBuildUnion_HoistIndependentOfBeforeGroupOrder(t *testing.T) {
	// Same graph as TestBuildUnion_DepthOverflowHoistsToTop, but the before
	// spec lists the removed groups in two different orders: parent-first
	// (p, g, h) and child-first (h, g, p). The schema preserves spec order
	// and does not topologically sort, so the hoist decision must not
	// depend on which order the removed groups happen to be listed in.
	after := parse(t, `{"type":"flow","nodes":[{"id":"a","label":"A"}],"edges":[],
	 "groups":[{"id":"q","label":"Q","contains":["p"]},{"id":"p","label":"P","contains":["a"]}]}`)
	orderings := map[string]string{
		"parent-first": `{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":[],
		 "groups":[{"id":"p","label":"P","contains":["g"]},{"id":"g","label":"G","contains":["h"]},{"id":"h","label":"H","contains":["a","b"]}]}`,
		"child-first": `{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":[],
		 "groups":[{"id":"h","label":"H","contains":["a","b"]},{"id":"g","label":"G","contains":["h"]},{"id":"p","label":"P","contains":["g"]}]}`,
	}
	for name, spec := range orderings {
		t.Run(name, func(t *testing.T) {
			before := parse(t, spec)
			u := BuildUnion(before, after)
			depth := map[string]int{}
			parentOf := map[string]string{}
			for _, g := range u.Graph.Groups {
				depth[g.ID] = g.Depth
				for _, c := range g.Children {
					parentOf[c] = g.ID
				}
			}
			assert.Equal(t, 1, depth["g"], "g is hoisted to the top")
			assert.Equal(t, 2, depth["h"])
			assert.Equal(t, "g", parentOf["h"], "h stays attached under g")
		})
	}
}

func TestBuildUnion_Deterministic(t *testing.T) {
	before, after := parse(t, specA), parse(t, `{"type":"flow","direction":"DOWN",
	 "nodes":[{"id":"d","label":"D"},{"id":"a","label":"A!"},{"id":"b","label":"B"}],
	 "edges":[{"from":"a","to":"b","label":"now"},{"from":"b","to":"d"}],
	 "groups":[{"id":"g","label":"G","contains":["b"]}]}`)
	first, _ := json.Marshal(BuildUnion(before, after))
	for i := 0; i < 20; i++ {
		again, _ := json.Marshal(BuildUnion(before, after))
		require.Equal(t, string(first), string(again))
	}
}

func TestBuildUnion_IdenticalIsAllSame(t *testing.T) {
	u := BuildUnion(parse(t, specA), parse(t, specA))
	for id, s := range u.Nodes {
		assert.Equal(t, Same, s, id)
	}
	for id, s := range u.Edges {
		assert.Equal(t, Same, s, id)
	}
	assert.True(t, u.Diff.Empty())
	assert.Equal(t, 3, len(u.Graph.Nodes))
}

const classBefore = `{"type":"class","classes":[
 {"id":"a","label":"A","methods":[{"visibility":"+","text":"go()"},{"visibility":"+","text":"legacy()"}]},
 {"id":"b","label":"B"}]}`

const classAfter = `{"type":"class","legend":true,"classes":[
 {"id":"a","label":"A","methods":[{"visibility":"+","text":"go()"},{"visibility":"+","text":"stop()"}]},
 {"id":"b","label":"B"}]}`

func TestBuildUnion_ClassMembers(t *testing.T) {
	u := BuildUnion(parseClass(t, classBefore), parseClass(t, classAfter))
	assert.True(t, u.Class)
	assert.Nil(t, u.Graph.Legend, "a diff picture carries no legend")
	var node model.Node
	for _, n := range u.Graph.Nodes {
		if n.ID == "a" {
			node = n
		}
	}
	require.NotNil(t, node.Members)
	var lines []string
	for _, mm := range node.Members.Methods {
		lines = append(lines, mm.Text)
	}
	assert.Equal(t, []string{"go()", "legacy()", "stop()"}, lines)
	assert.Equal(t, []Status{Same, Removed, Added}, u.Members["a"].MethodStatus)
}

func TestBuildUnion_UnchangedClassKeepsOwnMembers(t *testing.T) {
	u := BuildUnion(parseClass(t, classBefore), parseClass(t, classBefore))
	assert.True(t, u.Class)
	assert.Empty(t, u.Members)
	for _, n := range u.Graph.Nodes {
		if n.ID == "a" {
			require.NotNil(t, n.Members)
			assert.Len(t, n.Members.Methods, 2)
		}
	}
}

// The union is presentation-free: a title on either side
// never reaches the diff render.
func TestBuildUnion_DropsTitle(t *testing.T) {
	before := &model.Graph{Title: "Before", Nodes: []model.Node{{ID: "a", Label: "A"}}}
	after := &model.Graph{Title: "After", Nodes: []model.Node{{ID: "a", Label: "A"}}}
	u := BuildUnion(before, after)
	assert.Equal(t, "", u.Graph.Title)
}
