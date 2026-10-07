package nest

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/model"
)

// graph is a flow graph of the given nodes, edges ("from->to") and groups.
func graph(nodes []string, edges []string, groups ...model.Group) model.Graph {
	g := model.Graph{Groups: groups}
	for _, id := range nodes {
		g.Nodes = append(g.Nodes, model.Node{ID: id, Label: id})
	}
	for _, e := range edges {
		for i := range len(e) - 1 {
			if e[i:i+2] == "->" {
				g.Edges = append(g.Edges, model.Edge{ID: e + "#0", From: e[:i], To: e[i+2:]})
				break
			}
		}
	}
	return g
}

func node(n int) Child      { return Child{Node: n, Container: -1} }
func container(c int) Child { return Child{Node: -1, Container: c} }

func TestBuild(t *testing.T) {
	g := graph([]string{"x", "a", "b", "c", "y"}, []string{"x->a", "a->b", "b->c", "c->y", "x->y", "a->c"},
		model.Group{ID: "outer", Contains: []string{"c"}, Children: []string{"inner"}},
		model.Group{ID: "inner", Contains: []string{"a", "b"}},
	)
	tr, err := Build(g)
	require.NoError(t, err)
	assert.Equal(t, []int{0, 2, 2, 1, 0}, tr.In, "x and y in the root, a and b in inner, c in outer")
	assert.Equal(t, []Child{node(0), container(1), node(4)}, tr.Containers[0].Children, "outer enters the root where a, its first node at any depth, was declared")
	assert.Equal(t, []Child{container(2), node(3)}, tr.Containers[1].Children)
	assert.Equal(t, []Child{node(1), node(2)}, tr.Containers[2].Children)
	assert.Equal(t, []int{-1, 0, 1}, []int{tr.Containers[0].Parent, tr.Containers[1].Parent, tr.Containers[2].Parent})
	assert.Equal(t, []int{0, 2, 1, 0, 0, 1}, tr.Home, "x->a at the root, a->b in inner, b->c and a->c in outer")
	assert.Equal(t, container(1), tr.Rep(0, 1), "a's representative at the root")
	assert.Equal(t, container(2), tr.Rep(1, 1), "a's representative in outer")
	assert.Equal(t, node(1), tr.Rep(2, 1))
	assert.Equal(t, []int{1, 2}, tr.Path(0, 1), "x->a crosses outer, then inner")
	assert.Empty(t, tr.Path(2, 1))
}

func TestBuild_Errors(t *testing.T) {
	for _, tt := range []struct {
		name   string
		groups []model.Group
		want   string
	}{
		{"a node in two groups", []model.Group{{ID: "p", Contains: []string{"a"}}, {ID: "q", Contains: []string{"a"}}}, "node a is in two groups"},
		{"a group in two groups", []model.Group{{ID: "p", Children: []string{"r"}}, {ID: "q", Children: []string{"r"}}, {ID: "r"}}, "group r is in two groups"},
		{"a group holding itself", []model.Group{{ID: "p", Children: []string{"q"}}, {ID: "q", Children: []string{"p"}}}, "holds itself"},
		{"an unknown member", []model.Group{{ID: "p", Contains: []string{"zz"}}}, "contains zz, not a node"},
		{"an unknown child", []model.Group{{ID: "p", Children: []string{"zz"}}}, "holds zz, not a group"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Build(graph([]string{"a", "b"}, []string{"a->b"}, tt.groups...))
			assert.ErrorContains(t, err, tt.want)
		})
	}
	_, err := Build(graph([]string{"a"}, []string{"a->zz"}))
	assert.ErrorContains(t, err, "not both nodes")
}
