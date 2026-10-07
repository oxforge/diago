package lgraph_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/layout/layered/lgraph"
	"github.com/oxforge/diago/internal/layout/layered/lgraph/lgraphtest"
)

func TestBuild_Chains(t *testing.T) {
	lv := lgraphtest.Level(t, "a->b", "b->c", "a->c", "c->c")
	g, err := lgraph.Build(lv, []int{0, 1, 2}, nil, 8)
	require.NoError(t, err)

	require.Len(t, g.Vertices, 4)
	assert.Equal(t, lgraph.Vertex{ID: "a->c#0@1", Node: -1, Edge: 2, Layer: 1, W: 8}, g.Vertices[3])
	assert.Equal(t, lgraph.Vertex{ID: "b", Node: 1, Edge: -1, Layer: 1, W: 80}, g.Vertices[1])
	assert.Equal(t, [][]int{{0}, {1, 3}, {2}}, g.Layers, "nodes first, then dummies")
	assert.Equal(t, [][]int{{0, 1}, {1, 2}, {0, 3, 2}, nil}, g.Chains, "a self-loop has no chain")
	assert.True(t, g.Dummy(3))
	assert.False(t, g.Dummy(2))
	assert.Equal(t, []int{0, 0, 0, 1}, g.Index())
	assert.Equal(t, []lgraph.Hop{
		{Edge: 0, Upper: 0, Lower: 1}, {Edge: 1, Upper: 1, Lower: 2},
		{Edge: 2, Upper: 0, Lower: 3}, {Edge: 2, Upper: 3, Lower: 2},
	}, g.Hops())
}

func TestBuild_ReversedChainRunsUpstreamToDownstream(t *testing.T) {
	lv := lgraphtest.Level(t, "a->b", "b->a")
	g, err := lgraph.Build(lv, []int{0, 1}, []bool{false, true}, 8)
	require.NoError(t, err)
	assert.Equal(t, [][]int{{0, 1}, {0, 1}}, g.Chains)
}

func TestBuild_RejectsAnEdgeThatDoesNotDescend(t *testing.T) {
	lv := lgraphtest.Level(t, "a->b")
	_, err := lgraph.Build(lv, []int{1, 1}, nil, 8)
	assert.ErrorContains(t, err, "a->b#0")
	_, err = lgraph.Build(lv, []int{0}, nil, 8)
	assert.Error(t, err, "one layer per node")
}

func TestCrossings(t *testing.T) {
	lv := lgraphtest.Level(t, "a", "b", "c", "d", "a->d", "b->c")
	g, err := lgraph.Build(lv, []int{0, 0, 1, 1}, nil, 8)
	require.NoError(t, err)
	assert.Equal(t, [][]int{{0, 1}, {2, 3}}, g.Layers)
	assert.Equal(t, 1, g.Crossings(), "a->d and b->c cross under the seed")
	g.Layers[1] = []int{3, 2}
	assert.Equal(t, 0, g.Crossings())
}

func TestInversions(t *testing.T) {
	assert.Equal(t, 1, lgraph.Inversions([][2]int{{0, 1}, {1, 0}}))
	assert.Equal(t, 0, lgraph.Inversions([][2]int{{0, 0}, {0, 1}}), "hops sharing an end never cross")
	assert.Equal(t, 3, lgraph.Inversions([][2]int{{0, 2}, {1, 1}, {2, 0}}))
}

func TestLevel_EndFollowsTheReversal(t *testing.T) {
	lv := lgraphtest.Level(t, "a->g")
	lv.Edges[0].Anchor[1] = lgraph.Anchor{On: true, At: 12}
	assert.Equal(t, lgraph.Anchor{On: true, At: 12}, lv.End(0, 1, nil), "the To end is the tail")
	assert.Equal(t, lgraph.Anchor{}, lv.End(0, 0, nil))
	assert.Equal(t, lgraph.Anchor{On: true, At: 12}, lv.End(0, 0, []bool{true}), "reversed, the To end is the head")
}

// TestCrossings_CountsAnchoredEnds pins S6: two hops that leave one group
// cross when their anchors on its face oppose their order below.
func TestCrossings_CountsAnchoredEnds(t *testing.T) {
	lv := lgraphtest.Level(t, "g", "x", "y", "g->x", "g->y")
	lv.Nodes[0].Group, lv.Nodes[0].W = true, 200
	lv.Edges[0].Anchor[0] = lgraph.Anchor{On: true, At: 50}  // g->x leaves right
	lv.Edges[1].Anchor[0] = lgraph.Anchor{On: true, At: -50} // g->y leaves left
	g, err := lgraph.Build(lv, []int{0, 1, 1}, nil, 8)
	require.NoError(t, err)
	assert.Equal(t, 0.25, g.Shift(0, 0), "the anchor over the group's width")
	assert.Equal(t, 0.0, g.Shift(0, 1), "an end that is not anchored")
	assert.Equal(t, 1, g.Crossings(), "x left of y under the seed")
	g.Layers[1] = []int{2, 1}
	assert.Equal(t, 0, g.Crossings())
}

func TestInversions_OfPositions(t *testing.T) {
	assert.Equal(t, 1, lgraph.Inversions([][2]float64{{0.25, 0}, {-0.25, 1}}))
	assert.Equal(t, 0, lgraph.Inversions([][2]float64{{-0.25, 0}, {0.25, 1}}))
}

// TestCounterFlow pins which ends S6 counts at a side (S6, Side-aware
// crossings; S8): a reversed edge's end on a node, at either end of its
// chain, but not a dummy, not a forward edge's end, and not a group's or
// a terminal's end, which stays on the face.
func TestCounterFlow(t *testing.T) {
	lv := lgraphtest.Level(t, "a->b", "b->c", "c->a", "g->c", "c->g", "t->a", "a->t")
	lv.Nodes[3].Group, lv.Nodes[4].Terminal = true, true // g, t
	g, err := lgraph.Build(lv, []int{1, 2, 3, 0, 0}, []bool{false, false, true, false, true, false, true}, 8)
	require.NoError(t, err)
	ca, cg, at := g.Chains[2], g.Chains[4], g.Chains[6]
	require.Len(t, ca, 3, "c->a runs from a through a dummy to c")
	assert.True(t, g.CounterFlow(2, ca[0]), "c->a's end on a")
	assert.True(t, g.CounterFlow(2, ca[2]), "c->a's end on c")
	assert.False(t, g.CounterFlow(2, ca[1]), "a dummy")
	assert.False(t, g.CounterFlow(0, g.Chains[0][0]), "a forward edge's end")
	assert.False(t, g.CounterFlow(4, cg[0]), "a group's end")
	assert.True(t, g.CounterFlow(4, cg[len(cg)-1]), "c->g's end on c")
	assert.False(t, g.CounterFlow(6, at[0]), "a terminal's end")
	assert.True(t, g.CounterFlow(6, at[1]), "a->t's end on a")
	assert.False(t, g.CounterFlow(2, 1), "b is no end of c->a")
}
