package labels

import (
	"math"

	"github.com/oxforge/diago/internal/model"
)

// spotCost is spot s's overlap for the boxes of the edges all, e's and its
// copies', placed there together (boxAt): each box's own cost, and the
// area each two of the boxes share, the first grown by the blank a text
// label keeps along its row (slack), since they must clear each other
// too. A box's cost stops adding terms once the sum reaches bound, where
// the spot cannot win (cost).
func (p *placer) spotCost(s spot, e int, all []int, size func(Edge) Size, bound float64) float64 {
	c := 0.0
	for j, k := range all {
		c += p.cost(p.boxAt(s, e, k, size), k, c, bound)
		for _, l := range all[j+1:] {
			c += area(p.slack(p.boxAt(s, e, k, size)), p.boxAt(s, e, l, size))
		}
	}
	return c
}

// cost is how much spot s of edge e overlaps: the area it shares with node
// boxes, envelopes, placed labels and its own wire grown by the gap, plus
// the length of other edges' wires inside it, plus how much nearer than
// its own wire another edge's lies to it, beyond the slack. Zero is clear.
// Every term is non-negative, so once base, what the spot has cost so far,
// plus the terms summed here reaches bound, the least cost found, the spot
// cannot win: cost returns what it has summed, and the rest is skipped. It
// visits only the boxes near s (grid), and adds the terms in the order of
// the full scan, which leaves every sum as that scan gives it.
func (p *placer) cost(s Rect, e int, base, bound float64) float64 {
	c := 0.0
	for _, i := range p.nodeGrid.nearBox(s) {
		c += area(s, p.nodes[i])
	}
	for _, v := range p.envs {
		c += area(s, v)
	}
	for _, i := range p.placedGrid.nearBox(s) {
		c += area(s, p.placed[i])
	}
	if base+c >= bound {
		return c
	}
	lo, hi := p.first[e], p.first[e+1]
	near := p.segGrid.nearBox(s)
	j := 0
	for ; j < len(near) && near[j] < lo; j++ {
		c += through(s, p.segs[near[j]])
	}
	for _, g := range p.segs[lo:hi] {
		c += area(s, p.grow(g))
	}
	for ; j < len(near); j++ {
		if near[j] >= hi {
			c += through(s, p.segs[near[j]])
		}
	}
	if base+c >= bound {
		return c
	}
	if p.o.Text {
		c += p.ink(s) + p.away(s, e)
		if base+c >= bound {
			return c
		}
	}
	return c + p.misread(s, e)
}

// grow is segment g of a label's own wire grown by the label gap, the
// room the label keeps from it (C14.3): on every side on screen; in the
// text profile only along the label's row, since across a wire that runs
// along its row a label may take the next row, clear of the cells the
// wire is drawn in, which ink tests (S10).
func (p *placer) grow(g Rect) Rect {
	gap := p.o.Gap
	switch {
	case !p.o.Text:
		return Rect{g.X - gap, g.Y - gap, g.W + 2*gap, g.H + 2*gap}
	case p.sideways():
		return Rect{g.X, g.Y - gap, g.W, g.H + 2*gap}
	}
	return Rect{g.X - gap, g.Y, g.W + 2*gap, g.H}
}

// out is the low edge, on one axis of the engine's frame, of a box of
// extent ext in ring ring beside a wire at v on that axis: ring steps out,
// past the wire (high) or before it. In the text profile, across a wire
// that runs along a label's row in the output frame (rows), the rings
// count whole rows from the cell the wire is drawn in instead, the first
// the row next to it (S10, S14).
func (p *placer) out(v float64, ring int, step, ext float64, high, rows bool) float64 {
	if p.o.Text && rows {
		c := cell(v, p.o.Dir == model.Up)
		if high {
			return c + float64(ring)
		}
		return c + 1 - float64(ring) - ext
	}
	if high {
		return v + float64(ring)*step
	}
	return v - float64(ring)*step - ext
}

// misread is how much nearer than edge e's own wire another edge's wire
// lies to box s, beyond the slack (S10): zero when a reader takes the box
// for e's. A box's distance from a wire is the least between the box and
// a segment of the wire. Only another edge's wire nearer to s than its
// own, by more than the slack, counts: misread measures only the segments
// within the own wire's distance of s, and that grown by a negative slack
// (grid), and skips one farther along an axis than the nearest found.
func (p *placer) misread(s Rect, e int) float64 {
	lo, hi := p.first[e], p.first[e+1]
	own, other := math.Inf(1), math.Inf(1)
	for _, g := range p.segs[lo:hi] {
		own = min(own, apart(s, g))
	}
	r := own + max(0, -p.o.OwnSlack)
	for _, i := range p.segGrid.touching(s.X-r, s.Y-r, s.X+s.W+r, s.Y+s.H+r) {
		if i >= lo && i < hi {
			continue
		}
		// a segment lies at least as far as it lies along each axis
		g := p.segs[i]
		if dx, dy := max(s.X-(g.X+g.W), g.X-(s.X+s.W)), max(s.Y-(g.Y+g.H), g.Y-(s.Y+s.H)); dx < other && dy < other {
			other = min(other, apart(s, g))
		}
	}
	if x := own - other - p.o.OwnSlack; x > noise {
		return x
	}
	return 0
}

// apart is the distance between two boxes, zero when they touch or
// overlap.
func apart(a, b Rect) float64 {
	dx := max(max(a.X-(b.X+b.W), b.X-(a.X+a.W)), 0)
	dy := max(max(a.Y-(b.Y+b.H), b.Y-(a.Y+a.H)), 0)
	return math.Hypot(dx, dy)
}

// area is the area two boxes share.
func area(a, b Rect) float64 {
	w := min(a.X+a.W, b.X+b.W) - max(a.X, b.X)
	h := min(a.Y+a.H, b.Y+b.H) - max(a.Y, b.Y)
	if w <= 0 || h <= 0 {
		return 0
	}
	return w * h
}

// through is the length of the axis-aligned segment seg (a box with one
// zero extent) strictly inside box a.
func through(a, seg Rect) float64 {
	if seg.W == 0 {
		if seg.X <= a.X || seg.X >= a.X+a.W {
			return 0
		}
		return max(0, min(a.Y+a.H, seg.Y+seg.H)-max(a.Y, seg.Y))
	}
	if seg.Y <= a.Y || seg.Y >= a.Y+a.H {
		return 0
	}
	return max(0, min(a.X+a.W, seg.X+seg.W)-max(a.X, seg.X))
}
