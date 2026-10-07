package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/model"
)

// rect is an 80 x 40 rectangle centered on (x, y).
func rect(id string, x, y float64) model.PositionedNode {
	return model.PositionedNode{ID: id, Shape: model.ShapeRect, X: x, Y: y, Width: 80, Height: 40}
}

// wire is an edge from→to through the points (x0,y0), (x1,y1), ...
func wire(from, to string, xy ...float64) model.PositionedEdge {
	e := model.PositionedEdge{ID: from + "->" + to + "#0", From: from, To: to}
	for i := 0; i+1 < len(xy); i += 2 {
		e.Points = append(e.Points, model.Point{X: xy[i], Y: xy[i+1]})
	}
	return e
}

func measure(nodes []model.PositionedNode, edges ...model.PositionedEdge) Metrics {
	pg := &model.PositionedGraph{Width: 400, Height: 300, Nodes: nodes, Edges: edges}
	return Measure(pg, ScreenOptions(model.Down, nil))
}

func TestMeasure_Crossings(t *testing.T) {
	h := wire("a", "b", 20, 100, 200, 100)
	assert.Equal(t, 1, measure(nil, h, wire("c", "d", 100, 20, 100, 200)).Crossings, "a plus")
	assert.Equal(t, 0, measure(nil, h, wire("c", "d", 100, 100, 100, 200)).Crossings, "a T at the vertical's end")
	assert.Equal(t, 0, measure(nil, h, wire("c", "d", 20, 20, 20, 200)).Crossings, "a corner at the horizontal's end")
	assert.Equal(t, 0, measure(nil, h, wire("c", "d", 20, 110, 200, 110)).Crossings, "parallel")
	assert.Equal(t, 2, measure(nil, h, wire("c", "d", 60, 20, 60, 200, 140, 200, 140, 20)).Crossings, "one edge crossing twice")
	assert.Equal(t, 0, measure(nil, wire("a", "b", 20, 100, 200, 100, 200, 20, 100, 20, 100, 200)).Crossings,
		"an edge never crosses itself")
}

func TestMeasure_BendsJogsStraight(t *testing.T) {
	m := measure(nil,
		wire("a", "b", 100, 20, 100, 200),
		wire("c", "d", 20, 20, 20, 60, 30, 60, 30, 200),
		wire("e", "f", 300, 20, 300, 60, 300, 200))
	assert.Equal(t, 2, m.Bends, "c->d turns twice; e->f's middle point is no turn")
	assert.Equal(t, 1, m.Jogs, "c->d's 10 px interior segment")
	assert.Equal(t, 1, m.Straight, "a->b is one segment")

	m = measure(nil, wire("a", "b", 20, 20, 20, 60, 32, 60, 32, 200), wire("c", "d", 20, 20, 28, 20, 28, 200))
	assert.Equal(t, 0, m.Jogs, "12 px is no jog, nor is a short terminal segment")
}

// TestMeasure_AJogWithinFloatNoiseOfTheFloorIsNone pins Q2's tolerance: an
// interior segment a float error short of 12 px measures 12 px and is no
// micro-jog, one of 11.99 px is.
func TestMeasure_AJogWithinFloatNoiseOfTheFloorIsNone(t *testing.T) {
	x := 20 + (12 - 1e-9)
	m := measure(nil, wire("a", "b", 20, 20, 20, 60, x, 60, x, 200))
	assert.Equal(t, 0, m.Jogs, "a segment of 12 - 1e-9 px")
	m = measure(nil, wire("a", "b", 20, 20, 20, 60, 31.99, 60, 31.99, 200))
	assert.Equal(t, 1, m.Jogs, "a segment of 11.99 px")
}

func TestMeasure_AreaAndWire(t *testing.T) {
	m := measure(nil, wire("a", "b", 20, 20, 20, 60, 50, 60))
	assert.Equal(t, 400.0*300, m.Area)
	assert.Equal(t, 70.0, m.Wire)
}

func TestMeasure_Overlaps(t *testing.T) {
	assert.Equal(t, 1, measure([]model.PositionedNode{rect("a", 100, 100), rect("b", 150, 120)}).Overlaps)
	assert.Equal(t, 0, measure([]model.PositionedNode{rect("a", 100, 100), rect("b", 180, 100)}).Overlaps, "touching")
}

func TestMeasure_Fans(t *testing.T) {
	// s feeds a and b one row down; m gathers them one row further
	nodes := func(sx, mx float64) []model.PositionedNode {
		return []model.PositionedNode{rect("s", sx, 40), rect("a", 100, 140), rect("b", 300, 140), rect("m", mx, 240)}
	}
	edges := []model.PositionedEdge{wire("s", "a"), wire("s", "b"), wire("a", "m"), wire("b", "m")}
	fans := func(dir model.Direction, ns []model.PositionedNode, es ...model.PositionedEdge) [2]int {
		pg := &model.PositionedGraph{Nodes: ns, Edges: es}
		m := Measure(pg, ScreenOptions(dir, nil))
		return [2]int{m.Centered, m.Fans}
	}
	assert.Equal(t, [2]int{2, 2}, fans(model.Down, nodes(200, 200), edges...), "a fan-out and a fan-in, both centered")
	assert.Equal(t, [2]int{1, 2}, fans(model.Down, nodes(205, 200), edges...), "s is 5 px off its fan's middle")
	assert.Equal(t, [2]int{2, 2}, fans(model.Down, nodes(201, 199), edges...), "within 1 px counts as centered")
	assert.Equal(t, [2]int{0, 0}, fans(model.Up, nodes(200, 200), edges...), "under UP every edge runs upstream")
	assert.Equal(t, [2]int{0, 0}, fans(model.Auto, nodes(200, 200), edges...), "no flow axis under Auto")
	assert.Equal(t, [2]int{0, 0}, fans(model.Down, nodes(200, 200), wire("s", "a"), wire("s", "a")),
		"two edges to one node are one neighbor")

	// the same fan laid out RIGHT: the flow runs along x, the cross axis y
	right := []model.PositionedNode{rect("s", 40, 200), rect("a", 200, 100), rect("b", 200, 300)}
	assert.Equal(t, [2]int{1, 1}, fans(model.Right, right, wire("s", "a"), wire("s", "b")))
}

// TestMeasure_ADiamondsMainLineCountsAsCentered pins Q4's main line: of
// diamond d's two branches only main leads on (to next), and d's edge into
// it is one straight segment, so d's fan, 70 off its neighbors' middle,
// counts as centered. It does not when both branches lead on, when the
// main line bends, or when d is not a diamond.
func TestMeasure_ADiamondsMainLineCountsAsCentered(t *testing.T) {
	nodes := func(shape model.Shape) []model.PositionedNode {
		d := rect("d", 200, 40)
		d.Shape = shape
		return []model.PositionedNode{d, rect("main", 200, 140), rect("end", 340, 140), rect("next", 200, 240), rect("more", 340, 240)}
	}
	fans := func(ns []model.PositionedNode, es ...model.PositionedEdge) [2]int {
		m := Measure(&model.PositionedGraph{Nodes: ns, Edges: es}, ScreenOptions(model.Down, nil))
		return [2]int{m.Centered, m.Fans}
	}
	straight, side := wire("d", "main", 200, 60, 200, 120), wire("d", "end", 240, 40, 340, 40, 340, 120)
	on := wire("main", "next", 200, 160, 200, 220)
	assert.Equal(t, [2]int{1, 1}, fans(nodes(model.ShapeDiamond), straight, side, on), "the main line runs straight")
	assert.Equal(t, [2]int{0, 1}, fans(nodes(model.ShapeDiamond), straight, side, on, wire("end", "more", 340, 160, 340, 220)),
		"both branches lead on")
	assert.Equal(t, [2]int{0, 1}, fans(nodes(model.ShapeDiamond), straight, side), "neither leads on")
	bent := wire("d", "main", 160, 40, 150, 40, 150, 90, 200, 90, 200, 120)
	assert.Equal(t, [2]int{0, 1}, fans(nodes(model.ShapeDiamond), bent, side, on), "the main line bends")
	assert.Equal(t, [2]int{0, 1}, fans(nodes(model.ShapeRect), straight, side, on), "no diamond")
}

func TestMeasure_ContractDefects(t *testing.T) {
	// a->b runs straight through c
	nodes := []model.PositionedNode{rect("a", 100, 40), rect("b", 100, 240), rect("c", 100, 140)}
	m := measure(nodes, wire("a", "b", 100, 60, 100, 220))
	assert.Equal(t, 1, m.Intrusions, "C4")
	assert.Positive(t, m.Violations)

	m = measure(nodes[:2], wire("a", "b", 100, 60, 100, 70, 150, 70, 150, 200, 100, 200, 100, 220))
	assert.Equal(t, 1, m.Stubs, "C7: a 10 px first segment")

	m = measure([]model.PositionedNode{rect("a", 100, 40), rect("b", 100, 240), rect("c", 150, 140)},
		wire("a", "b", 100, 60, 100, 220))
	assert.Equal(t, 1, m.Clearance, "C5: the wire passes 10 px left of c's box")

	m = Measure(&model.PositionedGraph{
		Nodes:  []model.PositionedNode{rect("a", 100, 40), rect("b", 100, 240), rect("c", 156, 140)},
		Edges:  []model.PositionedEdge{wire("a", "b", 100, 60, 100, 220)},
		Groups: []model.PositionedGroup{{ID: "g", X: 106, Y: 110, Width: 100, Height: 60, Contains: []string{"c"}, Depth: 1}},
	}, ScreenOptions(model.Down, nil))
	assert.Equal(t, 2, m.Clearance, "C6.1 and C6.2: the wire runs 6 px left of c's group, along its side")

	e := wire("a", "b", 100, 60, 100, 220)
	e.LabelUnresolved = true
	e.FromCard = &model.EndLabel{Text: "1", Unresolved: true}
	e.ToCard = &model.EndLabel{Text: "*"}
	assert.Equal(t, 2, measure(nodes[:2], e).Labels, "a label and a cardinality placed as unresolved")

	e = wire("a", "b", 100, 60, 100, 220)
	e.Label, e.LabelPos, e.LabelWidth, e.LabelHeight = "x", &model.Point{X: 100, Y: 40}, 20, 14
	assert.Equal(t, 1, measure(nodes[:2], e).Labels, "C14.1: a label over a's box")

	pg := &model.PositionedGraph{Width: 400, Height: 300, Nodes: nodes[:2], Edges: []model.PositionedEdge{wire("a", "b", 100, 60, 100, 220)},
		Groups: []model.PositionedGroup{
			{ID: "g", Label: "G", X: 20, Y: 20, Width: 200, Height: 260, LabelWidth: 20, LabelHeight: 14, LabelBlocked: true},
			{ID: "h", Label: "H", X: 240, Y: 20, Width: 100, Height: 100, LabelWidth: 20, LabelHeight: 14},
		}}
	assert.Equal(t, 1, Measure(pg, ScreenOptions(model.Down, nil)).Labels, "a title placed blocked")
}

func TestMetrics_String(t *testing.T) {
	m := Metrics{Crossings: 1, Bends: 2, Jogs: 3, Straight: 4, Fans: 6, Centered: 5, Area: 1234.5, Wire: 99.49,
		Overlaps: 7, Intrusions: 8, Clearance: 9, Stubs: 10, Labels: 11, Violations: 12}
	assert.Equal(t, "cross=1 bends=2 jogs=3 straight=4 fans=5/6 area=1234 wire=99 overlaps=7 intrusions=8 clearance=9 stubs=10 labels=11 violations=12", m.String())
}

// TestMetrics_ParseRoundTrip checks Parse against String: on whole-number
// Area and Wire (String rounds both to whole px, so a round trip on a
// fractional value would not come back equal), Parse(m.String()) is m
// again. It also rejects a malformed line.
func TestMetrics_ParseRoundTrip(t *testing.T) {
	m := Metrics{Crossings: 1, Bends: 2, Jogs: 3, Straight: 4, Fans: 6, Centered: 5, Area: 345862, Wire: 864,
		Overlaps: 7, Intrusions: 8, Clearance: 9, Stubs: 10, Labels: 11, Violations: 12}
	got, err := Parse(m.String())
	require.NoError(t, err)
	assert.Equal(t, m, got)

	_, err = Parse("garbage")
	assert.Error(t, err)
}
