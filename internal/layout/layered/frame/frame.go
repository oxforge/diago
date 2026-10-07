// Package frame maps the engine's top-to-bottom layout onto the requested
// direction (S11). Resolve settles AUTO before the layout starts, since
// sizing and the text Config depend on the direction; Apply runs last.
package frame

import (
	"context"
	"math"

	"github.com/oxforge/diago/internal/layoutdbg"
	"github.com/oxforge/diago/internal/model"
)

// Resolve returns the direction g lays out in: its own, or for AUTO,
// RIGHT when the graph has more nodes than edges plus one and DOWN
// otherwise, counting only the edges that are not flat, since a flat edge
// implies no flow (S11, S9).
func Resolve(ctx context.Context, g model.Graph) model.Direction {
	dir, wasAuto := resolveQuiet(g)
	if wasAuto {
		n := ranked(g)
		layoutdbg.Decision(ctx, "direction_resolved",
			"phase", "frame", "module", "diago", "spec_ref", "S11",
			"nodes", len(g.Nodes), "edges", n, "flat", len(g.Edges)-n, "direction", dir.String())
	}
	return dir
}

// ResolveQuiet is Resolve without the decision record. A caller that needs
// the resolved direction only to build its own downstream options, not to
// run the layout itself, uses it so it does not double the
// "direction_resolved" record the actual layout run already emits (the
// pipeline's contract check, for one: it resolves AUTO the same way Resolve
// does, to orient the rules that read direction, on a graph a layout run
// already resolved and logged).
func ResolveQuiet(g model.Graph) model.Direction {
	dir, _ := resolveQuiet(g)
	return dir
}

// resolveQuiet is Resolve's pure computation: the resolved direction, and
// whether g.Direction was AUTO, which is when Resolve's decision fires.
func resolveQuiet(g model.Graph) (dir model.Direction, wasAuto bool) {
	if g.Direction != model.Auto {
		return g.Direction, false
	}
	dir = model.Down
	if len(g.Nodes) > ranked(g)+1 {
		dir = model.Right
	}
	return dir, true
}

// ranked counts g's edges that are not flat.
func ranked(g model.Graph) int {
	n := 0
	for _, e := range g.Edges {
		if !e.Flat {
			n++
		}
	}
	return n
}

// Apply maps pg from the engine's top-to-bottom frame onto dir and adds
// the margins (S11): DOWN keeps it, UP mirrors it vertically, RIGHT
// transposes it and LEFT mirrors RIGHT horizontally. Under RIGHT and LEFT
// every box swaps its extents, so labels, laid out in the engine frame
// with swapped extents, read horizontally again. Then everything moves so
// that its geometry starts marginX and marginY from the canvas's left and
// top, and the canvas is its extent plus the margins on every side. In the
// text profile, in cells, that extent runs from and to whole cells, so the
// move keeps every box on the cell grid (S7).
func Apply(ctx context.Context, pg *model.PositionedGraph, dir model.Direction, marginX, marginY float64, text bool) {
	sideways := dir == model.Right || dir == model.Left
	at := func(x, y float64) (float64, float64) {
		switch dir {
		case model.Up:
			return x, -y
		case model.Right:
			return y, x
		case model.Left:
			return -y, x
		}
		return x, y
	}
	swap := func(w, h *float64) {
		if sideways {
			*w, *h = *h, *w
		}
	}
	for i := range pg.Nodes {
		n := &pg.Nodes[i]
		n.X, n.Y = at(n.X, n.Y)
		swap(&n.Width, &n.Height)
	}
	for i := range pg.Groups {
		gr := &pg.Groups[i]
		cx, cy := at(gr.X+gr.Width/2, gr.Y+gr.Height/2)
		swap(&gr.Width, &gr.Height)
		gr.X, gr.Y = cx-gr.Width/2, cy-gr.Height/2
	}
	for i := range pg.Edges {
		e := &pg.Edges[i]
		for k := range e.Points {
			e.Points[k].X, e.Points[k].Y = at(e.Points[k].X, e.Points[k].Y)
		}
		if e.LabelPos != nil {
			e.LabelPos.X, e.LabelPos.Y = at(e.LabelPos.X, e.LabelPos.Y)
			swap(&e.LabelWidth, &e.LabelHeight)
		}
		for _, c := range []*model.EndLabel{e.FromCard, e.ToCard} {
			if c != nil && c.Pos != nil {
				c.Pos.X, c.Pos.Y = at(c.Pos.X, c.Pos.Y)
				swap(&c.Width, &c.Height)
			}
		}
	}
	b := bounds(pg)
	if text {
		b.left, b.top, b.right, b.bottom = math.Floor(b.left), math.Floor(b.top), math.Ceil(b.right), math.Ceil(b.bottom)
	}
	dx, dy := marginX-b.left, marginY-b.top
	for i := range pg.Nodes {
		pg.Nodes[i].X += dx
		pg.Nodes[i].Y += dy
	}
	for i := range pg.Groups {
		pg.Groups[i].X += dx
		pg.Groups[i].Y += dy
	}
	for i := range pg.Edges {
		e := &pg.Edges[i]
		for k := range e.Points {
			e.Points[k].X += dx
			e.Points[k].Y += dy
		}
		if e.LabelPos != nil {
			e.LabelPos.X += dx
			e.LabelPos.Y += dy
		}
		for _, c := range []*model.EndLabel{e.FromCard, e.ToCard} {
			if c != nil && c.Pos != nil {
				c.Pos.X += dx
				c.Pos.Y += dy
			}
		}
	}
	pg.Width = b.right - b.left + 2*marginX
	pg.Height = b.bottom - b.top + 2*marginY
	layoutdbg.Decision(ctx, "framed",
		"phase", "frame", "module", "diago", "spec_ref", "S11",
		"direction", dir.String(), "width", pg.Width, "height", pg.Height)
}

type extent struct{ left, top, right, bottom float64 }

// bounds is the extent of every node box, group box, route point, label
// box and cardinality box of pg.
func bounds(pg *model.PositionedGraph) extent {
	b := extent{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
	add := func(x0, y0, x1, y1 float64) {
		b.left, b.top = math.Min(b.left, x0), math.Min(b.top, y0)
		b.right, b.bottom = math.Max(b.right, x1), math.Max(b.bottom, y1)
	}
	box := func(cx, cy, w, h float64) { add(cx-w/2, cy-h/2, cx+w/2, cy+h/2) }
	for _, n := range pg.Nodes {
		box(n.X, n.Y, n.Width, n.Height)
	}
	for _, g := range pg.Groups {
		add(g.X, g.Y, g.X+g.Width, g.Y+g.Height)
	}
	for _, e := range pg.Edges {
		for _, p := range e.Points {
			add(p.X, p.Y, p.X, p.Y)
		}
		if e.LabelPos != nil {
			box(e.LabelPos.X, e.LabelPos.Y, e.LabelWidth, e.LabelHeight)
		}
		for _, c := range []*model.EndLabel{e.FromCard, e.ToCard} {
			if c != nil && c.Pos != nil {
				box(c.Pos.X, c.Pos.Y, c.Width, c.Height)
			}
		}
	}
	if math.IsInf(b.left, 1) {
		return extent{}
	}
	return b
}
