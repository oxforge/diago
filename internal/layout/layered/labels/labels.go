// Package labels places edge labels and cardinalities (S10) in the
// engine's top-to-bottom frame, clear of every node box, every adornment
// envelope, the labels placed before them and the other edges' wires, at
// least the label gap from their own wire and nearer to it than to any
// other edge's, within a slack. In the text profile a
// label's box is the whole cells the text renderer draws it in, clear of
// every cell drawn there.
package labels

import (
	"context"
	"iter"
	"math"
	"slices"

	"github.com/oxforge/diago/internal/layoutdbg"
	"github.com/oxforge/diago/internal/model"
)

// Options carries S10's values from the Config (S14).
type Options struct {
	Gap      float64 // the label gap: a label's distance from its own wire (C14.3)
	CardStep float64 // how far a cardinality's spot steps along its wire from the end
	EnvDepth float64 // an adornment envelope's depth along the first segment (C14)
	EnvHalf  float64 // an adornment envelope's half-width across it
	OwnSlack float64 // how much nearer than a label's own wire another edge's wire may lie to it
	Pad      float64 // the blank a text label keeps before and after it along its row
	// Text is the text profile, where a spot snaps to whole cells and is
	// tested against the cells the text renderer draws (S10).
	Text bool
	// Dir is the direction the frame turns the layout to (S11): it decides
	// the cell a wire on a cell boundary is drawn in and which of the
	// engine's axes a text label's row runs along.
	Dir model.Direction
}

// Rect is a box: its top-left corner and its extents.
type Rect struct{ X, Y, W, H float64 }

// Size is a label's extents; a zero W means there is none.
type Size struct{ W, H float64 }

// Edge is one edge's route, from its From node, and what it carries.
type Edge struct {
	ID       string
	Points   []model.Point
	Label    Size
	From, To Size  // the cardinalities at the From and the To end
	Adorned  bool  // an adornment sits at its first point (C14)
	Source   int   // its From node
	Loop     bool  // a self-loop
	Copies   []int // the edges whose label and cardinalities are this edge's, moved by the offset of their first route point from this edge's (C17.1, S12)
	// Home is the box of the innermost group holding both its ends, nil
	// for none: in the text profile its label and cardinalities stay
	// inside that group's frame (S10).
	Home *Rect
}

// Placed is where one edge's label and cardinalities went: nil when the
// edge has none. An unresolved one found no clear spot and took the one
// with the least overlap (C14.4).
type Placed struct {
	Label, From, To                               *Rect
	LabelUnresolved, FromUnresolved, ToUnresolved bool
}

// spot is a candidate box on a ladder, the ring of the ladder it lies in,
// and, for the text profile, the way each of its coordinates rounds to a
// whole cell: up when upX or upY, else down (S10).
type spot struct {
	Rect
	upX, upY bool
	ring     int
}

type placer struct {
	o      Options
	nodes  []Rect
	pinned []bool
	edges  []Edge
	envs   []Rect
	placed []Rect
	along  []Rect // the cells of the group frames' sides that run along a label's row, in the text profile
	cross  []Rect // the cells of the group frames' sides that cross a label's row, in the text profile
	segs   []Rect // every edge's segments, edge by edge in route order, each a box with a zero extent
	first  []int  // per edge, its first segment in segs, and one more entry: the end
	drawn  []Rect // per segment, the cells it is drawn in, in the text profile
	// homeless runs a ladder again without the home groups, none of
	// whose spots was clear inside them (S10)
	homeless bool
	// the grids of the node boxes, the placed labels, the segments and
	// their cells, which spare a spot's cost the boxes far from it
	nodeGrid, placedGrid, segGrid, drawnGrid *grid
}

// Place places every cardinality, then every edge label, in edge order
// (S10). nodes are the node boxes, pinned marks diamonds and circles,
// groups are the group boxes, whose frames the text renderer draws.
func Place(ctx context.Context, nodes []Rect, pinned []bool, groups []Rect, edges []Edge, o Options) []Placed {
	return placeWith(ctx, nodes, pinned, groups, edges, o, true)
}

// placeWith is Place, its grids of one cell each unless gridded: a full scan
// of every box, which places every label as the grids do.
func placeWith(ctx context.Context, nodes []Rect, pinned []bool, groups []Rect, edges []Edge, o Options, gridded bool) []Placed {
	p := &placer{o: o, nodes: nodes, pinned: pinned, edges: edges}
	for _, e := range edges {
		if env, ok := p.envelope(e); ok {
			p.envs = append(p.envs, env)
		}
	}
	p.first = make([]int, len(edges)+1)
	for i, e := range edges {
		p.first[i] = len(p.segs)
		for k := 0; k+1 < len(e.Points); k++ {
			a, b := e.Points[k], e.Points[k+1]
			p.segs = append(p.segs, Rect{min(a.X, b.X), min(a.Y, b.Y), math.Abs(b.X - a.X), math.Abs(b.Y - a.Y)})
		}
	}
	p.first[len(edges)] = len(p.segs)
	if o.Text {
		p.drawCells(groups)
	}
	p.index(gridded)
	out := make([]Placed, len(edges))
	copied := make([]bool, len(edges))
	for _, e := range edges {
		for _, k := range e.Copies {
			copied[k] = true
		}
	}
	for i, e := range edges {
		if len(e.Points) < 2 || copied[i] {
			continue
		}
		if e.From.W > 0 {
			rects, unresolved := p.pick(ctx, i, "from_card", slices.Values(p.cardSpots(e.Points, e.From)), func(e Edge) Size { return e.From })
			for k, r := range rects {
				out[k].From, out[k].FromUnresolved = r, unresolved
			}
		}
		if e.To.W > 0 {
			back := slices.Clone(e.Points)
			slices.Reverse(back)
			rects, unresolved := p.pick(ctx, i, "to_card", slices.Values(p.cardSpots(back, e.To)), func(e Edge) Size { return e.To })
			for k, r := range rects {
				out[k].To, out[k].ToUnresolved = r, unresolved
			}
		}
	}
	for i, e := range edges {
		if len(e.Points) < 2 || e.Label.W <= 0 || copied[i] {
			continue
		}
		rects, unresolved := p.pick(ctx, i, "label", p.labelSpots(e), func(e Edge) Size { return e.Label })
		for k, r := range rects {
			out[k].Label, out[k].LabelUnresolved = r, unresolved
		}
	}
	return out
}

// envelope is the box an adornment reserves at edge e's first point: the
// bounding box of p0 ± EnvHalf·n and p0 + EnvDepth·d ± EnvHalf·n (C14).
func (p *placer) envelope(e Edge) (Rect, bool) {
	if !e.Adorned || len(e.Points) < 2 {
		return Rect{}, false
	}
	a, b := e.Points[0], e.Points[1]
	dx, dy := sgn(b.X-a.X), sgn(b.Y-a.Y)
	x0, x1 := a.X-math.Abs(dy)*p.o.EnvHalf, a.X+math.Abs(dy)*p.o.EnvHalf
	y0, y1 := a.Y-math.Abs(dx)*p.o.EnvHalf, a.Y+math.Abs(dx)*p.o.EnvHalf
	ex, ey := a.X+dx*p.o.EnvDepth, a.Y+dy*p.o.EnvDepth
	x0, x1 = math.Min(x0, ex), math.Max(x1, ex)
	y0, y1 = math.Min(y0, ey), math.Max(y1, ey)
	return Rect{x0, y0, x1 - x0, y1 - y0}, true
}

// pick takes the first spot clear of everything, or else, in the text
// profile, the first of its ring clear but for the frame sides it covers
// (border) once the ring holds none clear outright, or else the one with
// the least overlap, marked unresolved; when that one lies outside the
// home groups (S10), the ladder runs again without them. It registers the
// spot: for edge e and, at
// its spot moved as their first route points are, for every copy of e
// (S12), each at its own size, the spot's overlap summed over them all
// and over the area each two of their boxes share, since they must clear
// each other too. In the text profile each box snaps to whole cells as
// its spot does, a copy's once centered, and two boxes must also keep
// their blank between them. It draws the spots from the ladder
// in order and stops at the first clear one, and it returns each edge's
// box by edge index.
func (p *placer) pick(ctx context.Context, e int, what string, spots iter.Seq[spot], size func(Edge) Size) (map[int]*Rect, bool) {
	at := func(s spot, k int) Rect {
		lead := p.snap(s.Rect, s)
		if k == e {
			return lead
		}
		o, c := p.edges[e].Points[0], p.edges[k].Points[0]
		z := size(p.edges[k])
		cx, cy := lead.X+lead.W/2+c.X-o.X, lead.Y+lead.H/2+c.Y-o.Y
		return p.snap(Rect{X: cx - z.W/2, Y: cy - z.H/2, W: z.W, H: z.H}, s)
	}
	all := []int{e}
	for _, k := range p.edges[e].Copies {
		if len(p.edges[k].Points) >= 2 && size(p.edges[k]).W > 0 {
			all = append(all, k)
		}
	}
	var best spot
	var onBorder *spot // the first spot of the ring clear but for the frame sides it covers
	var tried bool
	var bestCost float64
	climb := func() {
		best, onBorder, tried, bestCost = spot{}, nil, false, math.Inf(1)
		ring := 0
		for s := range spots {
			if onBorder != nil && s.ring != ring {
				break
			}
			ring = s.ring
			if !tried {
				best, tried = s, true
			}
			c := 0.0
			for j, k := range all {
				c += p.cost(at(s, k), k, c, bestCost)
				for _, l := range all[j+1:] {
					c += area(p.slack(at(s, k)), at(s, l))
				}
			}
			if c > 0 {
				if c < bestCost {
					best, bestCost = s, c
				}
				continue
			}
			covered := 0.0
			for _, k := range all {
				covered += p.border(at(s, k))
			}
			if covered == 0 {
				best, bestCost, onBorder = s, 0, nil
				break
			}
			if onBorder == nil {
				b := s
				onBorder = &b
			}
		}
	}
	climb()
	// a home group binds while some spot inside it is clear, outright or
	// but for a border: else the ladder runs again without it (S10)
	if tried && bestCost > 0 && onBorder == nil && slices.ContainsFunc(all, func(k int) bool { return p.edges[k].Home != nil }) {
		p.homeless = true
		climb()
		p.homeless = false
	}
	if !tried {
		return nil, false
	}
	if onBorder != nil {
		best, bestCost = *onBorder, 0
		layoutdbg.Decision(ctx, "label_on_border", "phase", "labels", "module", "diago", "spec_ref", "S10",
			"edge", p.edges[e].ID, "what", what)
	}
	out := make(map[int]*Rect, len(all))
	for _, k := range all {
		r := at(best, k)
		p.placedGrid.add(len(p.placed), r)
		p.placed = append(p.placed, r)
		out[k] = &r
	}
	if bestCost > 0 {
		attrs := []any{"phase", "labels", "module", "diago", "spec_ref", "S10",
			"edge", p.edges[e].ID, "what", what, "overlap", bestCost, "nearer", p.misread(*out[e], e)}
		if len(all) > 1 {
			attrs = append(attrs, "copies", len(all)-1)
		}
		layoutdbg.Decision(ctx, "label_unresolved", attrs...)
	}
	return out, bestCost > 0
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

// index builds the grids over the node boxes and the segments, and their
// cells in the text profile, with cells about as large as the labels'
// boxes; the placed labels join theirs as they are placed.
func (p *placer) index(gridded bool) {
	var bounds Rect
	empty := true
	for _, set := range [][]Rect{p.nodes, p.segs, p.drawn} {
		for _, r := range set {
			if empty {
				bounds, empty = r, false
				continue
			}
			x0, y0 := min(bounds.X, r.X), min(bounds.Y, r.Y)
			x1, y1 := max(bounds.X+bounds.W, r.X+r.W), max(bounds.Y+bounds.H, r.Y+r.H)
			bounds = Rect{x0, y0, x1 - x0, y1 - y0}
		}
	}
	side, n := 0.0, 0
	for _, e := range p.edges {
		for _, z := range []Size{e.Label, e.From, e.To} {
			if z.W > 0 {
				side += max(z.W, z.H)
				n++
			}
		}
	}
	if n > 0 && gridded {
		side /= float64(n)
	} else {
		side = 0
	}
	p.nodeGrid, p.placedGrid, p.segGrid, p.drawnGrid = newGrid(bounds, side), newGrid(bounds, side), newGrid(bounds, side), newGrid(bounds, side)
	for i, r := range p.nodes {
		p.nodeGrid.add(i, r)
	}
	for i, r := range p.segs {
		p.segGrid.add(i, r)
	}
	for i, r := range p.drawn {
		p.drawnGrid.add(i, r)
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

func sgn(v float64) float64 {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}
