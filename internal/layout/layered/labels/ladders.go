package labels

import (
	"iter"
	"math"

	"github.com/oxforge/diago/internal/model"
)

// labelSpots is an edge label's ladder (S10): beside a side exit's corner
// when the edge leaves a diamond's or circle's side vertex; then beside
// each vertical run, at its middle, right before left, and near its two
// ends, in rings one gap farther out each (out: in the text profile one
// row farther each across a wire along the label's row), where in the
// text profile a ring also tries the spots beside each run one gap along
// it at a time from its middle; then a self-loop's mouth; last, the
// slide, whose spots are made only as pick draws on them. In the text
// profile a spot rounds away from its wire across it, away from a run's
// end it sits near, and down otherwise.
func (p *placer) labelSpots(e Edge) iter.Seq[spot] {
	w, h, gap := e.Label.W, e.Label.H, p.o.Gap
	pts := e.Points
	var spots []spot
	if !e.Loop && p.pinned[e.Source] {
		src := p.nodes[e.Source]
		p0, corner := pts[0], pts[1]
		atMid := math.Abs(p0.Y-(src.Y+src.H/2)) < 1e-6
		atVertex := math.Abs(p0.X-src.X) < 1e-6 || math.Abs(p0.X-(src.X+src.W)) < 1e-6
		if atMid && atVertex && math.Abs(corner.Y-p0.Y) < 1e-6 {
			out, in := corner.X+gap, corner.X-gap-w
			outRight := corner.X >= p0.X
			if !outRight {
				out, in = corner.X-gap-w, corner.X+gap
			}
			above, below := p0.Y-gap-h, p0.Y+gap
			spots = append(spots,
				spot{Rect{out, above, w, h}, outRight, false, 0}, spot{Rect{out, below, w, h}, outRight, true, 0},
				spot{Rect{in, above, w, h}, !outRight, false, 0}, spot{Rect{in, below, w, h}, !outRight, true, 0})
		}
	}
	type run struct{ x, y1, y2 float64 }
	var runs []run
	for k := 0; k+1 < len(pts); k++ {
		a, b := pts[k], pts[k+1]
		if a.X == b.X && math.Abs(b.Y-a.Y) >= h {
			runs = append(runs, run{a.X, math.Min(a.Y, b.Y), math.Max(a.Y, b.Y)})
		}
	}
	if len(runs) == 0 {
		a, b := pts[0], pts[len(pts)-1]
		y := (a.Y + b.Y) / 2
		runs = append(runs, run{(a.X + b.X) / 2, y, y})
	}
	rows := p.sideways() // a vertical run runs along a label's row under RIGHT and LEFT
	beside := func(x, y float64, ring int, upY bool) {
		spots = append(spots, spot{Rect{p.out(x, ring, gap, w, true, rows), y, w, h}, true, upY, ring},
			spot{Rect{p.out(x, ring, gap, w, false, rows), y, w, h}, false, upY, ring})
	}
	for ring := 1; ring <= 4; ring++ {
		for _, r := range runs {
			beside(r.x, (r.y1+r.y2)/2-h/2, ring, false)
		}
		for _, r := range runs {
			if r.y2-r.y1 < h+2*gap {
				continue
			}
			beside(r.x, r.y1+gap, ring, true)
			beside(r.x, r.y2-gap-h, ring, false)
		}
		if !p.o.Text {
			continue
		}
		// in the text profile the ring also slides along each run, a cell
		// at a time from its middle, up before down, while the box still
		// reaches the run
		for _, r := range runs {
			mid, reach := (r.y1+r.y2)/2, (r.y2-r.y1)/2+h/2
			for i := 1.0; gap > 0 && i*gap <= reach+noise; i++ {
				beside(r.x, mid-i*gap-h/2, ring, false)
				beside(r.x, mid+i*gap-h/2, ring, false)
			}
		}
	}
	if e.Loop {
		box := p.nodes[e.Source]
		face, leg := box.X+box.W, pts[0].X
		for _, q := range pts {
			leg = math.Max(leg, q.X)
		}
		top, bottom := pts[0].Y, pts[len(pts)-1].Y
		spots = append(spots, spot{Rect: Rect{(face+leg)/2 - w/2, (top+bottom)/2 - h/2, w, h}, ring: 5})
	}
	return func(yield func(spot) bool) {
		for _, s := range spots {
			if !yield(s) {
				return
			}
		}
		p.slide(e, yield)
	}
}

// slide is the end of an edge label's ladder (S10): beside every segment
// of its route, in route order, right of a vertical one before left and
// above a horizontal one before below, centered on the segment's middle
// and then one gap farther toward its ends at a time, up or left before
// down or right, while the box still reaches the segment along it, in
// eight rings one gap farther out each (out, as for the runs), ring by
// ring. It serves a label
// longer than its runs, which overhangs a run's end past a turn, and one
// hemmed in beside its runs. In the text profile a spot rounds away from
// its segment across it and down along it. It hands each spot to yield and
// stops, reporting false, when yield does.
func (p *placer) slide(e Edge, yield func(spot) bool) bool {
	w, h, gap := e.Label.W, e.Label.H, p.o.Gap
	pts := e.Points
	for ring := 1; ring <= 8; ring++ {
		for k := 0; k+1 < len(pts); k++ {
			a, b := pts[k], pts[k+1]
			vertical := a.X == b.X
			lo, hi, along := math.Min(a.X, b.X), math.Max(a.X, b.X), w
			if vertical {
				lo, hi, along = math.Min(a.Y, b.Y), math.Max(a.Y, b.Y), h
			}
			at := func(c float64) bool {
				if vertical {
					rows := p.sideways()
					return yield(spot{Rect{p.out(a.X, ring, gap, w, true, rows), c - h/2, w, h}, true, false, slideRing + ring}) &&
						yield(spot{Rect{p.out(a.X, ring, gap, w, false, rows), c - h/2, w, h}, false, false, slideRing + ring})
				}
				rows := !p.sideways()
				return yield(spot{Rect{c - w/2, p.out(a.Y, ring, gap, h, false, rows), w, h}, false, false, slideRing + ring}) &&
					yield(spot{Rect{c - w/2, p.out(a.Y, ring, gap, h, true, rows), w, h}, false, true, slideRing + ring})
			}
			mid, reach := (lo+hi)/2, (hi-lo)/2+along/2
			if !at(mid) {
				return false
			}
			for i := 1; gap > 0 && float64(i)*gap <= reach+noise; i++ {
				if !at(mid-float64(i)*gap) || !at(mid+float64(i)*gap) {
					return false
				}
			}
		}
	}
	return true
}

// cardSpots is a cardinality's ladder at the end of the wire route
// starts at, route running on from that end (S10): spots one, two and
// three steps along the wire, beside it (right of a vertical wire before
// left, above a horizontal one before below), in four rings one step
// farther out each; then the same at every step along the route,
// following its turns up to its middle, each beside the segment it lies
// on. In the text profile a spot rounds away from the wire across it and
// toward the end, back along the route, along it.
func (p *placer) cardSpots(route []model.Point, s Size) []spot {
	var spots []spot
	step := p.o.CardStep
	for ring := 1; ring <= 4; ring++ {
		for k := 1; k <= 3; k++ {
			spots = append(spots, p.cardBeside(route[0], route[1], float64(k)*step, ring, s)...)
		}
	}
	half := 0.0
	for k := 0; k+1 < len(route); k++ {
		half += (math.Abs(route[k+1].X-route[k].X) + math.Abs(route[k+1].Y-route[k].Y)) / 2
	}
	for ring := 1; ring <= 4; ring++ {
		for i := 1; step > 0 && float64(i)*step <= half+noise; i++ {
			t := float64(i) * step
			for k := 0; k+1 < len(route); k++ {
				a, b := route[k], route[k+1]
				n := math.Abs(b.X-a.X) + math.Abs(b.Y-a.Y)
				if t > n && k+2 < len(route) {
					t -= n
					continue
				}
				spots = append(spots, p.cardBeside(a, b, t, slideRing+ring, s)...)
				break
			}
		}
	}
	return spots
}

// cardBeside is the pair of spots for a cardinality of size s centered o
// along the wire from a toward b and in ring ring across it, ring steps
// out (in the text profile across a wire along its row, the rows from the
// wire's: out): right of a vertical wire before left, above a horizontal
// one before below. In the text profile each rounds away from the wire
// across it and toward a along it. A ring past slideRing counts its steps
// from slideRing.
func (p *placer) cardBeside(a, b model.Point, o float64, ring int, s Size) []spot {
	step, n := p.o.CardStep, ring%slideRing
	if a.X == b.X {
		y, toward, rows := a.Y+sgn(b.Y-a.Y)*o, b.Y < a.Y, p.sideways()
		return []spot{{Rect{p.out(a.X, n, step, s.W, true, rows), y - s.H/2, s.W, s.H}, true, toward, ring},
			{Rect{p.out(a.X, n, step, s.W, false, rows), y - s.H/2, s.W, s.H}, false, toward, ring}}
	}
	x, toward, rows := a.X+sgn(b.X-a.X)*o, b.X < a.X, !p.sideways()
	return []spot{{Rect{x - s.W/2, p.out(a.Y, n, step, s.H, false, rows), s.W, s.H}, toward, false, ring},
		{Rect{x - s.W/2, p.out(a.Y, n, step, s.H, true, rows), s.W, s.H}, toward, true, ring}}
}

// slideRing numbers the rings of a ladder's last part, the slide and a
// cardinality's spots along its route, apart from its first: ring
// slideRing + n is the slide's ring n (S10).
const slideRing = 10
