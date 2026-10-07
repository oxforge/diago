package labels

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/model"
)

var screen = Options{Gap: 6, CardStep: 10, EnvDepth: 14, EnvHalf: 7}

// two boxes, a above b, joined by a vertical wire at x = 40
var (
	boxes = []Rect{{0, 0, 80, 40}, {0, 100, 80, 40}}
	down  = []model.Point{{X: 40, Y: 40}, {X: 40, Y: 100}}
)

func place(t *testing.T, nodes []Rect, pinned []bool, o Options, edges ...Edge) []Placed {
	t.Helper()
	if pinned == nil {
		pinned = make([]bool, len(nodes))
	}
	out := Place(context.Background(), nodes, pinned, nil, edges, o)
	require.Len(t, out, len(edges))
	return out
}

func TestPlace_ALabelSitsRightOfItsRunsMiddle(t *testing.T) {
	got := place(t, boxes, nil, screen, Edge{ID: "e", Points: down, Label: Size{30, 10}})
	assert.Equal(t, &Rect{46, 65, 30, 10}, got[0].Label, "a gap right of the wire, centered on the run")
	assert.False(t, got[0].LabelUnresolved)
}

func TestPlace_ALabelAvoidsNodesAndOtherWires(t *testing.T) {
	blocker := append(boxes[:2:2], Rect{50, 50, 60, 40})
	got := place(t, blocker, nil, screen, Edge{ID: "e", Points: down, Label: Size{30, 10}})
	assert.Equal(t, &Rect{4, 65, 30, 10}, got[0].Label, "the right spot holds a node: left")

	foreign := Edge{ID: "f", Points: []model.Point{{X: 60, Y: 0}, {X: 60, Y: 140}}}
	got = place(t, boxes, nil, screen, Edge{ID: "e", Points: down, Label: Size{30, 10}}, foreign)
	assert.Equal(t, &Rect{4, 65, 30, 10}, got[0].Label, "another edge's wire runs through the right spot")
}

func TestPlace_ALabelKeepsTheGapFromItsOwnWire(t *testing.T) {
	// the wire jogs right 5 below the first run's middle spot: right of
	// the run the label would come within the gap of the jog (C14.3)
	jog := []model.Point{{X: 40, Y: 40}, {X: 40, Y: 60}, {X: 120, Y: 60}, {X: 120, Y: 100}}
	got := place(t, []Rect{{0, 0, 80, 40}, {80, 100, 80, 40}}, nil, screen, Edge{ID: "e", Points: jog, Label: Size{30, 10}})
	assert.Equal(t, &Rect{4, 45, 30, 10}, got[0].Label)
	assert.False(t, got[0].LabelUnresolved)
}

func TestPlace_ABlockedLabelTakesTheLeastOverlapAndIsUnresolved(t *testing.T) {
	wall := []Rect{{-200, -200, 500, 500}}
	got := place(t, wall, nil, screen, Edge{ID: "e", Points: down, Label: Size{30, 10}})
	require.NotNil(t, got[0].Label)
	assert.True(t, got[0].LabelUnresolved)
}

func TestPlace_ADiamondsSideExitLabelHugsItsCorner(t *testing.T) {
	exit := []model.Point{{X: 80, Y: 20}, {X: 100, Y: 20}, {X: 100, Y: 100}}
	nodes := []Rect{{0, 0, 80, 40}, {60, 100, 80, 40}}
	got := place(t, nodes, []bool{true, false}, screen, Edge{ID: "e", Points: exit, Label: Size{30, 10}, Source: 0})
	assert.Equal(t, &Rect{106, 4, 30, 10}, got[0].Label, "outward of the descent, above the stub")

	got = place(t, nodes, nil, screen, Edge{ID: "e", Points: exit, Label: Size{30, 10}, Source: 0})
	assert.Equal(t, &Rect{106, 55, 30, 10}, got[0].Label, "a rect's exit: beside the run")
}

func TestPlace_ABlockedMiddleSendsTheLabelNearAnEnd(t *testing.T) {
	// a run 360 tall from a's bottom face to b's top face; node boxes wall
	// off the spots beside its middle on both sides, in every ring, and
	// the spots near one of its ends, so the first clear spot is the ring
	// one spot near its other end, right of the run before left (S10)
	run := []model.Point{{X: 100, Y: 40}, {X: 100, Y: 400}}
	middle := []Rect{{110, 200, 60, 40}, {30, 200, 60, 40}}
	for _, tc := range []struct {
		name  string
		walls []Rect
		want  Rect
	}{
		{"the top end walled: near the bottom end", []Rect{{110, 40, 60, 20}, {30, 40, 60, 20}}, Rect{106, 400 - 6 - 10, 30, 10}},
		{"the bottom end walled: near the top end", []Rect{{110, 380, 60, 20}, {30, 380, 60, 20}}, Rect{106, 40 + 6, 30, 10}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			nodes := append([]Rect{{60, 0, 80, 40}, {60, 400, 80, 40}}, middle...)
			nodes = append(nodes, tc.walls...)
			got := place(t, nodes, nil, screen, Edge{ID: "e", Points: run, Label: Size{30, 10}})
			assert.Equal(t, &tc.want, got[0].Label)
			assert.False(t, got[0].LabelUnresolved)
		})
	}
}

func TestPlace_ASelfLoopsLabelTakesItsMouthLast(t *testing.T) {
	// a's loop leaves its right face at y 15, runs out to a leg at x 160
	// and returns at y 45. Node boxes wall off every spot beside the leg,
	// one inside the loop and one outside it, so the label takes the last
	// spot of its ladder: the loop's mouth, centered between a's face and
	// the leg and between the two stubs (S10)
	loop := []model.Point{{X: 80, Y: 15}, {X: 160, Y: 15}, {X: 160, Y: 45}, {X: 80, Y: 45}}
	nodes := []Rect{{0, 0, 80, 60}, {128, 0, 20, 60}, {168, 0, 40, 60}}
	got := place(t, nodes, nil, screen, Edge{ID: "a->a#0", Points: loop, Label: Size{10, 10}, Source: 0, Loop: true})
	assert.Equal(t, &Rect{(80+160)/2 - 5, (15+45)/2 - 5, 10, 10}, got[0].Label)
	assert.False(t, got[0].LabelUnresolved)
}

func TestPlace_ALabelLongerThanItsRunsSlidesPastATurn(t *testing.T) {
	// a label turned for RIGHT (10 wide and 50 tall in the engine's frame,
	// S10) on a wire that leaves a down 20, jogs right and enters b 40
	// lower: no run is as tall as the label, and beside the midpoint of
	// the route's ends it lies on the jog. It slides beside the first run,
	// down from the run's middle one gap at a time, until it clears a,
	// left of the run and past its turn, where the wire leaves the other way
	route := []model.Point{{X: 40, Y: 40}, {X: 40, Y: 60}, {X: 120, Y: 60}, {X: 120, Y: 100}}
	nodes := []Rect{{0, 0, 80, 40}, {80, 100, 80, 40}}
	got := place(t, nodes, nil, screen, Edge{ID: "e", Points: route, Label: Size{10, 50}})
	assert.Equal(t, &Rect{24, 50 + 3*6 - 25, 10, 50}, got[0].Label, "three gaps down from the first run's middle, a gap left of it")
	assert.False(t, got[0].LabelUnresolved)
}

func TestPlace_ASlidingLabelTriesUpBeforeDown(t *testing.T) {
	// bars a pixel tall (node boxes) cross the run at y 51, 70 and 89,
	// through the spots beside its middle and near its two ends in every
	// ring. The label slides from the run's middle, one gap up before one
	// gap down: both are clear, and the upper one wins (S10)
	bar := func(y float64) Rect { return Rect{-30, y - 0.5, 140, 1} }
	got := place(t, append(boxes[:2:2], bar(51), bar(70), bar(89)), nil, screen, Edge{ID: "e", Points: down, Label: Size{30, 10}})
	assert.Equal(t, &Rect{46, 70 - 6 - 5, 30, 10}, got[0].Label)
	assert.False(t, got[0].LabelUnresolved)
}

func TestPlace_AHemmedInLabelSlidesBesideAHorizontalRun(t *testing.T) {
	// node boxes wall off both sides of the wire's two vertical runs in
	// every ring, and sliding along the first run the label meets a or a
	// wall: it slides on to the horizontal run between them and sits
	// above its middle (S10)
	route := []model.Point{{X: 40, Y: 40}, {X: 40, Y: 70}, {X: 200, Y: 70}, {X: 200, Y: 100}}
	nodes := []Rect{
		{0, 0, 80, 40}, {160, 100, 80, 40},
		{-20, 40, 54, 30}, {46, 40, 54, 30}, // beside the first run
		{146, 70, 48, 30}, {206, 70, 54, 30}, // beside the last
	}
	got := place(t, nodes, nil, screen, Edge{ID: "e", Points: route, Label: Size{30, 10}})
	assert.Equal(t, &Rect{120 - 15, 70 - 6 - 10, 30, 10}, got[0].Label)
	assert.False(t, got[0].LabelUnresolved)
}

func TestPlace_ASlidingLabelReachesEightRingsOut(t *testing.T) {
	// bars a pixel wide (node boxes) down the gap between a and b cross
	// every spot of the first four rings on both sides of the run, then
	// of the next four too: the slide reaches eight rings out, and no
	// farther (S10)
	bar := func(x float64) Rect { return Rect{x - 0.5, 40, 1, 60} }
	for _, tc := range []struct {
		name string
		bars []float64
		want *Rect
	}{
		{"four rings crossed: the fifth, right of the run's middle", []float64{65, 15}, &Rect{40 + 5*6, 65, 30, 10}},
		{"eight rings crossed: unresolved", []float64{65, 15, 99, -19}, nil},
		// rings five to eight crossed, the ninth clear on both sides: the
		// slide stops at the eighth
		{"eight rings crossed, the ninth clear: unresolved", []float64{65, 15, 90, -11}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			nodes := boxes[:2:2]
			for _, x := range tc.bars {
				nodes = append(nodes, bar(x))
			}
			got := place(t, nodes, nil, screen, Edge{ID: "e", Points: down, Label: Size{30, 10}})
			require.NotNil(t, got[0].Label)
			if tc.want == nil {
				assert.True(t, got[0].LabelUnresolved)
				return
			}
			assert.Equal(t, tc.want, got[0].Label)
			assert.False(t, got[0].LabelUnresolved)
		})
	}
}

// TestPlace_TheSlideTriesEverySegmentOneRingAtATime pins the order of
// S10's slide: ring by ring, every segment of the route in each ring,
// never one segment through all its rings first. The route runs down
// from a, then right into b's side. Bars a pixel wide beside the vertical
// run cross every spot of its first ring, and boxes cross its run spots
// (its middle and near its ends) in rings two to four, but leave slide
// spots of its second ring clear; the horizontal run's first ring is
// clear. The label takes the horizontal run's first ring, above its
// middle, where a slide segment by segment would take the vertical run's
// second.
func TestPlace_TheSlideTriesEverySegmentOneRingAtATime(t *testing.T) {
	route := []model.Point{{X: 40, Y: 40}, {X: 40, Y: 140}, {X: 200, Y: 140}}
	nodes := []Rect{{0, 0, 80, 40}, {200, 120, 80, 40},
		{46.5, 30, 1, 120}, {32.5, 30, 1, 120}, // the first ring, right and left
	}
	for _, y := range []float64{44, 83, 122} { // the run spots near the top, at the middle and near the bottom
		nodes = append(nodes, Rect{50, y, 50, 14}, Rect{-20, y, 45, 14})
	}
	got := place(t, nodes, nil, screen, Edge{ID: "e", Points: route, Label: Size{30, 10}})
	assert.Equal(t, &Rect{105, 124, 30, 10}, got[0].Label)
	assert.False(t, got[0].LabelUnresolved)
}

func TestPlace_ACardinalityFollowsItsRouteAroundATurn(t *testing.T) {
	// the wire enters b's top from a turn 20 above it: a node box walls
	// off the right of that last run, and its own jog, within the gap
	// below, the left, so none of the spots one to three steps from the
	// end is clear. Three steps along the route lie past the turn: the
	// cardinality sits above the jog there. Walls above and below the jog
	// up to 110 along the route, its middle, leave it no spot: the first
	// clear one lies past the middle, where the other end's would (S10)
	route := []model.Point{{X: 40, Y: 40}, {X: 40, Y: 80}, {X: 200, Y: 80}, {X: 200, Y: 100}}
	nodes := []Rect{{0, 0, 80, 40}, {160, 100, 80, 40}, {205, 50, 60, 50}}
	for _, tc := range []struct {
		name  string
		walls []Rect
		want  *Rect
	}{
		{"one step past the turn, a step above the jog", nil, &Rect{200 - 10 - 5, 80 - 10 - 10, 10, 10}},
		{"walled up to the route's middle: unresolved", []Rect{{106, 30, 99, 44}, {106, 86, 99, 44}}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := place(t, append(nodes[:3:3], tc.walls...), nil, screen, Edge{ID: "e", Points: route, To: Size{10, 10}})
			require.NotNil(t, got[0].To)
			if tc.want == nil {
				assert.True(t, got[0].ToUnresolved)
				return
			}
			assert.Equal(t, tc.want, got[0].To)
			assert.False(t, got[0].ToUnresolved)
		})
	}
}

func TestPlace_ALabelTakesNoSpotNearerAnotherEdgesWire(t *testing.T) {
	// f's wire runs down x = xf, beyond the spot right of the run's middle,
	// which lies 6 from the label's own wire. A reader takes a label for
	// the nearest wire's: the spot is not clear when f's wire lies nearer
	// by more than the slack, and the label goes left (S10)
	for _, tc := range []struct {
		name      string
		xf, slack float64
		want      Rect
	}{
		{"f 4 from the spot, no slack: left", 80, 0, Rect{4, 65, 30, 10}},
		{"f 4 from the spot, 2 of slack: right", 80, 2, Rect{46, 65, 30, 10}},
		{"f 3 from the spot, 2 of slack: left", 79, 2, Rect{4, 65, 30, 10}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := screen
			o.OwnSlack = tc.slack
			f := Edge{ID: "f", Points: []model.Point{{X: tc.xf, Y: 40}, {X: tc.xf, Y: 100}}}
			got := place(t, boxes, nil, o, Edge{ID: "e", Points: down, Label: Size{30, 10}}, f)
			assert.Equal(t, &tc.want, got[0].Label)
			assert.False(t, got[0].LabelUnresolved)
		})
	}
}

func TestPlace_ALabelNearerOtherWiresEverywhereFallsBack(t *testing.T) {
	// wires down x = 78 and x = 2 lie 2 from the spots beside the run's
	// middle in the first ring, 4 nearer than the label's own wire, and
	// run through every spot farther out on their side: no spot is clear,
	// and the label takes the least overlap, the first spot with the least
	// misreading, marked unresolved (S10)
	wire := func(x float64) Edge { return Edge{ID: "f", Points: []model.Point{{X: x, Y: 0}, {X: x, Y: 140}}} }
	got := place(t, boxes, nil, screen, Edge{ID: "e", Points: down, Label: Size{30, 10}}, wire(78), wire(2))
	assert.Equal(t, &Rect{46, 65, 30, 10}, got[0].Label)
	assert.True(t, got[0].LabelUnresolved)
}

func TestPlace_ACardinalityTakesNoSpotNearerAnotherEdgesWire(t *testing.T) {
	// f's wire down x = 62 lies 2 from the cardinality's first spot, right
	// of its wire one step down, and 10 nearer than its own: it goes left
	// (S10)
	f := Edge{ID: "f", Points: []model.Point{{X: 62, Y: 40}, {X: 62, Y: 100}}}
	got := place(t, boxes, nil, screen, Edge{ID: "e", Points: down, From: Size{10, 10}}, f)
	assert.Equal(t, &Rect{20, 45, 10, 10}, got[0].From)
	assert.False(t, got[0].FromUnresolved)
}

func TestPlace_CardinalitiesSitBesideTheirEnds(t *testing.T) {
	got := place(t, boxes, nil, screen, Edge{ID: "e", Points: down, From: Size{10, 10}, To: Size{10, 10}})
	assert.Equal(t, &Rect{50, 45, 10, 10}, got[0].From, "one step down the wire, one step right")
	assert.Equal(t, &Rect{50, 85, 10, 10}, got[0].To, "one step up from the end")
	assert.Nil(t, got[0].Label)
}

func TestPlace_AnAdornmentEnvelopeBlocksItsCorner(t *testing.T) {
	wide := screen
	wide.EnvHalf = 12
	adorned := Edge{ID: "e", Points: down, From: Size{10, 10}, Adorned: true}
	got := place(t, boxes, nil, wide, adorned)
	assert.Equal(t, &Rect{50, 55, 10, 10}, got[0].From, "two steps down, past the envelope")

	adorned.Adorned = false
	got = place(t, boxes, nil, wide, adorned)
	assert.Equal(t, &Rect{50, 45, 10, 10}, got[0].From)
}

func TestPlace_EveryLabelIsAnObstacleForTheNext(t *testing.T) {
	a := Edge{ID: "a", Points: down, Label: Size{30, 10}}
	b := Edge{ID: "b", Points: down, Label: Size{30, 10}}
	got := place(t, boxes, nil, screen, a, b)
	assert.Equal(t, &Rect{46, 65, 30, 10}, got[0].Label)
	assert.Equal(t, &Rect{4, 65, 30, 10}, got[1].Label)
}

func TestEnvelope(t *testing.T) {
	p := &placer{o: screen}
	env, ok := p.envelope(Edge{Points: down, Adorned: true})
	require.True(t, ok)
	assert.Equal(t, Rect{33, 40, 14, 14}, env, "p0 ± 7 across the wire, 14 along it (C14)")
	_, ok = p.envelope(Edge{Points: down})
	assert.False(t, ok)
}

func TestPlace_ACopyMovesWithItsEdge(t *testing.T) {
	// the boxes and wire moved 200 right: edge c copies edge e's label (S12)
	copied := func(blockers ...Rect) []Placed {
		nodes := append(boxes[:2:2], Rect{200, 0, 80, 40}, Rect{200, 100, 80, 40})
		return place(t, append(nodes, blockers...), nil, screen,
			Edge{ID: "e", Points: down, Label: Size{30, 10}, Copies: []int{1}},
			Edge{ID: "c", Points: []model.Point{{X: 240, Y: 40}, {X: 240, Y: 100}}, Label: Size{24, 10}})
	}
	got := copied()
	assert.Equal(t, &Rect{46, 65, 30, 10}, got[0].Label)
	assert.Equal(t, &Rect{249, 65, 24, 10}, got[1].Label, "moved 200 right, centered at its own size")

	got = copied(Rect{250, 50, 60, 40})
	assert.Equal(t, &Rect{4, 65, 30, 10}, got[0].Label, "the copy's right spot holds a node: both go left")
	assert.Equal(t, &Rect{207, 65, 24, 10}, got[1].Label)
	assert.False(t, got[0].LabelUnresolved || got[1].LabelUnresolved)

	got = copied(Rect{150, -100, 200, 300})
	assert.True(t, got[0].LabelUnresolved && got[1].LabelUnresolved, "no spot clear for the copy: both unresolved")
	assert.InDelta(t, got[0].Label.X+got[0].Label.W/2+200, got[1].Label.X+got[1].Label.W/2, 1e-9, "still moved together")
}

func TestPlace_ACopysCardinalitiesMoveWithItsEdge(t *testing.T) {
	// a node right of the copy's wire, where its cardinalities would go first
	nodes := append(boxes[:2:2], Rect{200, 0, 80, 40}, Rect{200, 100, 80, 40}, Rect{244, 40, 30, 60})
	got := place(t, nodes, nil, screen,
		Edge{ID: "e", Points: down, From: Size{8, 10}, To: Size{8, 10}, Copies: []int{1}},
		Edge{ID: "c", Points: []model.Point{{X: 240, Y: 40}, {X: 240, Y: 100}}, From: Size{8, 10}, To: Size{8, 10}})
	for _, end := range []func(Placed) *Rect{func(p Placed) *Rect { return p.From }, func(p Placed) *Rect { return p.To }} {
		a, b := end(got[0]), end(got[1])
		require.NotNil(t, a)
		require.NotNil(t, b)
		assert.Equal(t, Rect{a.X + 200, a.Y, a.W, a.H}, *b)
	}
}

func TestPlace_ACopyAndItsLeadClearEachOther(t *testing.T) {
	// e's label is 100 wide and its copy c's 10, c's wire dx right of e's
	// (S12): c's box sits dx right of e's box's center, inside e's box at
	// every spot when dx is under half their widths, so no spot is clear
	// (S10)
	copied := func(dx float64) []Placed {
		return place(t, boxes, nil, screen,
			Edge{ID: "e", Points: down, Label: Size{100, 10}, Copies: []int{1}},
			Edge{ID: "c", Points: []model.Point{{X: 40 + dx, Y: 40}, {X: 40 + dx, Y: 100}}, Label: Size{10, 10}})
	}
	got := copied(30)
	assert.True(t, got[0].LabelUnresolved && got[1].LabelUnresolved, "the two overlap at every spot: both unresolved")
	assert.Equal(t, &Rect{46, 65, 100, 10}, got[0].Label, "the least overlap: right, where c's wire crosses e's box, since left c's box lies 30 nearer e's wire than its own")
	assert.Equal(t, &Rect{121, 65, 10, 10}, got[1].Label, "still moved together")

	got = copied(120)
	assert.False(t, got[0].LabelUnresolved || got[1].LabelUnresolved, "far enough apart: both clear")
	assert.Equal(t, &Rect{46, 65, 100, 10}, got[0].Label)
	assert.Equal(t, &Rect{211, 65, 10, 10}, got[1].Label)
}

// text is the text profile's labels values (S14), in cells, under dir.
func text(dir model.Direction) Options {
	return Options{Gap: 1, CardStep: 1, EnvDepth: 1, EnvHalf: 1, Pad: 1, Text: true, Dir: dir}
}

// In the text profile, cells: a above b, joined by a wire down column 10
// from row 3 to row 9 (x = 10.5, a column's middle).
var (
	cellBoxes = []Rect{{6, 0, 9, 3}, {6, 9, 9, 3}}
	cellDown  = []model.Point{{X: 10.5, Y: 3}, {X: 10.5, Y: 9}}
)

func TestPlace_TextSnapsASpotToWholeCells(t *testing.T) {
	e := Edge{ID: "e", Points: cellDown, Label: Size{5, 1}}
	got := place(t, cellBoxes, nil, Options{Gap: 1, CardStep: 1, EnvDepth: 1, EnvHalf: 1}, e)
	assert.Equal(t, &Rect{11.5, 5.5, 5, 1}, got[0].Label, "unsnapped: the run's middle, a gap right of the wire")

	got = place(t, cellBoxes, nil, text(model.Down), e)
	assert.Equal(t, &Rect{12, 5, 5, 1}, got[0].Label, "away from the wire across it, down along it: one blank column after the wire's")
	assert.False(t, got[0].LabelUnresolved)

	blocker := append(cellBoxes[:2:2], Rect{12, 4, 6, 3})
	got = place(t, blocker, nil, text(model.Down), e)
	assert.Equal(t, &Rect{4, 5, 5, 1}, got[0].Label, "left of the wire, rounded away from it: one blank column before the wire's")
}

func TestPlace_TextCardinalitiesRoundTowardTheirEnds(t *testing.T) {
	got := place(t, cellBoxes, nil, text(model.Down), Edge{ID: "e", Points: cellDown, From: Size{1, 1}, To: Size{1, 1}})
	assert.Equal(t, &Rect{12, 3, 1, 1}, got[0].From, "on the row under a's face")
	assert.Equal(t, &Rect{12, 8, 1, 1}, got[0].To, "on the row over b's face")
}

func TestPlace_TextACardinalityPastATurnRoundsTowardItsEnd(t *testing.T) {
	// the wire enters b's top from a jog along the boundary above row 6,
	// drawn in row 6, and walls take both sides of its last run, so the
	// cardinality follows the route around the turn. One step along the
	// jog, on the row next to the jog's, row 5, its box starts half a
	// column off and rounds toward its end, back along the route (S10),
	// leaving the right wall the blank it needs after its cells
	route := []model.Point{{X: 4.5, Y: 3}, {X: 4.5, Y: 6}, {X: 20.5, Y: 6}, {X: 20.5, Y: 10}}
	nodes := []Rect{{0, 0, 9, 3}, {16, 10, 9, 3}, {22, 3, 5, 7}, {13, 7, 7, 3}}
	got := place(t, nodes, nil, text(model.Down), Edge{ID: "e", Points: route, To: Size{2, 1}})
	assert.Equal(t, &Rect{19, 5, 2, 1}, got[0].To, "columns 19 and 20 of row 5, not 18 and 19")
	assert.False(t, got[0].ToUnresolved)
}

func TestPlace_TextALabelTakesNoSpotNearerAnotherEdgesWire(t *testing.T) {
	// f's lane down the boundary before column 18 is drawn in column 18,
	// right after the blank the right spot needs: the spot is clear of
	// every cell drawn, but f lies a cell from it and its own wire a cell
	// and a half, so it reads as f's, and the label goes left (S10)
	f := Edge{ID: "f", Points: []model.Point{{X: 18, Y: 0}, {X: 18, Y: 12}}}
	got := place(t, cellBoxes, nil, text(model.Down), Edge{ID: "e", Points: cellDown, Label: Size{5, 1}}, f)
	assert.Equal(t, &Rect{4, 5, 5, 1}, got[0].Label)
	assert.False(t, got[0].LabelUnresolved)
}

func TestPlace_TextASpotNeedsABlankCellEachSide(t *testing.T) {
	// f's wire down column 17 leaves the right spot, columns 12..16, clear
	// on screen but takes the blank the renderer needs after it
	f := Edge{ID: "f", Points: []model.Point{{X: 17.5, Y: 0}, {X: 17.5, Y: 12}}}
	got := place(t, cellBoxes, nil, text(model.Down), Edge{ID: "e", Points: cellDown, Label: Size{5, 1}}, f)
	assert.Equal(t, &Rect{4, 5, 5, 1}, got[0].Label)
	assert.False(t, got[0].LabelUnresolved)
}

func TestPlace_TextAWireOnACellBoundaryIsDrawnBelowIt(t *testing.T) {
	// f's lane lies on the boundary above the right spot's row 5: the
	// renderer draws it in the row below the boundary in the output
	// frame, row 5 under DOWN, but row 4 under UP, where the frame turns
	// the engine's y over (S11). Two cells of slack let the lane lie
	// nearer the spot than the label's own wire, so the cells decide
	f := Edge{ID: "f", Points: []model.Point{{X: 11, Y: 5}, {X: 30, Y: 5}}}
	e := Edge{ID: "e", Points: cellDown, Label: Size{5, 1}}
	slack := func(o Options) Options { o.OwnSlack = 2; return o }
	got := place(t, cellBoxes, nil, slack(text(model.Down)), e, f)
	assert.Equal(t, &Rect{4, 5, 5, 1}, got[0].Label, "DOWN: the lane is drawn in the spot's row")
	got = place(t, cellBoxes, nil, slack(text(model.Up)), e, f)
	assert.Equal(t, &Rect{12, 5, 5, 1}, got[0].Label, "UP: the lane is drawn in the row above")
}

func TestPlace_TextSidewaysTheRowRunsAlongY(t *testing.T) {
	// under RIGHT a label's row runs along the engine's y: its extents
	// swapped, 1 x 5, and the blanks it needs lie above and below it in
	// the engine's frame. f's lane at y 14.5 is drawn in the cell under
	// the right spot, rows 9..13, and takes that blank. Across the run, a
	// wire along its row in the output frame, it takes the cell next to the
	// wire's, x 11 (S10).
	run := []model.Point{{X: 10.5, Y: 3}, {X: 10.5, Y: 20}}
	boxes := []Rect{{9, 0, 3, 3}, {9, 20, 3, 3}}
	e := Edge{ID: "e", Points: run, Label: Size{1, 5}}
	got := place(t, boxes, nil, text(model.Right), e)
	assert.Equal(t, &Rect{11, 9, 1, 5}, got[0].Label)
	f := Edge{ID: "f", Points: []model.Point{{X: 11, Y: 14.5}, {X: 20, Y: 14.5}}}
	got = place(t, boxes, nil, text(model.Right), e, f)
	assert.Equal(t, &Rect{9, 9, 1, 5}, got[0].Label, "left of the wire, in the cell next to its")
	down := text(model.Down)
	down.OwnSlack = 2 // f's lane may lie nearer than e's own wire: the cells decide
	got = place(t, boxes, nil, down, e, f)
	assert.Equal(t, &Rect{12, 9, 1, 5}, got[0].Label, "under DOWN the blanks lie beside it, and f misses them")
}

func TestPlace_TextAGroupFrameBlocksALabel(t *testing.T) {
	// g's frame crosses the run at row 5: its top border takes the spots
	// beside the run's middle, so the label sits near the run's top end,
	// clear of the border; on screen a label may straddle a border
	frame := []Rect{{4, 5, 13, 8}}
	e := Edge{ID: "e", Points: cellDown, Label: Size{5, 1}}
	got := Place(context.Background(), cellBoxes, make([]bool, 2), frame, []Edge{e}, text(model.Down))
	assert.Equal(t, &Rect{12, 4, 5, 1}, got[0].Label)
	assert.False(t, got[0].LabelUnresolved)
	got = Place(context.Background(), cellBoxes, make([]bool, 2), frame, []Edge{e}, Options{Gap: 1, CardStep: 1, EnvDepth: 1, EnvHalf: 1})
	assert.Equal(t, &Rect{11.5, 5.5, 5, 1}, got[0].Label, "on screen the frame blocks nothing")
}

func TestPlace_TextALabelMayShareARowWithItsOwnWire(t *testing.T) {
	// e leaves s along row 7 and turns down column 20, then along row 9
	// and down column 14. Left of the first run, rounded down along it,
	// the label shares row 7 with e's own lane, which runs from column 20
	// on: its cells and the blank after them, column 19, are clear and a
	// gap from the wire, so it takes the spot (S10), and the text renderer
	// draws it there, at its box
	route := []model.Point{{X: 27, Y: 7}, {X: 20.5, Y: 7}, {X: 20.5, Y: 9}, {X: 14.5, Y: 9}, {X: 14.5, Y: 15}}
	nodes := []Rect{{27, 5, 6, 4}, {10, 15, 9, 3}}
	got := place(t, nodes, nil, text(model.Down), Edge{ID: "e", Points: route, Label: Size{5, 1}})
	assert.Equal(t, &Rect{14, 7, 5, 1}, got[0].Label, "columns 14..18 of row 7, a blank column before the lane")
	assert.False(t, got[0].LabelUnresolved)
	// under RIGHT a label's row runs along the engine's y: right of the
	// first run's middle, in the cell next to it, the label's column 11 holds
	// e's second run, rows 15..25, well past the label's rows 6..10 and the
	// blanks above and below them
	sideways := []model.Point{{X: 10.5, Y: 3}, {X: 10.5, Y: 15}, {X: 11.5, Y: 15}, {X: 11.5, Y: 25}}
	got = place(t, []Rect{{8, 0, 5, 3}, {9, 25, 5, 3}}, nil, text(model.Right), Edge{ID: "e", Points: sideways, Label: Size{1, 5}})
	assert.Equal(t, &Rect{11, 6, 1, 5}, got[0].Label, "right of the first run's middle, in the second run's column")
	assert.False(t, got[0].LabelUnresolved)
}

func TestPlace_TextACopySnapsAsItsSpotDoes(t *testing.T) {
	// c copies e's label one column wider, its wire 20 columns right:
	// centered on the moved center it would start half a column off, so
	// it rounds away from its wire as e's spot does (S10, S12)
	got := place(t, append(cellBoxes[:2:2], Rect{26, 0, 9, 3}, Rect{26, 9, 9, 3}), nil, text(model.Down),
		Edge{ID: "e", Points: cellDown, Label: Size{4, 1}, Copies: []int{1}},
		Edge{ID: "c", Points: []model.Point{{X: 30.5, Y: 3}, {X: 30.5, Y: 9}}, Label: Size{5, 1}})
	assert.Equal(t, &Rect{12, 5, 4, 1}, got[0].Label)
	assert.Equal(t, &Rect{32, 5, 5, 1}, got[1].Label, "one blank column after its wire's, half a column right of the moved center")
}
