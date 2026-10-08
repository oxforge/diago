package route

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/layout/layered/cycle"
	"github.com/oxforge/diago/internal/layout/layered/lgraph"
	"github.com/oxforge/diago/internal/layout/layered/lgraph/lgraphtest"
	"github.com/oxforge/diago/internal/layout/layered/ports"
	"github.com/oxforge/diago/internal/layout/layered/rank"
	"github.com/oxforge/diago/internal/layoutdbg"
	"github.com/oxforge/diago/internal/model"
)

var screen = Options{Clearance: 12, Span: 16, Reach: 20, BaseGap: 40, LaneGap: 8, InLaneGap: 8, Snap: 0.5, Dir: model.Down}

// graph runs S2 and S3 over a fixture level and builds its layered graph
// in the declaration seed; widths overrides node widths by id. Nodes are
// 80 x 40 (lgraphtest).
func graph(t *testing.T, widths map[string]float64, specs ...string) *lgraph.Graph {
	t.Helper()
	lv := lgraphtest.Level(t, specs...)
	for i := range lv.Nodes {
		if w, ok := widths[lv.Nodes[i].ID]; ok {
			lv.Nodes[i].W = w
		}
	}
	ctx := context.Background()
	reversed := cycle.Break(ctx, lv, nil)
	g, err := lgraph.Build(lv, rank.Assign(ctx, lv, reversed, nil), reversed, 8)
	require.NoError(t, err)
	return g
}

// routed places every vertex of g at xs[its id] and routes g; it returns
// the result and an accessor of an edge's route by id.
func routed(t *testing.T, g *lgraph.Graph, o Options, xs map[string]float64) (*Result, func(id string) []model.Point) {
	t.Helper()
	x := make([]float64, len(g.Vertices))
	for v, vx := range g.Vertices {
		p, ok := xs[vx.ID]
		require.True(t, ok, "no x for %s", vx.ID)
		x[v] = p
	}
	r := Route(context.Background(), g, x, o)
	return r, func(id string) []model.Point {
		for e, ed := range g.Level.Edges {
			if ed.ID == id {
				return r.Edges[e]
			}
		}
		require.Failf(t, "no edge", "%s", id)
		return nil
	}
}

// twice routes g, then replays its side attachments, as Arrange's second
// pass does (without placing again): the attached ends leave their faces.
func twice(t *testing.T, g *lgraph.Graph, o Options, xs map[string]float64) (*Result, func(id string) []model.Point) {
	t.Helper()
	first, _ := routed(t, g, o, xs)
	o.Side = first.Side
	return routed(t, g, o, xs)
}

func pts(xy ...float64) []model.Point {
	out := make([]model.Point, 0, len(xy)/2)
	for i := 0; i+1 < len(xy); i += 2 {
		out = append(out, model.Point{X: xy[i], Y: xy[i+1]})
	}
	return out
}

func assertRoute(t *testing.T, want, got []model.Point) {
	t.Helper()
	require.Len(t, got, len(want), "%v", got)
	for i := range want {
		assert.InDelta(t, want[i].X, got[i].X, 1e-6, "point %d x of %v", i, got)
		assert.InDelta(t, want[i].Y, got[i].Y, 1e-6, "point %d y of %v", i, got)
	}
}

func TestRoute_AChainRunsStraightBetweenRows(t *testing.T) {
	g := graph(t, nil, "a->b", "b->c")
	r, route := routed(t, g, screen, map[string]float64{"a": 100, "b": 100, "c": 100})
	assert.Equal(t, []float64{0, 80, 160}, r.RowTop, "a row, then BaseGap and no lanes")
	assert.Equal(t, []float64{40, 40, 40}, r.RowH)
	assert.Equal(t, []int{0, 0}, r.Lanes)
	assertRoute(t, pts(100, 40, 100, 80), route("a->b#0"))
	assertRoute(t, pts(100, 120, 100, 160), route("b->c#0"))
}

func TestRoute_ARowIsItsTallestNodeAndEveryNodeStartsAtItsTop(t *testing.T) {
	lv := lgraphtest.Level(t, "a->c", "b->d")
	lv.Nodes[2].H = 60 // b
	ctx := context.Background()
	g, err := lgraph.Build(lv, rank.Assign(ctx, lv, nil, nil), nil, 8)
	require.NoError(t, err)
	r, route := routed(t, g, screen, map[string]float64{"a": 100, "b": 300, "c": 100, "d": 300})
	assert.Equal(t, []float64{60, 40}, r.RowH)
	assert.Equal(t, []float64{0, 100}, r.RowTop)
	assertRoute(t, pts(100, 40, 100, 100), route("a->c#0"))
	assertRoute(t, pts(300, 60, 300, 100), route("b->d#0"))
}

func TestRoute_PortsSpreadEvenlyInTheOrderOfTheirFarEnds(t *testing.T) {
	g := graph(t, nil, "a->b", "a->c", "a->d")
	r, route := routed(t, g, screen, map[string]float64{"a": 200, "b": 100, "c": 200, "d": 300})
	// three ends on an 80 wide face: at 1/4, 2/4 and 3/4 of it
	assertRoute(t, pts(180, 40, 180, 64, 100, 64, 100, 88), route("a->b#0"))
	assertRoute(t, pts(200, 40, 200, 88), route("a->c#0"))
	assertRoute(t, pts(220, 40, 220, 64, 300, 64, 300, 88), route("a->d#0"))
	assert.Equal(t, []int{1}, r.Lanes, "the two jogs share a lane, InLaneGap apart and more")
}

func TestRoute_ALaneOrderAvoidsACrossing(t *testing.T) {
	// a->d spans b's port; b->c leaves beyond a->d's end. b->c must run
	// above a->d, or its drop at b would cross a->d's lane.
	g := graph(t, nil, "a->d", "b->c")
	r, route := routed(t, g, screen, map[string]float64{"a": 100, "d": 500, "b": 300, "c": 600})
	assert.Equal(t, []int{2}, r.Lanes)
	// a channel of 40 + 2 x 8, its two lanes centered: at 64 and 72
	assertRoute(t, pts(300, 40, 300, 64, 600, 64, 600, 96), route("b->c#0"))
	assertRoute(t, pts(100, 40, 100, 72, 500, 72, 500, 96), route("a->d#0"))
	for _, cs := range Crossings(r.Edges, 5) {
		assert.Empty(t, cs)
	}
}

func TestRoute_ALongEdgeWithinTheSpanIsStraightened(t *testing.T) {
	g := graph(t, nil, "a->b", "b->c", "a->c")
	// a's two out-ends sit at 1/3 and 2/3 of its face, c's two in-ends too,
	// so a->c's ports line up at 100 + 40/3; its dummy, 10 off, is pulled
	// onto them.
	_, route := routed(t, g, screen, map[string]float64{"a": 100, "b": 0, "c": 100, "a->c#0@1": 110})
	port := 100 + 40.0/3
	assertRoute(t, pts(port, 40, port, 176), route("a->c#0"))
}

// TestRoute_APortSlidesToStraightenAWire pins S8's stops: a wire whose
// stops spread no wider than Span runs straight on its in-port's x, a
// port sliding along its face to it, or else on its out-port's. A pinned
// node's vertex never slides: the other end comes to it.
func TestRoute_APortSlidesToStraightenAWire(t *testing.T) {
	for _, tc := range []struct {
		name, spec, id string
		xs             map[string]float64
		want           []model.Point
	}{
		{"the out-port slides onto the in-port", "a->b", "a->b#0", map[string]float64{"a": 100, "b": 106}, pts(106, 40, 106, 80)},
		{"a pinned in-vertex stays", "a->m:diamond", "a->m#0", map[string]float64{"a": 106, "m": 100}, pts(100, 40, 100, 80)},
		{"a pinned bottom vertex stays", "m:diamond->a", "m->a#0", map[string]float64{"m": 100, "a": 106}, pts(100, 40, 100, 80)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, route := routed(t, graph(t, nil, tc.spec), screen, tc.xs)
			assertRoute(t, tc.want, route(tc.id))
			assert.Equal(t, []int{0}, r.Lanes, "no jog, no lane")
		})
	}
}

// TestRoute_APortSlidesOnlyOverItsFace pins S8's stops: a port slides
// only over the middle nine tenths of its face's usable span, and never
// past another end of its node; where the in-port's x is out of the
// out-port's reach, the in-port slides to the out-port's instead.
func TestRoute_APortSlidesOnlyOverItsFace(t *testing.T) {
	// a 60 wide hexagon's flat bottom spans ±15, ±13.5 usable: 114 lies
	// off it, so b's in-port comes to h's out-port
	g := graph(t, map[string]float64{"h": 60}, "h:hexagon->b")
	_, route := routed(t, g, screen, map[string]float64{"h": 100, "b": 114})
	assertRoute(t, pts(100, 40, 100, 80), route("h->b#0"))

	// under a Span of 40, a's out-port at 86.7 would pass its other one, at
	// 113.3, on its way to 125: 100 wide b's in-port comes to it
	o := screen
	o.Span = 40
	g = graph(t, map[string]float64{"b": 100}, "a->b", "a->c")
	_, route = routed(t, g, o, map[string]float64{"a": 100, "b": 125, "c": 300})
	first := 100 - 40.0/3 // a->c's jog takes a lane: b's row starts at 88
	assertRoute(t, pts(first, 40, first, 88), route("a->b#0"))
}

// TestRoute_APortSlidesOnlyClearOfOtherColumns pins S8's stops: a port
// slides only to an x no other node's port or side column, nor a dummy
// column, holds within InLaneGap in its channel. a->m's in-vertex on
// diamond m never moves, so a's port would slide onto it, 6 px from the
// dummy column of s->t: the jog stays, and takes a lane. With the dummy
// 14 px off, a's port slides.
func TestRoute_APortSlidesOnlyClearOfOtherColumns(t *testing.T) {
	lv := lgraphtest.Level(t, "a->m:diamond", "s->t")
	// layers: a and s 0, m 1, t 2; s->t runs through m's row at 112
	g, err := lgraph.Build(lv, []int{0, 1, 0, 2}, nil, 8)
	require.NoError(t, err)
	xs := map[string]float64{"a": 100, "m": 106, "s": 300, "t": 112, "s->t#0@1": 112}
	r, route := routed(t, g, screen, xs)
	assertRoute(t, pts(100, 40, 100, 64, 106, 64, 106, 96), route("a->m#0"))
	assert.Equal(t, []int{2, 0}, r.Lanes, "a->m's jog and s->t's")

	xs["s->t#0@1"], xs["t"] = 120, 120
	r, route = routed(t, g, screen, xs)
	assertRoute(t, pts(106, 40, 106, 88), route("a->m#0"))
	assert.Equal(t, []int{1, 0}, r.Lanes, "s->t's jog alone")
}

// TestRoute_APortSlidesOnlyHalfTheInLaneGapFromItsNodesOtherEnds pins
// S8's stops: a port slides only to an x that keeps InLaneGap / 2 from
// every other end of its node, even one it does not pass. Group g's port
// for g->p is anchored at 10 (S9), so it never gives way; diamond q's
// in-vertex never moves. g->q's port, at 0, would slide right onto q's
// vertex: at 7, 3 px short of the anchored port, it stays and g->q jogs in
// a lane; at 5, 5 px short, it slides and g->q runs straight.
func TestRoute_APortSlidesOnlyHalfTheInLaneGapFromItsNodesOtherEnds(t *testing.T) {
	for _, tc := range []struct {
		q     float64
		want  []model.Point
		lanes []int
	}{
		{7, pts(0, 200, 0, 232, 7, 232, 7, 256), []int{2}},
		{5, pts(5, 200, 5, 248), []int{1}},
	} {
		g := shaped(t, func(lv *lgraph.Level) {
			lv.Nodes[0].Group, lv.Nodes[0].W, lv.Nodes[0].H = true, 300, 200
			lv.Edges[0].Anchor[0] = lgraph.Anchor{On: true, At: 10}
		}, "g->p", "g->q:diamond")
		r, route := routed(t, g, screen, map[string]float64{"g": 0, "p": -200, "q": tc.q})
		assertRoute(t, tc.want, route("g->q#0"))
		assert.Equal(t, tc.lanes, r.Lanes, "q at %g", tc.q)
	}
}

// TestRoute_ATextPortSlidesOnlyInsideTheCorners pins S8's stops in the
// text profile: a port slides only onto a cell inside its box's corners.
// Under RIGHT, seven rows tall a's five out-ports fill the rows between
// its corners, a->e's at 12.5, a row short of e's in-port at 13.5: onto
// it a's port would reach its border, so e's in-port slides to 12.5
// instead, a row from a->d's out-port, the in-lane gap there.
func TestRoute_ATextPortSlidesOnlyInsideTheCorners(t *testing.T) {
	sideways := Options{Clearance: 1, Span: 1, Reach: 2, BaseGap: 4, LaneGap: 2, InLaneGap: 1, Snap: 0.5, Dir: model.Right, Text: true}
	cells := map[string]float64{"a": 7, "b": 7, "c": 7, "d": 7, "e": 7, "z": 7}
	g := graph(t, cells, "a->z", "a->b", "a->c", "a->d", "a->e")
	for i := range g.Level.Nodes {
		g.Level.Nodes[i].H = 7
	}
	_, route := routed(t, g, sideways, map[string]float64{"a": 10.5, "z": -30.5, "b": -21.5, "c": -12.5, "d": 3.5, "e": 13.5})
	ae := route("a->e#0")
	require.Len(t, ae, 2, "straight: %v", ae)
	assert.Equal(t, 12.5, ae[0].X, "%v", ae)
}

// TestRoute_AStraightenedStopKeepsClearOfALoopLeg pins S8's stops: a
// dummy stop moves only to an x that keeps InLaneGap from every self-loop
// leg of its row. s->c's dummy sits right of l at 176; c's in-port at 163
// lies within Span of it, but 3 px from l's loop leg at 160, a face loop's
// leg or the column a diamond's loop rises along. The dummy keeps its
// column whether the walk up or the pull onto both ports' x (s at 163)
// would move it. Mirrored, the dummy sits left of diamond l at 24, whose
// loop takes the left vertex since l->u's leaving end holds the right one
// (Self-loops), and rises along the column at 40, 3 px from c's in-port
// at 37: the dummy keeps its column the same way.
func TestRoute_AStraightenedStopKeepsClearOfALoopLeg(t *testing.T) {
	for _, tc := range []struct {
		name string
		l    string
		s    float64
	}{
		{"walk up, face loop", "l", 176},
		{"pull, face loop", "l", 163},
		{"walk up, diamond loop", "l:diamond", 176},
		{"pull, diamond loop", "l:diamond", 163},
	} {
		lv := lgraphtest.Level(t, tc.l+"->l", "l->c", "s->c")
		lv.Nodes[1].W = 60 // c: in-ports at 143 and 163
		// layers: s 0, l 1, c 2
		g, err := lgraph.Build(lv, []int{1, 2, 0}, nil, 8)
		require.NoError(t, err)
		res, route := routed(t, g, screen, map[string]float64{"l": 100, "c": 153, "s": tc.s, "s->c#0@1": 176})
		assertThrough(t, res, route("s->c#0"), 176, tc.name)
	}
	for _, tc := range []struct {
		name string
		s    float64
	}{
		{"mirrored walk up, diamond loop on the left", 24},
		{"mirrored pull, diamond loop on the left", 37},
	} {
		lv := lgraphtest.Level(t, "s", "l:diamond->l", "l->c", "s->c", "l->u")
		lv.Nodes[2].W = 60 // c: in-ports at 37 and 57
		// layers: s and u 0, l 1, c 2; l->u reversed
		g, err := lgraph.Build(lv, []int{0, 1, 2, 0}, []bool{false, false, false, true}, 8)
		require.NoError(t, err)
		g.Layers[1][0], g.Layers[1][1] = g.Layers[1][1], g.Layers[1][0] // the dummy left of l (S6)
		res, route := routed(t, g, screen, map[string]float64{"l": 100, "c": 47, "s": tc.s, "s->c#0@1": 24, "u": 200})
		ll := route("l->l#0")
		require.InDelta(t, 40, ll[1].X, 1e-9, "%s: l's loop rises along the column left of l: %v", tc.name, ll)
		assertThrough(t, res, route("s->c#0"), 24, tc.name)
	}
}

// TestRoute_AStraightenedStopKeepsClearOfALoopLegInText pins S8's stops
// in the text profile, where the rule's first repro was (the probe's seed
// 218 under RIGHT): a dummy stop moves only to an x that keeps InLaneGap,
// two cells, from every self-loop leg of its row. l's loop leg runs Reach
// out of its right face and half a cell farther, on a cell's middle, at
// 16.5 (Self-loops); s->c's dummy sits beside l at 18.5, two cells from
// it, and c's in-port at 17.5 lies within Span of it, but one cell from
// the leg. The dummy keeps its column whether the walk up or the pull
// onto both ports' x (s at 17.5) would move it. Under RIGHT and LEFT,
// where the engine's cross axis runs along the art's rows (S14: Reach two
// rows, InLaneGap one), the leg runs at 15.5 and the dummy at 16.5: c's
// in-port on the leg's own row is closer than InLaneGap, and the dummy
// keeps its row, while one row off the leg, at 16.5 with the dummy at
// 17.5, it moves.
func TestRoute_AStraightenedStopKeepsClearOfALoopLegInText(t *testing.T) {
	sideways := Options{Clearance: 1, Span: 1, Reach: 2, BaseGap: 4, LaneGap: 2, InLaneGap: 1, Snap: 0.5, Dir: model.Right, Text: true}
	for _, tc := range []struct {
		name             string
		o                Options
		leg, dummy, c, s float64
		through          float64 // where s->c runs through l's row
	}{
		{"walk up", text, 16.5, 18.5, 17.5, 18.5, 18.5},
		{"pull", text, 16.5, 18.5, 17.5, 17.5, 18.5},
		{"sideways, walk up", sideways, 15.5, 16.5, 15.5, 16.5, 16.5},
		{"sideways, pull", sideways, 15.5, 16.5, 15.5, 15.5, 16.5},
		{"sideways, a row off the leg", sideways, 15.5, 17.5, 16.5, 16.5, 16.5},
	} {
		lv := lgraphtest.Level(t, "l->l", "s->c")
		for i := range lv.Nodes {
			lv.Nodes[i].W, lv.Nodes[i].H = 5, 3
		}
		lv.Nodes[0].W = 6 // l: its right face at 13
		// layers: s 0, l 1, c 2
		g, err := lgraph.Build(lv, []int{1, 0, 2}, nil, 1)
		require.NoError(t, err)
		res, route := routed(t, g, tc.o, map[string]float64{"l": 10, "s": tc.s, "s->c#0@1": tc.dummy, "c": tc.c})
		ll := route("l->l#0")
		require.InDelta(t, tc.leg, ll[1].X, 1e-9, "%s: l's loop leg: %v", tc.name, ll)
		assertThrough(t, res, route("s->c#0"), tc.through, tc.name)
	}
}

// assertThrough asserts that wire passes the row of layer 1 (res's) on a
// vertical at x.
func assertThrough(t *testing.T, res *Result, wire []model.Point, x float64, name string) {
	t.Helper()
	top, bottom := res.RowTop[1], res.RowTop[1]+res.RowH[1]
	through := false
	for i := 1; i < len(wire); i++ {
		p, q := wire[i-1], wire[i]
		if p.X == q.X && math.Min(p.Y, q.Y) <= top && math.Max(p.Y, q.Y) >= bottom {
			through = true
			assert.Equal(t, x, p.X, "%s: through l's row on %v: %v", name, x, wire)
		}
	}
	assert.True(t, through, "%s: %v", name, wire)
}

// TestRoute_AStraightenedStopKeepsClearOfAnotherEdgesColumn pins S8's
// stops: a dummy stop moves only to an x that no other edge holds within
// InLaneGap in the stop's channels. q->p leaves diamond p's right vertex
// and runs down the column at 260 into the channel below p's row. s->c's
// first dummy, at 280 beside p, would move onto its second dummy at 266,
// within Span but 6 px from that column: it keeps its column, and s->c
// jogs in a lane below q->p's (24 wide c has no room to slide its in-port
// 14 px out, onto 280).
func TestRoute_AStraightenedStopKeepsClearOfAnotherEdgesColumn(t *testing.T) {
	lv := lgraphtest.Level(t, "s->c", "q->p:diamond")
	lv.Nodes[1].W = 24 // c
	// layers: s 0, p 1, q 2, c 3; q->p reversed
	g, err := lgraph.Build(lv, []int{0, 3, 2, 1}, []bool{false, true}, 8)
	require.NoError(t, err)
	_, route := routed(t, g, screen, map[string]float64{"s": 280, "s->c#0@1": 280, "s->c#0@2": 266, "c": 266, "p": 200, "q": 200})
	sc, qp := route("s->c#0"), route("q->p#0")
	assertRoute(t, pts(280, 40, 280, 152, 266, 152, 266, 256), sc)
	assertRoute(t, pts(200, 176, 200, 144, 260, 144, 260, 100, 240, 100), qp)
}

func TestRoute_StopsWithinSnapAreOneX(t *testing.T) {
	g := graph(t, nil, "a->b")
	_, route := routed(t, g, screen, map[string]float64{"a": 100, "b": 100.3})
	assertRoute(t, pts(100, 40, 100, 80), route("a->b#0"))

	// a pinned end keeps its vertex: the other end moves onto it
	g = graph(t, nil, "a->m:diamond")
	_, route = routed(t, g, screen, map[string]float64{"a": 100.3, "m": 100})
	assertRoute(t, pts(100, 40, 100, 80), route("a->m#0"))
}

func TestRoute_DiamondExitsTakeTheirOwnVertices(t *testing.T) {
	g := graph(t, nil, "m:diamond->a", "m->b", "m->c")
	r, route := routed(t, g, screen, map[string]float64{"m": 200, "a": 100, "b": 200, "c": 300})
	// left, bottom and right in order of heading; a side exit runs out
	// to its column, here over its target's in-port, and drops straight
	assertRoute(t, pts(160, 20, 100, 20, 100, 80), route("m->a#0"))
	assertRoute(t, pts(200, 40, 200, 80), route("m->b#0"))
	assertRoute(t, pts(240, 20, 300, 20, 300, 80), route("m->c#0"))
	assert.Equal(t, []int{0}, r.Lanes)
}

func TestRoute_ASideColumnKeepsReachFromItsVertex(t *testing.T) {
	g := graph(t, nil, "m:diamond->a", "m->b")
	// a sits just left of m: its column cannot come nearer than Reach
	_, route := routed(t, g, screen, map[string]float64{"m": 200, "a": 150, "b": 300})
	got := route("m->a#0")
	assert.InDelta(t, 140, got[1].X, 1e-6, "160 - Reach: %v", got)
}

func TestRoute_ArrivalsShareTheInVertexOnOneLane(t *testing.T) {
	g := graph(t, nil, "a->m:diamond", "b->m")
	r, route := routed(t, g, screen, map[string]float64{"a": 100, "b": 300, "m": 200})
	assert.Equal(t, []int{1}, r.Lanes, "one rail, though the two jogs meet end to end")
	assertRoute(t, pts(100, 40, 100, 64, 200, 64, 200, 88), route("a->m#0"))
	assertRoute(t, pts(300, 40, 300, 64, 200, 64, 200, 88), route("b->m#0"))
}

func TestRoute_ABackEdgeAttachesAtASideVertexAndASideFace(t *testing.T) {
	g := graph(t, nil, "m:diamond->a", "a->m")
	r, route := twice(t, g, screen, map[string]float64{"m": 200, "a": 200})
	// the forward exit keeps the bottom; the back edge leaves a's right
	// face at mid-height and enters m's right vertex (C8.4), both on the
	// column Reach outside them; replayed, a's face holds m->a alone
	assertRoute(t, pts(200, 40, 200, 80), route("m->a#0"))
	assertRoute(t, pts(240, 100, 260, 100, 260, 20, 240, 20), route("a->m#0"))
	assert.Equal(t, [2]ports.Vertex{ports.Right, ports.Right}, r.Side[1], "both ends are recorded: m's side vertex and a's side face")
}

func TestRoute_ASideColumnSlidesOntoItsPartnersColumn(t *testing.T) {
	g := graph(t, map[string]float64{"a": 100}, "m:circle->a", "a->m")
	// m's column would sit at 260, a's at 270: within Span, m's slides out
	_, route := twice(t, g, screen, map[string]float64{"m": 200, "a": 200})
	assertRoute(t, pts(250, 100, 270, 100, 270, 20, 240, 20), route("a->m#0"))
}

// TestRoute_ASideColumnMovesToStraightenAWire pins S8's stops: a wire
// whose stops spread no wider than Span runs straight on one x when every
// other stop may move there, a side column among them. a->m's column
// beside a runs over its dummy at 250, 10 px inside m's column Reach
// outside its right vertex (260): m's column may not move inward, but the
// dummy and a's column move out onto it.
func TestRoute_ASideColumnMovesToStraightenAWire(t *testing.T) {
	lv := lgraphtest.Level(t, "m:diamond", "a", "a->m")
	// layers: m 0, a 2; a->m reversed, through one dummy
	g, err := lgraph.Build(lv, []int{0, 2}, []bool{true}, 8)
	require.NoError(t, err)
	r, route := twice(t, g, screen, map[string]float64{"m": 200, "a->m#0@1": 250, "a": 150})
	assertRoute(t, pts(190, 140, 260, 140, 260, 20, 240, 20), route("a->m#0"))
	assert.Equal(t, []int{0, 0}, r.Lanes)
}

// TestRoute_ASideColumnMovesOnlyWhereItMayToStraightenAWire pins S8's
// stops: a side column moves to straighten a wire only where it stays
// Reach outside its side and keeps its row clear, however close the x it
// would move to. m's exit to a leaves its right vertex (240) on the column
// Reach out (260), over its dummy at 250.
func TestRoute_ASideColumnMovesOnlyWhereItMayToStraightenAWire(t *testing.T) {
	// a's in-port sits at 259.7, within Snap inside the column. The wire
	// runs straight on the column, not on a's port: the column holding at
	// 259.7 would leave a 19.7 px stub, under C7's floor
	lv := lgraphtest.Level(t, "m:diamond->a", "m->b")
	// layers: m 0, a 2, b 1
	g, err := lgraph.Build(lv, []int{0, 2, 1}, nil, 8)
	require.NoError(t, err)
	r, route := routed(t, g, screen, map[string]float64{"m": 200, "m->a#0@1": 250, "a": 259.7, "b": 100})
	got := route("m->a#0")
	assertRoute(t, pts(240, 20, 260, 20, 260, 160), got)
	assert.GreaterOrEqual(t, got[1].X-got[0].X, screen.Reach, "the stub out of m's side: %v", got)
	assert.Equal(t, []int{0, 0}, r.Lanes)

	// a's in-port sits at 260.3, within Snap outside the column, and n in
	// m's row lies 12.1 px from the column: holding at 260.3, the column
	// would come within the node clearance of n, so a's port moves instead
	lv = lgraphtest.Level(t, "m:diamond->a", "m->b", "n")
	// layers: m 0, a 2, b 1, n 0
	g, err = lgraph.Build(lv, []int{0, 2, 1, 0}, nil, 8)
	require.NoError(t, err)
	r, route = routed(t, g, screen, map[string]float64{"m": 200, "m->a#0@1": 250, "a": 260.3, "b": 100, "n": 260 + 12.1 + 40})
	got = route("m->a#0")
	assertRoute(t, pts(240, 20, 260, 20, 260, 160), got)
	assert.Equal(t, []int{0, 0}, r.Lanes)
}

// TestRoute_ADummyRunMovesOntoASideColumn pins S8's stops: when a side
// column may not slide onto its adjacent stop, the run of dummy stops next
// to it, on one x, moves onto the column, when each may move there and
// the stop past the run lies more than Span from the column. Diamond m's
// column sits Reach outside its right vertex (260), beside its dummies at
// 259, onto which it would come 19 px out; a's port, at 240, lies 20 px
// from the column, beyond the 18 px a 40 px face lets it slide, so the
// back edge cannot run straight on the column whole
// (TestRoute_ABackEdgeRunsOnItsPinnedSideColumn).
func TestRoute_ADummyRunMovesOntoASideColumn(t *testing.T) {
	lv := lgraphtest.Level(t, "m:diamond", "a", "a->m")
	lv.Nodes[1].W = 40 // a
	// layers: m 0, a 3; a->m reversed, through two dummies
	g, err := lgraph.Build(lv, []int{0, 3}, []bool{true}, 8)
	require.NoError(t, err)
	r, route := twice(t, g, screen, map[string]float64{"m": 200, "a->m#0@1": 259, "a->m#0@2": 259, "a": 240})
	am := route("a->m#0")
	for i := 1; i+1 < len(am)-1; i++ {
		assert.GreaterOrEqual(t, math.Hypot(am[i+1].X-am[i].X, am[i+1].Y-am[i].Y), 12.0, "segment %d of %v", i, am)
	}
	assert.Equal(t, 260.0, am[len(am)-3].X, "the dummies' run on m's column: %v", am)
	assert.Equal(t, []int{0, 0, 1}, r.Lanes, "one jog: from the column to a's port")

	// m's exit to diamond a leaves its right vertex, on the column Reach out
	// (260); its dummy, 9 px from a's in-vertex at 250, moved onto it, and
	// a's vertex lies within Span of the column: the dummy stays
	lv = lgraphtest.Level(t, "m:diamond->a:diamond", "m->b")
	// layers: m 0, a 2, b 1
	g, err = lgraph.Build(lv, []int{0, 2, 1}, nil, 8)
	require.NoError(t, err)
	r, route = routed(t, g, screen, map[string]float64{"m": 200, "m->a#0@1": 259, "a": 250, "b": 100})
	assertRoute(t, pts(240, 20, 260, 20, 260, 64, 250, 64, 250, 168), route("m->a#0"))
	assert.Equal(t, []int{1, 0}, r.Lanes)
}

// TestRoute_ABackEdgeRunsOnItsPinnedSideColumn pins S8's stops for a
// reversed edge whose end attaches at a pinned node's side vertex: it runs
// straight on that vertex's column, however wide its stops spread, when
// every other stop may move there. a->m's end on diamond m attaches at its
// right vertex (240), whose column runs Reach out (260); its two dummies
// sit at 230, 30 px inside, beyond Span, and a's port at 240 slides onto
// the column (a's 80 px face lets it slide 36 px): one x, no jog, the
// back edge a C into m's side vertex. With n, 40 wide, in the first
// dummy's row at 285, the column would come within the node clearance of
// n's box (253 to 317): the run stays off it, on a's port within Span,
// and the wire jogs onto the column.
func TestRoute_ABackEdgeRunsOnItsPinnedSideColumn(t *testing.T) {
	lv := lgraphtest.Level(t, "m:diamond", "a", "a->m")
	// layers: m 0, a 3; a->m reversed, through two dummies
	g, err := lgraph.Build(lv, []int{0, 3}, []bool{true}, 8)
	require.NoError(t, err)
	r, route := twice(t, g, screen, map[string]float64{"m": 200, "a->m#0@1": 230, "a->m#0@2": 230, "a": 240})
	am := route("a->m#0")
	for _, p := range am[1 : len(am)-1] {
		assert.Equal(t, 260.0, p.X, "on m's column: %v", am)
	}
	assert.Equal(t, []int{0, 0, 0}, r.Lanes, "no jog")

	lv = lgraphtest.Level(t, "m:diamond", "a", "a->m", "n")
	lv.Nodes[2].W = 40 // n
	// layers: m 0, a 3, n 1
	g, err = lgraph.Build(lv, []int{0, 3, 1}, []bool{true}, 8)
	require.NoError(t, err)
	r, route = twice(t, g, screen, map[string]float64{"m": 200, "a->m#0@1": 230, "a->m#0@2": 230, "a": 240, "n": 285})
	am = route("a->m#0")
	assert.NotEqual(t, []int{0, 0, 0}, r.Lanes, "a jog: %v", am)
	assert.Equal(t, 240.0, am[1].X, "the run on a's port, off m's column: %v", am)
}

// TestRoute_ADummyRunMovesOntoASideColumnOnlyWhereItIsFree pins S8's
// stops: the run of dummy stops next to a side column moves onto the
// column only when every stop of it is free to, no other edge holding an x
// within InLaneGap of the column in its channels. As in
// TestRoute_ADummyRunMovesOntoASideColumn, a->m's dummies at 259 would
// move onto diamond m's column at 260; s->t runs down beside them, 8 px
// wide boxes and their column at 267, 7 px from the column: the run
// stays on 259. At 269 it moves onto the column. The check reads only the
// run, the vertical a->m runs down at y = 100, between its dummies' rows:
// where the held run meets the column, a->m takes a 1 px step today, a
// micro-jog this clause does not decide.
func TestRoute_ADummyRunMovesOntoASideColumnOnlyWhereItIsFree(t *testing.T) {
	for _, tc := range []struct {
		s, run float64
	}{
		{267, 259},
		{269, 260},
	} {
		lv := lgraphtest.Level(t, "m:diamond", "a", "a->m", "s->t")
		lv.Nodes[2].W, lv.Nodes[3].W = 8, 8 // s, t
		// layers: m and s 0, a and t 3; a->m reversed, through two dummies
		g, err := lgraph.Build(lv, []int{0, 3, 0, 3}, []bool{true, false}, 8)
		require.NoError(t, err)
		_, route := twice(t, g, screen, map[string]float64{
			"m": 200, "a->m#0@1": 259, "a->m#0@2": 259, "a": 240,
			"s": tc.s, "s->t#0@1": tc.s, "s->t#0@2": tc.s, "t": tc.s})
		am := route("a->m#0")
		run := math.NaN()
		for i := 0; i+1 < len(am); i++ {
			p, q := am[i], am[i+1]
			if p.X == q.X && math.Min(p.Y, q.Y) < 100 && math.Max(p.Y, q.Y) > 100 {
				run = p.X
			}
		}
		assert.Equal(t, tc.run, run, "s->t at %g: %v", tc.s, am)
	}
}

// TestRoute_ASideColumnSlidesOnlyWhereItMayMove pins S8's stops: a side
// column slides onto its adjacent stop only where no other edge holds an x
// within InLaneGap in its channels. m's column at 260 would slide onto a's
// at 270, but diamond n's left column, where q->n leaves it, runs at 276
// down into the same channel: m's column stays, and a->m jogs in a lane
// below q->n's.
func TestRoute_ASideColumnSlidesOnlyWhereItMayMove(t *testing.T) {
	lv := lgraphtest.Level(t, "m:circle->a", "a->m", "q->n:diamond")
	lv.Nodes[1].W = 100 // a
	// layers: m and n 0, a 1, q 2; a->m and q->n reversed
	g, err := lgraph.Build(lv, []int{0, 1, 2, 0}, []bool{false, true, true}, 8)
	require.NoError(t, err)
	_, route := twice(t, g, screen, map[string]float64{"m": 200, "a": 200, "n": 336, "q->n#0@1": 300, "q": 300})
	am, qn := route("a->m#0"), route("q->n#0")
	assertRoute(t, pts(250, 116, 270, 116, 270, 72, 260, 72, 260, 20, 240, 20), am)
	assertRoute(t, pts(300, 176, 300, 64, 276, 64, 276, 20, 296, 20), qn)
}

func TestRoute_ReplayAttachesExactlyTheRecordedEnds(t *testing.T) {
	g := graph(t, nil, "a->b", "b->c", "c->a")
	xs := map[string]float64{"a": 100, "b": 100, "c": 100, "c->a#0@1": 150}
	fresh, route := routed(t, g, screen, xs)
	assert.Equal(t, [2]ports.Vertex{ports.Bottom, ports.Bottom}, fresh.Side[2], "a column 50 off the face does not clear it by Reach")
	assert.InDelta(t, 40, route("c->a#0")[len(route("c->a#0"))-1].Y, 1e-6, "c->a enters a's bottom face")

	o := screen
	o.Side = [][2]ports.Vertex{{}, {}, {ports.Right, ports.Right}}
	replay, route := routed(t, g, o, xs)
	assert.Equal(t, o.Side[2], replay.Side[2])
	got := route("c->a#0")
	assert.Equal(t, model.Point{X: 140, Y: 20}, got[len(got)-1], "a's right face at mid-height")

	g = graph(t, nil, "m:diamond->a", "a->m")
	o.Side = [][2]ports.Vertex{{}, {}}
	replay, route = routed(t, g, o, map[string]float64{"m": 200, "a": 200})
	assert.Equal(t, [2]ports.Vertex{ports.Right, ports.Bottom}, replay.Side[1],
		"a's end, not recorded, stays on its face; m's counter-flow end takes a side vertex all the same (C8.4)")
	assert.Equal(t, model.Point{X: 200 + 40.0/3, Y: 88}, route("a->m#0")[0], "a->m leaves a's top face, below m->a's lane")
}

// TestRoute_ReplayKeepsThePinnedSides pins S8's second pass on a pinned
// node: the first route records the side vertices it attaches ends at,
// and the replay attaches exactly those, each on its recorded vertex,
// whatever the positions would pick afresh, a counter-flow end's or an
// exit's; an exit not recorded leaves from the bottom vertex.
func TestRoute_ReplayKeepsThePinnedSides(t *testing.T) {
	g := graph(t, nil, "m:diamond->a", "m->b", "a->m")
	xs := map[string]float64{"m": 200, "a": 100, "b": 300}
	fresh, _ := routed(t, g, screen, xs)
	// a->m's end claims the side toward a, and of the two exits the one
	// heading right takes the free right vertex (S8, Diamonds and circles)
	assert.Equal(t, [][2]ports.Vertex{{ports.Bottom, ports.Bottom}, {ports.Right, ports.Bottom}, {ports.Left, ports.Left}}, fresh.Side)

	o := screen
	o.Side = [][2]ports.Vertex{{ports.Left, ports.Bottom}, {}, {ports.Right, ports.Bottom}}
	replay, route := routed(t, g, o, xs)
	assert.Equal(t, o.Side, replay.Side, "exactly the recorded ends attach")
	am := route("a->m#0")
	assert.Equal(t, model.Point{X: 240, Y: 20}, am[len(am)-1], "a->m enters m's right vertex, though a lies left: %v", am)
	assert.Equal(t, model.Point{X: 160, Y: 20}, route("m->a#0")[0], "m->a leaves m's left vertex")
	assert.Equal(t, model.Point{X: 200, Y: 40}, route("m->b#0")[0], "m->b, not recorded, leaves the bottom vertex")

	// with a->m on the right vertex, the exit rule would give the left one
	// to m->a, heading farthest left; the replay keeps m->b's record
	o.Side = [][2]ports.Vertex{{}, {ports.Left, ports.Bottom}, {ports.Right, ports.Bottom}}
	replay, route = routed(t, g, o, xs)
	assert.Equal(t, o.Side, replay.Side, "exactly the recorded ends attach")
	assert.Equal(t, model.Point{X: 160, Y: 20}, route("m->b#0")[0], "m->b leaves m's left vertex, though b lies right")
	assert.Equal(t, model.Point{X: 200, Y: 40}, route("m->a#0")[0], "m->a, not recorded, leaves the bottom vertex")
}

// TestRoute_AColumnMovesOutOnlyWhereItKeepsTheRowClear pins S8's side
// attachments: every end first takes its column Reach out; only then, in
// edge order, does a column move out over what it aims at, and only where
// it and its stub keep InLaneGap from every side column and stub of the
// row's other nodes, crossing none. Diamonds l and r face each other
// across their gap, each column aiming over top's center; rects a and b do
// too, a's aim right of b's, b's left of a's.
func TestRoute_AColumnMovesOutOnlyWhereItKeepsTheRowClear(t *testing.T) {
	facing := graph(t, nil, "top:diamond->l:diamond", "top->r:diamond", "l->top", "r->top")
	crossing := graph(t, map[string]float64{"c": 10, "d": 10}, "a->c", "c->a", "b->d", "d->b")
	for _, tc := range []struct {
		name         string
		g            *lgraph.Graph
		xs           map[string]float64
		left, right  string  // the edges whose columns face each other: the left node's and the right node's
		wantL, wantR float64 // their columns
		first        bool    // the facing nodes are the routes' first points, not their last
	}{
		{
			// both aims, 164, lie within InLaneGap of the other's Reach
			// column (160 | 168): neither moves, whichever comes first
			name: "no room for either", g: facing,
			xs:   map[string]float64{"top": 164, "l": 100, "r": 228},
			left: "l->top#0", right: "r->top#0", wantL: 160, wantR: 168, first: true,
		},
		{
			// l's column moves over top, 180, still 20 from r's Reach
			// column (200); r's would then meet it and stays. There l's
			// column runs on top's exits, which leave its bottom vertex:
			// it gives way by InLaneGap, to 188 (S8, Ports)
			name: "room for the first", g: facing,
			xs:   map[string]float64{"top": 180, "l": 100, "r": 260},
			left: "l->top#0", right: "r->top#0", wantL: 188, wantR: 200, first: true,
		},
		{
			// a's column moves over c, 180, InLaneGap from b's Reach
			// column (188); b's, over d at 168, would cross it and stays
			name: "crossing aims", g: crossing,
			xs:   map[string]float64{"a": 100, "b": 248, "c": 180, "d": 164},
			left: "c->a#0", right: "d->b#0", wantL: 180, wantR: 188,
		},
	} {
		_, route := routed(t, tc.g, screen, tc.xs)
		// the column is one point in from the facing node's side
		column := func(pts []model.Point) float64 {
			if tc.first {
				return pts[1].X
			}
			return pts[len(pts)-2].X
		}
		l, r := route(tc.left), route(tc.right)
		assert.InDelta(t, tc.wantL, column(l), 1e-9, "%s: the left node's column: %v", tc.name, l)
		assert.InDelta(t, tc.wantR, column(r), 1e-9, "%s: the right node's column: %v", tc.name, r)
	}
}

// TestRoute_AColumnMovesOutOnlyClearOfAPinnedLoop pins S8's side
// attachments: a column moves out only once every self-loop has its side,
// so a pinned node's loop counts where it runs. n's counter-flow end aims
// over its dummy at 166, inside the clearance of m's loop column (160,
// Reach right of m): it keeps its column Reach out, at 188.
func TestRoute_AColumnMovesOutOnlyClearOfAPinnedLoop(t *testing.T) {
	lv := lgraphtest.Level(t, "m:circle->m", "q->n:circle")
	// layers: m and n 0, q 2; q->n reversed, through its dummy on layer 1
	g, err := lgraph.Build(lv, []int{0, 2, 0}, []bool{false, true}, 8)
	require.NoError(t, err)
	_, route := routed(t, g, screen, map[string]float64{"m": 100, "n": 248, "q->n#0@1": 166, "q": 166})
	qn := route("q->n#0")
	assert.Equal(t, model.Point{X: 208, Y: 20}, qn[len(qn)-1], "q->n enters n's left vertex: %v", qn)
	assert.InDelta(t, 188, qn[len(qn)-2].X, 1e-9, "its column Reach out, clear of m's loop: %v", qn)
}

// TestRoute_ASideColumnSlidesOnlyWhereItKeepsTheRowClear pins S8's stops:
// a side column slides onto its adjacent stop only where it keeps the row
// clear, other nodes' side columns included. l->p leaves l's right vertex
// on the column Reach out (160), since the one over p's in-port (172)
// would cross r's column (168); nor does it slide there, although r's
// column rises into another channel.
func TestRoute_ASideColumnSlidesOnlyWhereItKeepsTheRowClear(t *testing.T) {
	lv := lgraphtest.Level(t, "l:diamond->q", "l->p", "r:diamond->t")
	// layers: t 0, l and r 1, q and p 2; r->t reversed
	g, err := lgraph.Build(lv, []int{1, 2, 2, 1, 0}, []bool{false, false, true}, 8)
	require.NoError(t, err)
	_, route := routed(t, g, screen, map[string]float64{"t": 200, "l": 100, "r": 228, "q": 20, "p": 172})
	lp, rt := route("l->p#0"), route("r->t#0")
	assert.InDelta(t, 160, lp[1].X, 1e-9, "l's column stays Reach out: %v", lp)
	assert.InDelta(t, 168, rt[1].X, 1e-9, "r's column Reach out: %v", rt)
}

// TestRoute_EndsOnOneSideVertexShareOneColumn pins S8's side attachments:
// ends that share a side vertex of a pinned node land on one column Reach
// out, a side rail (C9.1), and none of them moves farther out on its own
// or slides. Of m's four exits, c and d head right beyond the band and
// share m's right vertex (240), whose column Reach out is 260; c's in-port
// (252) lies inside it, d's (272) 12 px beyond it, where d's column would
// move out to, and slide to: d stays on c's column and jogs in the lane
// below c's.
func TestRoute_EndsOnOneSideVertexShareOneColumn(t *testing.T) {
	g := graph(t, map[string]float64{"c": 10, "d": 10}, "m:diamond->a", "m->b", "m->c", "m->d")
	_, route := routed(t, g, screen, map[string]float64{"m": 200, "a": 20, "b": 200, "c": 252, "d": 272})
	assertRoute(t, pts(160, 20, 20, 20, 20, 96), route("m->a#0"))
	assertRoute(t, pts(240, 20, 260, 20, 260, 64, 252, 64, 252, 96), route("m->c#0"))
	assertRoute(t, pts(240, 20, 260, 20, 260, 72, 272, 72, 272, 96), route("m->d#0"))
}

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

// TestRoute_ASideColumnGivesWayToAnotherEdgesPort pins S8's ports: a side
// column that lands within InLaneGap of another edge's port in the
// channel it enters moves outward by InLaneGap. Diamonds m and n form a
// two-cycle one hop apart: n->m's end on m runs out over n's center (300),
// where m->n enters n, and its end on n over m's center (100), where m->n
// leaves m. Both step out, to 308 and 92, and the verticals keep apart.
func TestRoute_ASideColumnGivesWayToAnotherEdgesPort(t *testing.T) {
	g := graph(t, nil, "m:diamond->n:diamond", "n->m")
	_, route := routed(t, g, screen, map[string]float64{"m": 100, "n": 300})
	assertRoute(t, pts(100, 40, 100, 64, 300, 64, 300, 96), route("m->n#0"))
	assertRoute(t, pts(260, 116, 92, 116, 92, 72, 308, 72, 308, 20, 140, 20), route("n->m#0"))
}

// TestRoute_ARectsSideColumnGivesWayToAnotherEdgesPort pins S8's ports on
// a node with faces, which the rule does not exempt: a side column that
// lands within InLaneGap of another edge's port in the channel it enters
// moves outward by InLaneGap. Replayed, the one-hop back edge b->a leaves
// rect b's side face and enters rect a's, on the side of b: its column on
// a aims over b's center, where a->b's lone in-port lies, so it steps out
// past that port, 8 px, and the port stays. Mirrored, the same on the
// right.
func TestRoute_ARectsSideColumnGivesWayToAnotherEdgesPort(t *testing.T) {
	for _, tc := range []struct {
		name         string
		a, b         float64
		column, face float64 // b->a's column on a, and a's side face it enters
	}{
		{"left", 200, 100, 92, 160},
		{"right", 100, 200, 208, 140},
	} {
		g := graph(t, nil, "a->b", "b->a")
		res, route := twice(t, g, screen, map[string]float64{"a": tc.a, "b": tc.b})
		require.NotEqual(t, ports.Bottom, res.Side[1][ports.Head], "%s: b->a's end on a attaches at a side face", tc.name)
		ab, ba := route("a->b#0"), route("b->a#0")
		assert.InDelta(t, tc.b, ab[len(ab)-1].X, 1e-9, "%s: a->b's in-port stays at b's center: %v", tc.name, ab)
		last, column := ba[len(ba)-1], ba[len(ba)-2]
		assert.InDelta(t, tc.face, last.X, 1e-9, "%s: b->a enters a's side face: %v", tc.name, ba)
		assert.Equal(t, last.Y, column.Y, "%s: a stub across to the column: %v", tc.name, ba)
		assert.InDelta(t, tc.column, column.X, 1e-9, "%s: the column steps out past a->b's in-port: %v", tc.name, ba)
	}
}

// TestRoute_ASideColumnGivesWayBeforeAnyPortMoves pins S8's ports: the
// side columns give way once they have landed, before any port moves.
// Replayed, n->m's end on diamond m runs out over n's center (300), where
// k1->n's in-port lies, the middle of n's three: the column steps out to
// 308, and the port stays, clear of it.
func TestRoute_ASideColumnGivesWayBeforeAnyPortMoves(t *testing.T) {
	lv := lgraphtest.Level(t, "m:diamond->n", "n->m", "k1->n", "k2->n")
	// layers: m, k1 and k2 0, n 1; n->m reversed
	g, err := lgraph.Build(lv, []int{0, 1, 0, 0}, []bool{false, true, false, false}, 8)
	require.NoError(t, err)
	_, route := twice(t, g, screen, map[string]float64{"m": 100, "k1": 400, "k2": 500, "n": 300})
	nm := route("n->m#0")
	assert.Equal(t, model.Point{X: 308, Y: 20}, nm[len(nm)-2], "n->m's column on m: %v", nm)
	kn := route("k1->n#0")
	assert.InDelta(t, 300, kn[len(kn)-1].X, 1e-9, "k1->n's in-port stays: %v", kn)
}

// TestRoute_ASideColumnMovesToTheFirstClearSpot pins S8's ports: a side
// column that gives way moves to the first spot, InLaneGap after
// InLaneGap outward, clear of the other edges' ports and dummy columns in
// its channel, and only where it keeps its row clear. m->c's column Reach out (160)
// lies 3 px from q->z's dummy column (163): 168 is still 5 px from it,
// 176 is clear. With q at 188, 176 comes within the node clearance of q,
// and the column stays.
func TestRoute_ASideColumnMovesToTheFirstClearSpot(t *testing.T) {
	lv := lgraphtest.Level(t, "m:diamond->a", "m->b", "m->c", "q->z")
	lv.Nodes[3].W, lv.Nodes[4].W = 10, 10 // c, q
	// layers: m and q 0, a, b, c and q->z's dummy 1, z 2
	g, err := lgraph.Build(lv, []int{0, 1, 1, 1, 0, 2}, nil, 8)
	require.NoError(t, err)
	for _, tc := range []struct {
		name   string
		q      float64
		column float64
	}{
		{"the second step", 400, 176},
		{"no spot keeps the row clear", 188, 160},
	} {
		_, route := routed(t, g, screen, map[string]float64{"m": 100, "q": tc.q, "a": 0, "b": 100, "c": 145, "q->z#0@1": 163, "z": 163})
		mc := route("m->c#0")
		assert.Equal(t, model.Point{X: 140, Y: 20}, mc[0], "%s: m->c leaves m's right vertex: %v", tc.name, mc)
		assert.InDelta(t, tc.column, mc[1].X, 1e-9, "%s: m->c's column: %v", tc.name, mc)
	}
}

// TestRoute_AMovedColumnGivesWayToADummyColumnOfTheOtherRow pins S8's
// ports on a column that moved out: the side columns give way once every
// column has landed, the ones that moved out over what they aim at
// included, and a dummy column of the row across the channel counts.
// m->b leaves diamond m's right vertex and moves out over b's in-port
// (300) in the row below; there q->z's dummy column (293), another edge's,
// lies 7 px from it in the channel between the rows, so the column steps
// out by InLaneGap to 308, the first spot clear of it, and m->b jogs back
// to b's port (narrow b, 4 wide, keeps its port). With the dummy at 400,
// m->b is an L.
func TestRoute_AMovedColumnGivesWayToADummyColumnOfTheOtherRow(t *testing.T) {
	lv := lgraphtest.Level(t, "m:diamond->a", "m->b", "q->z")
	lv.Nodes[2].W, lv.Nodes[3].W, lv.Nodes[4].W = 4, 10, 10 // b, q, z
	// layers: m and q 0, a, b and q->z's dummy 1, z 2
	g, err := lgraph.Build(lv, []int{0, 1, 1, 0, 2}, nil, 8)
	require.NoError(t, err)
	for _, tc := range []struct {
		name  string
		dummy float64
		want  []model.Point
	}{
		{"beside the dummy", 293, pts(140, 20, 308, 20, 308, 64, 300, 64, 300, 96)},
		{"clear of it", 400, pts(140, 20, 300, 20, 300, 88)},
	} {
		_, route := routed(t, g, screen, map[string]float64{"m": 100, "a": 100, "b": 300, "q": 500, "q->z#0@1": tc.dummy, "z": tc.dummy})
		got := route("m->b#0")
		require.Len(t, got, len(tc.want), "%s: %v", tc.name, got)
		assertRoute(t, tc.want, got)
	}
}

// TestRoute_ASideRailGivesWayAsOne pins S8's ports on a side rail: its
// ends move as one, clear of the other edges' ports and dummy columns in
// every channel they enter; a side column, theirs included, is no port.
// c and d share m's right vertex on the column Reach out (260), 5 px from
// d's in-port (265), another edge's port for c's end: the rail steps out
// past it, to 276, and both jog from there.
func TestRoute_ASideRailGivesWayAsOne(t *testing.T) {
	g := graph(t, map[string]float64{"c": 10, "d": 10}, "m:diamond->a", "m->b", "m->c", "m->d")
	_, route := routed(t, g, screen, map[string]float64{"m": 200, "a": 20, "b": 200, "c": 252, "d": 265})
	assertRoute(t, pts(240, 20, 276, 20, 276, 64, 252, 64, 252, 96), route("m->c#0"))
	assertRoute(t, pts(240, 20, 276, 20, 276, 72, 265, 72, 265, 96), route("m->d#0"))
}

// TestRoute_ASideRailGivesWayInBothChannelsOfItsRow pins S8's ports on a
// side rail whose ends enter both channels of its row: it moves as one,
// clear of other edges' ports in every channel its ends enter. Diamond m
// is in a two-cycle with u above it, and m->u's leaving end holds m's
// right vertex; m->a, heading farther to the right than m->b, shares it
// (TestRoute_ArrivingEndsGiveWayToTwoExits): the rail rises into the
// channel above m's row with m->u and descends into the one below with
// m->a, Reach out at 260. q->w's out-port on narrow q, at 265 in the
// channel above, or p's in-port of r->p, at 265 in the channel below,
// lies within InLaneGap of it: in either case both ends step out
// together, past it, to 276. With that port at 400, the rail stays.
func TestRoute_ASideRailGivesWayInBothChannelsOfItsRow(t *testing.T) {
	for _, tc := range []struct {
		name   string
		other  string // the edge whose port lies beside the rail
		ranks  []int  // u m b a and the other edge's two nodes
		xs     map[string]float64
		column float64
	}{
		{"a port in the channel above", "q->w", []int{0, 1, 2, 2, 0, 1}, map[string]float64{"q": 265, "w": 500}, 276},
		{"a port in the channel below", "r->p", []int{0, 1, 2, 2, 1, 2}, map[string]float64{"r": 500, "p": 265}, 276},
		{"no port beside it", "q->w", []int{0, 1, 2, 2, 0, 1}, map[string]float64{"q": 400, "w": 500}, 260},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lv := lgraphtest.Level(t, "u->m:diamond", "m->u", "m->b", "m->a", "b->m", tc.other)
			lv.Nodes[4].W, lv.Nodes[5].W = 10, 10
			g, err := lgraph.Build(lv, tc.ranks, []bool{false, true, false, false, true, false}, 8)
			require.NoError(t, err)
			xs := map[string]float64{"u": 300, "m": 200, "b": 140, "a": 380}
			for id, x := range tc.xs {
				xs[id] = x
			}
			_, route := routed(t, g, screen, xs)
			mu, ma := route("m->u#0"), route("m->a#0")
			require.Equal(t, 240.0, mu[0].X, "m->u leaves m's right vertex: %v", mu)
			require.Equal(t, mu[0], ma[0], "m->a shares it: %v", ma)
			assert.InDelta(t, tc.column, mu[1].X, 1e-9, "m->u's column, up into the channel above: %v", mu)
			assert.Less(t, mu[2].Y, mu[1].Y, "m->u rises: %v", mu)
			assert.InDelta(t, tc.column, ma[1].X, 1e-9, "m->a's column, down into the channel below: %v", ma)
			assert.Greater(t, ma[2].Y, ma[1].Y, "m->a descends: %v", ma)
		})
	}
}

// TestRoute_ARailRunsStraightIntoItsOwnEnd pins S8's ports on a side
// rail: when it gives way, a stop within Snap of its column on the wire of
// one of its own ends is the rail's own run, not another edge's port. c
// and d share m's right vertex on the column Reach out (260), with d's
// in-port under it, or within Snap of it: the L that S7's side-exit
// forcing aims d for. The rail stays, m->d runs straight down into d, and
// m->c jogs to c.
func TestRoute_ARailRunsStraightIntoItsOwnEnd(t *testing.T) {
	for _, tc := range []struct {
		name string
		d    float64
	}{{"under the column", 260}, {"within Snap of it", 260.4}} {
		t.Run(tc.name, func(t *testing.T) {
			g := graph(t, map[string]float64{"c": 10, "d": 10}, "m:diamond->a", "m->b", "m->c", "m->d")
			_, route := routed(t, g, screen, map[string]float64{"m": 200, "a": 20, "b": 200, "c": 245, "d": tc.d})
			assertRoute(t, pts(240, 20, 260, 20, 260, 88), route("m->d#0"))
			assertRoute(t, pts(240, 20, 260, 20, 260, 64, 245, 64, 245, 88), route("m->c#0"))
		})
	}
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

// TestRoute_ASideColumnGivesWayInText pins S8's ports in the text
// profile, which the rule does not exempt: a side column that lands within
// InLaneGap of another edge's port in the channel it enters moves outward
// by InLaneGap. Replayed, the one-hop back edge b->a leaves both left
// faces; its column on a, aimed over b's center (10), lands half a cell
// from a->b's in-port on b (10.5): the column steps out two cells, to 8,
// and the port stays.
func TestRoute_ASideColumnGivesWayInText(t *testing.T) {
	g := graph(t, map[string]float64{"a": 8, "b": 8}, "a->b", "b->a")
	for i := range g.Level.Nodes {
		g.Level.Nodes[i].H = 3
	}
	_, route := twice(t, g, text, map[string]float64{"a": 20, "b": 10})
	assertRoute(t, pts(20.5, 3, 20.5, 5, 10.5, 5, 10.5, 7), route("a->b#0"))
	assertRoute(t, pts(6, 8.5, 2.5, 8.5, 2.5, 5, 8, 5, 8, 1.5, 16, 1.5), route("b->a#0"))
}

// TestRoute_AReplayedColumnFallsBackToReach pins S8's second pass: a
// replayed side attachment always happens, and when the farther column it
// aims at does not keep the row clear, its column falls back to the column
// Reach out, the room S7 kept. Here n, in the attached node's row, lies
// under the column over the partner end's column, or over the anchored
// port. In the text profile the fallback sits half a cell farther, on a
// cell's middle: the column lands there at once, as it moves out no
// farther.
func TestRoute_AReplayedColumnFallsBackToReach(t *testing.T) {
	narrow := func(g *lgraph.Graph) *lgraph.Graph {
		for i := range g.Level.Nodes {
			g.Level.Nodes[i].W, g.Level.Nodes[i].H = 8, 3
		}
		return g
	}
	for _, tc := range []struct {
		name   string
		g      *lgraph.Graph
		text   bool
		side   [][2]ports.Vertex
		xs     map[string]float64
		edge   string
		column float64
		face   float64 // the attached node's side
		first  bool    // the attached end is the route's first point
	}{
		{
			// b->a leaves b's and a's left faces; b's column would run over
			// a's, at 40, its stub through n
			name:   "over its partner's column",
			g:      graph(t, nil, "c->n", "a->b", "b->a"),
			side:   [][2]ports.Vertex{{}, {}, {ports.Left, ports.Left}},
			xs:     map[string]float64{"c": -100, "a": 100, "n": 180, "b": 300},
			edge:   "b->a#0",
			column: 240,
			face:   260,
			first:  true,
		},
		{
			// the same in cells: b's column would run over a's, at 12.5, its
			// stub through n; it falls back to 3.5 cells outside b's left
			// face, 56
			name:   "in text, over its partner's column",
			g:      narrow(graph(t, nil, "c->n", "a->b", "b->a")),
			text:   true,
			side:   [][2]ports.Vertex{{}, {}, {ports.Left, ports.Left}},
			xs:     map[string]float64{"c": -20, "a": 20, "n": 40, "b": 60},
			edge:   "b->a#0",
			column: 52.5,
			face:   56,
			first:  true,
		},
		{
			// g->a enters a's right face; its column would run over its port
			// anchored on g, at 150, within the node clearance of n
			name: "over its anchored port",
			g: shaped(t, func(lv *lgraph.Level) {
				lv.Nodes[1].Group, lv.Nodes[1].W, lv.Nodes[1].H = true, 300, 200
				lv.Edges[0].Anchor[1] = lgraph.Anchor{On: true, At: -60}
				lv.Edges[1].Anchor[0] = lgraph.Anchor{On: true, At: 150}
			}, "a->g", "g->a", "n"),
			side:   [][2]ports.Vertex{{}, {ports.Right, ports.Bottom}},
			xs:     map[string]float64{"a": 0, "g": 0, "n": 200},
			edge:   "g->a#0",
			column: 60,
			face:   40,
		},
	} {
		o := screen
		if tc.text {
			o = text
		}
		o.Side = tc.side
		res, route := routed(t, tc.g, o, tc.xs)
		assert.Equal(t, tc.side, res.Side, "%s: the recorded ends attach", tc.name)
		got := route(tc.edge)
		end, next := got[len(got)-1], got[len(got)-2]
		if tc.first {
			end, next = got[0], got[1]
		}
		assert.Equal(t, tc.face, end.X, "%s: at the side face: %v", tc.name, got)
		assert.Equal(t, end.Y, next.Y, "%s: a stub across to the column: %v", tc.name, got)
		assert.Equal(t, tc.column, next.X, "%s: the column Reach out: %v", tc.name, got)
	}
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

func TestRoute_ABundleFollowsItsFarEnds(t *testing.T) {
	g := graph(t, nil, "m:diamond->a", "m->a")
	// both exits head for a's two in-ports, a third of a's width either
	// side of m's center: the first, whose port is the left one, counts as
	// the farther (the first in order of heading at equal distances) and
	// takes the left vertex, the second the bottom; a's in-ports follow
	// that order, so the pair runs side by side instead of crossing
	r, _ := routed(t, g, screen, map[string]float64{"m": 200, "a": 200})
	for _, cs := range Crossings(r.Edges, 5) {
		assert.Empty(t, cs)
	}
	first, second := r.Edges[0], r.Edges[1]
	assert.InDelta(t, 160, first[0].X, 1e-9, "the first leaves the left vertex: %v", first)
	assert.Less(t, first[len(first)-1].X, second[len(second)-1].X)
}

// TestRoute_ASideEndIsNoBundleMember pins S8's ports: an end attached at a
// side face holds no port, and its edge is no member of a bundle. Replayed,
// b->a leaves a's and b's left faces; the bundle a->b#0, a->b#1 shares out
// b's three slots without it (c->b holds the first), so its two edges
// enter b at their own ports, never both at b's center.
func TestRoute_ASideEndIsNoBundleMember(t *testing.T) {
	lv := lgraphtest.Level(t, "c", "a", "b", "y", "z", "c->b", "a->b", "a->b", "b->a", "a->y", "a->z")
	lv.Nodes[2].W = 160 // b
	g, err := lgraph.Build(lv, []int{0, 0, 1, 1, 1}, []bool{false, false, false, true, false, false}, 8)
	require.NoError(t, err)
	o := screen
	o.Side = [][2]ports.Vertex{{}, {}, {}, {ports.Left, ports.Left}, {}, {}}
	res, route := routed(t, g, o, map[string]float64{"c": 100, "a": 300, "b": 300, "y": 500, "z": 650})
	require.Equal(t, o.Side[3], res.Side[3], "b->a attaches at both left faces")
	last := func(id string) float64 { got := route(id); return got[len(got)-1].X }
	assert.Equal(t, 260.0, last("c->b#0"), "b's first slot")
	assert.Equal(t, 300.0, last("a->b#0"), "b's second slot")
	assert.Equal(t, 340.0, last("a->b#1"), "b's third slot, not b->a's center")
}

// TestRoute_ASideEndIsNoBundleMemberInText is TestRoute_ASideEndIsNoBundleMember
// in the text profile, in cells, where the random probe first showed it
// (seed 356 under RIGHT): replayed, b->a leaves a's and b's left faces,
// and the bundle a->b#0, a->b#1 shares out b's three slots without it
// (c->b holds the first), so its two edges enter b at ports of their own,
// each on its own cell, and neither on b->a's stray center slot.
func TestRoute_ASideEndIsNoBundleMemberInText(t *testing.T) {
	lv := lgraphtest.Level(t, "c", "a", "b", "y", "z", "c->b", "a->b", "a->b", "b->a", "a->y", "a->z")
	for i := range lv.Nodes {
		lv.Nodes[i].W, lv.Nodes[i].H = 8, 3
	}
	lv.Nodes[2].W = 16 // b: cells 22 to 38
	g, err := lgraph.Build(lv, []int{0, 0, 1, 1, 1}, []bool{false, false, false, true, false, false}, 1)
	require.NoError(t, err)
	o := text
	o.Side = [][2]ports.Vertex{{}, {}, {}, {ports.Left, ports.Left}, {}, {}}
	res, route := routed(t, g, o, map[string]float64{"c": 10, "a": 30, "b": 30, "y": 50, "z": 65})
	require.Equal(t, o.Side[3], res.Side[3], "b->a attaches at both left faces")
	last := func(id string) float64 { got := route(id); return got[len(got)-1].X }
	cb, ab0, ab1 := last("c->b#0"), last("a->b#0"), last("a->b#1")
	assert.Less(t, cb, ab0, "c->b holds b's first slot")
	assert.Less(t, ab0, ab1, "the bundle follows its out-ports")
	for _, x := range []float64{cb, ab0, ab1} {
		assert.Equal(t, 0.5, x-math.Floor(x), "on a cell's middle: %v", x)
		assert.True(t, x > 22 && x < 38, "on b's top face: %v", x)
	}
	assert.False(t, ab0 > 29 && ab0 < 31 && ab1 > 29 && ab1 < 31, "not both on b's center: %v, %v", ab0, ab1)
}

func TestRoute_DivergingJogsFromOneNodeShareALane(t *testing.T) {
	g := graph(t, map[string]float64{"a": 16}, "a->b", "a->c")
	// a's two ports sit 16/3 apart, under InLaneGap, but their jogs run
	// away from each other
	r, _ := routed(t, g, screen, map[string]float64{"a": 200, "b": 100, "c": 300})
	assert.Equal(t, []int{1}, r.Lanes)
}

func TestRoute_PortsLandOnTheDrawnOutline(t *testing.T) {
	g := graph(t, nil, "a->c:cylinder", "b->c")
	_, route := routed(t, g, screen, map[string]float64{"a": 100, "b": 300, "c": 200})
	got := route("a->c#0")
	end := got[len(got)-1]
	assert.InDelta(t, 200-40.0/3, end.X, 1e-6)
	top := 88.0 // one lane in the channel
	assert.InDelta(t, top+ports.Inset(model.ShapeCylinder, 80, 40, model.Down, false, ports.FaceIn, -40.0/3), end.Y, 1e-9)
	assert.Greater(t, end.Y, top, "on the cap's arc, below the box's top")
}

func TestRoute_LogsItsDecisions(t *testing.T) {
	var buf bytes.Buffer
	ctx := layoutdbg.NewContext(context.Background(), slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	g := graph(t, nil, "m:diamond->a", "a->m")
	Route(ctx, g, []float64{200, 200}, screen)
	var decisions []string
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var rec map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &rec))
		assert.Equal(t, "route", rec["phase"])
		assert.Equal(t, "S8", rec["spec_ref"])
		decisions = append(decisions, rec["decision"].(string))
	}
	assert.Equal(t, []string{"side_attached", "side_attached", "routed"}, decisions)
}

// TestRoute_LogsAMergedFanOut pins S8's debug record of a circle's merged
// exits: the circle and how many exits leave its bottom vertex. The replay
// takes them from the first route and records nothing new.
func TestRoute_LogsAMergedFanOut(t *testing.T) {
	var buf bytes.Buffer
	ctx := layoutdbg.NewContext(context.Background(), slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	g := graph(t, nil, "m:circle->a", "m->b", "m->c")
	first := Route(ctx, g, []float64{200, 100, 200, 300}, screen)
	replay := screen
	replay.Side = first.Side
	Route(ctx, g, []float64{200, 100, 200, 300}, replay)
	var merged []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var rec map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &rec))
		if rec["decision"] == "exits_merged" {
			merged = append(merged, rec)
		}
	}
	require.Len(t, merged, 1)
	assert.Equal(t, "route", merged[0]["phase"])
	assert.Equal(t, "S8", merged[0]["spec_ref"])
	assert.Equal(t, "m", merged[0]["node"])
	assert.InDelta(t, 3, merged[0]["exits"], 0)
}

func TestRoute_ABlockedSideColumnFallsBackToReach(t *testing.T) {
	g := graph(t, nil, "m:diamond->a", "m->b", "n")
	// the column over b's in-port would run its stub through n
	_, route := routed(t, g, screen, map[string]float64{"m": 200, "n": 340, "a": 0, "b": 500})
	assert.InDelta(t, 260, route("m->b#0")[1].X, 1e-6)
}

// beside is diamond m's level with a long edge s->c whose dummy runs
// through m's row: m on layer 1 above its two exits' ends a and b, s on
// layer 0 and c on layer 2. m is 80 wide at 200, so its right column lies
// at 260; a sits under m, b's lone in-port at 320, s and the dummy on one
// x, dummy, and c 200 beyond it, so that the router leaves the dummy
// where it is.
func beside(t *testing.T, dummy float64) func(id string) []model.Point {
	t.Helper()
	lv := lgraphtest.Level(t, "m:diamond->a", "m->b", "s->c")
	g, err := lgraph.Build(lv, []int{1, 2, 2, 0, 2}, nil, 8)
	require.NoError(t, err)
	_, route := routed(t, g, screen, map[string]float64{
		"m": 200, "a": 200, "b": 320, "s": dummy, "s->c#0@1": dummy, "c": dummy + 200,
	})
	return route
}

// TestRoute_APinnedColumnKeepsTheWireGapFromADummy pins S8's side
// attachments at a pinned node's side vertex: a dummy of the row is a
// wire, which the column keeps InLaneGap from, not the node clearance
// from its box. The dummy at 330 lies 10 from b's port: the column moves
// out over the port, and m->b is an L. Kept the node clearance from the
// dummy's 8 px box, 16 from its column, it could not.
func TestRoute_APinnedColumnKeepsTheWireGapFromADummy(t *testing.T) {
	route := beside(t, 330)
	assertRoute(t, pts(240, 100, 320, 100, 320, 168), route("m->b#0"))
	assertRoute(t, pts(200, 120, 200, 168), route("m->a#0"))
}

// TestRoute_ANodeAtTheClearanceLetsAColumnMoveOut pins S8's row test on
// float noise, on both sides: diamond m's side column moves out over b's
// port at 320 (or 80) where o, in m's row, keeps the node clearance (12)
// beyond it, also when the arithmetic that put o there leaves it short by
// float noise (1e-12, 1e-7), as a room S7 solved to the clearance does.
// Short by more than 1e-6 (1e-5, 1e-3), o blocks the column, which stays
// Reach out, and m->b jogs to its port.
func TestRoute_ANodeAtTheClearanceLetsAColumnMoveOut(t *testing.T) {
	lv := lgraphtest.Level(t, "m:diamond->a", "m->b", "o")
	g, err := lgraph.Build(lv, []int{0, 1, 1, 0}, nil, 8)
	require.NoError(t, err)
	for _, side := range []struct {
		name                  string
		b, o, vertex, columns float64 // o at b's port + o's offset; the column Reach out
	}{
		{"right", 320, 320 + 12 + 40, 240, 260},
		{"left", 80, 80 - 12 - 40, 160, 140},
	} {
		sign := 1.0
		if side.name == "left" {
			sign = -1
		}
		for _, tc := range []struct {
			short float64
			clear bool
		}{{0, true}, {1e-12, true}, {1e-7, true}, {1e-5, false}, {1e-3, false}} {
			o := side.o - sign*tc.short
			_, route := routed(t, g, screen, map[string]float64{"m": 200, "a": 200, "b": side.b, "o": o})
			mb := route("m->b#0")
			msg := fmt.Sprintf("%s, o %g short: %v", side.name, tc.short, mb)
			require.GreaterOrEqual(t, len(mb), 3, msg)
			assert.InDelta(t, side.vertex, mb[0].X, 1e-9, "from m's side vertex: "+msg)
			column := side.columns
			if tc.clear {
				column = side.b
				assert.Len(t, mb, 3, "an L: "+msg)
			}
			assert.InDelta(t, column, mb[1].X, 1e-9, "the column: "+msg)
		}
	}
}

// TestRoute_AColumnFollowsItsPortWhenItGivesWay pins S8's ports: b's port
// at 320 lies within InLaneGap of the dummy's column at 324, so it gives
// way, left to 312 (right, 328, is no clearer). m->b's column, which could
// not move out over 320 (4 from the dummy), follows the port to 312, 12
// clear of it, and m->b is an L, not a jog in the channel.
func TestRoute_AColumnFollowsItsPortWhenItGivesWay(t *testing.T) {
	route := beside(t, 324)
	assertRoute(t, pts(240, 100, 312, 100, 312, 168), route("m->b#0"))
}

// TestRoute_LogsAColumnThatFollowedItsPort pins S8's port_gave_way record
// on TestRoute_AColumnFollowsItsPortWhenItGivesWay's level: b's end of
// m->b gives way from 320 to 312, passing no other end of b and reordering
// no port, and m->b's side column follows it (column_followed). s->c's
// ends, clear of every other port, give way nowhere.
func TestRoute_LogsAColumnThatFollowedItsPort(t *testing.T) {
	var buf bytes.Buffer
	ctx := layoutdbg.NewContext(context.Background(), slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	lv := lgraphtest.Level(t, "m:diamond->a", "m->b", "s->c")
	g, err := lgraph.Build(lv, []int{1, 2, 2, 0, 2}, nil, 8)
	require.NoError(t, err)
	x := map[string]float64{"m": 200, "a": 200, "b": 320, "s": 324, "s->c#0@1": 324, "c": 524}
	xs := make([]float64, len(g.Vertices))
	for v, vx := range g.Vertices {
		xs[v] = x[vx.ID]
	}
	Route(ctx, g, xs, screen)
	var gave []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var rec map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &rec))
		if rec["decision"] == "port_gave_way" {
			gave = append(gave, rec)
		}
	}
	require.Len(t, gave, 1)
	assert.Equal(t, "m->b#0", gave[0]["edge"])
	assert.InDelta(t, ports.Tail, gave[0]["end"], 0)
	assert.InDelta(t, 320, gave[0]["from"], 1e-9)
	assert.InDelta(t, 312, gave[0]["to"], 1e-9)
	assert.Equal(t, false, gave[0]["passes"])
	assert.InDelta(t, 0, gave[0]["reordered"], 0)
	assert.Equal(t, true, gave[0]["column_followed"])
}

// TestRoute_AFacedSideKeepsTheNodeClearanceFromADummy pins the scope of
// S8's wire gap (Side attachments): only at a pinned node's side vertex is
// a dummy of the row a wire. The one-hop back edge b->a would attach at
// rect a's left face and at b's, on one column Reach out, at 140. The
// dummy of s->c in a's row, at 128, lies 12 from that column: beyond
// InLaneGap (8), within the node clearance of its 8 px box (4 + 12). So
// a's end keeps its face port, and the edge enters a's bottom face; only
// b's end attaches at a side. Kept only InLaneGap from the dummy's column,
// as a pinned node's column is, a's end would attach too.
func TestRoute_AFacedSideKeepsTheNodeClearanceFromADummy(t *testing.T) {
	lv := lgraphtest.Level(t, "a->b", "b->a", "s->c")
	// layers: s 0, a 1, b and c 2; b->a reversed, s->c through a's row
	g, err := lgraph.Build(lv, []int{1, 2, 0, 2}, []bool{false, true, false}, 8)
	require.NoError(t, err)
	r, route := routed(t, g, screen, map[string]float64{"a": 200, "b": 200, "s": 128, "s->c#0@1": 128, "c": -72})
	assert.Equal(t, [2]ports.Vertex{ports.Bottom, ports.Left}, r.Side[1], "a's end on its face, b's at its left face")
	ba := route("b->a#0")
	assert.Equal(t, model.Point{X: 160, Y: 188}, ba[0], "b->a leaves b's left face: %v", ba)
	assert.InDelta(t, 140, ba[1].X, 1e-9, "on b's column Reach out: %v", ba)
	last := ba[len(ba)-1]
	assert.InDelta(t, 120, last.Y, 1e-9, "and enters a's bottom face: %v", ba)
	assert.True(t, last.X > 160 && last.X < 240, "within a's face: %v", ba)
}

// TestRoute_ExitsHeadForTheirPorts pins S8's exit rule on ports: m->a's
// port on a lies at a's left third, 20 left of a's center, since z->a
// comes from the right. At 200 it lies exactly under m, while a's center,
// 220, lies beyond the band on m's right, and b beyond it on the left.
// Headed for the centers, the pair would take left and right, and m->a
// would leave the right vertex and hook back to the port; headed for the
// ports, m->a takes the bottom and runs straight, and m->b the left.
func TestRoute_ExitsHeadForTheirPorts(t *testing.T) {
	g := graph(t, map[string]float64{"a": 120}, "m:diamond->a", "m->b", "z->a")
	_, route := routed(t, g, screen, map[string]float64{"m": 200, "z": 400, "a": 220, "b": 60})
	assertRoute(t, pts(200, 40, 200, 88), route("m->a#0"))
	assert.InDelta(t, 160, route("m->b#0")[0].X, 1e-9, "m->b leaves the left vertex: %v", route("m->b#0"))
}

// pinnedCase is a level around diamond or circle m (80 x 40, centered at
// x = 200), built with explicit ranks and reversals, and the vertex of m
// each edge meets: where an edge m leaves starts, where one it enters
// ends.
type pinnedCase struct {
	name     string
	specs    []string
	ranks    []int  // per node, in order of first mention
	reversed []bool // per edge
	xs       map[string]float64
	want     map[string]string // edge id: "left", "right", "bottom" or "top"
}

// layout routes the case's level; it returns the top of m's row and an
// accessor of an edge's route by id.
func (tc pinnedCase) layout(t *testing.T) (float64, func(id string) []model.Point) {
	t.Helper()
	lv := lgraphtest.Level(t, tc.specs...)
	g, err := lgraph.Build(lv, tc.ranks, tc.reversed, 8)
	require.NoError(t, err)
	r, route := routed(t, g, screen, tc.xs)
	m := -1
	for i, n := range lv.Nodes {
		if n.ID == "m" {
			m = i
		}
	}
	require.GreaterOrEqual(t, m, 0)
	return r.RowTop[tc.ranks[m]], route
}

// run routes the case's level and checks the vertex of m each edge meets.
func (tc pinnedCase) run(t *testing.T) {
	t.Helper()
	top, route := tc.layout(t)
	for id, want := range tc.want {
		got := route(id)
		p := got[len(got)-1]
		if strings.HasPrefix(id, "m->") {
			p = got[0]
		}
		vertex := map[model.Point]string{
			{X: 160, Y: top + 20}: "left", {X: 240, Y: top + 20}: "right",
			{X: 200, Y: top + 40}: "bottom", {X: 200, Y: top}: "top",
		}[p]
		assert.Equal(t, want, vertex, "%s meets m at %v: %v", id, p, got)
	}
}

// TestRoute_ArrivingEndsGiveWayToTwoExits pins S8's diamonds and circles:
// when the counter-flow ends hold both side vertices of a diamond, so that
// its two exits would share the bottom one (C8.5), the arriving ends on
// the side the farther-heading exit heads for move to the other side and
// share it, and the exit heading farthest toward that side takes it, the
// other the bottom; when the farther-heading exit heads exactly at the
// center, the arriving ends clear the right. A leaving end never moves:
// when the farther-heading exit's side is a leaving end's, the exit
// shares it there instead, and the arriving end elsewhere stays put
// (C8.5 counts no back edge).
func TestRoute_ArrivingEndsGiveWayToTwoExits(t *testing.T) {
	for _, tc := range []pinnedCase{
		// both back edges come from the right: a->m claims the right
		// vertex, b->m the free left one (C8.4); m->b heads farther, to
		// the right, so a->m moves over beside b->m
		{"both arrivals from the right", []string{"m:diamond->a", "a->m", "m->b", "b->m"},
			[]int{0, 1, 1}, []bool{false, true, false, true},
			map[string]float64{"m": 200, "a": 300, "b": 450},
			map[string]string{"a->m#0": "left", "b->m#0": "left", "m->b#0": "right", "m->a#0": "bottom"}},
		// mirrored: a->m claims the left vertex, b->m the right one; m->b
		// heads farther, to the left, so a->m moves over beside b->m
		{"both arrivals from the left", []string{"m:diamond", "b", "a", "m->a", "a->m", "m->b", "b->m"},
			[]int{0, 1, 1}, []bool{false, true, false, true},
			map[string]float64{"m": 200, "a": 100, "b": -50},
			map[string]string{"a->m#0": "right", "b->m#0": "right", "m->b#0": "left", "m->a#0": "bottom"}},
		// both exits head exactly at the center, a diamond's in-vertex
		// under m: the first in edge order counts as the farther, and the
		// arriving end on the right, d->m, moves over beside c->m
		{"both exits head at the center", []string{"m:diamond", "c", "a:diamond", "d", "m->a", "m->a", "c->m", "d->m"},
			[]int{0, 1, 1, 1}, []bool{false, false, true, true},
			map[string]float64{"m": 200, "c": 100, "a": 200, "d": 300},
			map[string]string{"c->m#0": "left", "d->m#0": "left", "m->a#0": "right", "m->a#1": "bottom"}},
		// m->u leaves from the left, b->m arrives on the right; m->a heads
		// farther, to the left, the side m->u already leaves from: m->a
		// shares it with m->u, m->b takes the bottom, and b->m stays put
		{"an exit shares the farther side with a leaving end", []string{"u->m:diamond", "m->u", "m->a", "m->b", "b->m"},
			[]int{0, 1, 2, 2}, []bool{false, true, false, false, true},
			map[string]float64{"u": 100, "m": 200, "a": 20, "b": 260},
			map[string]string{"m->u#0": "left", "b->m#0": "right", "m->b#0": "bottom", "m->a#0": "left"}},
		// mirrored: m->u leaves from the right, b->m arrives on the left;
		// m->a heads farther, to the right, the side m->u already leaves
		// from: m->a shares it with m->u, m->b takes the bottom, and b->m
		// stays put
		{"an exit shares the farther side with a leaving end, mirrored", []string{"u->m:diamond", "m->u", "m->b", "m->a", "b->m"},
			[]int{0, 1, 2, 2}, []bool{false, true, false, false, true},
			map[string]float64{"u": 300, "m": 200, "b": 140, "a": 380},
			map[string]string{"m->u#0": "right", "b->m#0": "left", "m->b#0": "bottom", "m->a#0": "right"}},
		// m->u and m->w leave from both sides; m->b heads farther, to the
		// right, and shares that side with m->w
		{"leaving ends hold both sides", []string{"u->m:diamond", "m->u", "w->m", "m->w", "m->a", "m->b"},
			[]int{0, 1, 0, 2, 2}, []bool{false, true, false, true, false, false},
			map[string]float64{"u": 100, "w": 300, "m": 200, "a": 180, "b": 400},
			map[string]string{"m->u#0": "left", "m->w#0": "right", "m->b#0": "right", "m->a#0": "bottom"}},
	} {
		t.Run(tc.name, tc.run)
	}
}

// TestRoute_ThreeExitsShareTheHeldSides pins S8's diamonds and circles:
// when counter-flow ends hold a side vertex of a diamond, so that two of
// its three exits would share the bottom one (C8.5), the exits take left,
// bottom and right in order of heading, sharing each side with the
// counter-flow ends there, arriving or leaving.
func TestRoute_ThreeExitsShareTheHeldSides(t *testing.T) {
	for _, tc := range []pinnedCase{
		{"an arriving end holds the right", []string{"m:diamond->a", "m->b", "m->c", "c->m"},
			[]int{0, 1, 1, 1}, []bool{false, false, false, true},
			map[string]float64{"m": 200, "a": 100, "b": 200, "c": 300},
			map[string]string{"m->a#0": "left", "m->b#0": "bottom", "m->c#0": "right", "c->m#0": "right"}},
		{"a leaving end holds the left", []string{"u->m:diamond", "m->u", "m->a", "m->b", "m->c"},
			[]int{0, 1, 2, 2, 2}, []bool{false, true, false, false, false},
			map[string]float64{"u": 100, "m": 200, "a": 100, "b": 200, "c": 300},
			map[string]string{"m->u#0": "left", "m->a#0": "left", "m->b#0": "bottom", "m->c#0": "right"}},
		{"arriving ends hold both sides", []string{"m:diamond->a", "m->b", "m->c", "a->m", "c->m"},
			[]int{0, 1, 1, 1}, []bool{false, false, false, true, true},
			map[string]float64{"m": 200, "a": 100, "b": 200, "c": 300},
			map[string]string{"a->m#0": "left", "c->m#0": "right", "m->a#0": "left", "m->b#0": "bottom", "m->c#0": "right"}},
	} {
		t.Run(tc.name, tc.run)
	}
}

// mergedCase is a pinnedCase whose trunk lists the exits of m that leave
// its bottom vertex as a merged fan-out.
type mergedCase struct {
	pinnedCase
	trunk []string
}

// run checks the vertex of m each edge meets, and that the trunk's exits
// run down from the bottom vertex together, at least the stub C7 asks
// (20 px), into one lane, where each turns off toward its own column: one
// trunk, then a bracket (S8, Lanes).
func (tc mergedCase) run(t *testing.T) {
	t.Helper()
	tc.pinnedCase.run(t)
	top, route := tc.layout(t)
	bottom := model.Point{X: 200, Y: top + 40}
	trunk := route(tc.trunk[0])
	require.GreaterOrEqual(t, len(trunk), 3, "%s jogs: %v", tc.trunk[0], trunk)
	assert.GreaterOrEqual(t, trunk[1].Y-bottom.Y, 20.0, "the trunk keeps C7's stub: %v", trunk)
	var split []float64
	for _, id := range tc.trunk {
		got := route(id)
		require.GreaterOrEqual(t, len(got), 3, "%s jogs: %v", id, got)
		assert.Equal(t, []model.Point{bottom, {X: bottom.X, Y: trunk[1].Y}}, got[:2], "%s runs down the trunk: %v", id, got)
		assert.InDelta(t, got[1].Y, got[2].Y, 1e-9, "%s turns off along the trunk's lane: %v", id, got)
		split = append(split, got[2].X)
	}
	for i := range split {
		for j := i + 1; j < len(split); j++ {
			assert.NotEqual(t, split[i], split[j], "%s and %s split off the trunk", tc.trunk[i], tc.trunk[j])
		}
	}
}

// TestRoute_ACirclesExitsMerge pins S8's diamonds and circles: a
// circle's two or more forward exits all leave its bottom vertex, a
// fan-out merged onto one trunk (C8.5), however far apart they head and
// whatever side vertices its counter-flow ends hold, and those ends keep
// their sides (C8.4): none gives way to an exit. A diamond with the same
// exits spreads them over its vertices (C8.5 forbids it a shared one).
func TestRoute_ACirclesExitsMerge(t *testing.T) {
	for _, tc := range []mergedCase{
		// beyond the band on opposite sides: a diamond's left and right
		{pinnedCase{"two exits heading apart", []string{"m:circle->a", "m->b"},
			[]int{0, 1, 1}, nil,
			map[string]float64{"m": 200, "a": 100, "b": 300},
			map[string]string{"m->a#0": "bottom", "m->b#0": "bottom"}},
			[]string{"m->a#0", "m->b#0"}},
		// one heading at the center: a diamond sends the other to its side
		{pinnedCase{"two exits, one heading at the center", []string{"m:circle->a", "m->b"},
			[]int{0, 1, 1}, nil,
			map[string]float64{"m": 200, "a": 200, "b": 330},
			map[string]string{"m->a#0": "bottom", "m->b#0": "bottom"}},
			[]string{"m->b#0"}},
		{pinnedCase{"three exits", []string{"m:circle->a", "m->b", "m->c"},
			[]int{0, 1, 1, 1}, nil,
			map[string]float64{"m": 200, "a": 60, "b": 180, "c": 330},
			map[string]string{"m->a#0": "bottom", "m->b#0": "bottom", "m->c#0": "bottom"}},
			[]string{"m->a#0", "m->b#0", "m->c#0"}},
		// four or more: a diamond shares its sides by heading
		{pinnedCase{"four exits beyond the band", []string{"m:circle->a", "m->b", "m->c", "m->d"},
			[]int{0, 1, 1, 1, 1}, nil,
			map[string]float64{"m": 200, "a": -100, "b": 20, "c": 380, "d": 500},
			map[string]string{"m->a#0": "bottom", "m->b#0": "bottom", "m->c#0": "bottom", "m->d#0": "bottom"}},
			[]string{"m->a#0", "m->b#0", "m->c#0", "m->d#0"}},
		// an arriving end holds the right: a diamond's three exits take
		// left, bottom and right, sharing the right with c->m
		{pinnedCase{"three exits, an arriving end on the right", []string{"m:circle->a", "m->b", "m->c", "c->m"},
			[]int{0, 1, 1, 1}, []bool{false, false, false, true},
			map[string]float64{"m": 200, "a": 60, "b": 180, "c": 330},
			map[string]string{"m->a#0": "bottom", "m->b#0": "bottom", "m->c#0": "bottom", "c->m#0": "right"}},
			[]string{"m->a#0", "m->b#0", "m->c#0"}},
		// both arrivals from the right: a diamond moves a->m over beside
		// b->m and sends m->b out of the right vertex; the circle's ends
		// stay where they claimed (a->m the right, b->m the free left)
		{pinnedCase{"two arrivals", []string{"m:circle->a", "a->m", "m->b", "b->m"},
			[]int{0, 1, 1}, []bool{false, true, false, true},
			map[string]float64{"m": 200, "a": 300, "b": 450},
			map[string]string{"a->m#0": "right", "b->m#0": "left", "m->a#0": "bottom", "m->b#0": "bottom"}},
			[]string{"m->a#0", "m->b#0"}},
		{pinnedCase{"a leaving end and an arrival", []string{"u->m:circle", "m->u", "m->a", "m->b", "b->m"},
			[]int{0, 1, 2, 2}, []bool{false, true, false, false, true},
			map[string]float64{"u": 100, "m": 200, "a": 20, "b": 260},
			map[string]string{"m->u#0": "left", "b->m#0": "right", "m->a#0": "bottom", "m->b#0": "bottom"}},
			[]string{"m->a#0", "m->b#0"}},
		// a leaving end holds the left: a diamond's three exits take
		// left, bottom and right, sharing the left with m->u
		{pinnedCase{"three exits, a leaving end on the left", []string{"u->m:circle", "m->u", "m->a", "m->b", "m->c"},
			[]int{0, 1, 2, 2, 2}, []bool{false, true, false, false, false},
			map[string]float64{"u": 100, "m": 200, "a": 60, "b": 180, "c": 330},
			map[string]string{"m->u#0": "left", "m->a#0": "bottom", "m->b#0": "bottom", "m->c#0": "bottom"}},
			[]string{"m->a#0", "m->b#0", "m->c#0"}},
	} {
		t.Run(tc.name, tc.run)
	}
	// the first case with a diamond: its two exits take its side vertices
	pinnedCase{"a diamond", []string{"m:diamond->a", "m->b"},
		[]int{0, 1, 1}, nil,
		map[string]float64{"m": 200, "a": 100, "b": 300},
		map[string]string{"m->a#0": "left", "m->b#0": "right"}}.run(t)
}

func TestRoute_ExitsSpreadOverTheVerticesLeft(t *testing.T) {
	g := graph(t, nil, "m:diamond->a", "m->c", "c->m")
	// c->m holds the right vertex: of the two exits, the one heading
	// farthest left takes the left vertex and the other the bottom
	_, route := routed(t, g, screen, map[string]float64{"m": 200, "a": 100, "c": 300})
	assert.Equal(t, model.Point{X: 160, Y: 20}, route("m->a#0")[0])
	assert.Equal(t, model.Point{X: 200, Y: 40}, route("m->c#0")[0])
}

func TestRoute_FourExitsBeyondTheBandTakeTheFreeSide(t *testing.T) {
	g := graph(t, nil, "m:diamond->a", "m->b", "m->c", "m->d", "d->m")
	// d->m holds the right vertex, so the left one is the only side free.
	// Of four exits, every one heading beyond the band (16) on the left
	// takes it: a and b. c heads at the center and d toward the held
	// right side: both take the bottom (S8, C8.5).
	_, route := routed(t, g, screen, map[string]float64{"m": 400, "a": 100, "b": 220, "c": 400, "d": 520})
	dm := route("d->m#0")
	assert.Equal(t, model.Point{X: 440, Y: 20}, dm[len(dm)-1], "d->m enters m's right vertex")
	left, bottom := model.Point{X: 360, Y: 20}, model.Point{X: 400, Y: 40}
	assert.Equal(t, left, route("m->a#0")[0])
	assert.Equal(t, left, route("m->b#0")[0])
	assert.Equal(t, bottom, route("m->c#0")[0])
	assert.Equal(t, bottom, route("m->d#0")[0])
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
		assert.NotEqual(t, 260.0, p.X, "n->u must not run on m's loop column: %v", nu)
	}
	assert.Equal(t, ports.Bottom, r.Side[3][ports.Tail], "n keeps its face port instead of a blocked side column")
}

func TestRoute_ANudgedPortNeverLeavesADiagonal(t *testing.T) {
	lv := lgraphtest.Level(t, "a->c", "b->d", "d->c")
	g, err := lgraph.Build(lv, []int{0, 2, 0, 1}, nil, 8) // a, c, b, d
	require.NoError(t, err)
	// a->c's port on a lands on b->d's on d and moves right by the in-lane
	// gap, to within Snap of its own dummy: the dummy follows it
	_, route := routed(t, g, screen, map[string]float64{"a": 100, "b": 300, "d": 100, "c": 200, "a->c#0@1": 108.3})
	ac := route("a->c#0")
	for i := 1; i < len(ac); i++ {
		assert.True(t, ac[i-1].X == ac[i].X || ac[i-1].Y == ac[i].Y, "segment %d of %v", i-1, ac)
	}
	assert.Equal(t, ac[0].X, ac[1].X)
	assert.InDelta(t, 108, ac[0].X, 1e-9)
}

func TestRoute_ASideAttachmentMeetsASlantedSide(t *testing.T) {
	g := graph(t, nil, "a->p:parallelogram", "p->a")
	_, route := twice(t, g, screen, map[string]float64{"a": 200, "p": 200})
	// the parallelogram's left side leans in by half its slant (0.3 x 40)
	// at mid-height; a->p shares no bundle with the side-attached p->a: it
	// keeps its in-port at the middle of the stretch p's top and bottom
	// edges share, p's center line (200), under a's out-port
	assertRoute(t, pts(200, 40, 200, 80), route("a->p#0"))
	assertRoute(t, pts(166, 100, 140, 100, 140, 20, 160, 20), route("p->a#0"))
}

func TestRoute_APortClearsAnotherNodesPort(t *testing.T) {
	// a above d and b above e, each pair aligned: a->e's port on a lands
	// on b->d's port on d, and b->d's on b on a->e's on e, so the two
	// jogs would swap columns and share a run whatever their lanes
	g := graph(t, nil, "a->d", "a->e", "b->d")
	_, route := routed(t, g, screen, map[string]float64{"a": 40, "d": 40, "b": 160, "e": 160})
	ae, bd := route("a->e#0"), route("b->d#0")
	assert.InDelta(t, 40+40.0/3+8, ae[0].X, 1e-6, "moved right by the in-lane gap")
	assert.InDelta(t, 168, ae[len(ae)-1].X, 1e-6)
	assert.InDelta(t, 160, bd[0].X, 1e-6, "now clear: stays")
	assert.InDelta(t, 40+40.0/3, bd[len(bd)-1].X, 1e-6)
}

// TestRoute_ANudgedPortStaysOnItsUsableSpan pins S8's ports: a port that
// moves off another node's port moves only over the middle nine tenths of
// its face's usable span (Shape ports). Under DOWN, hexagon h's usable
// span is its flat top, ±20 (its middle nine tenths ±18): c->h's in-port
// at +10 lands 2 px from d's out-port. +18 is not clear of d, +2 not of
// h's port at 0, and +26 lies on the angled side; -6 and -14 are clear,
// but each passes another of h's ports. The port moves to -6, still on
// the flat top, and h's in-ports then take their positions in their
// order, as their far ends lie: a->h's -10, b->h's -6, c->h's 0. Under
// RIGHT, cylinder h's usable span is its side line between the caps,
// ±25.6 (±23.04): of four in-ports, c->h's at +15.36 lands 4 px from d's
// out-port. +23.36 is not clear of d, +7.36 not of h's port at +5.12,
// +31.36 lies on a cap, and -8.64 and -16.64 are not clear of h's ports
// at -5.12 and -15.36; -0.64 is clear but passes f->h's port. The port
// moves there, and the face takes its positions in order: f->h's -0.64,
// c->h's +5.12. Every port stays on the usable span, and no two of h's
// wires cross, where the move alone inverted two of its ports.
func TestRoute_ANudgedPortStaysOnItsUsableSpan(t *testing.T) {
	for _, tc := range []struct {
		name   string
		dir    model.Direction
		specs  []string
		xs     map[string]float64
		want   []float64 // h's in-ports, in the order of in
		usable float64   // the middle nine tenths of h's usable span, either side of its center
		in     []string  // h's in-edges, their far ends left to right
	}{
		{"hexagon, DOWN", model.Down,
			[]string{"a", "b", "d", "c", "h:hexagon", "e", "a->h", "b->h", "c->h", "d->e"},
			map[string]float64{"a": 0, "b": 100, "d": 212, "c": 400, "h": 200, "e": 500},
			[]float64{190, 194, 200}, 18, []string{"a->h#0", "b->h#0", "c->h#0"}},
		{"cylinder, RIGHT", model.Right,
			[]string{"a", "b", "d", "f", "c", "h:cylinder", "e", "a->h", "b->h", "f->h", "c->h", "d->e"},
			map[string]float64{"a": 0, "b": 100, "d": 219.36, "f": 300, "c": 400, "h": 200, "e": 500},
			[]float64{184.64, 194.88, 199.36, 205.12}, 23.04, []string{"a->h#0", "b->h#0", "f->h#0", "c->h#0"}},
	} {
		o := screen
		o.Dir = tc.dir
		res, route := routed(t, graph(t, nil, tc.specs...), o, tc.xs)
		var wires [][]model.Point
		for k, e := range tc.in {
			got := route(e)
			end := got[len(got)-1]
			wires = append(wires, got)
			assert.InDelta(t, tc.want[k], end.X, 1e-9, "%s: %s: %v", tc.name, e, got)
			assert.LessOrEqual(t, math.Abs(end.X-200), tc.usable, "%s: %s on the usable span: %v", tc.name, e, got)
		}
		ch := route("c->h#0")
		assert.Equal(t, res.RowTop[1], ch[len(ch)-1].Y, "%s: on the face's straight run, not an angled side: %v", tc.name, ch)
		for k, cs := range Crossings(wires, 5) {
			assert.Empty(t, cs, "%s: %s crosses another of h's wires", tc.name, tc.in[k])
		}
	}
}

// exits returns the routes of the edges ids, in their order, and asserts
// that they leave their node left to right and never cross each other.
func exits(t *testing.T, route func(id string) []model.Point, ids ...string) [][]model.Point {
	t.Helper()
	var wires [][]model.Point
	for k, id := range ids {
		wires = append(wires, route(id))
		if k > 0 {
			assert.Less(t, wires[k-1][0].X, wires[k][0].X, "%s leaves left of %s", ids[k-1], id)
		}
	}
	for k, cs := range Crossings(wires, 5) {
		assert.Empty(t, cs, "%s crosses another exit of its node", ids[k])
	}
	return wires
}

// TestRoute_AGiveWayPassesNoPortWhereASpotNeedNot pins S8's ports: a port
// that gives way takes the first clear spot that passes no other end of
// its node, though a spot past one comes first. h, 48 wide, has three
// exits at -12, 0 and +12; m's in-port at -6 and n's at +4 sit beside
// them. h->a's port gives way first, to -20 (-4 is not clear of m), and
// leaves room on the left. h->b's at 0 then gives way: +8 is not clear of
// n, -8 not of m, +16 is clear but passes h->c's port at +12, and -16 is
// clear and passes none. It takes -16, and h's exits keep the order of
// their far ends.
func TestRoute_AGiveWayPassesNoPortWhereASpotNeedNot(t *testing.T) {
	g := graph(t, map[string]float64{"h": 48, "m": 4, "n": 4},
		"h", "u", "w", "a", "m", "n", "b", "c", "h->a", "h->b", "h->c", "u->m", "w->n")
	_, route := routed(t, g, screen, map[string]float64{
		"h": 0, "u": 300, "w": 400, "a": -100, "m": -6, "n": 4, "b": 100, "c": 200})
	wires := exits(t, route, "h->a#0", "h->b#0", "h->c#0")
	assert.InDelta(t, -20, wires[0][0].X, 1e-9, "h->a moved left, clear of m")
	assert.InDelta(t, -16, wires[1][0].X, 1e-9, "h->b took the spot past no port")
	assert.InDelta(t, 12, wires[2][0].X, 1e-9, "h->c stays")
}

// TestRoute_AGiveWayPastAPortKeepsItsFaceInOrder pins S8's ports: when
// every clear spot passes another end of the node, the port takes the
// first and its face's ports then take their positions in their order.
// h, 40 wide, has three exits at 190, 200 and 210; e's in-port at 212
// sits 2 px from h->c's. +8 is not clear of e, -8 not of h->b's port, +16
// lies off the face's middle nine tenths, -16 (194) is clear but passes
// h->b's port and -24 (186) passes both. h->c takes 194, and the face's
// positions 190, 194 and 200 go to h->a, h->b and h->c: the exits keep
// the order of their far ends, as S6 chose it, and do not cross.
func TestRoute_AGiveWayPastAPortKeepsItsFaceInOrder(t *testing.T) {
	g := graph(t, map[string]float64{"h": 40}, "h", "d", "a", "b", "e", "c", "h->a", "h->b", "h->c", "d->e")
	_, route := routed(t, g, screen, map[string]float64{"h": 200, "d": 500, "a": 0, "b": 100, "e": 212, "c": 400})
	wires := exits(t, route, "h->a#0", "h->b#0", "h->c#0")
	for k, want := range []float64{190, 194, 200} {
		assert.InDelta(t, want, wires[k][0].X, 1e-9, "exit %d", k)
	}
	de := route("d->e#0")
	assert.InDelta(t, 212, de[len(de)-1].X, 1e-9, "e's in-port is clear of h->c's at 200: stays")
}

func TestCrossings(t *testing.T) {
	h := pts(0, 50, 100, 50)
	v := pts(40, 0, 40, 100)
	got := Crossings([][]model.Point{h, v}, 5)
	assert.Empty(t, got[0])
	assert.Equal(t, []model.Crossing{{SegmentIndex: 0, T: 0.5}}, got[1], "on the later edge, along its segment")

	corner := pts(3, -50, 3, 50)
	got = Crossings([][]model.Point{pts(0, 0, 100, 0), corner}, 5)
	assert.Empty(t, got[1], "within the radius of a segment end: a corner")

	got = Crossings([][]model.Point{pts(0, 0, 100, 0), pts(50, 0, 50, 100)}, 5)
	assert.Empty(t, got[1], "touching at a segment's end is no crossing")
}

// TestRoute_ASideColumnNeverGivesWayInward pins S8's stops: when two
// locked stops of one wire meet within Snap, the side that may move gives
// way, a side column only outward: r's column, Reach out, does not give
// way inward, and p's moves out onto it. When neither may move, the one
// that does not carry the arrowhead gives way all the same, a side column
// even inward: here r's, since the arrowhead is at p.
func TestRoute_ASideColumnNeverGivesWayInward(t *testing.T) {
	// r->p is reversed; replayed, it leaves r's right face and enters p's.
	// p is 40 wide, so its column sits over the dummy at 159.7, farther out
	// than Reach; r's column sits Reach out, at 160. The two meet within
	// Snap: p's moves out onto r's, which would otherwise be 0.3 px short.
	g := graph(t, map[string]float64{"p": 40}, "p->q", "q->r", "r->p", "n")
	o := screen
	o.Side = [][2]ports.Vertex{{ports.Bottom, ports.Bottom}, {ports.Bottom, ports.Bottom}, {ports.Right, ports.Right}}
	xs := map[string]float64{"p": 100, "q": 100, "r": 100, "r->p#0@1": 159.7, "n": 300}
	_, route := routed(t, g, o, xs)
	got := route("r->p#0")
	assert.Equal(t, 140.0, got[0].X, "leaves r's right face: %v", got)
	assert.Equal(t, 160.0, got[1].X, "on r's column, Reach out: %v", got)
	assert.Equal(t, 160.0, got[len(got)-2].X, "p's column moved out onto it: %v", got)

	// n sits in p's row, 12.1 px from p's column: moving that column out
	// would bring it within the node clearance, so r's gives way instead
	xs["n"] = 159.7 + 12.1 + 40
	_, route = routed(t, g, o, xs)
	got = route("r->p#0")
	assert.InDelta(t, 159.7, got[len(got)-2].X, 1e-9, "p's column keeps its row clear: %v", got)
	assert.InDelta(t, 159.7, got[1].X, 1e-9, "r's column gives way: %v", got)
}

// shaped is graph with fix applied to the level before S2 and S3: the
// groups, terminals and anchors S9 sets.
func shaped(t *testing.T, fix func(lv *lgraph.Level), specs ...string) *lgraph.Graph {
	t.Helper()
	lv := lgraphtest.Level(t, specs...)
	fix(lv)
	ctx := context.Background()
	reversed := cycle.Break(ctx, lv, nil)
	g, err := lgraph.Build(lv, rank.Assign(ctx, lv, reversed, nil), reversed, 8)
	require.NoError(t, err)
	return g
}

// TestRoute_ABundleFollowsItsAnchors pins S8 (S9): two parallel edges into
// a group keep their anchored in-ports, and their out-ports follow them,
// so the bundle does not cross itself.
func TestRoute_ABundleFollowsItsAnchors(t *testing.T) {
	g := shaped(t, func(lv *lgraph.Level) {
		lv.Nodes[1].Group, lv.Nodes[1].W, lv.Nodes[1].H = true, 300, 200
		lv.Edges[0].Anchor[1] = lgraph.Anchor{On: true, At: 50}
		lv.Edges[1].Anchor[1] = lgraph.Anchor{On: true, At: -50}
	}, "p->g", "p->g")
	_, route := routed(t, g, screen, map[string]float64{"p": 0, "g": 0})
	first, second := route("p->g#0"), route("p->g#1")
	assert.Equal(t, 50.0, first[len(first)-1].X, "on its anchor")
	assert.Equal(t, -50.0, second[len(second)-1].X, "on its anchor")
	assert.Greater(t, first[0].X, second[0].X, "the out-ports follow: p->g#0 leaves right")
}

// TestRoute_AnAnchoredPortNeverMoves pins S8 (S9): a port anchored on a
// group's face keeps its x where another node's port would make an
// unanchored one move by InLaneGap.
func TestRoute_AnAnchoredPortNeverMoves(t *testing.T) {
	fix := func(anchored bool) func(lv *lgraph.Level) {
		return func(lv *lgraph.Level) {
			lv.Nodes[1].Group, lv.Nodes[1].W, lv.Nodes[1].H = true, 300, 200
			if anchored {
				lv.Edges[0].Anchor[1] = lgraph.Anchor{On: true, At: -100}
			}
		}
	}
	specs := []string{"p->g", "q->r"}
	xs := map[string]float64{"p": -100, "g": 0, "q": -97, "r": -97}
	_, route := routed(t, shaped(t, fix(true), specs...), screen, xs)
	pg := route("p->g#0")
	assert.Equal(t, -100.0, pg[len(pg)-1].X, "anchored: stays")
	g := shaped(t, fix(false), specs...)
	g.Level.Nodes[1].W = 80
	_, route = routed(t, g, screen, map[string]float64{"p": -100, "g": -100, "q": -97, "r": -97})
	pg = route("p->g#0")
	assert.NotEqual(t, -100.0, pg[len(pg)-1].X, "a slot port beside q's column moves")
}

// TestRoute_AGroupEndNeverAttachesAtASide pins S8 (S9): a reversed edge's
// end on a group keeps its anchored face port; only its end on a node
// attaches at a side.
func TestRoute_AGroupEndNeverAttachesAtASide(t *testing.T) {
	g := shaped(t, func(lv *lgraph.Level) {
		lv.Nodes[0].Group, lv.Nodes[0].W, lv.Nodes[0].H = true, 300, 200
		lv.Edges[0].Anchor[0] = lgraph.Anchor{On: true, At: -60}
		lv.Edges[1].Anchor[1] = lgraph.Anchor{On: true, At: 60}
	}, "g->a", "a->g")
	res, route := routed(t, g, screen, map[string]float64{"g": 0, "a": 0})
	back := route("a->g#0")
	assert.Equal(t, model.Point{X: 60, Y: 200}, back[len(back)-1], "on g's bottom face, at its anchor")
	assert.Equal(t, ports.Bottom, res.Side[1][ports.Head], "g's end stays on its face")
	assert.Equal(t, ports.Right, res.Side[1][ports.Tail], "a's end attaches at its side")
}

// TestRoute_ExitsHeadForAnchoredPorts pins S8's exit rule (S9): a
// diamond's two exits into one group head for their anchored ports, so the
// one anchored left takes the left vertex.
func TestRoute_ExitsHeadForAnchoredPorts(t *testing.T) {
	g := shaped(t, func(lv *lgraph.Level) {
		lv.Nodes[1].Group, lv.Nodes[1].W, lv.Nodes[1].H = true, 400, 200
		lv.Edges[0].Anchor[1] = lgraph.Anchor{On: true, At: -120}
		lv.Edges[1].Anchor[1] = lgraph.Anchor{On: true, At: 120}
	}, "m:diamond->g", "m->g")
	_, route := routed(t, g, screen, map[string]float64{"m": 0, "g": 0})
	assert.Equal(t, model.Point{X: -40, Y: 20}, route("m->g#0")[0], "the left vertex")
	assert.Equal(t, model.Point{X: 40, Y: 20}, route("m->g#1")[0], "the right vertex")
}

// TestRoute_ATerminalChannelCloses pins S8 (S9): the channel below a row of
// terminals closes when it holds no lane, and keeps its height when a
// terminal's wire jogs there.
func TestRoute_ATerminalChannelCloses(t *testing.T) {
	fix := func(lv *lgraph.Level) { lv.Nodes[0].Terminal, lv.Nodes[0].W, lv.Nodes[0].H = true, 8, 0 }
	res, route := routed(t, shaped(t, fix, "t->n"), screen, map[string]float64{"t": 0, "n": 0})
	assert.Equal(t, []float64{0, 0}, res.RowTop, "straight: the rows meet")
	assert.Equal(t, pts(0, 0), route("t->n#0"), "a zero-length piece the join drops (S9)")
	res, _ = routed(t, shaped(t, fix, "t->n"), screen, map[string]float64{"t": 30, "n": 0})
	assert.Equal(t, []float64{0, 48}, res.RowTop, "a jog keeps BaseGap and its lane")
}

// TestRoute_AnAnchoredPortNeverSlides pins S8's stops (S9): an anchored
// port never slides, whichever end of the wire it holds; the port at the
// wire's other end slides onto its anchor.
func TestRoute_AnAnchoredPortNeverSlides(t *testing.T) {
	for _, tc := range []struct {
		spec, id string
		group    int // the group's node
		end      int // its end of the edge
	}{
		{"g->p", "g->p#0", 0, 0},
		{"p->g", "p->g#0", 1, 1},
	} {
		g := shaped(t, func(lv *lgraph.Level) {
			lv.Nodes[tc.group].Group, lv.Nodes[tc.group].W, lv.Nodes[tc.group].H = true, 300, 200
			lv.Edges[0].Anchor[tc.end] = lgraph.Anchor{On: true, At: 10}
		}, tc.spec)
		_, route := routed(t, g, screen, map[string]float64{"g": 0, "p": 0})
		got := route(tc.id)
		require.Len(t, got, 2, "%s straight: %v", tc.spec, got)
		assert.Equal(t, 10.0, got[0].X, "%s on the anchor: %v", tc.spec, got)
	}
}

// TestRoute_ATerminalsPortNeverSlides pins S8's stops (S9): a terminal's
// port, where the parent level anchors the wire, never slides; the port
// at the wire's other end slides onto it, and the terminal's channel
// closes. On the terminal's own face, 3 px out, it could.
func TestRoute_ATerminalsPortNeverSlides(t *testing.T) {
	fix := func(lv *lgraph.Level) { lv.Nodes[0].Terminal, lv.Nodes[0].W, lv.Nodes[0].H = true, 8, 0 }
	res, route := routed(t, shaped(t, fix, "t->n"), screen, map[string]float64{"t": 0, "n": 3})
	assert.Equal(t, pts(0, 0), route("t->n#0"), "on the terminal's x, a zero-length piece the join drops")
	assert.Equal(t, []float64{0, 0}, res.RowTop, "no jog: the rows meet")
}

// TestPack_SideBySideVerticalsTakeTheOtherOrder pins S8's lanes: a unit
// whose column reaches its lane beside a column that leaves another
// unit's lane downward sits above that unit, whatever that crosses.
func TestPack_SideBySideVerticalsTakeTheOtherOrder(t *testing.T) {
	r := &router{o: screen, lanes: laneMap{}}
	jogs := []jog{
		{edge: 0, from: 40, to: -20, toRail: -1, fromRail: -1, head: -1},  // leaves its lane down at -20
		{edge: 1, from: -20, to: 100, toRail: -1, fromRail: -1, head: -1}, // reaches its lane from above at -20
	}
	assert.Equal(t, 2, r.pack(0, jogs, nil))
	assert.Equal(t, 0, r.lanes[[2]int{1, 0}], "edge 1 above, though edge 0's column then crosses its run")
	assert.Equal(t, 1, r.lanes[[2]int{0, 0}])
}

// TestPack_ACycleReleasesACrossingBeforeSideBySideVerticals pins S8's
// lanes: four jogs of one text channel (probe seed 388, DOWN, inside G1)
// whose constraints form a cycle. Three keep two verticals from running
// side by side (d above b, c above d, a above c) and one only avoids a
// crossing (b above c). The cycle releases b, held by no side-by-side
// constraint: b crosses c, and no two verticals share a column. Releasing
// the unit with the fewest constraints, b first in edge order, once put b
// above d, d's terminal column on b's port column (C9.1).
func TestPack_ACycleReleasesACrossingBeforeSideBySideVerticals(t *testing.T) {
	r := &router{o: text, lanes: laneMap{}}
	jogs := []jog{
		{edge: 0, from: 35.5, to: 46.5, toRail: -1, fromRail: -1, head: -1}, // a
		{edge: 1, from: 37.5, to: 32.5, toRail: -1, fromRail: -1, head: -1}, // b, leaves its lane down at 32.5
		{edge: 2, from: 39.5, to: 34.5, toRail: -1, fromRail: -1, head: -1}, // c
		{edge: 3, from: 32.5, to: 39.5, toRail: -1, fromRail: -1, head: -1}, // d, reaches its lane from above at 32.5
	}
	assert.Equal(t, 4, r.pack(0, jogs, nil))
	lanes := []int{r.lanes[[2]int{0, 0}], r.lanes[[2]int{1, 0}], r.lanes[[2]int{2, 0}], r.lanes[[2]int{3, 0}]}
	assert.Equal(t, []int{0, 3, 1, 2}, lanes, "a, c, d, b from the top: b crosses c")
}

// TestPack_ACycleReleasesACrossingBeforeTwoVerticalsSideBySide pins S8's
// lanes: three jogs in a cycle of two side-by-side constraints and one
// crossing constraint. b above a keeps b's column from above (11) off a's
// column down (12), a column away; c above b keeps c's column from above
// (4) off b's column down (3); a above c only keeps a's column from above
// (5) off c's run (4..7). The cycle releases c, held by no side-by-side
// constraint, though a comes first in edge order: a's column crosses c's
// run, and no two verticals run side by side.
func TestPack_ACycleReleasesACrossingBeforeTwoVerticalsSideBySide(t *testing.T) {
	r := &router{o: text, lanes: laneMap{}}
	jogs := []jog{
		{edge: 0, from: 5, to: 12, toRail: -1, fromRail: -1, head: -1}, // a
		{edge: 1, from: 11, to: 3, toRail: -1, fromRail: -1, head: -1}, // b
		{edge: 2, from: 4, to: 7, toRail: -1, fromRail: -1, head: -1},  // c
	}
	assert.Equal(t, 3, r.pack(0, jogs, nil))
	lanes := []int{r.lanes[[2]int{0, 0}], r.lanes[[2]int{1, 0}], r.lanes[[2]int{2, 0}]}
	assert.Equal(t, []int{2, 1, 0}, lanes, "c, b, a from the top")
}

// TestPack_BothOrdersSideBySideRuleOutTheOneOnOneColumn pins S8's lanes:
// two jogs of one text channel (probe seed 376, DOWN, with a congruent
// copy) that each order draws with two verticals one column apart or
// closer. Edge 0 above puts edge 1's column from above (-4.5) on edge 0's
// column down (-4.5), a shared run (C9.1); edge 1 above puts them a column
// apart (1.5 and 0.5, C9.2). The one-column order is ruled out, though
// the other crosses: edge 0's column down from 1.5 is not on edge 1's run.
func TestPack_BothOrdersSideBySideRuleOutTheOneOnOneColumn(t *testing.T) {
	r := &router{o: text, lanes: laneMap{}}
	jogs := []jog{
		{edge: 0, from: 1.5, to: -4.5, toRail: -1, fromRail: -1, head: -1},
		{edge: 1, from: -4.5, to: 0.5, toRail: -1, fromRail: -1, head: -1},
	}
	assert.Equal(t, 2, r.pack(0, jogs, nil))
	assert.Equal(t, 0, r.lanes[[2]int{1, 0}], "edge 1 above")
	assert.Equal(t, 1, r.lanes[[2]int{0, 0}])
}

// TestPack_ASideBySideCycleKeepsVerticalsOffOneColumn pins S8's lanes:
// three jogs whose side-by-side constraints alone form a cycle (q above
// p, p above r, r above q), so one must give. q above p keeps two
// verticals off one column: below p, q's column from above would run on
// p's column down, both at 0. The other two keep them a column apart. The
// cycle releases q, held by no one-column constraint, though p comes
// first in edge order: r lands under q, its column from above (11) a
// column from q's column down (10, C9.2), and q stays above p.
func TestPack_ASideBySideCycleKeepsVerticalsOffOneColumn(t *testing.T) {
	r := &router{o: text, lanes: laneMap{}}
	jogs := []jog{
		{edge: 0, from: 21, to: 0, toRail: -1, fromRail: -1, head: -1},  // p
		{edge: 1, from: 0, to: 10, toRail: -1, fromRail: -1, head: -1},  // q
		{edge: 2, from: 11, to: 20, toRail: -1, fromRail: -1, head: -1}, // r
	}
	assert.Equal(t, 3, r.pack(0, jogs, nil))
	lanes := []int{r.lanes[[2]int{0, 0}], r.lanes[[2]int{1, 0}], r.lanes[[2]int{2, 0}]}
	assert.Equal(t, []int{1, 0, 2}, lanes, "q, p, r from the top")
}

// TestPack_AFanPairThatCannotAgreeIsReleased pins S8's fan pairing (S12,
// Rule C): two first jogs heading apart from one source, one rank of a
// fan, whose verticals would run side by side unless the right-hand one
// sits above, cannot share a lane. The pair is released and the channel
// packs as if unpaired: as many lanes, none of them empty.
func TestPack_AFanPairThatCannotAgreeIsReleased(t *testing.T) {
	jogs := func() []jog {
		return []jog{
			{edge: 0, from: 100, to: 90, toRail: -1, fromRail: -1, head: 0},  // leaves its lane down at 90
			{edge: 1, from: 105, to: 200, toRail: -1, fromRail: -1, head: 0}, // reaches its lane from above at 105
		}
	}
	o := screen
	o.InLaneGap = 30
	wires := []wire{{stops: []float64{100, 90}}, {stops: []float64{105, 200}}}
	alone := &router{o: o, wires: wires, lanes: laneMap{}}
	want := alone.pack(0, jogs(), nil)
	require.Equal(t, 2, want, "unpaired, edge 1 above edge 0")

	o.Fans = [][]int{{0, 1}}
	r := &router{o: o, wires: wires, lanes: laneMap{}}
	assert.Equal(t, want, r.pack(0, jogs(), nil), "as many lanes as unpaired")
	used := make([]bool, want)
	for _, e := range []int{0, 1} {
		lane := r.lanes[[2]int{e, 0}]
		require.Less(t, lane, want, "edge %d", e)
		used[lane] = true
	}
	assert.Equal(t, []bool{true, true}, used, "no lane left empty")
	assert.Equal(t, alone.lanes, r.lanes, "the lanes unpaired")
}

// TestPack_AFanPairsItsSecondRankToo pins S8's fan pairing (S12, Rule
// C) past a fan's outer rank: four first jogs from one source, two
// heading left and two right, and another wire's jog that must sit above
// the inner left one (its column from above crosses that jog's span
// otherwise). Unpaired, the inner left jog drops below it while the inner
// right one stays a lane higher; paired, the fan's second rank, ranked
// from both ends of its columns, shares one lane as its first does, in as
// many lanes.
func TestPack_AFanPairsItsSecondRankToo(t *testing.T) {
	jogs := func() []jog {
		return []jog{
			{edge: 0, from: 100, to: 0, toRail: -1, fromRail: -1, head: 0},   // outer left
			{edge: 1, from: 104, to: 50, toRail: -1, fromRail: -1, head: 0},  // inner left
			{edge: 2, from: 108, to: 160, toRail: -1, fromRail: -1, head: 0}, // inner right
			{edge: 3, from: 112, to: 210, toRail: -1, fromRail: -1, head: 0}, // outer right
			{edge: 4, from: 70, to: 40, toRail: -1, fromRail: -1, head: -1},  // another wire, above the inner left
		}
	}
	wires := []wire{
		{stops: []float64{100, 0}}, {stops: []float64{104, 50}}, {stops: []float64{108, 160}},
		{stops: []float64{112, 210}}, {stops: []float64{70, 40}},
	}
	lanes := func(r *router) []int {
		out := make([]int, len(wires))
		for e := range out {
			out[e] = r.lanes[[2]int{e, 0}]
		}
		return out
	}
	alone := &router{o: screen, wires: wires, lanes: laneMap{}}
	require.Equal(t, 3, alone.pack(0, jogs(), nil))
	require.Equal(t, []int{0, 2, 1, 0, 1}, lanes(alone), "unpaired, the second rank on two lanes")

	o := screen
	o.Fans = [][]int{{0, 1, 2, 3}}
	r := &router{o: o, wires: wires, lanes: laneMap{}}
	assert.Equal(t, 3, r.pack(0, jogs(), nil), "as many lanes")
	assert.Equal(t, []int{0, 2, 2, 0, 1}, lanes(r), "each rank on one lane")
}

// TestPack_AFanPairsOnlyLoneJogsHeadingApart pins which units S8's fan
// pairing (S12, Rule C) pairs: a rank's two first jogs that run in the
// channel alone and head apart. Two heading the same way overlap at the
// source and are not paired, nor are the jogs of a rail, one unit drawn
// as one run, nor a rank whose other edge runs no first jog here.
func TestPack_AFanPairsOnlyLoneJogsHeadingApart(t *testing.T) {
	pairs := func(jogs []jog) [][2]int {
		wires := make([]wire, len(jogs))
		for _, j := range jogs {
			wires[j.edge] = wire{stops: []float64{j.from, j.to}}
		}
		o := screen
		o.Fans = [][]int{{0, 1}}
		r := &router{o: o, wires: wires, lanes: laneMap{}}
		return r.fanPairs(jogs, r.units(jogs, nil))
	}
	assert.Equal(t, [][2]int{{0, 1}}, pairs([]jog{
		{edge: 0, from: 100, to: 0, toRail: -1, fromRail: -1, head: 0},
		{edge: 1, from: 104, to: 200, toRail: -1, fromRail: -1, head: 0},
	}), "heading apart: paired")
	assert.Empty(t, pairs([]jog{
		{edge: 0, from: 100, to: 0, toRail: -1, fromRail: -1, head: 0},
		{edge: 1, from: 104, to: 50, toRail: -1, fromRail: -1, head: 0},
	}), "heading the same way")
	assert.Empty(t, pairs([]jog{
		{edge: 0, from: 100, to: 0, toRail: -1, fromRail: 0, head: 0},
		{edge: 1, from: 100, to: 200, toRail: -1, fromRail: 0, head: 0},
	}), "a rail out of one bottom vertex")
	assert.Empty(t, pairs([]jog{
		{edge: 0, from: 100, to: 0, toRail: -1, fromRail: -1, head: 0},
		{edge: 1, from: 150, to: 200, toRail: -1, fromRail: -1, head: -1},
	}), "edge 1's jog here is not its first")
}

// TestRoute_StopsSnapOntoAnAnchoredPort pins S8's stops (S9): a stop
// within Snap of an anchored port takes the port's x, and the port keeps
// it.
func TestRoute_StopsSnapOntoAnAnchoredPort(t *testing.T) {
	g := shaped(t, func(lv *lgraph.Level) {
		lv.Nodes[1].Group, lv.Nodes[1].W, lv.Nodes[1].H = true, 300, 200
		lv.Edges[0].Anchor[1] = lgraph.Anchor{On: true, At: 0.3}
	}, "p->g")
	_, route := routed(t, g, screen, map[string]float64{"p": 0, "g": 0})
	assert.Equal(t, pts(0.3, 40, 0.3, 80), route("p->g#0"), "straight, on the anchor")
}

// TestRoute_StraighteningNeverMovesAnAnchoredPort pins S8's stops (S9):
// an anchored port never moves, also when the straightening runs a wire
// on its other end's x. g->p leaves group g on its port anchored at 0.3
// and runs through a dummy at 10 to p's in-port at 0: the dummy lies
// between them, so no snap meets the anchor first, and the stops spread
// 10, within Span. Run on the in-port's x the wire would take the anchor
// along by 0.3; it runs straight on the anchor's x instead, p's in-port
// sliding onto it.
func TestRoute_StraighteningNeverMovesAnAnchoredPort(t *testing.T) {
	lv := lgraphtest.Level(t, "g->p")
	lv.Nodes[0].Group, lv.Nodes[0].W, lv.Nodes[0].H = true, 300, 200
	lv.Edges[0].Anchor[ports.Head] = lgraph.Anchor{On: true, At: 0.3}
	// layers: g 0, the dummy 1, p 2
	g, err := lgraph.Build(lv, []int{0, 2}, nil, 8)
	require.NoError(t, err)
	_, route := routed(t, g, screen, map[string]float64{"g": 0, "g->p#0@1": 10, "p": 0})
	assert.Equal(t, pts(0.3, 200, 0.3, 280), route("g->p#0"), "straight, on the anchor")
}

// text is the S14 text Config's routing under DOWN, in cells.
var text = Options{Clearance: 2, Span: 1, Reach: 3, BaseGap: 3, LaneGap: 1, InLaneGap: 2, Snap: 0.5, Dir: model.Down, Text: true}

// TestRoute_ASideColumnKeepsReachAgainstAnAnchoredPort pins S8's stops
// (S9): a back edge's side column meets its anchored port within Snap,
// inward. The port never moves, and against it a side column does not
// move inward either: it keeps Reach, the stub C7 asks, and the 0.3 px jog
// between them stays and takes a lane. The column is the wire's upper
// stop with the group below, its lower stop with the group above.
func TestRoute_ASideColumnKeepsReachAgainstAnAnchoredPort(t *testing.T) {
	g := shaped(t, func(lv *lgraph.Level) {
		lv.Nodes[1].Group, lv.Nodes[1].W, lv.Nodes[1].H = true, 300, 200
		lv.Edges[0].Anchor[1] = lgraph.Anchor{On: true, At: -60}
		lv.Edges[1].Anchor[0] = lgraph.Anchor{On: true, At: 59.7}
	}, "a->g", "g->a")
	res, route := routed(t, g, screen, map[string]float64{"a": 0, "g": 0})
	require.Equal(t, ports.Right, res.Side[1][ports.Head], "a's end attaches at its right side, its column 60")
	// from g's top face at its anchor up into the channel's one lane, across
	// to the column Reach outside a's right face, and into it at mid-height
	assertRoute(t, pts(59.7, 88, 59.7, 64, 60, 64, 60, 20, 40, 20), route("g->a#0"))
	assert.Equal(t, []int{1}, res.Lanes, "one lane: the 0.3 px jog shares it with a->g's, far away")

	// the group above, the node below
	g = shaped(t, func(lv *lgraph.Level) {
		lv.Nodes[0].Group, lv.Nodes[0].W, lv.Nodes[0].H = true, 300, 200
		lv.Edges[0].Anchor[0] = lgraph.Anchor{On: true, At: -60}
		lv.Edges[1].Anchor[1] = lgraph.Anchor{On: true, At: 59.7}
	}, "g->a", "a->g")
	res, route = routed(t, g, screen, map[string]float64{"g": 0, "a": 0})
	require.Equal(t, ports.Right, res.Side[1][ports.Tail], "a's end attaches at its right side, its column 60")
	assert.Equal(t, []int{1}, res.Lanes, "one lane: the 0.3 px jog shares it with g->a's, far away")
	// from a's right face at mid-height out to the column Reach outside it,
	// up into the channel's one lane, across to g's anchor and into g's
	// bottom face there
	assertRoute(t, pts(40, 268, 60, 268, 60, 224, 59.7, 224, 59.7, 200), route("a->g#0"))
}

// TestRoute_APortGivesWayToAnAnchoredPort pins S8's stops (S9): a port
// the in-lane gap moved within Snap of its wire's anchored port moves onto
// it, though the port carries the arrowhead.
func TestRoute_APortGivesWayToAnAnchoredPort(t *testing.T) {
	g := shaped(t, func(lv *lgraph.Level) {
		lv.Nodes[0].Group, lv.Nodes[0].W, lv.Nodes[0].H = true, 300, 200
		lv.Edges[0].Anchor[0] = lgraph.Anchor{On: true, At: 0}
	}, "g->p", "h->y", "y->z", "h->z")
	// h->z's dummy column in p's row, 3 px right of p's in-port: the port
	// moves 8 left, to 0.234, within Snap of g's anchor at 0
	xs := map[string]float64{"g": 0, "h": 400, "p": 8.234, "y": 400, "z": 400, "h->z#0@1": 11.234}
	_, route := routed(t, g, screen, xs)
	gp := route("g->p#0")
	require.Len(t, gp, 2, "straight: %v", gp)
	assert.Equal(t, 0.0, gp[0].X, "on g's anchor: %v", gp)
	assert.Equal(t, 0.0, gp[1].X, "p's port moved onto it: %v", gp)
}

// TestRoute_AnchoredPortsKeepTheirJog pins S8's stops and lanes (S9): two
// anchored ports within Snap never move, and the jog between them takes
// a lane; within a millionth of a unit they are one x.
func TestRoute_AnchoredPortsKeepTheirJog(t *testing.T) {
	fix := func(at float64) func(lv *lgraph.Level) {
		return func(lv *lgraph.Level) {
			for v := range lv.Nodes {
				lv.Nodes[v].Group, lv.Nodes[v].W, lv.Nodes[v].H = true, 300, 200
			}
			lv.Edges[0].Anchor = [2]lgraph.Anchor{{On: true, At: 0}, {On: true, At: at}}
		}
	}
	res, route := routed(t, shaped(t, fix(0.3), "g->k"), screen, map[string]float64{"g": 0, "k": 0})
	got := route("g->k#0")
	assertRoute(t, pts(0, 200, 0, 224, 0.3, 224, 0.3, 248), got)
	assert.Equal(t, []int{1}, res.Lanes, "the jog takes a lane")
	_, route = routed(t, shaped(t, fix(1e-9), "g->k"), screen, map[string]float64{"g": 0, "k": 0})
	assert.Len(t, route("g->k#0"), 2, "float noise: one x")
}

// TestRoute_AnAnchoredPortMeetsAPinnedVertex pins S8's stops (S9): a
// diamond's bottom vertex and an anchored port within Snap never move; the
// jog between them takes a lane.
func TestRoute_AnAnchoredPortMeetsAPinnedVertex(t *testing.T) {
	g := shaped(t, func(lv *lgraph.Level) {
		lv.Nodes[1].Group, lv.Nodes[1].W, lv.Nodes[1].H = true, 300, 200
		lv.Edges[0].Anchor[1] = lgraph.Anchor{On: true, At: 0.3}
	}, "m:diamond->g")
	_, route := routed(t, g, screen, map[string]float64{"m": 0, "g": 0})
	assertRoute(t, pts(0, 40, 0, 64, 0.3, 64, 0.3, 88), route("m->g#0"))
}

// TestRoute_TextSideColumnsSitOnCellMiddles pins S8's side attachments in
// the text profile: a column Reach out sits half a cell farther, on a
// cell's middle, on either side.
func TestRoute_TextSideColumnsSitOnCellMiddles(t *testing.T) {
	for _, tc := range []struct {
		name  string
		xs    map[string]float64
		right bool
	}{
		{"left", map[string]float64{"m": 20, "a": 20}, false},
		{"right", map[string]float64{"m": 20, "a": 21}, true},
	} {
		g := graph(t, map[string]float64{"m": 8, "a": 8}, "m->a", "a->m")
		for i := range g.Level.Nodes {
			g.Level.Nodes[i].H = 3
		}
		_, route := twice(t, g, text, tc.xs)
		back := route("a->m#0")
		require.Len(t, back, 4, "%s: side to side: %v", tc.name, back)
		side, col := back[0].X, back[1].X
		if tc.right {
			assert.Equal(t, 3.5, col-side, "%s: Reach and half a cell out: %v", tc.name, back)
		} else {
			assert.Equal(t, -3.5, col-side, "%s: Reach and half a cell out: %v", tc.name, back)
		}
		assert.Equal(t, 0.5, col-math.Floor(col), "%s: a cell's middle: %v", tc.name, back)
	}
}

// TestRoute_JogsExactlyTheInLaneGapApartShareALane pins S8 (Lanes): a unit
// takes a lane that keeps at least InLaneGap to every run already there,
// a floor with no tolerance beyond float noise. a->c jogs from 100 to 120
// and b->d from 128 to 148, exactly InLaneGap (8) apart: one lane. 0.1
// nearer, they take two.
func TestRoute_JogsExactlyTheInLaneGapApartShareALane(t *testing.T) {
	widths := map[string]float64{"a": 8, "b": 8, "c": 8, "d": 8}
	r, _ := routed(t, graph(t, widths, "a->c", "b->d"), screen, map[string]float64{"a": 100, "b": 128, "c": 120, "d": 148})
	assert.Equal(t, []int{1}, r.Lanes, "8 apart: one lane")
	r, _ = routed(t, graph(t, widths, "a->c", "b->d"), screen, map[string]float64{"a": 100, "b": 127.9, "c": 120, "d": 147.9})
	assert.Equal(t, []int{2}, r.Lanes, "7.9 apart: two lanes")
}
