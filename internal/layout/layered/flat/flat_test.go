package flat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/layout/contract"
	"github.com/oxforge/diago/internal/layoutdbg"
	"github.com/oxforge/diago/internal/model"
)

// The distances a caller fills from the Config (S14), per profile: the
// screen's px, and the text cells under DOWN (x columns, y rows) and under
// RIGHT (x rows, y columns), each distance as many px on both axes.
var (
	screen    = Options{Clearance: XY{12, 12}, Stub: XY{20, 24}, TrackGap: XY{8, 8}, PortGap: XY{4, 4}, Dir: model.Down}
	textDown  = Options{Clearance: XY{2, 1}, Stub: XY{3, 2}, TrackGap: XY{2, 1}, PortGap: XY{1, 0.5}, Dir: model.Down, Text: true}
	textRight = Options{Clearance: XY{1, 2}, Stub: XY{2, 3}, TrackGap: XY{1, 2}, PortGap: XY{1, 2}, Dir: model.Right, Text: true}
)

const (
	rect    = model.ShapeRect
	diamond = model.ShapeDiamond
)

// box is node id of shape s whose box spans [x0, x1] × [y0, y1] in the
// engine's frame.
func box(id string, s model.Shape, x0, y0, x1, y1 float64) model.PositionedNode {
	return model.PositionedNode{ID: id, Shape: s, X: (x0 + x1) / 2, Y: (y0 + y1) / 2, Width: x1 - x0, Height: y1 - y0}
}

// group is group id spanning [x0, x1] × [y0, y1], holding the nodes ids.
func group(id string, x0, y0, x1, y1 float64, ids ...string) model.PositionedGroup {
	return model.PositionedGroup{ID: id, X: x0, Y: y0, Width: x1 - x0, Height: y1 - y0, Contains: ids, Children: []string{}}
}

// wire is an edge already drawn, from node from to node to along pts.
func wire(id, from, to string, pts ...model.Point) model.PositionedEdge {
	return model.PositionedEdge{ID: id, From: from, To: to, Points: pts}
}

func pt(x, y float64) model.Point { return model.Point{X: x, Y: y} }

// composed is a composed layout holding nodes, groups and wires, with the
// flat edge from a to b last, not drawn yet; it returns the layout and the
// flat edge's index.
func composed(nodes []model.PositionedNode, groups []model.PositionedGroup, wires ...model.PositionedEdge) func(from, to string) (*model.PositionedGraph, int) {
	return func(from, to string) (*model.PositionedGraph, int) {
		pg := &model.PositionedGraph{Nodes: nodes, Groups: groups, Edges: slices.Clone(wires)}
		pg.Edges = append(pg.Edges, model.PositionedEdge{ID: "flat", From: from, To: to})
		return pg, len(pg.Edges) - 1
	}
}

func nodes(ns ...model.PositionedNode) []model.PositionedNode { return ns }

// route routes the flat edge from a to b on the scene and, when it keeps
// a route, checks it with the contract (clean).
func route(t *testing.T, s func(from, to string) (*model.PositionedGraph, int), from, to string, o Options) Result {
	t.Helper()
	pg, e := s(from, to)
	res := Route(context.Background(), pg, e, o)
	if res.Kind != None {
		clean(t, pg, e, res.Points, o)
	}
	return res
}

// routeRules are the contract's rules a flat route must meet (S9, Flat
// edges), and C2.2, which its ends meet on the drawn outlines.
var routeRules = []string{"C2.2", "C3", "C4", "C5", "C6.1", "C6.2", "C7", "C8.1", "C8.2", "C8.3", "C8.4", "C8.5", "C9.1", "C9.2", "C10"}

// clean checks the scene alone, and then with the flat edge along pts,
// against routeRules with the contract, in the output frame and px: so
// that the router's own checks and the contract agree on every kept route.
func clean(t *testing.T, pg *model.PositionedGraph, e int, pts []model.Point, o Options) {
	t.Helper()
	require.Empty(t, violations(pg, o), "the scene alone")
	with := *pg
	with.Edges = slices.Clone(pg.Edges)
	with.Edges[e].Points = pts
	assert.Empty(t, violations(&with, o), "the scene with the flat route %v", pts)
}

// TestRouteRules_AreTheContractsIDs pins routeRules to ids the contract
// reports, so that a renamed rule cannot drop out of every kept route's
// check unnoticed.
func TestRouteRules_AreTheContractsIDs(t *testing.T) {
	for _, r := range routeRules {
		assert.Contains(t, contract.RuleIDs, r)
	}
}

func violations(pg *model.PositionedGraph, o Options) []contract.Violation {
	lim := contract.ScreenLimits()
	if o.Text {
		lim = contract.TextLimits(o.Dir)
	}
	var out []contract.Violation
	for _, v := range contract.Check(output(pg, o), contract.Options{Limits: lim, Direction: o.Dir, Text: o.Text}) {
		if slices.Contains(routeRules, v.Rule) {
			out = append(out, v)
		}
	}
	return out
}

// output turns a layout in the engine's frame into the output frame, as
// S11 does but for the margins, and in the text profile scales cells to
// 8 × 16 px, as the adapter does (S14).
func output(pg *model.PositionedGraph, o Options) *model.PositionedGraph {
	sx, sy := 1.0, 1.0
	if o.Text {
		sx, sy = 8, 16
	}
	turn := func(x, y float64) (float64, float64) {
		switch o.Dir {
		case model.Up:
			y = -y
		case model.Right:
			x, y = y, x
		case model.Left:
			x, y = -y, x
		}
		return x * sx, y * sy
	}
	sideways := o.Dir == model.Right || o.Dir == model.Left
	out := &model.PositionedGraph{}
	for _, n := range pg.Nodes {
		n.X, n.Y = turn(n.X, n.Y)
		if sideways {
			n.Width, n.Height = n.Height, n.Width
		}
		n.Width, n.Height = n.Width*sx, n.Height*sy
		out.Nodes = append(out.Nodes, n)
	}
	for _, g := range pg.Groups {
		x0, y0 := turn(g.X, g.Y)
		x1, y1 := turn(g.X+g.Width, g.Y+g.Height)
		g.X, g.Y, g.Width, g.Height = min(x0, x1), min(y0, y1), math.Abs(x1-x0), math.Abs(y1-y0)
		out.Groups = append(out.Groups, g)
	}
	for _, e := range pg.Edges {
		pts := make([]model.Point, len(e.Points))
		for i, p := range e.Points {
			pts[i].X, pts[i].Y = turn(p.X, p.Y)
		}
		e.Points = pts
		out.Edges = append(out.Edges, e)
	}
	return out
}

// TestRoute_StraightBetweenFacingSides pins the first candidate: two nodes
// side by side whose side ports share a y are joined by one run between
// their facing side faces, at the middle of the shorter side's range.
func TestRoute_StraightBetweenFacingSides(t *testing.T) {
	for _, tt := range []struct {
		name string
		a, b model.PositionedNode
		want []model.Point
	}{
		{"b on the right", box("a", rect, 0, 0, 80, 40), box("b", rect, 160, 0, 240, 40), []model.Point{pt(80, 20), pt(160, 20)}},
		{"b on the left", box("a", rect, 160, 0, 240, 40), box("b", rect, 0, 0, 80, 40), []model.Point{pt(160, 20), pt(80, 20)}},
		{"the shorter side's middle", box("a", rect, 0, 0, 80, 40), box("b", rect, 160, 0, 240, 80), []model.Point{pt(80, 20), pt(160, 20)}},
		{"the nearest y both share", box("a", rect, 0, 0, 80, 40), box("b", rect, 160, 30, 240, 110), []model.Point{pt(80, 34), pt(160, 34)}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			res := route(t, composed(nodes(tt.a, tt.b), nil), "a", "b", screen)
			assert.Equal(t, Straight, res.Kind)
			assert.Equal(t, tt.want, res.Points)
			assert.Empty(t, res.Refused)
			assert.Zero(t, res.Skipped)
		})
	}
}

// TestRoute_StraightAlongBetweenStackedNodes pins the second candidate:
// two stacked nodes whose facing faces share a column are joined by one
// run along the flow, from the upper one's bottom into the lower one's
// top, at the middle of the shorter face's range; a diamond joins at its
// vertex, and a run that would leave it upward from its in-vertex is
// refused (C8.3) for the next candidate.
func TestRoute_StraightAlongBetweenStackedNodes(t *testing.T) {
	for _, tt := range []struct {
		name     string
		a, b     model.PositionedNode
		from, to string
		want     []model.Point
	}{
		{"rect over rect", box("a", rect, 0, 0, 80, 40), box("b", rect, 20, 120, 100, 160), "a", "b", []model.Point{pt(40, 40), pt(40, 120)}},
		{"rect over rect, pointing up", box("a", rect, 0, 0, 80, 40), box("b", rect, 20, 120, 100, 160), "b", "a", []model.Point{pt(60, 120), pt(60, 40)}},
		{"the nearest x both share", box("a", rect, 0, 0, 80, 40), box("b", rect, 60, 120, 140, 160), "a", "b", []model.Point{pt(64, 40), pt(64, 120)}},
		{"diamond over rect", box("a", diamond, 0, 0, 80, 60), box("b", rect, 0, 140, 80, 180), "a", "b", []model.Point{pt(40, 60), pt(40, 140)}},
		{"rect over diamond, into its in-vertex", box("a", rect, 0, 0, 160, 40), box("b", diamond, 60, 120, 140, 180), "a", "b", []model.Point{pt(100, 40), pt(100, 120)}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			res := route(t, composed(nodes(tt.a, tt.b), nil), tt.from, tt.to, screen)
			assert.Equal(t, StraightAlong, res.Kind)
			assert.Equal(t, tt.want, res.Points)
			assert.Equal(t, []Refusal{{Kind: Straight, Reason: "not side by side"}}, res.Refused)
		})
	}
	t.Run("rect over diamond, pointing up: not out of its in-vertex (C8.3)", func(t *testing.T) {
		s := composed(nodes(box("r", rect, 0, 0, 160, 40), box("d", diamond, 60, 120, 140, 180)), nil)
		res := route(t, s, "d", "r", screen)
		assert.Equal(t, L, res.Kind)
		assert.Equal(t, []model.Point{pt(60, 150), pt(40, 150), pt(40, 40)}, res.Points, "out of its left vertex, into r's bottom")
		assert.Equal(t, []Refusal{{Kind: Straight, Reason: "not side by side"}, {Kind: StraightAlong, Tried: 1, Reason: "C8.3"}}, res.Refused)
	})
}

// TestRoute_ZWhenNoPortFitsOnBoth pins the last candidate: two nodes side
// by side, offset along the flow so that no y holds a port on both but not
// stacked, are joined out of one side face, by a cross run in the middle
// of the gap, into the other's; the straight runs and the L do not apply.
func TestRoute_ZWhenNoPortFitsOnBoth(t *testing.T) {
	s := composed(nodes(box("a", rect, 0, 0, 80, 40), box("b", rect, 160, 38, 240, 78)), nil)
	res := route(t, s, "a", "b", screen)
	assert.Equal(t, Z, res.Kind)
	assert.Equal(t, []model.Point{pt(80, 20), pt(120, 20), pt(120, 58), pt(160, 58)}, res.Points)
	assert.Equal(t, []Refusal{{Kind: Straight, Reason: "no side port on both"}, {Kind: StraightAlong, Reason: "not stacked"},
		{Kind: L, Reason: "not stacked"}}, res.Refused)

	res = route(t, s, "b", "a", screen)
	assert.Equal(t, []model.Point{pt(160, 58), pt(120, 58), pt(120, 20), pt(80, 20)}, res.Points, "from b's side face")
}

// TestRoute_ZAroundANodeOnTheStraightRow pins the Z around a node that
// blocks every straight run: b and c share a row beside the tall a, and
// the Z leaves a above c, its cross run the first x of the gap clear of
// c, beyond it, so that its run into b passes c by.
func TestRoute_ZAroundANodeOnTheStraightRow(t *testing.T) {
	s := composed(nodes(box("a", rect, 0, 0, 80, 120), box("b", rect, 320, 80, 400, 120), box("c", rect, 160, 80, 240, 120)), nil)
	res := route(t, s, "a", "b", screen)
	assert.Equal(t, Z, res.Kind)
	assert.Equal(t, []model.Point{pt(80, 60), pt(256, 60), pt(256, 100), pt(320, 100)}, res.Points)
	assert.Equal(t, []Refusal{{Kind: Straight, Tried: 4, Reason: "C4 node c"}, {Kind: StraightAlong, Reason: "not stacked"},
		{Kind: L, Reason: "not stacked"}}, res.Refused)
	assert.Equal(t, 14, res.Skipped, "the cross run's xs from the gap's middle, 200, out to 256, the 15th")
}

// TestRoute_LBetweenStackedNodes pins the third candidate: two stacked
// nodes, overlapping across the flow too little for a column on both, are
// joined out of the From node's side face toward the middle of the To
// node's facing face, into that face; neither straight run applies.
func TestRoute_LBetweenStackedNodes(t *testing.T) {
	res := route(t, composed(nodes(box("a", rect, 0, 0, 80, 40), box("b", rect, 70, 120, 270, 160)), nil), "a", "b", screen)
	assert.Equal(t, L, res.Kind)
	assert.Equal(t, []model.Point{pt(80, 20), pt(170, 20), pt(170, 120)}, res.Points, "into b's top")
	assert.Equal(t, []Refusal{{Kind: Straight, Reason: "not side by side"}, {Kind: StraightAlong, Reason: "no column on both"}}, res.Refused)

	res = route(t, composed(nodes(box("a", rect, 0, 120, 80, 160), box("b", rect, 70, 0, 270, 40)), nil), "a", "b", screen)
	assert.Equal(t, []model.Point{pt(80, 140), pt(170, 140), pt(170, 40)}, res.Points, "into b's bottom")
}

// TestRoute_LBeforeZ pins the order by bends (Q2): two nodes disjoint on
// both axes, which a Z would join too, take the one-bend L.
func TestRoute_LBeforeZ(t *testing.T) {
	res := route(t, composed(nodes(box("a", rect, 0, 0, 80, 40), box("b", rect, 160, 120, 240, 160)), nil), "a", "b", screen)
	assert.Equal(t, L, res.Kind)
	assert.Equal(t, []model.Point{pt(80, 20), pt(200, 20), pt(200, 120)}, res.Points)
}

// TestRoute_TheLsRightFaceOnATie pins the L's side face (S9, Flat edges,
// L): the one toward the middle of the other node's facing face, the right
// one on a tie. b's top face is centered under a, and c, as wide as a,
// blocks every column the straight run along could take, so the L leaves
// a's right face, where the left one would do as well, and turns down
// into b's top at the first column of its list a stub beyond a.
func TestRoute_TheLsRightFaceOnATie(t *testing.T) {
	s := composed(nodes(box("a", rect, 0, 0, 80, 40), box("c", rect, 0, 80, 80, 120), box("b", rect, -60, 160, 140, 200)), nil)
	res := route(t, s, "a", "b", screen)
	assert.Equal(t, L, res.Kind)
	assert.Equal(t, []model.Point{pt(80, 20), pt(104, 20), pt(104, 160)}, res.Points, "out of a's right face")
}

// TestRoute_Diamonds pins C8 on the flat routes: a diamond attaches at its
// vertices only, never leaves from its in-vertex (the top), takes a
// forward flat edge at the top and a back one at a side vertex; where no
// candidate reaches the vertex a route needs, there is none.
func TestRoute_Diamonds(t *testing.T) {
	t.Run("side by side: its side vertex", func(t *testing.T) {
		s := composed(nodes(box("d", diamond, 0, 0, 80, 60), box("r", rect, 160, 10, 240, 50)), nil)
		res := route(t, s, "d", "r", screen)
		assert.Equal(t, Straight, res.Kind)
		assert.Equal(t, []model.Point{pt(80, 30), pt(160, 30)}, res.Points)
		res = route(t, s, "r", "d", screen)
		assert.Equal(t, []model.Point{pt(160, 30), pt(80, 30)}, res.Points)
	})
	s := composed(nodes(box("d", diamond, 0, 0, 80, 60), box("r", rect, 60, 140, 260, 180)), nil)
	t.Run("above a rect, leaving forward: its side vertex", func(t *testing.T) {
		res := route(t, s, "d", "r", screen)
		assert.Equal(t, L, res.Kind)
		assert.Equal(t, []model.Point{pt(80, 30), pt(160, 30), pt(160, 140)}, res.Points)
	})
	t.Run("above a rect, entered back: its side vertex (C8.4), out of the rect's side is refused", func(t *testing.T) {
		res := route(t, s, "r", "d", screen)
		assert.Equal(t, L, res.Kind)
		assert.Equal(t, []model.Point{pt(160, 140), pt(160, 30), pt(80, 30)}, res.Points, "out of r's top, into d's right vertex")
		assert.Equal(t, 5, res.Skipped, "r's five side ports into d's bottom vertex")
	})
	t.Run("below a rect, entered forward: its in-vertex (C8.2)", func(t *testing.T) {
		s := composed(nodes(box("r", rect, 0, 0, 80, 40), box("d", diamond, 120, 120, 200, 180)), nil)
		res := route(t, s, "r", "d", screen)
		assert.Equal(t, L, res.Kind)
		assert.Equal(t, []model.Point{pt(80, 20), pt(160, 20), pt(160, 120)}, res.Points)
		assert.Equal(t, []Refusal{{Kind: Straight, Reason: "no side port on both"}, {Kind: StraightAlong, Reason: "no column on both"}}, res.Refused)
	})
	t.Run("below a rect, too near its column to reach its in-vertex: no route into its side (C8.2)", func(t *testing.T) {
		s := composed(nodes(box("r", rect, 0, 0, 80, 40), box("d", diamond, 50, 120, 130, 180)), nil)
		res := route(t, s, "r", "d", screen)
		assert.Equal(t, None, res.Kind)
		assert.Equal(t, []Refusal{{Kind: Straight, Reason: "not side by side"}, {Kind: StraightAlong, Reason: "no column on both"},
			{Kind: L, Tried: 3, Reason: "C8.2"}, {Kind: Z, Reason: "not side by side"}}, res.Refused)
	})
	t.Run("over a rect, aligned, entered back: no route (C8.4)", func(t *testing.T) {
		s := composed(nodes(box("d", diamond, 0, 0, 80, 60), box("r", rect, 0, 140, 80, 180)), nil)
		res := route(t, s, "r", "d", screen)
		assert.Equal(t, None, res.Kind)
		assert.Equal(t, []Refusal{{Kind: Straight, Reason: "not side by side"}, {Kind: StraightAlong, Tried: 1, Reason: "C8.4"},
			{Kind: L, Reason: "no column beyond the side node"}, {Kind: Z, Reason: "not side by side"}}, res.Refused)
	})
}

// TestRoute_DiamondExits pins C8.5 on a flat edge leaving a diamond
// forward: its right vertex already sends a forward exit, so the L out of
// it is refused at each of its nine columns, and the L into the far
// node's side leaves its bottom vertex; without that exit, the first L
// leaves the right vertex.
func TestRoute_DiamondExits(t *testing.T) {
	d, r, f := box("d", diamond, 0, 0, 80, 60), box("r", rect, 100, 140, 140, 180), box("f", rect, 240, 240, 320, 280)
	res := route(t, composed(nodes(d, r, f), nil, wire("e", "d", "r", pt(80, 30), pt(120, 30), pt(120, 140))), "d", "f", screen)
	assert.Equal(t, L, res.Kind)
	assert.Equal(t, []model.Point{pt(40, 60), pt(40, 260), pt(240, 260)}, res.Points)
	assert.Equal(t, 9, res.Skipped)

	res = route(t, composed(nodes(d, r, f), nil), "d", "f", screen)
	assert.Equal(t, []model.Point{pt(80, 30), pt(280, 30), pt(280, 240)}, res.Points)
	assert.Zero(t, res.Skipped)
}

// TestRoute_TheKindByC0 pins the edge's kind as C0 gives it, with C0's
// tolerance on distance floors: two boxes that overlap on the flow axis
// by less than it are forward or back, not lateral, so that a flat edge
// between them meets C8.2, C8.4 and C8.5 as the contract applies them.
func TestRoute_TheKindByC0(t *testing.T) {
	t.Run("into a diamond overlapping by less than the tolerance: forward, no Z into its side vertex (C8.2)", func(t *testing.T) {
		for _, ov := range []float64{0.001, 0.03, 0.049} {
			s := composed(nodes(box("a", rect, 0, 0, 80, 40), box("b", diamond, 160, 40-ov, 240, 100-ov)), nil)
			res := route(t, s, "a", "b", screen) // a kept route must meet the contract
			assert.Equal(t, None, res.Kind, "overlap %v: kept %v", ov, res.Points)
			assert.Equal(t, []Refusal{{Kind: Straight, Reason: "no side port on both"}, {Kind: StraightAlong, Reason: "not stacked"},
				{Kind: L, Reason: "not stacked"}, {Kind: Z, Tried: 25, Reason: "C8.2"}}, res.Refused, "overlap %v", ov)
		}
	})
	t.Run("overlapping by more: lateral, the Z into its side vertex", func(t *testing.T) {
		s := composed(nodes(box("a", rect, 0, 0, 80, 40), box("b", diamond, 160, 39.94, 240, 99.94)), nil)
		res := route(t, s, "a", "b", screen)
		assert.Equal(t, Z, res.Kind)
		assert.Equal(t, []model.Point{pt(80, 20), pt(120, 20), pt(120, 69.94), pt(160, 69.94)}, res.Points)
	})
	t.Run("a forward exit overlapping by less than the tolerance: counted (C8.5)", func(t *testing.T) {
		// e leaves d's right vertex for t, whose box overlaps d's by 0.03
		// on the flow axis: a forward exit, so the flat edge to f leaves
		// d's bottom vertex, not the right one beside e.
		s := composed(nodes(box("d", diamond, 0, 0, 80, 60), box("t", rect, 160, 59.97, 240, 99.97), box("f", rect, 240, 140, 320, 180)), nil,
			wire("e", "d", "t", pt(80, 30), pt(200, 30), pt(200, 59.97)))
		res := route(t, s, "d", "f", screen)
		assert.Equal(t, L, res.Kind)
		assert.Equal(t, []model.Point{pt(40, 60), pt(40, 160), pt(240, 160)}, res.Points)
		assert.Equal(t, 9, res.Skipped, "the nine columns on f's top out of d's right vertex")
	})
}

// TestKindOf pins the kind by C0's tolerance on both sides: forward and
// back up to 0.05 px of overlap on the flow axis, lateral beyond it (C0).
func TestKindOf(t *testing.T) {
	f := bounds{0, 40, 80, 80}
	for _, tt := range []struct {
		name string
		t    bounds
		want kind
	}{
		{"downstream, touching", bounds{160, 80, 240, 120}, forward},
		{"downstream, overlapping by 0.03", bounds{160, 79.97, 240, 119.97}, forward},
		{"downstream, overlapping by 0.06", bounds{160, 79.94, 240, 119.94}, lateral},
		{"upstream, touching", bounds{160, 0, 240, 40}, back},
		{"upstream, overlapping by 0.03", bounds{160, 0.03, 240, 40.03}, back},
		{"upstream, overlapping by 0.06", bounds{160, 0.06, 240, 40.06}, lateral},
	} {
		assert.Equal(t, tt.want, kindOf(f, tt.t), tt.name)
	}
}

// TestRoute_KeepsTheTrackGapFromAWire pins C9 against a wire already
// drawn: a Z whose cross run would share the wire's run (C9.1), or come
// within the track gap of it (C9.2), is refused, and the next x kept.
func TestRoute_KeepsTheTrackGapFromAWire(t *testing.T) {
	for _, tt := range []struct {
		name string
		x    float64
	}{
		{"on its run (C9.1)", 120},
		{"within the track gap (C9.2)", 124},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := composed(nodes(box("a", rect, 0, 0, 80, 40), box("b", rect, 160, 38, 240, 78),
				box("c", rect, tt.x-40, -100, tt.x+40, -60), box("d", rect, tt.x-40, 160, tt.x+40, 200)), nil,
				wire("w", "c", "d", pt(tt.x, -60), pt(tt.x, 160)))
			pg, e := s("a", "b")
			res := Route(context.Background(), pg, e, screen)
			assert.Equal(t, Z, res.Kind)
			assert.Equal(t, []model.Point{pt(80, 20), pt(112, 20), pt(112, 58), pt(160, 58)}, res.Points)
			assert.Equal(t, 1, res.Skipped)
			clean(t, pg, e, res.Points, screen)
		})
	}
}

// TestRoute_Groups pins C6.1: a group box that holds neither end keeps the
// node clearance from the route, which moves off it; a group that holds
// one end is crossed.
func TestRoute_Groups(t *testing.T) {
	a, b := box("a", rect, 0, 0, 80, 80), box("b", rect, 240, 0, 320, 80)
	t.Run("holding neither end", func(t *testing.T) {
		s := composed(nodes(a, b, box("n", rect, 132, -20, 188, 20)), []model.PositionedGroup{group("g", 120, -32, 200, 32, "n")})
		res := route(t, s, "a", "b", screen)
		assert.Equal(t, Straight, res.Kind)
		assert.Equal(t, []model.Point{pt(80, 48), pt(240, 48)}, res.Points, "12 px below it, past 40 and 32")
		assert.Equal(t, 2, res.Skipped)
	})
	t.Run("holding one end", func(t *testing.T) {
		s := composed(nodes(a, b), []model.PositionedGroup{group("g", -12, -12, 120, 92, "a")})
		res := route(t, s, "a", "b", screen)
		assert.Equal(t, []model.Point{pt(80, 40), pt(240, 40)}, res.Points, "across its border")
		assert.Zero(t, res.Skipped)
	})
}

// alongBorder reports the first segment of pts that runs parallel to a
// side of group g within the track gap o gives its axis, overlapping the
// side by more than noise, as its index, or -1 when none does (C6.2; S9,
// Flat edges, Checks); in the text profile a side is its border's own
// cell.
func alongBorder(pts []model.Point, g model.PositionedGroup, o Options) int {
	l, t, r, b := g.X, g.Y, g.X+g.Width, g.Y+g.Height
	if o.Text {
		l, t, r, b = l+0.5, t+0.5, r-0.5, b-0.5
	}
	for i := 0; i+1 < len(pts); i++ {
		p, q := pts[i], pts[i+1]
		switch {
		case vertical(p, q) && overlap(p.Y, q.Y, t, b) > noise:
			if math.Abs(p.X-l) < o.TrackGap.X-noise || math.Abs(p.X-r) < o.TrackGap.X-noise {
				return i
			}
		case horizontal(p, q) && overlap(p.X, q.X, l, r) > noise:
			if math.Abs(p.Y-t) < o.TrackGap.Y-noise || math.Abs(p.Y-b) < o.TrackGap.Y-noise {
				return i
			}
		}
	}
	return -1
}

// TestRoute_OffTheBorderOfAGroupHoldingAnEnd pins C6.2 against a group
// that holds one of the flat edge's ends: the route crosses its border
// freely (C6.1), but a segment parallel to one of its sides keeps
// the track gap from it, so that the wire never runs on the border line
// and vanishes into it. Where the first position would, the next one off
// the border is kept: an L's column on the right side of a's group, 8 px
// in from it; an L's side run on the top side of b's group, a side port 8
// px above it; a Z's cross run on the right side of a's group, 8 px in
// from it; and in the text profile under DOWN, an L's column on the
// border's own cell, two columns in from the side.
func TestRoute_OffTheBorderOfAGroupHoldingAnEnd(t *testing.T) {
	for _, tt := range []struct {
		name    string
		o       Options
		a, b    model.PositionedNode
		g       model.PositionedGroup
		kind    Kind
		want    []model.Point
		skipped int
	}{
		{"an L's column on the right side", screen, box("a", rect, 0, 0, 80, 40), box("b", rect, 100, 120, 180, 160),
			group("g", -24, -24, 140, 64, "a"), L, []model.Point{pt(80, 20), pt(132, 20), pt(132, 120)}, 1},
		{"an L's side run on the top side", screen, box("a", rect, 0, 0, 80, 40), box("b", rect, 120, 120, 200, 160),
			group("g", 96, 20, 224, 184, "b"), L, []model.Point{pt(80, 12), pt(160, 12), pt(160, 120)}, 9},
		{"a Z's cross run on the right side", screen, box("a", rect, 0, 0, 80, 40), box("b", rect, 200, 38, 280, 78),
			group("g", -24, -24, 140, 100, "a"), Z, []model.Point{pt(80, 20), pt(132, 20), pt(132, 58), pt(200, 58)}, 1},
		{"text, an L's column on the border's cell", textDown, box("a", rect, 0, 0, 10, 3), box("b", rect, 12, 7, 22, 10),
			group("g", -2, -1, 18, 5, "a"), L, []model.Point{pt(10, 1.5), pt(15.5, 1.5), pt(15.5, 7)}, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			res := route(t, composed(nodes(tt.a, tt.b), []model.PositionedGroup{tt.g}), "a", "b", tt.o)
			assert.Equal(t, tt.kind, res.Kind)
			assert.Equal(t, tt.want, res.Points)
			assert.Equal(t, tt.skipped, res.Skipped, "the positions on or beside the border")
			assert.Equal(t, -1, alongBorder(res.Points, tt.g, tt.o), "a segment runs along a side of %s", tt.g.ID)
		})
	}
}

// TestRoute_TextBesideTheBorderCell pins where the text profile measures
// the gap from a holding group's side (S9, Flat edges, Checks): from the
// border's own cell, the box's edge cell the text art draws it in, not
// from the box's edge line. Under DOWN, a's group draws its right side in
// column 11; the Z's only cross run, on column 13, keeps one blank column
// between itself and that border, two columns from its cell, and is kept.
func TestRoute_TextBesideTheBorderCell(t *testing.T) {
	g := group("g", -2, -1, 12, 5, "a")
	res := route(t, composed(nodes(box("a", rect, 0, 0, 10, 3), box("b", rect, 17, 3, 27, 6)), []model.PositionedGroup{g}), "a", "b", textDown)
	assert.Equal(t, Z, res.Kind, "refused: %v", res.Refused)
	assert.Equal(t, []model.Point{pt(10, 1.5), pt(13.5, 1.5), pt(13.5, 4.5), pt(17, 4.5)}, res.Points)
	assert.Equal(t, -1, alongBorder(res.Points, g, textDown), "a segment runs along a side of g")
}

// TestRoute_NoneWhenEveryCandidateIsBlocked pins the report of no route: a
// node between the two on their row blocks the straight and every Z, and
// the two are not stacked for the straight along or an L; each candidate
// says why.
func TestRoute_NoneWhenEveryCandidateIsBlocked(t *testing.T) {
	s := composed(nodes(box("a", rect, 0, 0, 80, 40), box("c", rect, 160, 0, 240, 40), box("b", rect, 320, 0, 400, 40)), nil)
	res := route(t, s, "a", "b", screen)
	assert.Equal(t, None, res.Kind)
	assert.Nil(t, res.Points)
	require.Len(t, res.Refused, 4)
	assert.Equal(t, Refusal{Kind: Straight, Tried: 5, Reason: "C4 node c"}, res.Refused[0])
	assert.Equal(t, Refusal{Kind: StraightAlong, Reason: "not stacked"}, res.Refused[1])
	assert.Equal(t, Refusal{Kind: L, Reason: "not stacked"}, res.Refused[2])
	assert.Equal(t, Refusal{Kind: Z, Tried: 12 * 25, Reason: "C4 node c"}, res.Refused[3], "12 pairs of side ports a clearance apart, 25 cross runs each")
}

// TestRoute_TheUpperOfTwoEqualPositions pins the tie-break of every list
// of positions: at each step from its start, the lower (up or left)
// before the higher, so of two ys as far from the middle, both clear of a
// node in the gap, the upper is kept.
func TestRoute_TheUpperOfTwoEqualPositions(t *testing.T) {
	s := composed(nodes(box("a", rect, 0, 0, 80, 120), box("b", rect, 320, 0, 400, 120), box("c", rect, 160, 40, 240, 80)), nil)
	res := route(t, s, "a", "b", screen)
	assert.Equal(t, []model.Point{pt(80, 28), pt(320, 28)}, res.Points, "the clearance above c, not below it at 92")
	assert.Equal(t, 7, res.Skipped)
}

// TestRoute_Text pins the text profile: every shape is a box (a diamond
// takes a side port off its middle, and a column off its vertex), and
// every end and run lies on the middle of a cell, inside the box's
// corners, under DOWN (x columns, y rows) and RIGHT (x rows, y columns);
// a cross run or a column lies at least the stub and half a cell beyond a
// box, as S8's side columns do.
func TestRoute_Text(t *testing.T) {
	for _, tt := range []struct {
		name string
		o    Options
		a, b model.PositionedNode
		kind Kind
		want []model.Point
	}{
		{"down, straight", textDown, box("a", rect, 0, 0, 10, 3), box("b", diamond, 15, 0, 25, 5), Straight,
			[]model.Point{pt(10, 1.5), pt(15, 1.5)}},
		{"down, Z", textDown, box("a", rect, 0, 0, 10, 3), box("b", diamond, 18, 2, 28, 5), Z,
			[]model.Point{pt(10, 1.5), pt(14.5, 1.5), pt(14.5, 3.5), pt(18, 3.5)}},
		{"right, straight", textRight, box("a", rect, 0, 0, 3, 10), box("b", rect, 8, 0, 11, 10), Straight,
			[]model.Point{pt(3, 5.5), pt(8, 5.5)}},
		{"right, Z", textRight, box("a", rect, 0, 0, 3, 10), box("b", rect, 10, 8, 13, 18), Z,
			[]model.Point{pt(3, 5.5), pt(6.5, 5.5), pt(6.5, 13.5), pt(10, 13.5)}},
		{"down, L", textDown, box("a", rect, 0, 0, 10, 3), box("b", diamond, 8, 7, 24, 10), L,
			[]model.Point{pt(10, 1.5), pt(16.5, 1.5), pt(16.5, 7)}},
		{"right, L", textRight, box("a", rect, 0, 0, 3, 10), box("b", rect, 2, 14, 12, 30), L,
			[]model.Point{pt(3, 5.5), pt(7.5, 5.5), pt(7.5, 14)}},
		{"down, straight along", textDown, box("a", rect, 0, 0, 10, 3), box("b", rect, 2, 6, 12, 9), StraightAlong,
			[]model.Point{pt(5.5, 3), pt(5.5, 6)}},
		{"right, straight along", textRight, box("a", rect, 0, 0, 3, 10), box("b", rect, 0, 14, 3, 24), StraightAlong,
			[]model.Point{pt(1.5, 10), pt(1.5, 14)}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			res := route(t, composed(nodes(tt.a, tt.b), nil), "a", "b", tt.o)
			assert.Equal(t, tt.kind, res.Kind)
			assert.Equal(t, tt.want, res.Points)
		})
	}
}

// TestRoute_TextPortGap pins the text port gap along the flow under
// RIGHT, where a column is 8 px and the contract's port gap 16 px: a side
// port one column from another wire's terminal at the same node is
// refused (C9.2), three columns away it is kept.
func TestRoute_TextPortGap(t *testing.T) {
	// w leaves a's right face on column 4.5 and runs on to c, beside b.
	s := composed(nodes(box("a", rect, 0, 0, 3, 10), box("b", rect, 8, 0, 11, 10), box("c", rect, 8, 14, 11, 24)), nil,
		wire("w", "a", "c", pt(3, 4.5), pt(5.5, 4.5), pt(5.5, 19.5), pt(8, 19.5)))
	res := route(t, s, "a", "b", textRight)
	assert.Equal(t, Straight, res.Kind)
	assert.Equal(t, []model.Point{pt(3, 7.5), pt(8, 7.5)}, res.Points, "past 5.5 and 3.5, a column either side of w's port")
	assert.Equal(t, 2, res.Skipped)
}

// TestRoute_EveryShapeMeetsTheContract routes a flat edge between every
// two shapes, both ways, in every direction, beside each other on one
// row, offset along the flow, stacked and one straight below the other,
// and checks every kept route with the contract: each end on the drawn
// outline (C2.2), perpendicular to its side (C7), a diamond's and a
// circle's on a vertex (C8). Each placement takes the candidates its
// geometry allows, fewest bends first (Q2): two nodes on one row, their
// centers level, are always joined straight; a forward edge from a node
// to the one straight below it straight along, and so is the back edge
// unless a diamond or a circle ends it (C8.3, C8.4), which an L joins or
// none; offset, the forward edge takes the L, the back one the L or, from
// a diamond or a circle, the Z; stacked, the straight run along or the L,
// and a back edge with a diamond or a circle end may have none. Of the
// 1,568 routes 108 are refused, every one a back edge with a diamond or a
// circle end.
func TestRoute_EveryShapeMeetsTheContract(t *testing.T) {
	drawn := map[model.Shape][2]float64{ // each shape's drawn width and height, as S1 sizes it
		model.ShapeRect: {120, 60}, model.ShapeRounded: {120, 60}, model.ShapeHexagon: {80, 40 * math.Sqrt(3)},
		model.ShapeParallelogram: {140, 60}, model.ShapeCylinder: {120, 76}, diamond: {60 * math.Sqrt(3), 60},
		model.ShapeCircle: {60, 60},
	}
	shapes := []model.Shape{model.ShapeRect, model.ShapeRounded, model.ShapeHexagon, model.ShapeParallelogram,
		model.ShapeCylinder, diamond, model.ShapeCircle}
	vertex := func(sh model.Shape) bool { return sh == diamond || sh == model.ShapeCircle }
	kept, refused := 0, 0
	for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
		o := screen
		o.Dir = dir
		frame := func(sh model.Shape) (w, h float64) { // the engine's extents: the drawn ones, turned under RIGHT and LEFT
			w, h = drawn[sh][0], drawn[sh][1]
			if dir == model.Right || dir == model.Left {
				w, h = h, w
			}
			return w, h
		}
		for _, sa := range shapes {
			for _, sb := range shapes {
				wa, ha := frame(sa)
				wb, hb := frame(sb)
				a := box("a", sa, -wa/2, -ha/2, wa/2, ha/2)
				for _, at := range []struct {
					name string
					x, y float64
				}{
					{"one row", wa/2 + 80 + wb/2, 0},
					{"offset", wa/2 + 80 + wb/2, ha/2 + 40 + hb/2},
					{"stacked", wa/2 + 20, ha/2 + 80 + hb/2},
					{"aligned", 0, ha/2 + 80 + hb/2},
				} {
					b := box("b", sb, at.x-wb/2, at.y-hb/2, at.x+wb/2, at.y+hb/2)
					for _, ends := range [][2]string{{"a", "b"}, {"b", "a"}} {
						name := dir.String() + "/" + sa.String() + "-" + sb.String() + "/" + at.name + "/" + ends[0] + "->" + ends[1]
						t.Run(name, func(t *testing.T) {
							res := route(t, composed(nodes(a, b), nil), ends[0], ends[1], o)
							if res.Kind == None {
								refused++
							} else {
								kept++
							}
							back, corner := ends[0] == "b", vertex(sa) || vertex(sb)
							switch {
							case at.name == "one row":
								assert.Equal(t, Straight, res.Kind)
							case at.name == "aligned" && (!back || !corner):
								assert.Equal(t, StraightAlong, res.Kind)
							case at.name == "aligned":
								assert.Contains(t, []Kind{L, None}, res.Kind)
							case at.name == "offset" && (!back || !corner):
								assert.Equal(t, L, res.Kind)
							case at.name == "offset":
								assert.Contains(t, []Kind{L, Z}, res.Kind)
							case back && corner: // stacked
								assert.Contains(t, []Kind{StraightAlong, L, None}, res.Kind)
							default: // stacked
								assert.Contains(t, []Kind{StraightAlong, L}, res.Kind)
							}
						})
					}
				}
			}
		}
	}
	assert.Equal(t, [2]int{1460, 108}, [2]int{kept, refused}, "routes kept and refused")
}

// TestRoute_ASecondFlatEdgeBesideTheFirst pins the flat routes kept
// before an edge among the wires it keeps clear of: a second flat edge
// between the same two nodes cannot share the first's run (C9.1) and
// takes the next y, a track gap away, beside it as a rake at both ends
// (C9.2).
func TestRoute_ASecondFlatEdgeBesideTheFirst(t *testing.T) {
	a, b := box("a", rect, 0, 0, 80, 40), box("b", rect, 160, 0, 240, 40)
	first := route(t, composed(nodes(a, b), nil), "a", "b", screen)
	require.Equal(t, []model.Point{pt(80, 20), pt(160, 20)}, first.Points)
	res := route(t, composed(nodes(a, b), nil, wire("first", "a", "b", first.Points...)), "b", "a", screen)
	assert.Equal(t, []model.Point{pt(160, 12), pt(80, 12)}, res.Points)
	assert.Equal(t, 1, res.Skipped)
}

// TestRoute_TheStubs pins C7's stubs as the router takes them from the
// Config: a terminal segment at a side face at least 20 px long across
// the flow, and at a top or bottom face at least 24 px along it, above
// the contract's 20. Two nodes 16 px apart side by side, or 22 px apart
// stacked, have no route; 20 and 24 px apart, the straight run.
func TestRoute_TheStubs(t *testing.T) {
	t.Run("16 px apart across the flow: no straight run (C7)", func(t *testing.T) {
		res := route(t, composed(nodes(box("a", rect, 0, 0, 80, 40), box("b", rect, 96, 0, 176, 40)), nil), "a", "b", screen)
		assert.Equal(t, None, res.Kind, "kept %v", res.Points)
		assert.Equal(t, []Refusal{{Kind: Straight, Tried: 5, Reason: "C7"}, {Kind: StraightAlong, Reason: "not stacked"},
			{Kind: L, Reason: "not stacked"}, {Kind: Z, Reason: "no room for the cross run"}}, res.Refused)
	})
	t.Run("20 px apart across the flow: the straight run", func(t *testing.T) {
		res := route(t, composed(nodes(box("a", rect, 0, 0, 80, 40), box("b", rect, 100, 0, 180, 40)), nil), "a", "b", screen)
		assert.Equal(t, Straight, res.Kind)
		assert.Equal(t, []model.Point{pt(80, 20), pt(100, 20)}, res.Points)
	})
	t.Run("22 px apart along the flow: no straight run along it (C7)", func(t *testing.T) {
		res := route(t, composed(nodes(box("a", rect, 0, 0, 80, 40), box("b", rect, 0, 62, 80, 102)), nil), "a", "b", screen)
		assert.Equal(t, None, res.Kind, "kept %v", res.Points)
		assert.Equal(t, []Refusal{{Kind: Straight, Reason: "not side by side"}, {Kind: StraightAlong, Tried: 9, Reason: "C7"},
			{Kind: L, Reason: "no column beyond the side node"}, {Kind: Z, Reason: "not side by side"}}, res.Refused)
	})
	t.Run("24 px apart along the flow: the straight run along it", func(t *testing.T) {
		res := route(t, composed(nodes(box("a", rect, 0, 0, 80, 40), box("b", rect, 0, 64, 80, 104)), nil), "a", "b", screen)
		assert.Equal(t, StraightAlong, res.Kind)
		assert.Equal(t, []model.Point{pt(40, 40), pt(40, 64)}, res.Points)
	})
}

// TestRoute_TheTrackGapEndToEnd pins C9.2's closest-point distance between
// two parallel runs that lie end to end, in track gaps on each axis: the
// Z's cross run and a wire's run beside it, 5 px apart across, their ends
// 5 px apart along, are refused and the next x kept; their ends 8 px
// apart, the Z is kept, though the runs lie within the track gap across.
// A run collinear with the Z's would end at a bend whose leg meets the
// Z's leg at a corner, which the track gap across refuses on its own.
func TestRoute_TheTrackGapEndToEnd(t *testing.T) {
	for _, tt := range []struct {
		name    string
		y       float64 // where w's run ends above the Z's cross run, which starts at y 20
		want    []model.Point
		skipped int
	}{
		{"5 px apart: refused (C9.2)", 15, []model.Point{pt(80, 20), pt(112, 20), pt(112, 58), pt(160, 58)}, 1},
		{"8 px apart: kept", 12, []model.Point{pt(80, 20), pt(120, 20), pt(120, 58), pt(160, 58)}, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// w runs down x 125 from c, 5 px right of the Z's cross run at
			// x 120, and turns right at y into d, away from the Z's first run.
			s := composed(nodes(box("a", rect, 0, 0, 80, 40), box("b", rect, 160, 38, 240, 78),
				box("c", rect, 85, -140, 165, -100), box("d", rect, 300, tt.y-20, 380, tt.y+20)), nil,
				wire("w", "c", "d", pt(125, -100), pt(125, tt.y), pt(300, tt.y)))
			res := route(t, s, "a", "b", screen)
			assert.Equal(t, Z, res.Kind)
			assert.Equal(t, tt.want, res.Points)
			assert.Equal(t, tt.skipped, res.Skipped)
		})
	}
}

// TestRoute_ARakeTakesThePortGap pins C9.2's port gap on screen: a
// straight run leaving a's right face beside another wire's terminal
// segment on that face is a rake, whose floor is the port gap, 4 px, not
// the track gap, 8 px. 4 px from w's port it is kept; 3 px from it, it is
// refused and the next y kept.
func TestRoute_ARakeTakesThePortGap(t *testing.T) {
	for _, tt := range []struct {
		name    string
		y       float64 // w's port on a's right face; the straight run's first y is 20
		want    []model.Point
		skipped int
	}{
		{"4 px apart: kept", 24, []model.Point{pt(80, 20), pt(160, 20)}, 0},
		{"3 px apart: refused (C9.2)", 23, []model.Point{pt(80, 12), pt(160, 12)}, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := composed(nodes(box("a", rect, 0, 0, 80, 40), box("b", rect, 160, 0, 240, 40), box("c", rect, 70, 100, 150, 140)), nil,
				wire("w", "a", "c", pt(80, tt.y), pt(110, tt.y), pt(110, 100)))
			res := route(t, s, "a", "b", screen)
			assert.Equal(t, Straight, res.Kind)
			assert.Equal(t, tt.want, res.Points)
			assert.Equal(t, tt.skipped, res.Skipped)
		})
	}
}

// TestRoute_AComb pins C9.2's comb: an L into diamond b's in-vertex meets
// w, which comes into the same vertex from the other side, end to end on
// one track, with no floor between the two runs, and shares its column as
// a shared-vertex bundle.
func TestRoute_AComb(t *testing.T) {
	s := composed(nodes(box("a", rect, 0, 0, 80, 40), box("b", diamond, 120, 100, 200, 160), box("c", rect, 240, 0, 320, 40)), nil,
		wire("w", "c", "b", pt(240, 20), pt(160, 20), pt(160, 100)))
	res := route(t, s, "a", "b", screen)
	assert.Equal(t, L, res.Kind)
	assert.Equal(t, []model.Point{pt(80, 20), pt(160, 20), pt(160, 100)}, res.Points)
	assert.Zero(t, res.Skipped)
}

// TestRoute_AShapesVertexSharedWithAnotherEnd pins C9's exemptions at a
// diamond's vertex: a flat edge may share it with another edge's end, a
// shared-vertex bundle along one run out of it, and a rail where a Z's
// cross run joins the column another edge takes into it.
func TestRoute_AShapesVertexSharedWithAnotherEnd(t *testing.T) {
	t.Run("a bundle: out of d's right vertex beside e", func(t *testing.T) {
		s := composed(nodes(box("d", diamond, 0, 0, 80, 60), box("r", rect, 160, 10, 240, 50), box("t", rect, 80, 140, 160, 180)), nil,
			wire("e", "d", "t", pt(80, 30), pt(120, 30), pt(120, 140)))
		res := route(t, s, "d", "r", screen)
		assert.Equal(t, Straight, res.Kind)
		assert.Equal(t, []model.Point{pt(80, 30), pt(160, 30)}, res.Points)
	})
	t.Run("a rail: into b's left vertex along e's column", func(t *testing.T) {
		// e, a back edge, comes up x 120 from c into b's left vertex; the
		// Z from a joins its column at y 90.
		s := composed(nodes(box("a", rect, 0, 70, 80, 110), box("b", diamond, 160, 30, 240, 90), box("c", rect, 80, 160, 160, 200)), nil,
			wire("e", "c", "b", pt(120, 160), pt(120, 60), pt(160, 60)))
		res := route(t, s, "a", "b", screen)
		assert.Equal(t, Z, res.Kind)
		assert.Equal(t, []model.Point{pt(80, 90), pt(120, 90), pt(120, 60), pt(160, 60)}, res.Points)
		assert.Zero(t, res.Skipped)
	})
}

// TestRoute_ACirclesMergedFanOut pins C8.5 at a circle whose forward
// exits, w1 and w2, all leave its bottom vertex, a merged fan-out: a flat
// edge leaving it forward joins them there, and is refused at any other
// vertex.
func TestRoute_ACirclesMergedFanOut(t *testing.T) {
	fan := func(f model.PositionedNode) func(from, to string) (*model.PositionedGraph, int) {
		return composed(nodes(box("o", model.ShapeCircle, 0, 0, 60, 60), box("p", rect, -10, 140, 70, 180), box("q", rect, -140, 140, -60, 180), f), nil,
			wire("w1", "o", "p", pt(30, 60), pt(30, 140)),
			wire("w2", "o", "q", pt(30, 60), pt(30, 90), pt(-100, 90), pt(-100, 140)))
	}
	t.Run("from the fan-out's vertex: kept", func(t *testing.T) {
		res := route(t, fan(box("f", rect, 200, 80, 280, 120)), "o", "f", screen)
		assert.Equal(t, L, res.Kind)
		assert.Equal(t, []model.Point{pt(30, 60), pt(30, 100), pt(200, 100)}, res.Points, "out of o's bottom vertex, into f's left")
		assert.Equal(t, 9, res.Skipped, "the nine Ls out of o's right vertex (C8.5)")
	})
	t.Run("from another vertex: refused (C8.5)", func(t *testing.T) {
		// p blocks every route out of o's bottom vertex to f.
		res := route(t, fan(box("f", rect, 200, 140, 280, 180)), "o", "f", screen)
		assert.Equal(t, None, res.Kind, "kept %v", res.Points)
		assert.Equal(t, []Refusal{{Kind: Straight, Reason: "no side port on both"}, {Kind: StraightAlong, Reason: "no column on both"},
			{Kind: L, Tried: 14, Reason: "C8.5"}, {Kind: Z, Tried: 65, Reason: "C8.5"}}, res.Refused)
	})
}

// TestRoute_NoneForAnEdgeItCannotRead pins the report of an edge the
// router has no nodes for: every candidate is refused with the reason.
func TestRoute_NoneForAnEdgeItCannotRead(t *testing.T) {
	pg, e := composed(nodes(box("a", rect, 0, 0, 80, 40)), nil)("a", "x")
	res := Route(context.Background(), pg, e, screen)
	assert.Equal(t, None, res.Kind)
	assert.Equal(t, []Refusal{{Kind: Straight, Reason: "an unknown node"}, {Kind: StraightAlong, Reason: "an unknown node"},
		{Kind: L, Reason: "an unknown node"}, {Kind: Z, Reason: "an unknown node"}}, res.Refused)
	pg, e = composed(nodes(box("a", rect, 0, 0, 80, 40)), nil)("a", "a")
	assert.Equal(t, "a self-loop", Route(context.Background(), pg, e, screen).Refused[0].Reason)
	assert.Equal(t, "no such edge", Route(context.Background(), pg, 5, screen).Refused[0].Reason)
	assert.Equal(t, "no such edge", Route(context.Background(), nil, 0, screen).Refused[0].Reason)
}

// TestRoute_Deterministic routes the same scenes 50 times: the same
// result every time (C1).
func TestRoute_Deterministic(t *testing.T) {
	scenes := []func(from, to string) (*model.PositionedGraph, int){
		composed(nodes(box("a", rect, 0, 0, 80, 120), box("b", rect, 320, 0, 400, 120), box("c", rect, 160, 40, 240, 80)), nil),
		composed(nodes(box("a", rect, 0, 0, 80, 40), box("c", rect, 160, 0, 240, 40), box("b", rect, 320, 0, 400, 40)), nil),
		composed(nodes(box("d", diamond, 0, 0, 80, 60), box("r", rect, 100, 140, 140, 180), box("f", rect, 240, 240, 320, 280)), nil,
			wire("e", "d", "r", pt(80, 30), pt(120, 30), pt(120, 140))),
	}
	for i, s := range scenes {
		from, to := "a", "b"
		if i == 2 {
			from, to = "d", "f"
		}
		pg, e := s(from, to)
		first := Route(context.Background(), pg, e, screen)
		for range 50 {
			require.Equal(t, first, Route(context.Background(), pg, e, screen), "scene %d", i)
		}
	}
}

// TestRoute_LogsItsDecisions pins the router's debug records: phase flat,
// spec_ref S9, a flat_edge_routed with the candidate kept and the
// positions refused before it, and a flat_edge_unrouted with each
// candidate's reason and positions tried.
func TestRoute_LogsItsDecisions(t *testing.T) {
	var buf bytes.Buffer
	ctx := layoutdbg.NewContext(context.Background(), slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	pg, e := composed(nodes(box("a", rect, 0, 0, 80, 40), box("b", rect, 160, 38, 240, 78)), nil)("a", "b")
	Route(ctx, pg, e, screen)
	pg, e = composed(nodes(box("a", rect, 0, 0, 80, 40), box("c", rect, 160, 0, 240, 40), box("b", rect, 320, 0, 400, 40)), nil)("a", "b")
	Route(ctx, pg, e, screen)
	var recs []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var rec map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &rec))
		assert.Equal(t, "flat", rec["phase"])
		assert.Equal(t, "S9", rec["spec_ref"])
		assert.Equal(t, "diago", rec["module"])
		recs = append(recs, rec)
	}
	require.Len(t, recs, 2)
	assert.Equal(t, "flat_edge_routed", recs[0]["decision"])
	assert.Equal(t, "flat", recs[0]["edge"])
	assert.Equal(t, "z", recs[0]["kind"])
	assert.InDelta(t, 0, recs[0]["refused"], 0)
	assert.Equal(t, "flat_edge_unrouted", recs[1]["decision"])
	assert.Equal(t, "C4 node c", recs[1]["straight"])
	assert.InDelta(t, 5, recs[1]["straight_tried"], 0)
	assert.Equal(t, "not stacked", recs[1]["straight_along"])
	assert.Equal(t, "C4 node c", recs[1]["z"])
	assert.Equal(t, "not stacked", recs[1]["l"])
	assert.InDelta(t, 0, recs[1]["l_tried"], 0)
}

// TestLadder pins the lists of positions: from the start outward by the
// step, the lower before the higher at each step, within the range; on
// cells, the middles of the whole cells in the range, from the middle of
// the cell the start falls in.
func TestLadder(t *testing.T) {
	assert.Equal(t, []float64{20, 12, 28, 4, 36}, ladder(2, 38, 20, 8, false))
	assert.Equal(t, []float64{34}, ladder(34, 38, 34, 8, false), "one position")
	assert.Equal(t, []float64{100, 92, 108, 84}, ladder(82, 114, 100, 8, false), "on past one end while the other lasts")
	assert.Nil(t, ladder(40, 38, 39, 8, false), "an empty range")
	assert.Equal(t, []float64{14.5}, ladder(13, 15, 14, 2, true), "13.5 and 14.5 fit; the start's cell is 14.5, and 12.5 lies out")
	assert.Equal(t, []float64{5.5, 3.5, 7.5, 1.5}, ladder(1, 9, 5, 2, true))
	assert.Equal(t, []float64{8.5, 6.5, 4.5, 2.5}, ladder(1, 9, 9, 2, true), "a start past the range's last cell moves onto it")
	assert.Nil(t, ladder(2, 2.4, 2.2, 1, true), "no whole cell")
}

// BenchmarkRoute_UnroutableZ is the record of the flat router's time on a
// flat edge no candidate routes: two 300
// px tall rects 1,500 px apart on one row, a group holding neither of
// them filling the gap, which refuses every straight run and every Z
// (C6.1), and 100 other nodes on a row below, which every check visits
// (C5). Its position lists are unbounded: the Z tries every pair of side
// ports more than the node clearance apart at every x of the gap, and each
// x rechecks both terminal runs, which do not depend on it.
// positions/op counts the positions the four candidates tried.
func BenchmarkRoute_UnroutableZ(b *testing.B) {
	ns := nodes(box("a", rect, 0, 0, 100, 300), box("b", rect, 1600, 0, 1700, 300))
	for i := range 100 {
		x := float64(i) * 17
		ns = append(ns, box(fmt.Sprintf("n%d", i), rect, x, 600, x+12, 640))
	}
	pg, e := composed(ns, []model.PositionedGroup{group("g", 150, -100, 1550, 400)})("a", "b")
	var res Result
	for b.Loop() {
		res = Route(context.Background(), pg, e, screen)
	}
	if res.Kind != None {
		b.Fatalf("routed %v: want every candidate refused", res.Kind)
	}
	tried := 0
	for _, r := range res.Refused {
		tried += r.Tried
	}
	b.ReportMetric(float64(tried), "positions/op")
}
