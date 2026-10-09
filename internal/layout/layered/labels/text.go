package labels

import (
	"math"

	"github.com/oxforge/diago/internal/model"
)

// sideways reports whether a label's row runs along the engine's y, as
// under RIGHT and LEFT, where the frame turns y into the output's x.
func (p *placer) sideways() bool { return p.o.Dir == model.Right || p.o.Dir == model.Left }

// cell is the whole cell a coordinate v of the engine's frame is drawn in
// along an axis: the cell after v, or, on an axis the frame turns over
// (the engine's y under UP and LEFT), the cell before it, since the text
// renderer draws a point on a cell boundary in the cell right of it or
// below it in the output frame.
func cell(v float64, turned bool) float64 {
	if turned {
		return math.Ceil(v-noise) - 1
	}
	return math.Floor(v + noise)
}

// drawCells notes, for the text profile, the cells of every group frame's
// sides, those along a label's row apart from those that cross it, and of
// every wire segment (S10).
func (p *placer) drawCells(groups []Rect) {
	for _, g := range groups {
		along := []Rect{{g.X, g.Y, g.W, 1}, {g.X, g.Y + g.H - 1, g.W, 1}}
		cross := []Rect{{g.X, g.Y, 1, g.H}, {g.X + g.W - 1, g.Y, 1, g.H}}
		if p.sideways() {
			along, cross = cross, along
		}
		p.along = append(p.along, along...)
		p.cross = append(p.cross, cross...)
	}
	turned := p.o.Dir == model.Up || p.o.Dir == model.Left
	for _, e := range p.edges {
		for k := 0; k+1 < len(e.Points); k++ {
			a, b := e.Points[k], e.Points[k+1]
			x0, x1 := cell(math.Min(a.X, b.X), false), cell(math.Max(a.X, b.X), false)
			y0, y1 := cell(math.Min(a.Y, b.Y), turned), cell(math.Max(a.Y, b.Y), turned)
			p.drawn = append(p.drawn, Rect{x0, y0, x1 - x0 + 1, y1 - y0 + 1})
		}
	}
}

// slack is box s grown by the blank a text label keeps before and after
// it along its row, Pad (zero on screen).
func (p *placer) slack(s Rect) Rect {
	if p.sideways() {
		return Rect{s.X, s.Y - p.o.Pad, s.W, s.H + 2*p.o.Pad}
	}
	return Rect{s.X - p.o.Pad, s.Y, s.W + 2*p.o.Pad, s.H}
}

// ink counts, for spot s in the text profile, the cells the text
// renderer draws where s needs blanks (S10): in s and in the cell before
// and after it along its row, the cells of node boxes, group frames' sides
// along its row (their corners included), placed labels and wires, its own
// included; of a side that crosses its row, only a cell where s needs a
// blank, the cell before or after it, since s may cover the side itself
// (border).
func (p *placer) ink(s Rect) float64 {
	g := p.slack(s)
	c := 0.0
	for _, i := range p.nodeGrid.nearBox(g) {
		c += area(g, p.nodes[i])
	}
	for _, r := range p.along {
		c += area(g, r)
	}
	for _, r := range p.cross {
		c += area(g, r) - area(s, r)
	}
	for _, i := range p.placedGrid.nearBox(g) {
		c += area(g, p.placed[i])
	}
	for _, i := range p.drawnGrid.nearBox(g) {
		c += area(g, p.drawn[i])
	}
	return c
}

// border is how many cells of the group frames' sides that cross its row
// spot s covers in the text profile, zero on screen: a spot clear but for
// them is taken only when no spot of its ring is clear outright (S10).
func (p *placer) border(s Rect) float64 {
	c := 0.0
	for _, r := range p.cross {
		c += area(s, r)
	}
	return c
}

// away is how much of spot s, with the blank before and after it along its
// row, lies outside the frame of edge e's home group in the text profile:
// zero when the box and its blanks lie inside the frame, when e has no
// home group or the ladder runs without it (homeless), and always on
// screen (S10).
func (p *placer) away(s Rect, e int) float64 {
	h := p.edges[e].Home
	if !p.o.Text || h == nil || p.homeless {
		return 0
	}
	g := p.slack(s)
	return g.W*g.H - area(g, Rect{h.X + 1, h.Y + 1, h.W - 2, h.H - 2})
}

// snap rounds box r to whole cells in the text profile, each coordinate up
// or down as spot s says; r itself on screen.
func (p *placer) snap(r Rect, s spot) Rect {
	if !p.o.Text {
		return r
	}
	round := func(v float64, up bool) float64 {
		if up {
			return math.Ceil(v - noise)
		}
		return math.Floor(v + noise)
	}
	return Rect{round(r.X, s.upX), round(r.Y, s.upY), r.W, r.H}
}
