package route

import (
	"math"
	"strings"
	"testing"

	"github.com/oxforge/diago/internal/layout/layered/lgraph"
	"github.com/oxforge/diago/internal/layout/layered/lgraph/lgraphtest"
	"github.com/oxforge/diago/internal/layout/layered/ports"
	"github.com/oxforge/diago/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestRoute_PortsSpreadEvenlyInTheOrderOfTheirFarEnds(t *testing.T) {
	g := graph(t, nil, "a->b", "a->c", "a->d")
	r, route := routed(t, g, screen, map[string]float64{"a": 200, "b": 100, "c": 200, "d": 300})
	// three ends on an 80 wide face: at 1/4, 2/4 and 3/4 of it
	assertRoute(t, pts(180, 40, 180, 64, 100, 64, 100, 88), route("a->b#0"))
	assertRoute(t, pts(200, 40, 200, 88), route("a->c#0"))
	assertRoute(t, pts(220, 40, 220, 64, 300, 64, 300, 88), route("a->d#0"))
	assert.Equal(t, []int{1}, r.Lanes, "the two jogs share a lane, InLaneGap apart and more")
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

func TestRoute_ArrivalsShareTheInVertexOnOneLane(t *testing.T) {
	g := graph(t, nil, "a->m:diamond", "b->m")
	r, route := routed(t, g, screen, map[string]float64{"a": 100, "b": 300, "m": 200})
	assert.Equal(t, []int{1}, r.Lanes, "one rail, though the two jogs meet end to end")
	assertRoute(t, pts(100, 40, 100, 64, 200, 64, 200, 88), route("a->m#0"))
	assertRoute(t, pts(300, 40, 300, 64, 200, 64, 200, 88), route("b->m#0"))
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

// TestRoute_AnAnchoredPortMeetsAPinnedVertex pins S8's stops (S9): a
// diamond's vertex and an anchored port within Snap never move, the
// diamond above the group or below it, and neither does a dummy stop
// snapped onto the port; the jog between them takes a lane.
func TestRoute_AnAnchoredPortMeetsAPinnedVertex(t *testing.T) {
	g := shaped(t, func(lv *lgraph.Level) {
		lv.Nodes[1].Group, lv.Nodes[1].W, lv.Nodes[1].H = true, 300, 200
		lv.Edges[0].Anchor[1] = lgraph.Anchor{On: true, At: 0.3}
	}, "m:diamond->g")
	_, route := routed(t, g, screen, map[string]float64{"m": 0, "g": 0})
	assertRoute(t, pts(0, 40, 0, 64, 0.3, 64, 0.3, 88), route("m->g#0"))

	above := func(lv *lgraph.Level) {
		lv.Nodes[0].Group, lv.Nodes[0].W, lv.Nodes[0].H = true, 300, 200
		lv.Edges[0].Anchor[0] = lgraph.Anchor{On: true, At: 0.3}
	}
	g = shaped(t, above, "g->m:diamond")
	_, route = routed(t, g, screen, map[string]float64{"g": 0, "m": 0})
	assertRoute(t, pts(0.3, 200, 0.3, 224, 0, 224, 0, 248), route("g->m#0"))

	lv := lgraphtest.Level(t, "g->m:diamond")
	above(lv)
	g, err := lgraph.Build(lv, []int{0, 2}, nil, 8)
	require.NoError(t, err)
	_, route = routed(t, g, screen, map[string]float64{"g": 0, "m": 0, "g->m#0@1": 0.3})
	got := route("g->m#0")
	assert.Equal(t, model.Point{X: 0.3, Y: 200}, got[0], "on g's anchor: %v", got)
	assert.Equal(t, model.Point{X: 0, Y: got[len(got)-1].Y}, got[len(got)-1], "on m's in-vertex: %v", got)
}
