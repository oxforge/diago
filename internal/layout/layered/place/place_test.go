package place

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"testing"

	"github.com/oxforge/diago/internal/layout/layered/cycle"
	"github.com/oxforge/diago/internal/layout/layered/lgraph"
	"github.com/oxforge/diago/internal/layout/layered/lgraph/lgraphtest"
	"github.com/oxforge/diago/internal/layout/layered/order"
	"github.com/oxforge/diago/internal/layout/layered/rank"
	"github.com/oxforge/diago/internal/layoutdbg/layoutdbgtest"
	"github.com/oxforge/diago/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var screen = Options{Gap: 40, Span: 16, Reach: 20, Clearance: 12, Dir: model.Down}

// placed runs S2 to S7 over a fixture level; widths overrides node widths
// by id before layering. It returns the graph, the positions Place gave it
// (so a caller needing the raw slice does not have to call Place again),
// and an id-keyed accessor.
func placed(t *testing.T, o Options, widths map[string]float64, specs ...string) (*lgraph.Graph, []float64, func(id string) float64) {
	t.Helper()
	g := ordered(t, context.Background(), level(t, widths, specs...))
	x := Place(context.Background(), g, o)
	require.Len(t, x, len(g.Vertices))
	return g, x, func(id string) float64 { return x[vertexOf(t, g, id)] }
}

// level is the fixture level of specs, widths overriding node widths by id.
func level(t *testing.T, widths map[string]float64, specs ...string) *lgraph.Level {
	t.Helper()
	lv := lgraphtest.Level(t, specs...)
	for i := range lv.Nodes {
		if w, ok := widths[lv.Nodes[i].ID]; ok {
			lv.Nodes[i].W = w
		}
	}
	return lv
}

// ordered runs S2 to S6 over lv under ctx, S4's equalizing aside: the
// layered graph, ordered, that Place takes.
func ordered(t *testing.T, ctx context.Context, lv *lgraph.Level) *lgraph.Graph {
	t.Helper()
	reversed := cycle.Break(ctx, lv, nil)
	g, err := lgraph.Build(lv, rank.Assign(ctx, lv, reversed, nil), reversed, 8)
	require.NoError(t, err)
	g.Layers = order.Minimize(ctx, g, false)
	return g
}

func TestPlace_AChainIsStraight(t *testing.T) {
	_, _, x := placed(t, screen, nil, "a->b", "b->c", "c->d")
	for _, id := range []string{"b", "c", "d"} {
		assert.Equal(t, x("a"), x(id), id)
	}
}

func TestPlace_DiamondBranchesAreSymmetric(t *testing.T) {
	_, _, x := placed(t, screen, nil, "a->b", "a->c", "b->d", "c->d")
	assert.InDelta(t, x("a"), x("d"), 1e-6)
	left, right := min(x("b"), x("c")), max(x("b"), x("c"))
	assert.InDelta(t, x("a")-left, right-x("a"), 1e-6)
}

func TestPlace_AFanIsCenteredAndEven(t *testing.T) {
	_, _, x := placed(t, screen, nil, "root->a", "root->b", "root->c", "root->d")
	assert.InDelta(t, (x("a")+x("d"))/2, x("root"), 1e-6)
	assert.InDelta(t, x("b")-x("a"), x("c")-x("b"), 1e-6)
	assert.InDelta(t, x("b")-x("a"), x("d")-x("c"), 1e-6)
}

func TestPlace_ABinaryTreeIsMirrorSymmetric(t *testing.T) {
	_, _, x := placed(t, screen, nil, "r->a", "r->b", "a->aa", "a->ab", "b->ba", "b->bb")
	axis := x("r")
	assert.InDelta(t, 2*axis, x("aa")+x("bb"), 1e-6)
	assert.InDelta(t, 2*axis, x("ab")+x("ba"), 1e-6)
	assert.InDelta(t, 2*axis, x("a")+x("b"), 1e-6)
}

func TestPlace_TextProfileHasNoPinnedShapes(t *testing.T) {
	text := Options{Gap: 5, Span: 1, Reach: 3, Clearance: 2, Dir: model.Down, Text: true}
	_, _, x := placed(t, text, map[string]float64{"m": 10, "yes": 8, "no": 8}, "m:diamond->yes", "m->no")
	// in text the diamond is a box whose out-face spreads two ports at
	// thirds of its width; the children center under them if they fit
	// (8 + 5 = 13 apart), which a 10-wide face (ports 3.33 apart) cannot give;
	// on the character grid, within half a cell
	assert.InDelta(t, x("m"), (x("yes")+x("no"))/2, 0.5)
	assert.GreaterOrEqual(t, x("no")-x("yes"), 13-1e-9)
}

func TestPlace_ASelfLoopKeepsItsRoom(t *testing.T) {
	narrow := Options{Gap: 10, Span: 16, Reach: 20, Clearance: 12, Dir: model.Down}
	// a and b are both x's parents, so they squeeze to their minimum
	_, _, x := placed(t, narrow, nil, "a->a", "a->x", "b->x")
	assert.InDelta(t, 80+20+12, x("b")-x("a"), 1e-6, "Reach and the clearance right of a, more than the gap")

	_, _, x = placed(t, narrow, nil, "a", "b->b", "a->x", "b->x")
	assert.InDelta(t, 80+10, x("b")-x("a"), 1e-6, "b's loop runs on its right: the gap on its left stays")
}

// TestPlace_NestedSelfLoopsKeepTheirRoom pins S7's separation: right of a
// node with self-loops the gap is at least its outermost loop leg's
// distance out plus the clearance. n loops nest on a node with faces, the
// outermost Reach + (n - 1) InLaneGap out (S8); a diamond's loops share the
// column Reach out.
func TestPlace_NestedSelfLoopsKeepTheirRoom(t *testing.T) {
	narrow := Options{Gap: 10, Span: 16, Reach: 20, Clearance: 12, InLaneGap: 8, Dir: model.Down}
	// a and b are both x's parents, so they squeeze to their minimum
	_, _, x := placed(t, narrow, nil, "a->a", "a->a", "a->a", "a->x", "b->x")
	assert.InDelta(t, 80+20+2*8+12, x("b")-x("a"), 1e-6, "three nested loops right of a")

	_, _, x = placed(t, narrow, nil, "a:diamond->a", "a->a", "a->a", "a->x", "b->x")
	assert.InDelta(t, 80+20+12, x("b")-x("a"), 1e-6, "a diamond's loops run on one column")
}

// TestPlace_AVertexWithoutNeighborsRidesWithItsRow pins S7's act 1: a
// vertex without neighbors on either side keeps its minimum separation
// from the nearest vertex before it in its layer that has neighbors, or,
// first in its layer, from the nearest after it, instead of holding the
// place the packing from 0 gave it while that vertex moves away.
func TestPlace_AVertexWithoutNeighborsRidesWithItsRow(t *testing.T) {
	widths := map[string]float64{"t": 8, "a": 140, "w": 150, "iso": 190}
	built := func(layers []int, specs ...string) *lgraph.Graph {
		lv := lgraphtest.Level(t, specs...)
		for i := range lv.Nodes {
			lv.Nodes[i].W = widths[lv.Nodes[i].ID]
		}
		g, err := lgraph.Build(lv, layers, make([]bool, len(lv.Edges)), 8)
		require.NoError(t, err)
		return g
	}

	// a moves left under t, a narrow parent, as under a terminal in a
	// group's level (S9)
	g := built([]int{0, 1, 2, 1}, "t->a", "a->w", "iso")
	x := Place(context.Background(), g, screen)
	assert.InDelta(t, 70+40+95, x[3]-x[1], 1e-6, "right of a")

	// one downward projection of layer [iso a] toward t, far right of the
	// packing from 0
	g = built([]int{0, 1, 1}, "t->a", "iso")
	g.Layers[1] = []int{2, 1}
	p := newPlacer(g, screen)
	x = []float64{644, 300, 95} // t, a, iso
	p.project(x, 1, p.up)
	assert.InDelta(t, 644, x[1], 1e-6, "a under t")
	assert.InDelta(t, 95+40+70, x[1]-x[2], 1e-6, "left of a, first in its layer")
}

// TestPlace_AVertexWithOnlySelfLoopsRidesWithItsRow pins S7's projection
// for a node whose only edges are self-loops: a self-loop has no chain,
// so it gives the node no neighbor, and the node takes no part in the fit
// but rides with its row, as an edgeless one does
// (TestPlace_AVertexWithoutNeighborsRidesWithItsRow).
func TestPlace_AVertexWithOnlySelfLoopsRidesWithItsRow(t *testing.T) {
	widths := map[string]float64{"t": 8, "a": 140, "s": 190}
	lv := lgraphtest.Level(t, "t->a", "s->s", "s->s")
	for i := range lv.Nodes {
		lv.Nodes[i].W = widths[lv.Nodes[i].ID]
	}
	g, err := lgraph.Build(lv, []int{0, 1, 1}, make([]bool, len(lv.Edges)), 8)
	require.NoError(t, err)
	g.Layers[1] = []int{2, 1} // s, then a
	p := newPlacer(g, screen)
	require.True(t, p.alone(2), "s has no neighbor")

	// one downward projection of layer [s a] toward t, far right of the
	// packing from 0: s keeps left of a, at the minimum separation
	x := []float64{644, 300, 95} // t, a, s
	p.project(x, 1, p.up)
	assert.InDelta(t, 644, x[1], 1e-6, "a under t")
	assert.InDelta(t, 95+40+70, x[1]-x[2], 1e-6, "s left of a, first in its layer")
}

func TestPlace_TextBoxesSitOnTheCharacterGrid(t *testing.T) {
	text := Options{Gap: 5, Span: 1, Reach: 3, Clearance: 2, Dir: model.Right, Text: true}
	// a 3-row box (RIGHT: the cross extent is rows) and a long edge's dummy
	g, x, _ := placed(t, text, map[string]float64{"a": 3, "b": 3, "c": 3}, "a->b", "b->c", "a->c", "a->d")
	for v, vx := range g.Vertices {
		left := x[v] - vx.W/2
		assert.Equal(t, math.Floor(left), left, "%s starts on a cell boundary", vx.ID)
	}
}

func TestPlace_KeepsTheMinimumSeparation(t *testing.T) {
	rng := rand.New(rand.NewPCG(13, 17))
	shapes := []string{"", ":diamond", ":circle", ":hexagon", ":parallelogram", ":cylinder"}
	// a Reach past the node gap, so a self-loop leg asks for more room than
	// the gap beside its node (Reach + Clearance = 52 > Gap)
	o := screen
	o.Reach, o.InLaneGap = 40, 8
	roomier := 0
	for trial := range 120 {
		n := 2 + rng.IntN(16)
		specs := make([]string, 0, 3*n)
		for i := range n {
			specs = append(specs, fmt.Sprintf("n%d%s", i, shapes[rng.IntN(len(shapes))]))
		}
		for range n + rng.IntN(2*n) {
			specs = append(specs, fmt.Sprintf("n%d->n%d", rng.IntN(n), rng.IntN(n)))
		}
		widths := map[string]float64{}
		for i := range n {
			widths[fmt.Sprintf("n%d", i)] = float64(40 + rng.IntN(160))
		}
		g, x, _ := placed(t, o, widths, specs...)
		space := newPlacer(g, o).space // S7's whole separation: side columns and loop legs included
		for l, layer := range g.Layers {
			for i := 1; i < len(layer); i++ {
				a, b := layer[i-1], layer[i]
				need := space.sep(g, a, b)
				if need > (g.Vertices[a].W+g.Vertices[b].W)/2+o.Gap {
					roomier++
				}
				assert.GreaterOrEqual(t, x[b]-x[a], need-1e-6, "trial %d layer %d: %s | %s", trial, l, g.Vertices[a].ID, g.Vertices[b].ID)
			}
		}
		for v, xv := range x {
			assert.False(t, math.IsNaN(xv) || math.IsInf(xv, 0), "trial %d vertex %d", trial, v)
		}
	}
	assert.Positive(t, roomier, "some neighbors need more than the gap, or the loop is vacuous")
}

// decisions runs Place with a recording layoutdbg context and returns the
// graph (ordered, so a caller can check layer preconditions) and the
// place-phase decision records as JSON objects, in emission order.
func decisions(t *testing.T, o Options, widths map[string]float64, specs ...string) (*lgraph.Graph, []map[string]any) {
	t.Helper()
	ctx, buf := layoutdbgtest.Capture()
	g := ordered(t, ctx, level(t, widths, specs...))
	Place(ctx, g, o)
	var out []map[string]any
	for _, entry := range layoutdbgtest.Records(t, buf) {
		if entry["phase"] == "place" {
			out = append(out, entry)
		}
	}
	return g, out
}

// vertexOf returns g's vertex index for real node id.
func vertexOf(t *testing.T, g *lgraph.Graph, id string) int {
	t.Helper()
	for i, n := range g.Level.Nodes {
		if n.ID == id {
			return i
		}
	}
	require.Failf(t, "no node", "%s", id)
	return -1
}

// laid runs S2 to S7 over lv as it is.
func laid(t *testing.T, lv *lgraph.Level, o Options) (*lgraph.Graph, func(id string) float64) {
	t.Helper()
	g := ordered(t, context.Background(), lv)
	x := Place(context.Background(), g, o)
	return g, func(id string) float64 { return x[vertexOf(t, g, id)] }
}

// TestPlace_TerminalsKeepTheTerminalGap pins S7 (S9): two terminals, wire
// columns only, keep the terminal gap between their centers, not the node
// gap, so both align on ports about 26.7 px apart.
func TestPlace_TerminalsKeepTheTerminalGap(t *testing.T) {
	lv := lgraphtest.Level(t, "ta", "tb", "n", "ta->n", "tb->n")
	for _, i := range []int{0, 1} {
		lv.Nodes[i].Terminal, lv.Nodes[i].W, lv.Nodes[i].H = true, 8, 0
	}
	o := screen
	o.TerminalGap = 16
	_, x := laid(t, lv, o)
	assert.InDelta(t, x("n")-40.0/3, x("ta"), 1e-6, "on n's first in-port")
	assert.InDelta(t, x("n")+40.0/3, x("tb"), 1e-6, "on n's second in-port")
}

// TestPlace_TextTerminalsLineUpWithPortsOneCellApart pins S7 (S9, S14):
// under RIGHT a text row's terminals keep only the terminal gap, one cell,
// between their centers, so three of them sit on the three in-ports of a
// node five cells across, one cell apart: each wire runs straight. With
// the lane gap between them, two cells, two would miss their ports.
func TestPlace_TextTerminalsLineUpWithPortsOneCellApart(t *testing.T) {
	lv := lgraphtest.Level(t, "ta", "tb", "tc", "n", "ta->n", "tb->n", "tc->n")
	for _, i := range []int{0, 1, 2} {
		lv.Nodes[i].Terminal, lv.Nodes[i].W, lv.Nodes[i].H = true, 1, 0
	}
	lv.Nodes[3].W, lv.Nodes[3].H = 5, 16
	o := Options{Gap: 2, TerminalGap: 1, Span: 1, Reach: 2, Clearance: 1, InLaneGap: 1, Dir: model.Right, Text: true}
	_, x := laid(t, lv, o)
	assert.InDelta(t, x("n")-1, x("ta"), 1e-9, "on n's first in-port")
	assert.InDelta(t, x("n"), x("tb"), 1e-9, "on n's second in-port")
	assert.InDelta(t, x("n")+1, x("tc"), 1e-9, "on n's third in-port")
}

// TestPlace_ReturnsEveryVertexWhenTheContextIsDone pins the deadline in
// S7 on TestPlace_ForcingIsDecidedAgainAfterTheRerun's graph, whose
// rounds 0 and 1 each force an exit: a live context is consulted before
// every round, and one done at the first round's check stops the rounds
// there, before they move a and b, with an x for every vertex.
func TestPlace_ReturnsEveryVertexWhenTheContextIsDone(t *testing.T) {
	widths := map[string]float64{"m": 160, "a": 60, "b": 160, "z": 80}
	g, _, _ := placed(t, screen, widths, "m:diamond->b:diamond", "m->a:circle", "z:diamond->b")
	live := &lgraphtest.Countdown{Context: context.Background(), N: math.MaxInt}
	full := Place(live, g, screen)
	require.Greater(t, live.Calls, 2, "a live context is consulted before every round")
	done := &lgraphtest.Countdown{Context: context.Background()}
	x := Place(done, g, screen)
	assert.Equal(t, 1, done.Calls, "the first round's check stops the rounds")
	assert.Len(t, x, len(g.Vertices))
	assert.NotEqual(t, full, x, "the forcing rounds did not run")
}
