// Package flat routes flat edges on the composed layout (S9, Flat edges).
// A flat edge carries no order: it takes no part in the layering, and
// once the levels are composed this router draws it between its two
// nodes, as a straight run across or along the flow, an L or a Z, whichever
// first, in order of bends, meets the output contract against everything
// already placed. Everything is in the
// engine's top-to-bottom frame: y is the flow axis, downstream +y, and
// S11 turns the result with the rest of the layout.
package flat

import (
	"context"

	"github.com/oxforge/diago/internal/layoutdbg"
	"github.com/oxforge/diago/internal/model"
)

// XY is one distance on each of the engine's two axes: X across the flow,
// where two points lie apart on x, and Y along it, apart on y.
type XY struct{ X, Y float64 }

// Options are the distances a flat route keeps (S9, Flat edges), each on
// the engine's two axes. On screen the two are the same px; in the text
// profile X counts columns and Y rows under DOWN and UP, the other way
// round under RIGHT and LEFT, and a row is two columns tall, so one
// distance takes different counts on the two axes. The caller fills them
// from the Config (S14):
//
//   - Clearance: the node clearance, Config.Clearance across the flow, as
//     many px along it (in text, half as many rows under DOWN and UP,
//     twice as many columns under RIGHT and LEFT);
//   - Stub: a terminal segment's least length, Config.Reach across the
//     flow (at a side face) and (Config.BaseGap + Config.LaneGap) / 2
//     along it (at a top or bottom face), S8's stubs;
//   - TrackGap: Config.InLaneGap across the flow (between two runs along
//     it) and Config.LaneGap along it (between two runs across it);
//   - PortGap: the gap between side-by-side terminal segments at a shared
//     end node (C9.2), Config.InLaneGap / 2 on screen and in text one cell
//     across the flow, as many px along it as the node clearance is.
type Options struct {
	Clearance XY
	Stub      XY
	TrackGap  XY
	PortGap   XY
	Dir       model.Direction // the output direction, for the drawn outlines (C2.2)
	Text      bool            // the text profile: every shape a box, every end and run on the cell grid
}

// Kind is a flat route's shape, and the candidates in the order the
// router tries them: by their bends, fewer first (Q2).
type Kind int

// The candidates. None is no route: every candidate was refused.
const (
	None          Kind = iota
	Straight           // one run across the flow between two nodes' facing side faces
	StraightAlong      // one run along the flow between two stacked nodes' facing faces
	L                  // one bend: a side face of one to the facing top or bottom face of the other
	Z                  // two bends: out of one side face, a cross run in the gap, into the other's
)

func (k Kind) String() string {
	return [...]string{"none", "straight", "straight_along", "l", "z"}[k]
}

// Refusal is why a candidate was refused: it does not apply (Tried 0), or
// the first of its Tried positions breaks the rule Reason names.
type Refusal struct {
	Kind   Kind
	Tried  int
	Reason string
}

// Result is a flat edge's route in the engine's frame, from its From node
// to its To node, or none.
type Result struct {
	Kind    Kind          // the candidate kept; None when every one was refused
	Points  []model.Point // the kept route; nil for None
	Refused []Refusal     // the candidates refused, in order: those before the kept one, or all four
	Skipped int           // the kept candidate's positions refused before the kept one
}

// noise is the float tolerance of every comparison but the edge's kind
// (kindSlack), a millionth of a unit (S9): the floors themselves are the
// Config's distances, never loosened.
const noise = 1e-6

// Route routes pg's edge e, a flat edge, on pg's composed layout in the
// engine's frame (S9, Flat edges): pg's nodes, its group boxes and every
// other edge's route drawn so far, an edge with fewer than two points not
// drawn yet. It tries the straight run across the flow, the straight run
// along it, the L and the Z, in order of bends, each over its positions in
// order, and keeps the first route that meets the output contract against
// all of them. Its decisions are recorded as phase flat.
func Route(ctx context.Context, pg *model.PositionedGraph, e int, o Options) Result {
	s, why := newScene(pg, e, o)
	if s == nil {
		res := Result{Refused: []Refusal{{Kind: Straight, Reason: why}, {Kind: StraightAlong, Reason: why}, {Kind: L, Reason: why}, {Kind: Z, Reason: why}}}
		id := ""
		if pg != nil && e >= 0 && e < len(pg.Edges) {
			id = pg.Edges[e].ID
		}
		unrouted(ctx, id, res)
		return res
	}
	var res Result
	refused := 0 // the positions refused so far, over every candidate
	for _, k := range []Kind{Straight, StraightAlong, L, Z} {
		tried, first := 0, ""
		var kept *candidate
		try := func(c candidate) bool {
			if r := s.check(c); r != "" {
				tried++
				if first == "" {
					first = r
				}
				return false
			}
			kept = &c
			return true
		}
		var why string
		switch k {
		case Straight:
			why = s.straight(try)
		case StraightAlong:
			why = s.along(try)
		case L:
			why = s.l(try)
		default:
			why = s.z(try)
		}
		refused += tried
		if kept != nil {
			res.Kind, res.Points, res.Skipped = k, kept.pts, tried
			layoutdbg.Decision(ctx, "flat_edge_routed",
				"phase", "flat", "module", "diago", "spec_ref", "S9",
				"edge", s.id, "kind", k.String(), "refused", refused)
			return res
		}
		if tried == 0 {
			first = why
		}
		res.Refused = append(res.Refused, Refusal{Kind: k, Tried: tried, Reason: first})
	}
	unrouted(ctx, s.id, res)
	return res
}

// unrouted records a flat edge no candidate routes, with each candidate's
// reason and how many positions it tried.
func unrouted(ctx context.Context, id string, res Result) {
	args := []any{"phase", "flat", "module", "diago", "spec_ref", "S9", "edge", id}
	for _, r := range res.Refused {
		args = append(args, r.Kind.String(), r.Reason, r.Kind.String()+"_tried", r.Tried)
	}
	layoutdbg.Decision(ctx, "flat_edge_unrouted", args...)
}
