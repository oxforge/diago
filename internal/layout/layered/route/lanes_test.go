package route

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestRoute_DivergingJogsFromOneNodeShareALane(t *testing.T) {
	g := graph(t, map[string]float64{"a": 16}, "a->b", "a->c")
	// a's two ports sit 16/3 apart, under InLaneGap, but their jogs run
	// away from each other
	r, _ := routed(t, g, screen, map[string]float64{"a": 200, "b": 100, "c": 300})
	assert.Equal(t, []int{1}, r.Lanes)
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
