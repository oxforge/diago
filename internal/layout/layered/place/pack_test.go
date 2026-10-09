package place

import (
	"context"
	"math"
	"slices"
	"testing"

	"github.com/oxforge/diago/internal/layout/layered/lgraph"
	"github.com/oxforge/diago/internal/layout/layered/lgraph/lgraphtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// beforePacking is act 2's result on g, the baseline without forcing: what
// Place returns before it packs the level's parts, on a level without
// pinned nodes.
func beforePacking(g *lgraph.Graph, o Options) []float64 {
	p := newPlacer(g, o)
	return align(alignInput{g: g, space: p.space, span: o.Span, ref: p.reference(), links: p.links, weight: p.weight})
}

// parts numbers g's hop-connected parts, per vertex.
func parts(g *lgraph.Graph) []int {
	part := make([]int, len(g.Vertices))
	for v := range part {
		part[v] = v
	}
	var find func(int) int
	find = func(v int) int {
		if part[v] != v {
			part[v] = find(part[v])
		}
		return part[v]
	}
	for _, h := range g.Hops() {
		part[find(h.Upper)] = find(h.Lower)
	}
	for v := range part {
		part[v] = find(v)
	}
	return part
}

// TestPlace_UnconnectedPartsPackTowardTheCenter pins S7's *Packing*: o's
// out-face holds two ports, p's and the long edge o->q's column. In a
// downward pass p and the column, squeezed, pool at their weighted mean
// (the column weighs 8, p 3), while in an upward pass o's target is their
// unweighted mean, so every upward pass moves o's part left, and c->d,
// which shares no edge with it, does not follow. After act 2 the two parts
// sit at their minimum separation, each moved rigidly, the level's
// weighted center where it was.
func TestPlace_UnconnectedPartsPackTowardTheCenter(t *testing.T) {
	g, x, _ := placed(t, screen, nil, "o->p", "p->q", "o->q", "c->d")
	before := beforePacking(g, screen)
	part := parts(g)
	p := newPlacer(g, screen)
	slack := func(x []float64) float64 {
		least := math.Inf(1)
		for _, layer := range g.Layers {
			for i := 1; i < len(layer); i++ {
				a, b := layer[i-1], layer[i]
				if part[a] != part[b] {
					least = min(least, x[b]-x[a]-p.sep(a, b))
				}
			}
		}
		return least
	}
	require.Greater(t, slack(before), 10.0, "act 2 leaves the parts apart")
	assert.InDelta(t, 0, slack(x), 1e-6, "packed at the minimum separation")
	shift := map[int]float64{}
	for v := range x {
		if s, ok := shift[part[v]]; ok {
			assert.InDelta(t, s, x[v]-before[v], 1e-6, "%s moves with its part", g.Vertices[v].ID)
			continue
		}
		shift[part[v]] = x[v] - before[v]
	}
	center := func(x []float64) float64 {
		sum, weight := 0.0, 0.0
		for v := range x {
			sum += p.weight[v] * x[v]
			weight += p.weight[v]
		}
		return sum / weight
	}
	assert.InDelta(t, center(before), center(x), 1e-6, "the weighted center stays")
}

// TestPlace_PackingLeavesInterleavedPartsAndASinglePartAlone pins S7's
// *Packing*: a level whose parts no one left-to-right order of them agrees
// with keeps act 2's result, and so does a level with a single part. r's
// two fans hold a1 and a2 apart with room between them, where a part of
// its own, an isolated node or b with its long edge to z, would slide
// toward the center; o->p and c->d cross, so their parts swap sides
// between the layers, each with room to close up.
func TestPlace_PackingLeavesInterleavedPartsAndASinglePartAlone(t *testing.T) {
	// built lays specs out on the given layers, each layer ordered by ids
	// (nodes and dummies) where order gives it
	built := func(layers map[string]int, order [][]string, specs ...string) *lgraph.Graph {
		lv := lgraphtest.Level(t, specs...)
		ls := make([]int, len(lv.Nodes))
		for i, n := range lv.Nodes {
			ls[i] = layers[n.ID]
		}
		g, err := lgraph.Build(lv, ls, make([]bool, len(lv.Edges)), 8)
		require.NoError(t, err)
		for l, ids := range order {
			if ids == nil {
				continue
			}
			g.Layers[l] = g.Layers[l][:0]
			for _, id := range ids {
				v := slices.IndexFunc(g.Vertices, func(vx lgraph.Vertex) bool { return vx.ID == id })
				require.GreaterOrEqual(t, v, 0, "no vertex %s", id)
				g.Layers[l] = append(g.Layers[l], v)
			}
		}
		return g
	}
	fans := []string{"r->a1", "r->a2", "a1->x1", "a1->x2", "a1->x3", "a2->y1", "a2->y2", "a2->y3"}
	layers := map[string]int{"r": 0, "a1": 1, "a2": 1, "iso": 1, "b": 1, "x1": 2, "x2": 2, "x3": 2,
		"y1": 2, "y2": 2, "y3": 2, "z": 3, "o": 0, "c": 1, "q": 2}
	for _, tc := range []struct {
		name string
		g    *lgraph.Graph
	}{
		{"an isolated node inside r's part", built(layers, [][]string{nil, {"a1", "iso", "a2"}}, append(fans, "iso")...)},
		{"b's part inside r's", built(layers, [][]string{nil, {"a1", "b", "a2"}}, append(fans, "b->z")...)},
		{"a single part", built(layers, nil, "o->c", "c->q", "o->q")},
	} {
		assert.Equal(t, beforePacking(tc.g, screen), Place(context.Background(), tc.g, screen), tc.name)
	}

	g := built(map[string]int{"o": 0, "c": 0, "p": 1, "d": 1}, [][]string{{"o", "c"}, {"d", "p"}}, "o->p", "c->d")
	x := []float64{0, 0, 400, -300} // o, p, c, d
	newPlacer(g, screen).pack(context.Background(), x, nil)
	assert.Equal(t, []float64{0, 0, 400, -300}, x, "two parts that swap sides")
}
