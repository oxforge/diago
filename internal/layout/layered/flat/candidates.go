package flat

import (
	"math"

	"github.com/oxforge/diago/internal/layout/layered/ports"
	"github.com/oxforge/diago/internal/model"
)

// candidate is a route tried for the flat edge: its points from A to B,
// and the face each end lies on (a pinned node's vertex).
type candidate struct {
	pts      []model.Point
	from, to ports.Face
}

// The candidates below each call try with their positions in order,
// until try keeps one (returns true), and return why the candidate does
// not apply when they call it with none (S9, Flat edges).

// facing returns A's and B's facing side faces when their boxes are
// disjoint on x: the right face of the one on the left, the left face of
// the other.
func (s *scene) facing() (fa, fb ports.Face, ok bool) {
	a, b := s.box(s.a), s.box(s.b)
	switch {
	case a.r <= b.l+noise:
		return ports.FaceRight, ports.FaceLeft, true
	case b.r <= a.l+noise:
		return ports.FaceLeft, ports.FaceRight, true
	}
	return 0, 0, false
}

// straight tries one run between A's and B's facing side faces at each y
// their side ports' ranges share, from the middle of the shorter range
// (A's on a tie), moved into the other when it lies outside.
func (s *scene) straight(try func(candidate) bool) string {
	fa, fb, ok := s.facing()
	if !ok {
		return "not side by side"
	}
	aLo, aHi := s.sideRange(s.a, fa)
	bLo, bHi := s.sideRange(s.b, fb)
	start := (aLo + aHi) / 2
	if bHi-bLo < aHi-aLo {
		start = (bLo + bHi) / 2
	}
	ys := ladder(max(aLo, bLo), min(aHi, bHi), start, s.o.TrackGap.Y, s.o.Text)
	if len(ys) == 0 {
		return "no side port on both"
	}
	for _, y := range ys {
		if try(candidate{pts: []model.Point{s.sidePoint(s.a, fa, y), s.sidePoint(s.b, fb, y)}, from: fa, to: fb}) {
			break
		}
	}
	return ""
}

// along tries one run along the flow between two stacked nodes' facing
// faces, the upper one's bottom and the lower one's top, at each x their
// columns' ranges share, from the middle of the shorter range (A's on a
// tie), moved into the other when it lies outside. A diamond or a circle
// joins it at its top or bottom vertex.
func (s *scene) along(try func(candidate) bool) string {
	a, b := s.box(s.a), s.box(s.b)
	var fa, fb ports.Face
	switch {
	case a.b <= b.t+noise: // B lies below A
		fa, fb = ports.FaceOut, ports.FaceIn
	case b.b <= a.t+noise:
		fa, fb = ports.FaceIn, ports.FaceOut
	default:
		return "not stacked"
	}
	aLo, aHi := s.faceRange(s.a, fa)
	bLo, bHi := s.faceRange(s.b, fb)
	start := (aLo + aHi) / 2
	if bHi-bLo < aHi-aLo {
		start = (bLo + bHi) / 2
	}
	xs := ladder(max(aLo, bLo), min(aHi, bHi), start, s.o.TrackGap.X, s.o.Text)
	if len(xs) == 0 {
		return "no column on both"
	}
	for _, x := range xs {
		if try(candidate{pts: []model.Point{s.facePoint(s.a, fa, x), s.facePoint(s.b, fb, x)}, from: fa, to: fb}) {
			break
		}
	}
	return ""
}

// z tries a route out of A's facing side face, along one cross run in the
// gap between the two boxes, into B's facing side face: A's side port,
// then, for each, B's, then, for each pair at least the node clearance
// apart on the flow axis (a nearer pair's cross run would be a micro-jog,
// Q2), the cross run's x over the gap less the stub beside each box.
func (s *scene) z(try func(candidate) bool) string {
	fa, fb, ok := s.facing()
	if !ok {
		return "not side by side"
	}
	a, b := s.box(s.a), s.box(s.b)
	lo, hi := a.r+s.o.Stub.X, b.l-s.o.Stub.X
	if fa == ports.FaceLeft {
		lo, hi = b.r+s.o.Stub.X, a.l-s.o.Stub.X
	}
	xs := ladder(lo, hi, (lo+hi)/2, s.o.TrackGap.X, s.o.Text)
	if len(xs) == 0 {
		return "no room for the cross run"
	}
	aLo, aHi := s.sideRange(s.a, fa)
	bLo, bHi := s.sideRange(s.b, fb)
	yielded := false
	for _, ya := range ladder(aLo, aHi, (aLo+aHi)/2, s.o.TrackGap.Y, s.o.Text) {
		for _, yb := range ladder(bLo, bHi, (bLo+bHi)/2, s.o.TrackGap.Y, s.o.Text) {
			if math.Abs(yb-ya) < s.o.Clearance.Y-noise {
				continue
			}
			for _, x := range xs {
				yielded = true
				pts := []model.Point{s.sidePoint(s.a, fa, ya), {X: x, Y: ya}, {X: x, Y: yb}, s.sidePoint(s.b, fb, yb)}
				if try(candidate{pts: pts, from: fa, to: fb}) {
					return ""
				}
			}
		}
	}
	if !yielded {
		return "no side ports a clearance apart"
	}
	return ""
}

// l tries a route between a side face of one node and the facing face of
// the other (the lower one's top, the upper one's bottom) when their
// boxes are disjoint on y: first out of A's side face into B's facing
// face, then out of B's into A's, the route always from A to B. Of the
// side node's two faces, the one toward the middle of the other's facing
// face comes first (the right one on a tie); on each, its side port, then,
// for each, the column on the facing face, at least the stub beyond the
// side node's box.
func (s *scene) l(try func(candidate) bool) string {
	a, b := s.box(s.a), s.box(s.b)
	below := a.b <= b.t+noise // B lies below A
	if !below && b.b > a.t+noise {
		return "not stacked"
	}
	yielded := false
	for _, fromA := range []bool{true, false} {
		sn, tn := s.a, s.b // the side node and the node whose facing face the column ends on
		if !fromA {
			sn, tn = s.b, s.a
		}
		tf := ports.FaceOut
		if below == fromA { // tn lies below sn
			tf = ports.FaceIn
		}
		cLo, cHi := s.faceRange(tn, tf)
		mid := (cLo + cHi) / 2
		cols := ladder(cLo, cHi, mid, s.o.TrackGap.X, s.o.Text)
		sb := s.box(sn)
		faces := []ports.Face{ports.FaceRight, ports.FaceLeft}
		if mid < s.pg.Nodes[sn].X-noise {
			faces[0], faces[1] = faces[1], faces[0]
		}
		for _, sf := range faces {
			yLo, yHi := s.sideRange(sn, sf)
			for _, y := range ladder(yLo, yHi, (yLo+yHi)/2, s.o.TrackGap.Y, s.o.Text) {
				for _, x := range cols {
					if (sf == ports.FaceRight && x < sb.r+s.o.Stub.X-noise) || (sf == ports.FaceLeft && x > sb.l-s.o.Stub.X+noise) {
						continue
					}
					yielded = true
					side, corner, end := s.sidePoint(sn, sf, y), model.Point{X: x, Y: y}, s.facePoint(tn, tf, x)
					c := candidate{pts: []model.Point{side, corner, end}, from: sf, to: tf}
					if !fromA {
						c = candidate{pts: []model.Point{end, corner, side}, from: tf, to: sf}
					}
					if try(c) {
						return ""
					}
				}
			}
		}
	}
	if !yielded {
		return "no column beyond the side node"
	}
	return ""
}

// ladder lists the positions of the range [lo, hi] from start outward by
// step, the lower before the higher at each step (S9, Flat edges): so of
// two positions as far from the start, the upper or the left one comes
// first. On cells (the text profile) the positions are the middles of the
// whole cells inside the range, from the middle of the cell start falls
// in, and step is whole cells. A start outside the range moves onto its
// nearest end. An empty range has no positions.
func ladder(lo, hi, start, step float64, cells bool) []float64 {
	if cells {
		lo, hi, start = math.Ceil(lo-0.5)+0.5, math.Floor(hi-0.5)+0.5, math.Floor(start)+0.5
	}
	if hi < lo-noise {
		return nil
	}
	start = min(max(start, lo), hi)
	out := []float64{start}
	if step <= 0 {
		return out
	}
	for k := 1; ; k++ {
		d := float64(k) * step
		in := false
		if start-d >= lo-noise {
			out = append(out, start-d)
			in = true
		}
		if start+d <= hi+noise {
			out = append(out, start+d)
			in = true
		}
		if !in {
			return out
		}
	}
}
