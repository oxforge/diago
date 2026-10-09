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

// pick takes the spot of edge e's what, its label or a cardinality, from
// the ladder spots (climb): the first spot clear of everything, or else,
// in the text profile, the first of its ring clear but for the frame
// sides it covers, or else the one with the least overlap, marked
// unresolved; when no spot is clear, outright or but for a border, and a
// home group (S10) holds one of the boxes, the ladder runs again without
// the home groups. It registers the spot: for edge e and, at its spot
// moved as their first route points are, for every copy of e (S12) with
// a route and a box, each at its own size (boxAt). It returns each edge's
// box by edge index.
func (p *placer) pick(ctx context.Context, e int, what string, spots iter.Seq[spot], size func(Edge) Size) (map[int]*Rect, bool) {
	all := []int{e}
	for _, k := range p.edges[e].Copies {
		if len(p.edges[k].Points) >= 2 && size(p.edges[k]).W > 0 {
			all = append(all, k)
		}
	}
	best, onBorder, bestCost := p.climb(spots, e, all, size)
	// a home group binds while some spot inside it is clear, outright or
	// but for a border: else the ladder runs again without it (S10)
	if bestCost > 0 && onBorder == nil && slices.ContainsFunc(all, func(k int) bool { return p.edges[k].Home != nil }) {
		p.homeless = true
		best, onBorder, bestCost = p.climb(spots, e, all, size)
		p.homeless = false
	}
	if onBorder != nil {
		best, bestCost = *onBorder, 0
		layoutdbg.Decision(ctx, "label_on_border", "phase", "labels", "module", "diago", "spec_ref", "S10",
			"edge", p.edges[e].ID, "what", what)
	}
	out := make(map[int]*Rect, len(all))
	for _, k := range all {
		r := p.boxAt(best, e, k, size)
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

// climb draws the spots from the ladder in order and stops at the first
// clear one (pick): it returns that spot, its cost 0; or else, in the
// text profile, the first spot of its ring clear but for the frame sides
// it covers (border), onBorder, once the ring holds none clear outright;
// or else the spot with the least overlap, the first of them, and that
// overlap. A spot's overlap counts every box all places at it, e's and
// its copies' (spotCost).
func (p *placer) climb(spots iter.Seq[spot], e int, all []int, size func(Edge) Size) (best spot, onBorder *spot, bestCost float64) {
	bestCost = math.Inf(1)
	ring := 0
	first := true
	for s := range spots {
		if onBorder != nil && s.ring != ring {
			break
		}
		ring = s.ring
		if first {
			best, first = s, false
		}
		if c := p.spotCost(s, e, all, size, bestCost); c > 0 {
			if c < bestCost {
				best, bestCost = s, c
			}
			continue
		}
		covered := 0.0
		for _, k := range all {
			covered += p.border(p.boxAt(s, e, k, size))
		}
		if covered == 0 {
			return s, nil, 0
		}
		if onBorder == nil {
			b := s
			onBorder = &b
		}
	}
	return best, onBorder, bestCost
}

// boxAt is edge k's box at spot s of edge e's ladder (pick): for e, the
// spot's box snapped; for a copy of e, a box of the copy's own size
// centered where that snapped box's center moves by the offset of the
// copy's first route point from e's, snapped as the spot is. In the text
// profile each box snaps to whole cells (snap).
func (p *placer) boxAt(s spot, e, k int, size func(Edge) Size) Rect {
	lead := p.snap(s.Rect, s)
	if k == e {
		return lead
	}
	o, c := p.edges[e].Points[0], p.edges[k].Points[0]
	z := size(p.edges[k])
	cx, cy := lead.X+lead.W/2+c.X-o.X, lead.Y+lead.H/2+c.Y-o.Y
	return p.snap(Rect{X: cx - z.W/2, Y: cy - z.H/2, W: z.W, H: z.H}, s)
}

func sgn(v float64) float64 {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}
