package route

import (
	"math"
	"testing"

	"github.com/oxforge/diago/internal/layout/layered/lgraph"
	"github.com/oxforge/diago/internal/layout/layered/lgraph/lgraphtest"
	"github.com/oxforge/diago/internal/layout/layered/ports"
	"github.com/oxforge/diago/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRoute_ASelfLoopJoinsASideRail pins S8's side attachments: a pinned
// node's self-loop that shares a side vertex with another end is on its
// side rail. Counter-flow ends hold both of m's side vertices, so its loop
// takes the right one (240), with m->t's end, whose column would move out
// over its dummy (265), 5 px from the loop's column Reach out (260): it
// stays on the loop's column and rises past the loop's lane, its dummy
// and t's in-port coming onto it (Stops).
func TestRoute_ASelfLoopJoinsASideRail(t *testing.T) {
	lv := lgraphtest.Level(t, "s", "m:diamond->m", "m->t", "m->s")
	// layers: s and t 0, m 2; m->t and m->s reversed, each through a dummy
	g, err := lgraph.Build(lv, []int{0, 2, 0}, []bool{false, true, true}, 8)
	require.NoError(t, err)
	_, route := routed(t, g, screen, map[string]float64{"s": 100, "m->s#0@1": 100, "m->t#0@1": 265, "t": 265, "m": 200})
	assertRoute(t, pts(240, 148, 260, 148, 260, 104, 200, 104, 200, 128), route("m->m#0"))
	assertRoute(t, pts(240, 148, 260, 148, 260, 40), route("m->t#0"))
	assertRoute(t, pts(160, 148, 100, 148, 100, 40), route("m->s#0"))
}

// TestRoute_ALoopedSideRailStaysReachOut pins S8's ports on a side rail
// that a self-loop runs on: the loop rises along the column Reach out, so
// the rail does not move. m->t's end shares m's right vertex with m's
// loop (TestRoute_ASelfLoopJoinsASideRail), on the column 260, 6 px from
// q->w's dummy column (266) in the channel above m: it stays there.
func TestRoute_ALoopedSideRailStaysReachOut(t *testing.T) {
	lv := lgraphtest.Level(t, "s", "m:diamond->m", "m->s", "m->t", "q->w")
	lv.Nodes[2].W, lv.Nodes[3].W = 10, 10 // t, q
	// layers: s, t and q 0, the dummies 1, m and w 2; m->s and m->t reversed
	g, err := lgraph.Build(lv, []int{0, 2, 0, 0, 2}, []bool{false, true, true, false}, 8)
	require.NoError(t, err)
	_, route := routed(t, g, screen, map[string]float64{
		"s": 100, "t": 250, "q": 290, "m->s#0@1": 100, "m->t#0@1": 250, "q->w#0@1": 266, "m": 200, "w": 400,
	})
	mm, mt := route("m->m#0"), route("m->t#0")
	assert.InDelta(t, 260, mm[1].X, 1e-9, "the loop's column: %v", mm)
	assert.Equal(t, model.Point{X: 240, Y: mt[0].Y}, mt[0], "m->t leaves m's right vertex: %v", mt)
	assert.InDelta(t, 260, mt[1].X, 1e-9, "m->t's column, the loop's: %v", mt)
}

func TestRoute_ASelfLoopOnARect(t *testing.T) {
	g := graph(t, nil, "a->a", "a->b")
	_, route := routed(t, g, screen, map[string]float64{"a": 100, "b": 100})
	// out a quarter down the right face, Reach outside, back three
	// quarters down
	assertRoute(t, pts(140, 10, 160, 10, 160, 30, 140, 30), route("a->a#0"))
}

// TestRoute_SelfLoopsNestOnTheRightFace pins S8's self-loops: of n loops
// on a node with faces, loop j leaves h/4 + j LaneGap down, runs Reach +
// (n-1-j) InLaneGap outside the right face and returns 3h/4 - j LaneGap
// down, the first in edge order outermost. a is 80 long, (4n - 2)
// LaneGaps for its three loops.
func TestRoute_SelfLoopsNestOnTheRightFace(t *testing.T) {
	g := shaped(t, func(lv *lgraph.Level) { lv.Nodes[0].H = 80 }, "a->a", "a->a", "a->a", "a->b")
	_, route := routed(t, g, screen, map[string]float64{"a": 100, "b": 100})
	assertRoute(t, pts(140, 20, 176, 20, 176, 60, 140, 60), route("a->a#0"))
	assertRoute(t, pts(140, 28, 168, 28, 168, 52, 140, 52), route("a->a#1"))
	assertRoute(t, pts(140, 36, 160, 36, 160, 44, 140, 44), route("a->a#2"))
}

// TestRoute_NestedLoopsStayOnTheUsableSpan pins S8's self-loops: nested
// loops step inward, between the outer loop's ports, so every port stays
// in the middle half of the right face, on its drawn outline and inside
// its usable span. c is a cylinder 48 long with two loops: under DOWN the
// right face is a side line, whose caps take 0.18 of the length at each
// end; under RIGHT it is the lower cap's arc.
func TestRoute_NestedLoopsStayOnTheUsableSpan(t *testing.T) {
	for _, dir := range []model.Direction{model.Down, model.Right} {
		g := shaped(t, func(lv *lgraph.Level) { lv.Nodes[0].H = 48 }, "c:cylinder->c", "c->c", "c->b")
		o := screen
		o.Dir = dir
		_, route := routed(t, g, o, map[string]float64{"c": 100, "b": 100})
		outer, inner := route("c->c#0"), route("c->c#1")
		for _, loop := range [][]model.Point{outer, inner} {
			for _, p := range []model.Point{loop[0], loop[len(loop)-1]} {
				off := p.Y - 24
				assert.LessOrEqual(t, math.Abs(off), 12.0, "%s: in the middle half: %v", dir, loop)
				if dir == model.Down {
					assert.LessOrEqual(t, math.Abs(off), 24-ports.CapRatio*48, "on the side line, off the caps: %v", loop)
				}
				assert.InDelta(t, 140-ports.Inset(model.ShapeCylinder, 80, 48, dir, false, ports.FaceRight, off), p.X, 1e-9, "%s: on the outline: %v", dir, loop)
			}
		}
		assert.Greater(t, inner[0].Y, outer[0].Y, "%s: the inner loop leaves below the outer one: %v %v", dir, outer, inner)
		assert.Less(t, inner[len(inner)-1].Y, outer[len(outer)-1].Y, "%s: and returns above it: %v %v", dir, outer, inner)
		assert.Less(t, inner[1].X, outer[1].X, "%s: its leg runs inside the outer one's: %v %v", dir, outer, inner)
	}
}

func TestRoute_ASelfLoopOnADiamondRisesToTheInVertex(t *testing.T) {
	g := graph(t, nil, "m:diamond->m", "m->a")
	_, route := routed(t, g, screen, map[string]float64{"m": 200, "a": 200})
	// the right vertex, Reach out, up into a lane above the row (half a
	// channel's pad above layer 0) and down into the in-vertex
	assertRoute(t, pts(240, 20, 260, 20, 260, -24, 200, -24, 200, 0), route("m->m#0"))

	g = graph(t, nil, "m:diamond->m", "m->b", "m->c")
	_, route = routed(t, g, screen, map[string]float64{"m": 200, "b": 400, "c": 200})
	// m->b holds the right vertex, so the loop takes the free left one
	assertRoute(t, pts(160, 20, 140, 20, 140, -24, 200, -24, 200, 0), route("m->m#0"))
}

func TestRoute_ABackEdgeTakesTheSideAwayFromASelfLoop(t *testing.T) {
	g := graph(t, nil, "a->a", "a->b", "b->a")
	// b sits right of a, but a's right face is its loop's: a one-hop back
	// edge takes the left side of both
	_, route := twice(t, g, screen, map[string]float64{"a": 100, "b": 110})
	ba := route("b->a#0")
	assert.Equal(t, model.Point{X: 60, Y: 20}, ba[len(ba)-1], "a's left face at mid-height")
}

func TestRoute_ALongBackEdgeLeavesASelfLoopsFace(t *testing.T) {
	g := graph(t, nil, "a->a", "a->b", "b->c", "c->a")
	// c->a's column clears a's right face, but the loop runs there
	_, route := twice(t, g, screen, map[string]float64{"a": 100, "b": 100, "c": 100, "c->a#0@1": 250})
	ca := route("c->a#0")
	assert.Equal(t, 40.0, ca[len(ca)-1].Y, "c->a enters a's bottom face")
}

func TestRoute_ASelfLoopBlocksANeighborsSideColumn(t *testing.T) {
	g := graph(t, nil, "a->a", "n->m", "m->n")
	// n's left column, Reach outside it, would run on a's loop leg
	_, route := twice(t, g, screen, map[string]float64{"a": 100, "n": 220, "m": 220})
	mn := route("m->n#0")
	assert.Equal(t, 40.0, mn[len(mn)-1].Y, "m->n enters n's bottom face")
}

// TestRoute_NestedSelfLoopsBlockANeighborsSideColumn pins S8's side
// attachments: a node with n self-loops reaches as far as its outermost
// leg, Reach + (n-1) InLaneGap out. a's two loops put that leg at 168; n's
// left column, Reach outside it at 176, would come within the node
// clearance of it, though one loop's leg at 160 leaves room.
func TestRoute_NestedSelfLoopsBlockANeighborsSideColumn(t *testing.T) {
	g := shaped(t, func(lv *lgraph.Level) { lv.Nodes[0].H = 48 }, "a->a", "a->a", "n->m", "m->n")
	_, route := twice(t, g, screen, map[string]float64{"a": 100, "n": 236, "m": 236})
	mn := route("m->n#0")
	assert.Equal(t, 40.0, mn[len(mn)-1].Y, "m->n enters n's bottom face: %v", mn)
}

func TestRoute_APinnedSelfLoopBlocksANeighborsSideColumn(t *testing.T) {
	g := graph(t, nil, "u->m:diamond", "m->m", "u->n", "n->u")
	// m's right vertex column, Reach outside it, is where its own loop
	// runs; n->u's side attachment on n's end would otherwise land on
	// that same column and run right through the loop's leg. Replayed,
	// n->u leaves u's side face and is no member of u->n's bundle: the
	// channel holds three lanes.
	r, route := twice(t, g, screen, map[string]float64{"u": 370, "m": 200, "n": 320})
	assertRoute(t, pts(240, 124, 260, 124, 260, 72, 200, 72, 200, 104), route("m->m#0"))
	nu := route("n->u#0")
	for _, p := range nu {
		assert.GreaterOrEqual(t, math.Abs(p.X-260), screen.LaneGap, "n->u keeps a lane gap from m's loop column: %v", nu)
	}
	assert.Equal(t, ports.Bottom, r.Side[3][ports.Tail], "n keeps its face port instead of a blocked side column")
}
