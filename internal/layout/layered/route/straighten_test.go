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

// TestRoute_ABackEdgeRunsOnItsPinnedSideColumnPastOtherWires pins S8's
// stops: a dummy stop moving onto a back edge's pinned column keeps the
// node clearance from every node box of its row and InLaneGap from every
// other wire, at the column. As in TestRoute_ABackEdgeRunsOnItsPinnedSideColumn,
// a->m's dummies at 230 move onto diamond m's column at 260: past s->t's
// dummy 15 px right of the column, or between 230 and the column, and past
// a 4 px node 13 px left of it.
func TestRoute_ABackEdgeRunsOnItsPinnedSideColumnPastOtherWires(t *testing.T) {
	for _, dx := range []float64{275, 245} {
		lv := lgraphtest.Level(t, "m:diamond", "a", "a->m", "s->t")
		// layers: m and s 0, a 3, t 2; a->m reversed through two dummies, s->t through one
		g, err := lgraph.Build(lv, []int{0, 3, 0, 2}, []bool{true, false}, 8)
		require.NoError(t, err)
		_, route := twice(t, g, screen, map[string]float64{"m": 200, "a->m#0@1": 230, "a->m#0@2": 230, "a": 240,
			"s": 420, "s->t#0@1": dx, "t": 420})
		assertRoute(t, pts(260, 216, 260, 20, 240, 20), route("a->m#0"))
	}
	lv := lgraphtest.Level(t, "m:diamond", "a", "a->m", "n")
	lv.Nodes[2].W = 4 // n
	g, err := lgraph.Build(lv, []int{0, 3, 1}, []bool{true}, 8)
	require.NoError(t, err)
	_, route := twice(t, g, screen, map[string]float64{"m": 200, "a->m#0@1": 230, "a->m#0@2": 230, "a": 240, "n": 245})
	assertRoute(t, pts(260, 200, 260, 20, 240, 20), route("a->m#0"))
}

// TestRoute_APinnedVertexNeverSnapsOffItsVertex pins S8's stops: a port
// within Snap of the x a wire runs straight on snaps onto it only over
// the middle nine tenths of its face's usable span, and a diamond's zero
// span is its vertex alone (C8). m->b leaves m's bottom vertex at 100,
// runs through its dummy at 110 and enters b at 100.3, within Snap of the
// vertex: the wire runs straight on the vertex, b's in-port snapping onto
// it, never on b's port, which would take the vertex's stop off it.
func TestRoute_APinnedVertexNeverSnapsOffItsVertex(t *testing.T) {
	lv := lgraphtest.Level(t, "m:diamond->b")
	g, err := lgraph.Build(lv, []int{0, 2}, nil, 8)
	require.NoError(t, err)
	_, route := routed(t, g, screen, map[string]float64{"m": 100, "b": 100.3, "m->b#0@1": 110})
	wire := route("m->b#0")
	for _, p := range wire {
		assert.Equal(t, 100.0, p.X, "on m's bottom vertex: %v", wire)
	}
}

// TestRoute_ATextPortSnapsOverACornerCell pins S8's stops in the text
// profile: a port within Snap of the x a wire runs straight on snaps onto
// it anywhere over the middle nine tenths of its face's usable span, a
// corner cell of its box included, where it never slides
// (TestRoute_ATextPortSlidesOnlyInsideTheCorners). Under RIGHT, three rows
// tall a's two out-ports take its corner rows, a->c's at 9.5 and b->a's
// at 11.5. b->a's stops run from that port through its dummy at 12.25 to
// b's in-port at 11.25, a quarter row inside the port, over the same
// corner row: a's port snaps onto it, and the wire runs straight on
// 11.25. The edge is reversed, its arrowhead on a: were the snap refused,
// the stops would settle onto a's port, and the wire would run on 11.5.
func TestRoute_ATextPortSnapsOverACornerCell(t *testing.T) {
	sideways := Options{Clearance: 1, Span: 1, Reach: 2, BaseGap: 4, LaneGap: 2, InLaneGap: 1, Snap: 0.5, Dir: model.Right, Text: true}
	lv := lgraphtest.Level(t, "a->c", "b->a")
	for i := range lv.Nodes {
		lv.Nodes[i].W, lv.Nodes[i].H = 3, 5
	}
	// layers: a 0, c 1, b 2; b->a reversed, through one dummy
	g, err := lgraph.Build(lv, []int{0, 1, 2}, []bool{false, true}, 1)
	require.NoError(t, err)
	_, route := routed(t, g, sideways, map[string]float64{"a": 10.5, "c": 9.5, "b": 11.25, "b->a#0@1": 12.25})
	assertRoute(t, pts(9.5, 5, 9.5, 9), route("a->c#0"))
	assertRoute(t, pts(11.25, 18, 11.25, 5), route("b->a#0"))
}
