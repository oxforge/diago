package ports

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/layout/layered/cycle"
	"github.com/oxforge/diago/internal/layout/layered/lgraph"
	"github.com/oxforge/diago/internal/layout/layered/lgraph/lgraphtest"
	"github.com/oxforge/diago/internal/layout/layered/rank"
)

// graph duplicates order_test's helper because it cannot move into lgraphtest (cycle and rank's internal tests import lgraphtest: an import cycle).
func graph(t *testing.T, specs ...string) *lgraph.Graph {
	t.Helper()
	lv := lgraphtest.Level(t, specs...)
	reversed := cycle.Break(context.Background(), lv, nil)
	g, err := lgraph.Build(lv, rank.Assign(context.Background(), lv, reversed, nil), reversed, 8)
	require.NoError(t, err)
	return g
}

func TestRanks_OutFaceFollowsTheNextLayer(t *testing.T) {
	g := graph(t, "p->a", "p->b")
	g.Layers[1] = []int{2, 1} // b left of a
	r := Ranks(g, nil)
	assert.Equal(t, Rank{At: 1, Of: 2}, r[0][Head], "p->a lands right")
	assert.Equal(t, Rank{At: 0, Of: 2}, r[1][Head], "p->b lands left")
	assert.Equal(t, Rank{At: 0, Of: 1}, r[0][Tail])
}

func TestRanks_InFaceFollowsThePreviousLayer(t *testing.T) {
	g := graph(t, "a->t", "b->t")
	g.Layers[0] = []int{2, 0} // b left of a
	r := Ranks(g, nil)
	assert.Equal(t, Rank{At: 1, Of: 2}, r[0][Tail])
	assert.Equal(t, Rank{At: 0, Of: 2}, r[1][Tail])
}

func TestRanks_TiesGoByEdgeOrder(t *testing.T) {
	g := graph(t, "p->a", "p->a")
	r := Ranks(g, nil)
	assert.Equal(t, Rank{At: 0, Of: 2}, r[0][Head])
	assert.Equal(t, Rank{At: 1, Of: 2}, r[1][Head])
}

func TestRanks_ALongEdgeRanksByItsDummy(t *testing.T) {
	g := graph(t, "a->b", "b->c", "a->c")
	r := Ranks(g, nil)
	assert.Equal(t, Rank{At: 0, Of: 2}, r[0][Head], "b sits left of the dummy")
	assert.Equal(t, Rank{At: 1, Of: 2}, r[2][Head])
	assert.Equal(t, Rank{At: 1, Of: 2}, r[2][Tail], "the dummy sits right of b above c")
}

func TestRanks_SideEndsTakeNoSlot(t *testing.T) {
	g := graph(t, "p->a", "p->b", "p->c")
	side := make([][2]bool, len(g.Level.Edges))
	side[1][Head] = true
	r := Ranks(g, side)
	assert.Equal(t, Rank{}, r[1][Head])
	assert.Equal(t, Rank{At: 0, Of: 2}, r[0][Head])
	assert.Equal(t, Rank{At: 1, Of: 2}, r[2][Head])
}

func TestRanks_SelfLoopsHaveNoSlot(t *testing.T) {
	g := graph(t, "a->a", "a->b")
	r := Ranks(g, nil)
	assert.Equal(t, [2]Rank{}, r[0])
	assert.Equal(t, Rank{At: 0, Of: 1}, r[1][Head])
}

func TestRank_Offset(t *testing.T) {
	s := Span{Lo: -60, Hi: 60}
	assert.Equal(t, 0.0, Rank{}.Offset(s))
	assert.Equal(t, -20.0, Rank{At: 0, Of: 2}.Offset(s))
	assert.Equal(t, 20.0, Rank{At: 1, Of: 2}.Offset(s))
}

// TestRanks_AnchoredEndTakesItsAnchor pins S8: an end anchored on a group's
// face takes its anchor and no slot, and the face's other ends spread
// without it.
func TestRanks_AnchoredEndTakesItsAnchor(t *testing.T) {
	g := graph(t, "p->a", "p->b")
	g.Level.Edges[0].Anchor[0] = lgraph.Anchor{On: true, At: -30}
	r := Ranks(g, nil)
	assert.Equal(t, Rank{Anchored: true, Fixed: -30}, r[0][Head])
	assert.Equal(t, Rank{At: 0, Of: 1}, r[1][Head], "p->b alone on the spread")
	assert.Equal(t, -30.0, r[0][Head].Offset(Span{Lo: -40, Hi: 40}))
}

func TestLoopOut(t *testing.T) {
	// three loops nest, the first outermost, each InLaneGap inside the one
	// before it; in the text profile every one runs half a cell farther
	// out, on a cell's middle, as a side column (S8, Self-loops)
	for j, want := range []float64{36, 28, 20} {
		assert.Equal(t, want, LoopOut(20, 8, j, 3, false), "screen, loop %d", j)
	}
	for j, want := range []float64{7.5, 5.5, 3.5} {
		assert.Equal(t, want, LoopOut(3, 2, j, 3, true), "text, loop %d", j)
	}
	assert.Equal(t, Reach(2, true), LoopOut(2, 1, 0, 1, true), "a lone loop runs where a side column does")
}
