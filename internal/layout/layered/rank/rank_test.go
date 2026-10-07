package rank

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/layout/layered/cycle"
	"github.com/oxforge/diago/internal/layout/layered/lgraph"
	"github.com/oxforge/diago/internal/layout/layered/lgraph/lgraphtest"
	"github.com/oxforge/diago/internal/layoutdbg"
)

// layered breaks cycles and runs longest-path layering, as Assign does
// before tightening.
func layered(t *testing.T, specs ...string) (*lgraph.Level, []bool, []int) {
	t.Helper()
	lv := lgraphtest.Level(t, specs...)
	reversed := cycle.Break(context.Background(), lv, nil)
	return lv, reversed, LongestPath(lv, reversed)
}

// at returns the layer of the node with id.
func at(t *testing.T, lv *lgraph.Level, layers []int, id string) int {
	t.Helper()
	for i, n := range lv.Nodes {
		if n.ID == id {
			return layers[i]
		}
	}
	require.Failf(t, "no node", "%s", id)
	return -1
}

func TestSpan(t *testing.T) {
	lv, reversed, layers := layered(t, "a->b", "b->c", "a->c")
	assert.Equal(t, 4, Span(lv, reversed, layers), "a:0 b:1 c:2, spans 1 + 1 + 2")

	lv, reversed, layers = layered(t, "a->b", "b->a")
	assert.Equal(t, []bool{false, true}, reversed)
	assert.Equal(t, 2, Span(lv, reversed, layers), "a reversed edge spans the way the layering sees it")

	lv, reversed, layers = layered(t, "a->b", "a->a")
	assert.Equal(t, 1, Span(lv, reversed, layers), "self-loops span nothing")
}

func TestTighten_PullDown(t *testing.T) {
	t.Run("a lone deep feeder settles one layer above its sink", func(t *testing.T) {
		lv, reversed, layers := layered(t, "a->b", "b->c", "c->d", "e->d")
		assert.Equal(t, 0, at(t, lv, layers, "e"))
		tight := Tighten(lv, reversed, layers, nil)
		assert.Equal(t, 2, at(t, lv, tight, "e"))
		for _, id := range []string{"a", "b", "c", "d"} {
			assert.Equal(t, at(t, lv, layers, id), at(t, lv, tight, id), id)
		}
		assert.Equal(t, 4, Span(lv, reversed, tight))
		assert.Equal(t, 6, Span(lv, reversed, layers))
	})
	t.Run("two out-edges to different depths settle above the nearer", func(t *testing.T) {
		lv, reversed, layers := layered(t, "c0->c1", "c1->c2", "c2->c3", "n->c2", "n->c3")
		assert.Equal(t, 1, at(t, lv, Tighten(lv, reversed, layers, nil), "n"))
	})
	t.Run("a feeder of a moved node follows it in a later sweep", func(t *testing.T) {
		lv, reversed, layers := layered(t, "c0->c1", "c1->c2", "c2->c3", "c3->c4", "a->n", "n->c3", "n->c4")
		tight := Tighten(lv, reversed, layers, nil)
		assert.Equal(t, 2, at(t, lv, tight, "n"))
		assert.Equal(t, 1, at(t, lv, tight, "a"))
	})
	t.Run("a feeder into a broken cycle tightens across the reversed edge", func(t *testing.T) {
		lv, reversed, layers := layered(t,
			"entry->work", "work->review", "review->work", "review->done", "hotfix->done")
		assert.True(t, reversed[2], "review->work is the back edge")
		assert.Equal(t, 2, at(t, lv, Tighten(lv, reversed, layers, nil), "hotfix"))
	})
}

func TestTighten_KeepsOptimalLayerings(t *testing.T) {
	for _, specs := range [][]string{
		{"a->b", "b->c", "c->d"},
		{"r->l", "r->rt", "l->ll", "l->lr", "rt->rl", "rt->rr"},
		{"a->b", "b->c", "a->c", "lonely"},
	} {
		lv, reversed, layers := layered(t, specs...)
		assert.Equal(t, layers, Tighten(lv, reversed, layers, nil), "%v", specs)
	}
}

func TestTighten_IsPure(t *testing.T) {
	lv, reversed, layers := layered(t, "a->b", "b->c", "c->d", "e->d")
	before := slices.Clone(layers)
	Tighten(lv, reversed, layers, nil)
	assert.Equal(t, before, layers)
}

func TestTighten_PullUpAndRenumbering(t *testing.T) {
	// Synthetic layers: longest-path never leaves incoming-side slack.
	lv := lgraphtest.Level(t, "a->b", "a->z")
	assert.Equal(t, []int{0, 1, 1}, Tighten(lv, nil, []int{0, 1, 5}, nil), "z pulls up to its predecessor")

	lv = lgraphtest.Level(t, "a->b", "iso")
	assert.Equal(t, []int{0, 1, 2}, Tighten(lv, nil, []int{0, 1, 5}, nil), "layers renumber to a contiguous range")
}

func TestTighten_AnchoredNodesStay(t *testing.T) {
	lv, reversed, layers := layered(t, "a->b", "b->c", "c->d", "e->d")
	anchored := make([]bool, len(lv.Nodes))
	anchored[4] = true // e
	assert.Equal(t, layers, Tighten(lv, reversed, layers, anchored))
}

func TestAssign(t *testing.T) {
	lv := lgraphtest.Level(t, "a->b", "b->c", "c->d", "e->d", "d->a")
	reversed := cycle.Break(context.Background(), lv, nil)
	layers := Assign(context.Background(), lv, reversed, nil)
	for e := range lv.Edges {
		if lv.SelfLoop(e) {
			continue
		}
		upper, lower := lv.Oriented(e, reversed)
		assert.GreaterOrEqual(t, layers[lower]-layers[upper], 1, lv.Edges[e].ID)
	}
	used := slices.Clone(layers)
	slices.Sort(used)
	used = slices.Compact(used)
	assert.Equal(t, used[len(used)-1]+1, len(used), "contiguous from 0")
	assert.Equal(t, 0, used[0])
}

// TestAssign_TerminalsTakeRowsOfTheirOwn pins S3: the other nodes are
// layered as if the terminals were not there, a terminal whose edge
// leaves it takes a new first row and one whose edge enters it a new last
// row, below every other node.
func TestAssign_TerminalsTakeRowsOfTheirOwn(t *testing.T) {
	lv := lgraphtest.Level(t, "in", "a", "b", "c", "out", "in->b", "a->b", "b->c", "a->out")
	lv.Nodes[0].Terminal, lv.Nodes[4].Terminal = true, true
	layers := Assign(context.Background(), lv, nil, nil)
	assert.Equal(t, []int{0, 1, 2, 3, 4}, layers, "a, b and c keep their own rows one lower; out sits below c, not beside b")

	lv = lgraphtest.Level(t, "a", "b", "out", "a->b", "b->out")
	lv.Nodes[2].Terminal = true
	assert.Equal(t, []int{0, 1, 2}, Assign(context.Background(), lv, nil, nil), "no first-row terminal, no shift")
}

// adopting runs Assign over the level of specs with previous layers by
// node id (every other node takes none) and returns the level, its layers
// and the anchoring records, each as its decision, its node and its
// reason, or its shift.
func adopting(t *testing.T, previous map[string]int, specs ...string) (*lgraph.Level, []int, []string) {
	t.Helper()
	lv := lgraphtest.Level(t, specs...)
	prev := make([]int, len(lv.Nodes))
	for v, n := range lv.Nodes {
		prev[v] = -1
		if l, ok := previous[n.ID]; ok {
			prev[v] = l
		}
	}
	var buf bytes.Buffer
	ctx := layoutdbg.NewContext(context.Background(), slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	layers := Assign(ctx, lv, cycle.Break(ctx, lv, nil), prev)
	var recs []string
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var rec struct {
			Decision, Phase, Node, Reason string
			Shift                         *int
		}
		require.NoError(t, json.Unmarshal([]byte(line), &rec))
		if rec.Phase == "anchoring" {
			if rec.Shift != nil {
				rec.Reason = strconv.Itoa(*rec.Shift)
			}
			recs = append(recs, strings.TrimSpace(rec.Decision+" "+rec.Node+" "+rec.Reason))
		}
	}
	return lv, layers, recs
}

func TestAssign_AdoptsAFeasiblePreviousLayer(t *testing.T) {
	// Fresh, c sits on layer 1 beside b; a previous layout had it below b.
	lv, layers, recs := adopting(t, map[string]int{"a": 0, "b": 1, "c": 2}, "a->b", "a->c")
	assert.Equal(t, 2, at(t, lv, layers, "c"))
	assert.Contains(t, recs, "layer_adopted c")
}

func TestAssign_RejectsALayerNotBelowAPredecessor(t *testing.T) {
	_, layers, recs := adopting(t, map[string]int{"a": 1, "b": 0}, "a->b")
	assert.Equal(t, []int{0, 1}, layers, "b cannot sit above a")
	assert.Contains(t, recs, "layer_rejected b predecessor:a")
}

func TestAssign_AlignsThePreviousLayers(t *testing.T) {
	// S13: the previous layers shift alike, by the amount under which the
	// most of them are adopted. n, new, feeds in, which sat on layer 0:
	// every node moves one layer down and keeps it. In the second level,
	// a and b sat from layer 3, below rows since emptied: they shift three
	// up, and z, new, sits beside a.
	lv, layers, recs := adopting(t, map[string]int{"in": 0, "a": 1, "b": 2}, "n->in", "in->a", "a->b", "in->b")
	assert.Equal(t, []int{0, 1, 2, 3}, []int{at(t, lv, layers, "n"), at(t, lv, layers, "in"), at(t, lv, layers, "a"), at(t, lv, layers, "b")})
	assert.Contains(t, recs, "layers_aligned  1")
	assert.NotContains(t, strings.Join(recs, "\n"), "layer_rejected")

	lv, layers, recs = adopting(t, map[string]int{"a": 3, "b": 4}, "a->b", "z")
	assert.Equal(t, []int{0, 1, 0}, []int{at(t, lv, layers, "a"), at(t, lv, layers, "b"), at(t, lv, layers, "z")})
	assert.Contains(t, recs, "layers_aligned  -3")
}

func TestAssign_RejectsALayerNotAboveASuccessor(t *testing.T) {
	// b is visited first (its fresh layer is deeper) and keeps 1; a's 3
	// would put it below b.
	_, layers, recs := adopting(t, map[string]int{"a": 3, "b": 1}, "a->b")
	assert.Equal(t, []int{0, 1}, layers)
	assert.Contains(t, recs, "layer_adopted b")
	assert.Contains(t, recs, "layer_rejected a successor:b")
}

func TestAssign_ANewNodeIsPushedBelowAnAdoptedOne(t *testing.T) {
	// a adopts layer 2 over its fresh 1, and n, new, follows it down.
	lv, layers, recs := adopting(t, map[string]int{"r": 0, "a": 2}, "r->a", "a->n")
	assert.Contains(t, recs, "layer_adopted a")
	assert.Contains(t, recs, "layer_pushed n")
	assert.Equal(t, at(t, lv, layers, "a")+1, at(t, lv, layers, "n"))
}

func TestAssign_ANewNodeKeepsItsRoom(t *testing.T) {
	// c keeps its layer. a sat one layer above it, and n, new, sits between
	// them now: a cannot keep its layer above n.
	lv, layers, recs := adopting(t, map[string]int{"a": 2, "c": 3}, "a->n", "n->c")
	assert.Contains(t, recs, "layer_adopted c")
	assert.Contains(t, recs, "layer_rejected a successor:n")
	assert.Less(t, at(t, lv, layers, "a"), at(t, lv, layers, "n"))
	assert.Less(t, at(t, lv, layers, "n"), at(t, lv, layers, "c"))
}

func TestAssign_AnAdoptedNodeIsNotTightened(t *testing.T) {
	// Fresh, x moves down beside c, one layer above d; adopted on layer 0,
	// it stays there.
	lv, fresh, _ := adopting(t, nil, "a->b", "b->c", "c->d", "x->d")
	require.Equal(t, 2, at(t, lv, fresh, "x"))
	lv, layers, _ := adopting(t, map[string]int{"a": 0, "b": 1, "c": 2, "d": 3, "x": 0}, "a->b", "b->c", "c->d", "x->d")
	assert.Equal(t, 0, at(t, lv, layers, "x"))
}

func TestAssign_ClosesTheLayerADeletedNodeLeft(t *testing.T) {
	// b sat between a and c, and is gone: C18.5's deletion. c adopts its
	// layer 2, and the renumbering closes layer 1.
	_, layers, recs := adopting(t, map[string]int{"a": 0, "c": 2}, "a->c")
	assert.Contains(t, recs, "layer_adopted c")
	assert.Equal(t, []int{0, 1}, layers)
}

func TestAssign_ATerminalHoldsNoLayerOfItsOwn(t *testing.T) {
	// n adopts layer 1 in a level whose only other vertex is its edge's
	// terminal. The terminal, on layer 0 until it takes its own last row,
	// must not keep layer 0 open above n.
	lv := lgraphtest.Level(t, "n", "t", "n->t")
	lv.Nodes[1].Terminal = true
	layers := Assign(context.Background(), lv, cycle.Break(context.Background(), lv, nil), []int{1, -1})
	assert.Equal(t, []int{0, 1}, layers)
}

func TestAssign_ANodePushedDeeperMovesAlone(t *testing.T) {
	// b now feeds c, which sat beside it: c must move a layer down. Shifted
	// one down, so that c's previous layer would be its longest-path one, a
	// and b could not keep theirs above c; unshifted, only c moves (S13).
	lv, layers, recs := adopting(t, map[string]int{"a": 0, "b": 1, "c": 1}, "a->b", "a->c", "b->c")
	assert.Equal(t, []int{0, 1, 2}, []int{at(t, lv, layers, "a"), at(t, lv, layers, "b"), at(t, lv, layers, "c")})
	assert.Contains(t, recs, "layers_aligned  0")
	assert.Contains(t, recs, "layer_rejected c predecessor:b")
	assert.Contains(t, recs, "layer_adopted b")
	assert.Contains(t, recs, "layer_adopted a")
}

func TestAssign_ASourceAlignedAboveTheTopKeepsItsLayer(t *testing.T) {
	// f, new below c, now takes c's old layer, so only a shift of one up
	// keeps a, b and c; d, which sat one row above a, then aligns on
	// layer -1, above the top, and its only child e is new: e can sink as
	// deep as it likes, so d keeps its row above a (S13).
	lv, layers, recs := adopting(t, map[string]int{"a": 1, "b": 2, "c": 3, "f": 3, "d": 0}, "a->b", "b->c", "c->f", "d->e")
	assert.Contains(t, recs, "layers_aligned  -1")
	assert.Contains(t, recs, "layer_adopted d")
	assert.Equal(t, []int{0, 1, 1, 2, 3, 4}, []int{
		at(t, lv, layers, "d"), at(t, lv, layers, "e"), at(t, lv, layers, "a"),
		at(t, lv, layers, "b"), at(t, lv, layers, "c"), at(t, lv, layers, "f")})
}

// TestAssign_AReversedEdgeBindsAdoptionAsItIsLaidOut: S13's adoption
// reads every edge as S2 lays it out, a reversed one pointing the other
// way. a and b feed each other; the search reverses b->a, so a lies above
// b both ways round. A previous layout had b above a: under the shift
// nearest zero that adopts the most, a keeps its layer and b, which must
// sit below a, rejects its own.
func TestAssign_AReversedEdgeBindsAdoptionAsItIsLaidOut(t *testing.T) {
	lv, layers, recs := adopting(t, map[string]int{"a": 1, "b": 0}, "a->b", "b->a")
	assert.Equal(t, []int{0, 1}, []int{at(t, lv, layers, "a"), at(t, lv, layers, "b")})
	assert.Contains(t, recs, "layers_aligned  -1")
	assert.Contains(t, recs, "layer_adopted a")
	assert.Contains(t, recs, "layer_rejected b predecessor:a")
}

// TestAssign_APushCascadesThroughEveryPass: S13's push moves every node
// below its predecessors, pass after pass, until none moves. x keeps
// layer 0 and a adopts layer 3, below its fresh 0; b and c, new, follow it
// down. The nodes are declared c, b, a, x, so the first pass checks b->c
// before it pushes b below a, and only a second pass pushes c below b.
func TestAssign_APushCascadesThroughEveryPass(t *testing.T) {
	lv, layers, recs := adopting(t, map[string]int{"x": 0, "a": 3}, "c", "b", "a->b", "b->c", "x")
	require.Equal(t, []string{"c", "b", "a", "x"}, []string{lv.Nodes[0].ID, lv.Nodes[1].ID, lv.Nodes[2].ID, lv.Nodes[3].ID})
	assert.Equal(t, []int{0, 1, 2, 3}, []int{at(t, lv, layers, "x"), at(t, lv, layers, "a"), at(t, lv, layers, "b"), at(t, lv, layers, "c")})
	assert.Contains(t, recs, "layers_aligned  0")
	assert.Contains(t, recs, "layer_adopted a")
	pushed := slices.DeleteFunc(slices.Clone(recs), func(r string) bool { return !strings.HasPrefix(r, "layer_pushed") })
	assert.Equal(t, []string{"layer_pushed b", "layer_pushed c"}, pushed, "c only once b has moved")
}

// TestAdopt_ExtremeLayersDoNotWrap: a carrier's layers are unbounded, so
// a candidate shift (Adopt) can align a layer near either end of the int
// range; each check of adoptAt compares as it would without bounds.
func TestAdopt_ExtremeLayersDoNotWrap(t *testing.T) {
	for _, tt := range []struct {
		name     string
		specs    []string
		previous []int // per node, in order of first appearance
		shift    int
		adopted  bool // whether the one node with a previous layer adopts it
		layer    int  // and its layer then
	}{
		{"a new sink below a layer above the top", []string{"d->e"}, []int{0, -1}, -1, true, -1},
		{"a predecessor below a layer far above the top", []string{"a->b", "b->p", "p->v"}, []int{-1, -1, -1, 0}, -math.MaxInt, false, 3},
		{"an aligned layer past the int range", []string{"x"}, []int{math.MaxInt}, 1, true, math.MaxInt},
	} {
		t.Run(tt.name, func(t *testing.T) {
			lv, reversed, layers := layered(t, tt.specs...)
			v := slices.IndexFunc(tt.previous, func(l int) bool { return l >= 0 })
			out, adopted := adoptAt(context.Background(), lv, adjacent(lv, reversed), layers, tt.previous, []int{v}, tt.shift, false)
			assert.Equal(t, tt.adopted, adopted[v])
			assert.Equal(t, tt.layer, out[v])
		})
	}
}

// room is the screen profile's node gap and dummy width (S14).
var room = Room{Gap: 40, Dummy: 8}

// isolated runs Assign and then Isolated over a fixture level, heights
// overriding node heights by id; kept marks the nodes that adopted a
// previous layer, by id.
func isolated(t *testing.T, heights map[string]float64, kept []string, specs ...string) (*lgraph.Level, []int, []int) {
	t.Helper()
	lv := lgraphtest.Level(t, specs...)
	for i := range lv.Nodes {
		if h, ok := heights[lv.Nodes[i].ID]; ok {
			lv.Nodes[i].H = h
		}
	}
	ctx := context.Background()
	reversed := cycle.Break(ctx, lv, nil)
	layers := Assign(ctx, lv, reversed, nil)
	var adopted []bool
	if kept != nil {
		adopted = make([]bool, len(lv.Nodes))
		for i, n := range lv.Nodes {
			adopted[i] = slices.Contains(kept, n.ID)
		}
	}
	return lv, layers, Isolated(ctx, lv, reversed, layers, adopted, room)
}

// TestIsolated pins S3's isolated nodes: an edgeless node moves to the
// row with the most room where it fits, the earliest of equal rooms, and
// otherwise keeps its layer; every node is 80 x 40, so a row of two
// nodes is 200 wide and a node needs 120 of room.
func TestIsolated(t *testing.T) {
	t.Run("an edgeless node joins the row with the most room", func(t *testing.T) {
		lv, _, out := isolated(t, nil, nil, "a->b", "b->c", "d->e", "f->g", "iso")
		assert.Equal(t, at(t, lv, out, "c"), at(t, lv, out, "iso"), "rows [a d f] and [b e g] are 320 wide, c's has 240 of room")
	})
	t.Run("of equal rooms the earliest row wins", func(t *testing.T) {
		lv, _, out := isolated(t, nil, nil, "a->c", "b->c", "c->d", "iso")
		assert.Equal(t, at(t, lv, out, "c"), at(t, lv, out, "iso"), "c's row and d's both have 120 of room")
	})
	t.Run("a node that fits no row keeps its layer", func(t *testing.T) {
		lv, layers, out := isolated(t, nil, nil, "a->b", "iso")
		assert.Equal(t, layers, out)
		assert.Equal(t, at(t, lv, out, "a"), at(t, lv, out, "iso"))
	})
	t.Run("a node taller than a row does not fit it", func(t *testing.T) {
		lv, layers, out := isolated(t, map[string]float64{"iso": 60}, nil, "a->c", "b->c", "c->d", "iso")
		assert.Equal(t, layers, out)
		assert.Equal(t, at(t, lv, out, "a"), at(t, lv, out, "iso"))
	})
	t.Run("an edge passing a row takes its room", func(t *testing.T) {
		lv, _, out := isolated(t, nil, nil, "a->c", "b->c", "c->d", "a->d", "iso")
		assert.Equal(t, at(t, lv, out, "d"), at(t, lv, out, "iso"), "a->d's dummy leaves c's row 72 of room, d's has 120")
	})
	t.Run("a node placed counts in its row", func(t *testing.T) {
		lv, _, out := isolated(t, nil, nil, "a->c", "b->c", "iso1", "iso2")
		assert.Equal(t, at(t, lv, out, "c"), at(t, lv, out, "iso1"), "the first takes c's room")
		assert.Equal(t, at(t, lv, out, "a"), at(t, lv, out, "iso2"), "the second finds none and stays")
	})
	t.Run("a node that adopted its previous layer keeps it and counts there", func(t *testing.T) {
		lv, _, out := isolated(t, nil, []string{"iso1"}, "a->c", "b->c", "iso1", "iso2")
		assert.Equal(t, at(t, lv, out, "a"), at(t, lv, out, "iso1"), "kept on layer 0, where row [a b iso1] is 320 wide")
		assert.Equal(t, at(t, lv, out, "c"), at(t, lv, out, "iso2"), "so c's row has 240 of room")
	})
	// given lays the level specs give out on the layers given, by id, as a
	// previous layout can leave them (S13), kept marking the nodes that
	// adopted theirs, and places its isolated nodes.
	given := func(t *testing.T, layers map[string]int, kept []string, specs ...string) (*lgraph.Level, []int) {
		lv := lgraphtest.Level(t, specs...)
		in := make([]int, len(lv.Nodes))
		adopted := make([]bool, len(lv.Nodes))
		for i, n := range lv.Nodes {
			in[i] = layers[n.ID]
			adopted[i] = slices.Contains(kept, n.ID)
		}
		ctx := context.Background()
		return lv, Isolated(ctx, lv, cycle.Break(ctx, lv, nil), in, adopted, room)
	}
	t.Run("the layer an isolated node leaves closes", func(t *testing.T) {
		// iso leaves layer 0 for c's row, which has room: the layers renumber
		lv, out := given(t, map[string]int{"a": 1, "b": 1, "c": 2, "iso": 0}, nil, "a->c", "b->c", "iso")
		assert.Equal(t, []int{0, 0, 1, 1}, []int{at(t, lv, out, "a"), at(t, lv, out, "b"), at(t, lv, out, "c"), at(t, lv, out, "iso")})
	})
	t.Run("a layer only dummies hold is no row and closes", func(t *testing.T) {
		// a->d and b->d pass layer 1, where iso sits: their dummies make no
		// row, so iso joins d's, and layer 1 closes
		lv, out := given(t, map[string]int{"a": 0, "b": 0, "d": 2, "iso": 1}, nil, "a->d", "b->d", "iso")
		assert.Equal(t, []int{0, 0, 1, 1}, []int{at(t, lv, out, "a"), at(t, lv, out, "b"), at(t, lv, out, "d"), at(t, lv, out, "iso")})
	})
	t.Run("a node that adopted a deeper layer keeps it and counts there", func(t *testing.T) {
		// placed afresh, iso1 would take c's row, the earlier of the two
		// with the most room; kept beside d, it fills d's row, and iso2
		// takes c's
		lv, out := given(t, map[string]int{"a": 0, "b": 0, "c": 1, "d": 2, "iso1": 2, "iso2": 0}, []string{"iso1"}, "a->c", "b->c", "c->d", "iso1", "iso2")
		assert.Equal(t, []int{1, 2, 2, 1}, []int{at(t, lv, out, "c"), at(t, lv, out, "d"), at(t, lv, out, "iso1"), at(t, lv, out, "iso2")})
	})
	t.Run("a self-loop is an edge", func(t *testing.T) {
		_, layers, out := isolated(t, nil, nil, "a->c", "b->c", "s->s")
		assert.Equal(t, layers, out)
	})
	t.Run("a level of isolated nodes stays on one row", func(t *testing.T) {
		_, layers, out := isolated(t, nil, nil, "x", "y", "z")
		assert.Equal(t, []int{0, 0, 0}, out)
		assert.Equal(t, layers, out)
	})
	t.Run("a terminal row is no row", func(t *testing.T) {
		lv := lgraphtest.Level(t, "t->a", "a->b", "b->u", "iso")
		for _, id := range []int{0, 3} {
			lv.Nodes[id].Terminal, lv.Nodes[id].W, lv.Nodes[id].H = true, 8, 0
		}
		lv.Nodes[4].W, lv.Nodes[4].H = 20, 0 // it would fit a terminal row, 72 of room and no height
		ctx := context.Background()
		reversed := cycle.Break(ctx, lv, nil)
		layers := Assign(ctx, lv, reversed, nil)
		out := Isolated(ctx, lv, reversed, layers, nil, room)
		assert.Equal(t, layers, out, "rows [a] and [b] have no room, and the terminal rows are none")
		assert.Equal(t, at(t, lv, out, "a"), at(t, lv, out, "iso"))
	})
}

// TestIsolated_LogsEachPlacement pins S3's debug record: one
// isolated_placed per isolated node that adopted no layer.
func TestIsolated_LogsEachPlacement(t *testing.T) {
	var buf bytes.Buffer
	ctx := layoutdbg.NewContext(context.Background(), slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	lv := lgraphtest.Level(t, "a->c", "b->c", "iso1", "iso2")
	reversed := cycle.Break(ctx, lv, nil)
	Isolated(ctx, lv, reversed, Assign(context.Background(), lv, reversed, nil), nil, room)
	var got []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		if entry["decision"] == "isolated_placed" {
			got = append(got, map[string]any{"node": entry["node"], "layer": entry["layer"], "fits": entry["fits"]})
		}
	}
	assert.Equal(t, []map[string]any{
		{"node": "iso1", "layer": 1.0, "fits": true},
		{"node": "iso2", "layer": 0.0, "fits": false},
	}, got)
}
