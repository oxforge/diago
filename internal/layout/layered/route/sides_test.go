package route

import (
	"fmt"
	"math"
	"testing"

	"github.com/oxforge/diago/internal/layout/layered/lgraph"
	"github.com/oxforge/diago/internal/layout/layered/lgraph/lgraphtest"
	"github.com/oxforge/diago/internal/layout/layered/ports"
	"github.com/oxforge/diago/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoute_ASideColumnKeepsReachFromItsVertex(t *testing.T) {
	g := graph(t, nil, "m:diamond->a", "m->b")
	// a sits just left of m: its column cannot come nearer than Reach
	_, route := routed(t, g, screen, map[string]float64{"m": 200, "a": 150, "b": 300})
	got := route("m->a#0")
	assert.InDelta(t, 140, got[1].X, 1e-6, "160 - Reach: %v", got)
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

func TestRoute_ABlockedSideColumnFallsBackToReach(t *testing.T) {
	g := graph(t, nil, "m:diamond->a", "m->b", "n")
	// the column over b's in-port would run its stub through n
	_, route := routed(t, g, screen, map[string]float64{"m": 200, "n": 340, "a": 0, "b": 500})
	assert.InDelta(t, 260, route("m->b#0")[1].X, 1e-6)
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
