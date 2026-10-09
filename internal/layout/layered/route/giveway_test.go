package route

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/oxforge/diago/internal/layout/layered/lgraph"
	"github.com/oxforge/diago/internal/layout/layered/lgraph/lgraphtest"
	"github.com/oxforge/diago/internal/layout/layered/ports"
	"github.com/oxforge/diago/internal/layoutdbg"
	"github.com/oxforge/diago/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
