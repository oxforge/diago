package place

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/oxforge/diago/internal/layout/layered/lgraph"
)

// flat builds a layered graph of w-wide nodes from layers of ids, and the
// ids' vertex indices.
func flat(w float64, layers ...[]string) (*lgraph.Graph, map[string]int) {
	g := &lgraph.Graph{Level: &lgraph.Level{}}
	at := map[string]int{}
	for l, layer := range layers {
		g.Layers = append(g.Layers, nil)
		for _, id := range layer {
			at[id] = len(g.Vertices)
			g.Level.Nodes = append(g.Level.Nodes, lgraph.Node{ID: id, W: w, H: 40})
			g.Vertices = append(g.Vertices, lgraph.Vertex{ID: id, Node: at[id], Edge: -1, Layer: l, W: w})
			g.Layers[l] = append(g.Layers[l], at[id])
		}
	}
	return g, at
}

// fixture is an align input with gap 10, span 16 and unit weights.
func fixture(g *lgraph.Graph, at map[string]int, ref map[string]float64, links ...link) alignInput {
	r := make([]float64, len(g.Vertices))
	for id, x := range ref {
		r[at[id]] = x
	}
	w := make([]float64, len(g.Vertices))
	for i := range w {
		w[i] = 1
	}
	return alignInput{g: g, space: spacing{gap: 10, out: make([][2]float64, len(g.Vertices))}, span: 16, ref: r, links: links, weight: w}
}

func hop(at map[string]int, edge int, upper, lower string, delta float64) link {
	return link{edge: edge, upper: at[upper], lower: at[lower], delta: delta}
}

func forcedLink(l link) link {
	l.forced = true
	return l
}

func TestAlign_MixedPortOffsetsBecomeOneBlock(t *testing.T) {
	// the reference is a staircase (0, 5, 12); the hops want +3 and -4 and
	// both fit, so the run becomes one block at its members' mean
	g, at := flat(30, []string{"a"}, []string{"b"}, []string{"c"})
	in := fixture(g, at, map[string]float64{"a": 0, "b": 5, "c": 12},
		hop(at, 0, "a", "b", 3), hop(at, 1, "b", "c", -4))
	x := align(in)
	assert.InDelta(t, 3, x[at["b"]]-x[at["a"]], 1e-9)
	assert.InDelta(t, -4, x[at["c"]]-x[at["b"]], 1e-9)
	assert.InDelta(t, (0+(5-3)+(12+1))/3.0, x[at["a"]], 1e-9)
}

func TestAlign_AMisalignmentWiderThanTheSpanStays(t *testing.T) {
	g, at := flat(30, []string{"a"}, []string{"b"})
	in := fixture(g, at, map[string]float64{"a": 0, "b": 40}, hop(at, 0, "a", "b", 0))
	assert.Equal(t, in.ref, align(in))
}

func TestAlign_NeverOverridesSeparation(t *testing.T) {
	// both children want the parent's port; only one gets it, and the pair
	// keeps a full node gap
	g, at := flat(30, []string{"p"}, []string{"b", "c"})
	in := fixture(g, at, map[string]float64{"p": 0, "b": -12, "c": 28},
		hop(at, 0, "p", "b", 0), hop(at, 1, "p", "c", 0))
	x := align(in)
	assert.GreaterOrEqual(t, x[at["c"]]-x[at["b"]], 40-1e-9)
}

func TestAlign_ASymmetricFanStaysSymmetric(t *testing.T) {
	// both branches sit equally far from their ports: they are one batch,
	// and aligning both is infeasible, so neither tilts the fan
	g, at := flat(30, []string{"a"}, []string{"b", "c"})
	in := fixture(g, at, map[string]float64{"a": 0, "b": -20, "c": 20},
		hop(at, 0, "a", "b", -10), hop(at, 1, "a", "c", 10))
	assert.Equal(t, in.ref, align(in))
}

func TestAlign_CrossedHopsWiderThanTheSpanStay(t *testing.T) {
	g, at := flat(30, []string{"a", "x"}, []string{"y", "b"})
	in := fixture(g, at, map[string]float64{"a": 0, "x": 40, "y": 2, "b": 42},
		hop(at, 0, "a", "b", 0), hop(at, 1, "x", "y", 0))
	assert.Equal(t, in.ref, align(in))
}

func TestAlign_AForcedLinkAlignsAtAnyDistance(t *testing.T) {
	g, at := flat(30, []string{"a"}, []string{"b"})
	in := fixture(g, at, map[string]float64{"a": 0, "b": 40}, forcedLink(hop(at, 0, "a", "b", 0)))
	x := align(in)
	assert.InDelta(t, 0, x[at["b"]]-x[at["a"]], 1e-9)
}

func TestAlign_AForcedPairSnapsBothBranches(t *testing.T) {
	g, at := flat(30, []string{"p"}, []string{"b", "c"})
	in := fixture(g, at, map[string]float64{"p": 0, "b": -20, "c": 20},
		forcedLink(hop(at, 0, "p", "b", -40)), forcedLink(hop(at, 1, "p", "c", 40)))
	x := align(in)
	assert.InDelta(t, -40, x[at["b"]]-x[at["p"]], 1e-9)
	assert.InDelta(t, 40, x[at["c"]]-x[at["p"]], 1e-9)
}

func TestAlign_AnInfeasibleForcedBatchIsDroppedWhole(t *testing.T) {
	// m between the two children needs 80 of room; the forced pair leaves 40
	g, at := flat(30, []string{"p"}, []string{"b", "m", "c"})
	in := fixture(g, at, map[string]float64{"p": 0, "b": -80, "m": 0, "c": 80},
		forcedLink(hop(at, 0, "p", "b", -20)), forcedLink(hop(at, 1, "p", "c", 20)))
	assert.Equal(t, in.ref, align(in))
}

func TestAlign_TheStraighterOfTwoCrossingHopsWins(t *testing.T) {
	// a->b is 0 off and x->y 4 off; after a->b is taken, x->y would make
	// the two blocks swap sides between the layers
	g, at := flat(10, []string{"a", "x"}, []string{"y", "b"})
	in := fixture(g, at, map[string]float64{"a": 0, "x": 20, "y": 3, "b": 25},
		hop(at, 0, "a", "b", 25), hop(at, 1, "x", "y", -13))
	x := align(in)
	assert.InDelta(t, 25, x[at["b"]]-x[at["a"]], 1e-9)
	assert.InDelta(t, -17, x[at["y"]]-x[at["x"]], 1e-9)
}

func TestAlign_ASpanExemptLinkAlignsAtAnyDistance(t *testing.T) {
	g, at := flat(30, []string{"a"}, []string{"b"})
	l := hop(at, 0, "a", "b", 0)
	l.exempt = true
	in := fixture(g, at, map[string]float64{"a": 0, "b": 40}, l)
	x := align(in)
	assert.InDelta(t, 0, x[at["b"]]-x[at["a"]], 1e-9)
	assert.InDelta(t, 20, x[at["a"]], 1e-9, "the block sits at its members' mean")
}
