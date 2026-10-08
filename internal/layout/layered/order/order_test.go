package order

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/layout/layered/cycle"
	"github.com/oxforge/diago/internal/layout/layered/lgraph"
	"github.com/oxforge/diago/internal/layout/layered/lgraph/lgraphtest"
	"github.com/oxforge/diago/internal/layout/layered/rank"
	"github.com/oxforge/diago/internal/layoutdbg"
)

// graph cannot move into lgraphtest: cycle and rank's own internal tests
// import lgraphtest, and lgraphtest importing cycle and rank back would be
// an import cycle.
func graph(t *testing.T, specs ...string) *lgraph.Graph {
	t.Helper()
	lv := lgraphtest.Level(t, specs...)
	reversed := cycle.Break(context.Background(), lv, nil)
	layers := rank.Assign(context.Background(), lv, reversed, nil)
	g, err := lgraph.Build(lv, layers, reversed, 8)
	require.NoError(t, err)
	return g
}

func ids(g *lgraph.Graph) [][]string {
	out := make([][]string, len(g.Layers))
	for l, layer := range g.Layers {
		for _, v := range layer {
			out[l] = append(out[l], g.Vertices[v].ID)
		}
	}
	return out
}

func TestMinimize_UntanglesACrossing(t *testing.T) {
	g := graph(t, "a", "b", "c", "d", "a->d", "b->c")
	require.Equal(t, 1, g.Crossings())
	g.Layers = Minimize(context.Background(), g, false)
	assert.Equal(t, [][]string{{"a", "b"}, {"d", "c"}}, ids(g))
	assert.Equal(t, 0, g.Crossings())
}

func TestMinimize_KeepsTheSeedAmongEqualCrossings(t *testing.T) {
	// Every ordering of this K2,2 has one crossing. The seed (declaration
	// order: b before a, d before c) is nearest to itself and wins; a
	// lexicographic tie-break would have reordered it to a b / c d.
	g := graph(t, "b", "a", "d", "c", "b->d", "b->c", "a->d", "a->c")
	g.Layers = Minimize(context.Background(), g, false)
	assert.Equal(t, [][]string{{"b", "a"}, {"d", "c"}}, ids(g))
	assert.Equal(t, 1, g.Crossings())
}

func TestMinimize_PrefersTheSeedOverALaterEqualOrdering(t *testing.T) {
	// The K2,2 keeps one crossing in every ordering. The first upward sweep
	// moves the lone node i, which targets its own index 1, past b, whose
	// children's median is 0.5: a b i ties the seed at one crossing but
	// orders one pair opposite to it, so the seed stays.
	g := graph(t, "a", "i", "b", "a->c", "a->d", "b->c", "b->d")
	g.Layers = Minimize(context.Background(), g, false)
	assert.Equal(t, [][]string{{"a", "i", "b"}, {"c", "d"}}, ids(g))
	assert.Equal(t, 1, g.Crossings())
}

func TestMinimize_TakesTheEarliestAmongEqualDistances(t *testing.T) {
	// The first sweep reaches one crossing at distance 2 from the seed. The
	// third, fifth and seventh reach another one-crossing ordering (n2
	// before n1 on top), also at distance 2, and the earliest stays.
	g := graph(t, "n0", "n1", "n2", "n3", "n4", "n1->n4", "n2->n4", "n4->n0", "n4->n3", "n2->n0", "n1->n0")
	g.Layers = Minimize(context.Background(), g, false)
	assert.Equal(t, [][]string{{"n1", "n2"}, {"n1->n0#0@1", "n4", "n2->n0#0@1"}, {"n0", "n3"}}, ids(g))
	assert.Equal(t, 1, g.Crossings())
}

func TestMedian_TiesFollowTheSeedRank(t *testing.T) {
	// c and d both target a's index 0; the layer arrives as d c, and the
	// tie goes to the seed, which has c first.
	g := graph(t, "a->c", "a->d")
	seed := g.Index()
	g.Layers[1][0], g.Layers[1][1] = g.Layers[1][1], g.Layers[1][0]
	require.Equal(t, [][]string{{"a"}, {"d", "c"}}, ids(g))
	s := newSweeper(g)
	s.median(1, s.up, seed)
	assert.Equal(t, [][]string{{"a"}, {"c", "d"}}, ids(g))
}

func TestTranspose_KeepsAPairWhoseSwapGainsNothing(t *testing.T) {
	// Hops that share an end never cross, so every order of r's children
	// has no crossing: no swap strictly reduces anything, and nothing moves.
	g := graph(t, "r->x", "r->y", "r->z")
	s := newSweeper(g)
	s.transpose()
	assert.Equal(t, [][]string{{"r"}, {"x", "y", "z"}}, ids(g))
}

func TestMinimize_LeavesACrossingFreeSeed(t *testing.T) {
	g := graph(t, "r->c", "r->a", "r->b", "a->x")
	before := ids(g)
	g.Layers = Minimize(context.Background(), g, false)
	assert.Equal(t, before, ids(g))
}

func TestMinimize_IsPure(t *testing.T) {
	g := graph(t, "a", "b", "c", "d", "a->d", "b->c")
	before := ids(g)
	result := Minimize(context.Background(), g, false)
	assert.Equal(t, before, ids(g), "Minimize must not modify its argument")
	g.Layers = result
	assert.Equal(t, [][]string{{"a", "b"}, {"d", "c"}}, ids(g))
}

func TestTranspose_TriesTheOnlyPairOfATwoVertexLayer(t *testing.T) {
	g := graph(t, "a", "b", "c", "d", "a->d", "b->c")
	s := newSweeper(g)
	s.transpose()
	assert.Equal(t, [][]string{{"b", "a"}, {"c", "d"}}, ids(g))
	assert.Equal(t, 0, g.Crossings())
}

func TestMedian(t *testing.T) {
	tests := []struct {
		positions []int
		want      float64
	}{
		{[]int{3}, 3},
		{[]int{1, 5}, 3},
		{[]int{0, 2, 9}, 2},
		{[]int{0, 1, 5, 6}, 3},
		{[]int{0, 4, 5, 6}, 4.8},
		{[]int{2, 2, 2, 2}, 2},
	}
	for _, tt := range tests {
		assert.InDelta(t, tt.want, median(tt.positions), 1e-12, "%v", tt.positions)
	}
}

func TestDistance(t *testing.T) {
	seed := []int{0, 1, 2}
	assert.Equal(t, 0, distance([][]int{{0, 1, 2}}, seed))
	assert.Equal(t, 1, distance([][]int{{1, 0, 2}}, seed))
	assert.Equal(t, 3, distance([][]int{{2, 1, 0}}, seed))
}

func TestMinimize_NeverWorseThanTheSeed(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 5))
	for trial := range 150 {
		n := 3 + rng.IntN(14)
		specs := make([]string, 0, 3*n)
		for i := range n {
			specs = append(specs, fmt.Sprintf("n%d", i))
		}
		for range n + rng.IntN(2*n) {
			specs = append(specs, fmt.Sprintf("n%d->n%d", rng.IntN(n), rng.IntN(n)))
		}
		g := graph(t, specs...)
		seedLayers := make([][]int, len(g.Layers))
		for l, layer := range g.Layers {
			seedLayers[l] = slices.Sorted(slices.Values(layer))
		}
		before := g.Crossings()
		g.Layers = Minimize(context.Background(), g, false)
		assert.LessOrEqual(t, g.Crossings(), before, "trial %d", trial)
		for l, layer := range g.Layers {
			assert.Equal(t, seedLayers[l], slices.Sorted(slices.Values(layer)), "trial %d layer %d is a permutation", trial, l)
		}
	}
}

// build lays specs out on the given layers, marks the ids in terminals as
// terminals and anchors the given heads, each an edge index and its
// anchor, on a node 200 wide.
func build(t *testing.T, layers []int, terminals []string, heads map[int]float64, specs ...string) *lgraph.Graph {
	t.Helper()
	lv := lgraphtest.Level(t, specs...)
	for i, n := range lv.Nodes {
		lv.Nodes[i].Terminal = slices.Contains(terminals, n.ID)
	}
	for e, at := range heads {
		lv.Nodes[lv.Edges[e].From].W = 200
		lv.Edges[e].Anchor[0] = lgraph.Anchor{On: true, At: at}
	}
	g, err := lgraph.Build(lv, layers, nil, 8)
	require.NoError(t, err)
	return g
}

// TestMinimize_KeepsTheBands pins S6's bands (S9): a terminal row in
// bands keeps their order, and the other rows adapt to it.
func TestMinimize_KeepsTheBands(t *testing.T) {
	specs := []string{"x", "y", "tx", "ty", "x->ty", "y->tx"}
	g := build(t, []int{0, 0, 1, 1}, []string{"tx", "ty"}, nil, specs...)
	g.Layers = Minimize(context.Background(), g, false)
	assert.Equal(t, [][]string{{"x", "y"}, {"ty", "tx"}}, ids(g), "free, the terminal row moves")

	g = build(t, []int{0, 0, 1, 1}, []string{"tx", "ty"}, nil, specs...)
	g.Bands = []int{-1, -1, 0, 1}
	g.Layers = Minimize(context.Background(), g, false)
	assert.Equal(t, [][]string{{"y", "x"}, {"tx", "ty"}}, ids(g), "in two bands it stays, and x and y swap")

	g = build(t, []int{0, 0, 1, 1}, []string{"tx", "ty"}, nil, specs...)
	g.Bands = []int{-1, -1, 0, 0}
	g.Layers = Minimize(context.Background(), g, false)
	assert.Equal(t, [][]string{{"x", "y"}, {"ty", "tx"}}, ids(g), "within one band it moves")
}

// TestMinimize_CountsAnchoredEnds pins S6: two ends anchored on one
// group's face cross when their anchors oppose the order below them.
func TestMinimize_CountsAnchoredEnds(t *testing.T) {
	g := build(t, []int{0, 1, 1}, nil, map[int]float64{0: -50, 1: 50}, "g", "b", "a", "g->a", "g->b")
	require.Equal(t, 1, g.Crossings(), "g->a leaves left of g->b, a sits right of b")
	g.Layers = Minimize(context.Background(), g, false)
	assert.Equal(t, [][]string{{"g"}, {"a", "b"}}, ids(g))
}

// TestMinimize_TriesUpwardFirst pins S6's second run: two swaps untangle
// this ordering together, neither alone, and a vertex without neighbors
// above holds its place in the downward sweep; the run that starts
// upward finds them.
func TestMinimize_TriesUpwardFirst(t *testing.T) {
	g := build(t, []int{0, 1, 1, 2, 2}, nil, map[int]float64{1: 50, 2: -20},
		"u", "rt", "ix", "db", "web", "u->rt", "rt->web", "rt->db", "ix->db")
	require.Equal(t, 1, g.Crossings())
	g.Layers = Minimize(context.Background(), g, false)
	assert.Equal(t, [][]string{{"u"}, {"ix", "rt"}, {"db", "web"}}, ids(g))
	assert.Equal(t, 0, g.Crossings())
}

// TestMinimize_TheUpwardRunMustBeatTheFirst pins S6's second run (S6,
// *Algorithm*): the run that starts upward replaces the first run's best
// only with fewer crossings. On a tie the first run's best stays, even
// when the upward one lies as near the seed or nearer, which the tie-break
// within a run would prefer.
func TestMinimize_TheUpwardRunMustBeatTheFirst(t *testing.T) {
	for _, tc := range []struct {
		name  string
		specs []string
	}{
		// a K2,2 with a doubled a->d: the first run swaps the bottom
		// layer, the upward one the top, one crossing each, both one
		// swap from the seed
		{"as near the seed", []string{"a", "b", "c", "d", "a->c", "b->d", "b->c", "a->d", "a->d"}},
		// the first run's best lies three swaps from the seed, the upward
		// run's two
		{"the upward run nearer the seed", []string{"n0", "n1", "n2", "n3",
			"n0->n4", "n1->n3", "n0->n3", "n0->n2", "n5->n1", "n1->n4"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := graph(t, tc.specs...)
			down, c1, d1, _ := minimize(g, Sweeps, false)
			up, c2, d2, _ := minimize(g, Sweeps, true)
			require.Positive(t, c1, "the first run leaves a crossing")
			require.Equal(t, c1, c2, "the upward run ties it")
			require.NotEqual(t, down, up, "with another ordering")
			require.LessOrEqual(t, d2, d1, "as near the seed or nearer")
			best, crossings, _ := run(g, Sweeps)
			assert.Equal(t, down, best, "the first run's best stays")
			assert.Equal(t, c1, crossings)
		})
	}
}

func TestMinimize_RestartsFromItsBest(t *testing.T) {
	// The first run's 8 sweeps from the declaration seed end at one
	// crossing; a run of 2 sweeps from that best finds none, so S6 takes
	// it and runs again from there. The graph is the random-graph probe's
	// that found the case, n0->n5 declared twice as the probe draws a
	// parallel edge.
	g := graph(t, "n0", "n1", "n2", "n3", "n4", "n5", "n6",
		"n3->n0", "n6->n0", "n2->n0", "n0->n5", "n3->n1", "n0->n5", "n2->n5")
	first, crossings, _ := run(g, Sweeps)
	require.Equal(t, 1, crossings, "the fixture must leave the first run a crossing")
	g.Layers = Minimize(context.Background(), g, false)
	assert.Equal(t, 0, g.Crossings())
	assert.NotEqual(t, first, g.Layers)
}

func TestMinimize_IsItsOwnAnchoredOrdering(t *testing.T) {
	// S6 and C18.1: seeded from its own result, as a level anchored on a
	// previous layout is, Minimize returns that result unchanged.
	rng := rand.New(rand.NewPCG(5, 17))
	for trial := range 300 {
		n := 3 + rng.IntN(10)
		specs := make([]string, 0, 3*n)
		for i := range n {
			specs = append(specs, fmt.Sprintf("n%d", i))
		}
		for range n + rng.IntN(2*n) {
			specs = append(specs, fmt.Sprintf("n%d->n%d", rng.IntN(n), rng.IntN(n)))
		}
		g := graph(t, specs...)
		g.Layers = Minimize(context.Background(), g, false)
		want := slices.Clone(g.Layers)
		for l := range want {
			want[l] = slices.Clone(want[l])
		}
		assert.Equal(t, want, Minimize(context.Background(), g, true), "trial %d: %v", trial, specs)
	}
}

// ordered runs order on a fresh graph of specs and returns the graph in
// its order and the order phase's decisions by name, the last of each.
func ordered(t *testing.T, order func(context.Context, *lgraph.Graph) [][]int, specs ...string) (*lgraph.Graph, map[string]map[string]any) {
	t.Helper()
	g := graph(t, specs...)
	var buf bytes.Buffer
	ctx := layoutdbg.NewContext(context.Background(), slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	g.Layers = order(ctx, g)
	got := map[string]map[string]any{}
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		if entry["phase"] == "order" {
			got[entry["decision"].(string)] = entry
		}
	}
	return g, got
}

// TestMinimize_StopsAtItsReplacementCap pins S6's safeguard on its
// replacements, with a cap of one. This graph's best is replaced twice:
// first by the loop transposition, which takes the return dummy e->a out
// from between d and f, then by a run of 2 sweeps from that ordering,
// which crosses nothing. Capped after the first, S6 keeps its ordering,
// with one crossing, and records order_capped; Minimize takes both and
// records no cap.
func TestMinimize_StopsAtItsReplacementCap(t *testing.T) {
	specs := []string{"a", "b", "c", "d", "e", "f", "c->f", "c->d", "e->a", "e->d", "f->c", "d->a", "b->d"}
	g, got := ordered(t, func(ctx context.Context, g *lgraph.Graph) [][]int { return Minimize(ctx, g, false) }, specs...)
	chosen := got["order_chosen"]
	require.Equal(t, []any{1.0, 0.0, 1.0}, []any{chosen["restarts"], chosen["side_moves"], chosen["loop_moves"]}, "two replacements")
	assert.Zero(t, g.Crossings())
	assert.NotContains(t, got, "order_capped")

	g, got = ordered(t, func(ctx context.Context, g *lgraph.Graph) [][]int { return settle(ctx, g, false, 1) }, specs...)
	assert.Equal(t, [][]string{{"b", "e", "c"}, {"e->a#0@1", "d", "f"}, {"a"}}, ids(g), "the loop transposition's ordering")
	assert.Equal(t, 1, g.Crossings())
	assert.Zero(t, newSweeper(g).enclosed())
	require.Contains(t, got, "order_capped")
	assert.Equal(t, 1.0, got["order_capped"]["replacements"])
	assert.Equal(t, 1.0, got["order_capped"]["crossings"])
	assert.Equal(t, "S6", got["order_capped"]["spec_ref"])
}

func TestSeed_SortsTheSeededVerticesInTheirOwnSlots(t *testing.T) {
	g := graph(t, "r", "a", "b", "c", "d", "r->a", "r->b", "r->c", "r->d")
	prev := []int{0, -1, 7, -1, 2}
	assert.Equal(t, 3, Seed(g, prev), "r, b and d")
	// b and d hold slots 1 and 3; d's index 2 comes before b's 7. a and c
	// keep theirs.
	assert.Equal(t, [][]string{{"r"}, {"a", "d", "c", "b"}}, ids(g))
}

func TestSeed_KeepsABandsVerticesInIt(t *testing.T) {
	g := graph(t, "a", "b", "c", "d", "x", "a->x", "b->x", "c->x", "d->x")
	g.Bands = []int{0, 0, 1, 1, -1}
	// The previous order reverses the row, but a band keeps its slots: a
	// and b trade places, and so do c and d.
	Seed(g, []int{3, 2, 1, 0, 0})
	assert.Equal(t, [][]string{{"b", "a", "d", "c"}, {"x"}}, ids(g))
}

func TestSeed_TiesKeepTheCurrentOrder(t *testing.T) {
	g := graph(t, "r", "a", "b", "c", "r->a", "r->b", "r->c")
	Seed(g, []int{-1, 1, 1, 0})
	assert.Equal(t, [][]string{{"r"}, {"c", "a", "b"}}, ids(g))
}

// TestMinimize_InterleavedChainsTakeOppositeSides pins S6's counter-flow
// sides: the back edges d->a (layers 0 to 3) and e->b (1 to 4) interleave.
// The sweeps leave both chains right of the main column, crossing once
// between layers 2 and 3, and no swap of neighbors moves a whole chain
// across it. Of two equal spans the later chain, e->b, moves to the left
// ends of its layers, and the crossing goes. Seeded from the result, as
// a level anchored on it is, S6 keeps it (C18.1).
func TestMinimize_InterleavedChainsTakeOppositeSides(t *testing.T) {
	g := graph(t, "a->b", "b->c", "c->d", "d->e", "d->a", "e->b")
	g.Layers = Minimize(context.Background(), g, false)
	assert.Equal(t, [][]string{
		{"a"},
		{"b", "d->a#0@1"},
		{"e->b#0@2", "c", "d->a#0@2"},
		{"e->b#0@3", "d"},
		{"e"},
	}, ids(g))
	assert.Equal(t, 0, g.Crossings())
	assert.Equal(t, g.Layers, Minimize(context.Background(), g, true))
}

// TestMinimize_TheShorterInterleavedChainMoves pins the counter-flow
// sides' tie-break: of d->a (layers 0 to 3) and f->b (1 to 5), the shorter
// d->a moves, whichever of the two is declared first.
func TestMinimize_TheShorterInterleavedChainMoves(t *testing.T) {
	want := [][]string{
		{"a"},
		{"d->a#0@1", "b"},
		{"d->a#0@2", "c", "f->b#0@2"},
		{"d", "f->b#0@3"},
		{"e", "f->b#0@4"},
		{"f"},
	}
	for _, backs := range [][]string{{"d->a", "f->b"}, {"f->b", "d->a"}} {
		g := graph(t, append([]string{"a->b", "b->c", "c->d", "d->e", "e->f"}, backs...)...)
		g.Layers = Minimize(context.Background(), g, false)
		assert.Equal(t, want, ids(g), "%v", backs)
		assert.Equal(t, 0, g.Crossings(), "%v", backs)
	}
}

// setLayers sets g's order from each layer's vertex ids.
func setLayers(t *testing.T, g *lgraph.Graph, rows [][]string) {
	t.Helper()
	require.Len(t, rows, len(g.Layers))
	for l, row := range rows {
		require.Len(t, row, len(g.Layers[l]), "layer %d", l)
		for i, id := range row {
			v := slices.IndexFunc(g.Vertices, func(x lgraph.Vertex) bool { return x.ID == id })
			require.Equal(t, l, g.Vertices[v].Layer, "%s", id)
			g.Layers[l][i] = v
		}
	}
}

// moved returns the crossings a run of AnchoredSweeps leaves once edge
// id's dummies move to the left ends of their layers, or to the right ends
// when right: what a counter-flow side move of that chain reaches from g's
// order.
func moved(t *testing.T, g *lgraph.Graph, id string, right bool) int {
	t.Helper()
	e := slices.IndexFunc(g.Level.Edges, func(ed lgraph.Edge) bool { return ed.ID == id })
	require.GreaterOrEqual(t, e, 0, "no edge %s", id)
	work := *g
	work.Layers = moveChain(g, e, right)
	_, c, _ := run(&work, AnchoredSweeps)
	return c
}

// TestMinimize_NestedChainsKeepTheirSides pins that the counter-flow sides
// move interleaved chains only. x->a (layers 0 to 3) nests in e->a (0 to
// 4), both leaving a: the sweeps leave them one crossing, between x->a's
// last dummy and x, so the side moves run, and moving x->a to the left
// ends would untangle the pair. But nested spans keep the sides the sweeps
// gave them, and the crossing stays. (The loop transposition then moves
// it up a layer: x's upper neighbors c and x->a's last dummy enclosed
// e->a's dummy, and swapping the two dummies frees it at equal
// crossings.)
func TestMinimize_NestedChainsKeepTheirSides(t *testing.T) {
	g := graph(t, "a->b", "b->c", "c->d", "d->e", "c->x", "e->a", "x->a")
	g.Layers = Minimize(context.Background(), g, false)
	assert.Equal(t, [][]string{
		{"a"},
		{"b", "e->a#0@1", "x->a#0@1"},
		{"c", "x->a#0@2", "e->a#0@2"},
		{"d", "x", "e->a#0@3"},
		{"e"},
	}, ids(g))
	assert.Equal(t, 1, g.Crossings(), "a crossing is left, so the side moves ran")
	assert.Zero(t, moved(t, g, "x->a#0", false), "a side move would untangle the pair")
}

// TestMinimize_ForwardChainsKeepTheirSides pins that the counter-flow sides
// move reversed chains only. Each pair's spans interleave, 0 to 3 and 1 to
// 4, as d->a's and e->b's do, and the sweeps leave its chains one crossing;
// moving the later chain to the left ends would untangle them. But a->d
// and b->e both run forward, and d->a beside b->e is one reversed chain,
// not two, so each pair keeps its sides and its crossing. (With b->e
// declared first, the loop transposition then swaps the two dummies of
// layer 2, which moves the crossing up a layer: d's upper neighbors c and
// d->a's last dummy enclosed b->e's.)
func TestMinimize_ForwardChainsKeepTheirSides(t *testing.T) {
	for _, f := range []struct {
		backs []string
		mover string
		want  [][]string
	}{
		{[]string{"a->d", "b->e"}, "b->e#0", [][]string{
			{"a"},
			{"b", "a->d#0@1"},
			{"c", "a->d#0@2", "b->e#0@2"},
			{"d", "b->e#0@3"},
			{"e"},
		}},
		{[]string{"b->e", "d->a"}, "d->a#0", [][]string{
			{"a"},
			{"b", "d->a#0@1"},
			{"c", "d->a#0@2", "b->e#0@2"},
			{"d", "b->e#0@3"},
			{"e"},
		}},
		{[]string{"d->a", "b->e"}, "b->e#0", [][]string{
			{"a"},
			{"b", "d->a#0@1"},
			{"c", "d->a#0@2", "b->e#0@2"},
			{"d", "b->e#0@3"},
			{"e"},
		}},
	} {
		g := graph(t, append([]string{"a->b", "b->c", "c->d", "d->e"}, f.backs...)...)
		g.Layers = Minimize(context.Background(), g, false)
		assert.Equal(t, f.want, ids(g), "%v", f.backs)
		assert.Equal(t, 1, g.Crossings(), "%v: a crossing is left, so the side moves ran", f.backs)
		assert.Zero(t, moved(t, g, f.mover, false), "%v: a side move would untangle the pair", f.backs)
	}
}

// TestMinimize_AChainCrossingTwoKeepsItsSide pins that the counter-flow
// sides move a pair only when its hops cross each other and no other hop.
// g->b (layers 1 to 6) interleaves with d->a (0 to 3) and with h->e (4 to
// 7), and the sweeps leave its hops crossing both. d->a's hops cross
// g->b's and no other, and so do h->e's, but g->b's cross two chains', so
// neither pair moves and both crossings stay, although moving d->a to the
// left ends would take one away.
func TestMinimize_AChainCrossingTwoKeepsItsSide(t *testing.T) {
	g := graph(t, "a->b", "b->c", "c->d", "d->e", "e->f", "f->g", "g->h", "d->a", "h->e", "g->b")
	g.Layers = Minimize(context.Background(), g, false)
	assert.Equal(t, [][]string{
		{"a"},
		{"b", "d->a#0@1"},
		{"c", "d->a#0@2", "g->b#0@2"},
		{"d", "g->b#0@3"},
		{"e", "g->b#0@4"},
		{"f", "h->e#0@5", "g->b#0@5"},
		{"g", "h->e#0@6"},
		{"h"},
	}, ids(g))
	assert.Equal(t, 2, g.Crossings(), "crossings are left, so the side moves ran")
	assert.Less(t, moved(t, g, "d->a#0", false), 2, "a side move would take a crossing away")
}

// TestMinimize_TheRightEndsWhenTheLeftEndsFail pins the counter-flow sides'
// fallback. Seeded with the interleaved d->a and e->b both left of the
// main column, the sweeps keep the seed and its crossing. e->b, the later
// of two equal spans, moves: at the left ends of its layers it still
// crosses d->a, and at the right ends it crosses nothing, so it runs
// right of the column and d->a left.
func TestMinimize_TheRightEndsWhenTheLeftEndsFail(t *testing.T) {
	g := graph(t, "a->b", "b->c", "c->d", "d->e", "d->a", "e->b")
	setLayers(t, g, [][]string{
		{"a"},
		{"d->a#0@1", "b"},
		{"d->a#0@2", "e->b#0@2", "c"},
		{"e->b#0@3", "d"},
		{"e"},
	})
	first, crossings, _ := run(g, Sweeps)
	require.Equal(t, g.Layers, first, "the sweeps keep the seed")
	require.Equal(t, 1, crossings)
	require.Equal(t, 1, moved(t, g, "e->b#0", false), "the left ends keep the crossing")
	g.Layers = Minimize(context.Background(), g, false)
	assert.Equal(t, [][]string{
		{"a"},
		{"d->a#0@1", "b"},
		{"d->a#0@2", "c", "e->b#0@2"},
		{"d", "e->b#0@3"},
		{"e"},
	}, ids(g))
	assert.Equal(t, 0, g.Crossings())
}

// TestInterleave pins the counter-flow sides' interleaving: the span from
// b0 to b1 starts strictly inside the one from a0 to a1 and ends outside
// it. Nested spans, spans sharing an end layer and disjoint ones do not
// interleave, whichever comes first.
func TestInterleave(t *testing.T) {
	tests := []struct {
		name string
		a, b [2]int
		want bool
	}{
		{"b starts inside a, ends beyond", [2]int{0, 3}, [2]int{1, 4}, true},
		{"a starts inside b, ends beyond", [2]int{1, 4}, [2]int{0, 3}, false},
		{"b nested in a", [2]int{0, 4}, [2]int{1, 3}, false},
		{"a nested in b", [2]int{1, 3}, [2]int{0, 4}, false},
		{"shared start", [2]int{0, 4}, [2]int{0, 3}, false},
		{"shared end", [2]int{0, 4}, [2]int{2, 4}, false},
		{"b starts where a ends", [2]int{0, 2}, [2]int{2, 4}, false},
		{"disjoint", [2]int{0, 1}, [2]int{2, 4}, false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, interleave(tt.a[0], tt.a[1], tt.b[0], tt.b[1]), tt.name)
	}
}

// TestCrossedBy pins the counter-flow sides' crossing partners: a->A's hop
// crosses nothing, b->C's and c->B's cross each other only, and d->F's
// crosses both e->D's and f->E's, which cross d->F's only.
func TestCrossedBy(t *testing.T) {
	g := graph(t, "a", "b", "c", "d", "e", "f", "A", "B", "C", "D", "E", "F",
		"a->A", "b->C", "c->B", "d->F", "e->D", "f->E")
	require.Equal(t, [][]string{{"a", "b", "c", "d", "e", "f"}, {"A", "B", "C", "D", "E", "F"}}, ids(g))
	assert.Equal(t, []int{crossedByNone, 2, 1, crossedBySeveral, 3, 3}, crossedBy(g))
}
