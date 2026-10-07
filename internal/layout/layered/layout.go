package layered

import (
	"context"
	"slices"
	"sort"

	"github.com/oxforge/diago/internal/layout/layered/frame"
	"github.com/oxforge/diago/internal/layout/layered/labels"
	"github.com/oxforge/diago/internal/layout/layered/ports"
	"github.com/oxforge/diago/internal/layout/layered/route"
	"github.com/oxforge/diago/internal/layout/layered/size"
	"github.com/oxforge/diago/internal/model"
)

// The text adapter's cell, in px (S14): the text renderer's grid.
const (
	cellW = 8.0
	cellH = 16.0
)

// hopRadius is how far from a segment's end a crossing must lie to be
// recorded, in px: the renderers' hop radius (C15.2).
const hopRadius = 5.0

// noise is the float tolerance of S9's joins and of S10's title rows: a
// millionth of a unit.
const noise = 1e-6

// Layout lays g out (S0): S1 to S8 through Arrange, then the nesting
// (nested, S9) and the flat edges routed on it (composed, S9), the group
// titles and the labels (S10), the frame (S11) and, in the text profile,
// the adapter that scales cells to 8 x 16 px. Crossings are found last,
// on the final geometry. A non-nil previous is a previous layout's
// carrier to anchor on (S13); the result carries its own (C18).
func Layout(ctx context.Context, g model.Graph, cfg Config, previous *model.LayoutHints) (*model.PositionedGraph, error) {
	dir, err := resolve(ctx, g, cfg)
	if err != nil {
		return nil, err
	}
	sizes := size.Measure(ctx, g.Nodes, cfg.Size)
	pg, err := composed(ctx, g, sizes, cfg, dir, previous)
	if err != nil {
		return nil, err
	}
	placeTitles(ctx, dir, cfg, pg)
	placeLabels(ctx, dir, g, cfg, pg)
	frame.Apply(ctx, pg, dir, cfg.MarginX, cfg.MarginY, cfg.Text)
	if cfg.Text {
		scale(pg, dir, cellW, cellH)
	}
	routes := make([][]model.Point, len(pg.Edges))
	for i, e := range pg.Edges {
		routes[i] = e.Points
	}
	for i, cs := range route.Crossings(routes, hopRadius) {
		pg.Edges[i].Crossings = cs
	}
	pg.Title = g.Title
	pg.Legend = legend(g)
	return pg, nil
}

// ForText returns g with AUTO resolved (S11) and the text Config for the
// direction it lays out in, ready for Layout.
func ForText(ctx context.Context, g model.Graph) (model.Graph, Config) {
	g.Direction = frame.Resolve(ctx, g)
	return g, TextConfig(g.Direction)
}

// placeTitles places every group title in its band (S10), in the engine's
// frame, where the groups and the routes are. The band runs along the
// output frame's horizontal axis, and whatever crosses the title's row
// blocks the span it covers there: every wire, every node box, and every
// group box but the group's own and its ancestors'.
func placeTitles(ctx context.Context, dir model.Direction, cfg Config, pg *model.PositionedGraph) {
	parent := make(map[string]string, len(pg.Groups))
	for _, gr := range pg.Groups {
		for _, ch := range gr.Children {
			parent[ch] = gr.ID
		}
	}
	for i := range pg.Groups {
		gr := &pg.Groups[i]
		if gr.LabelWidth <= 0 {
			continue
		}
		// u runs along the band from the group's left side in the output
		// frame, v down from its top side.
		uv := func(x, y float64) (u, v float64) {
			switch dir {
			case model.Up:
				return x - gr.X, gr.Y + gr.Height - y
			case model.Right:
				return y - gr.Y, x - gr.X
			case model.Left:
				return gr.Y + gr.Height - y, x - gr.X
			}
			return x - gr.X, y - gr.Y
		}
		width := gr.Width
		if sideways(dir) {
			width = gr.Height
		}
		top, bottom := cfg.TitleMid-gr.LabelHeight/2, cfg.TitleMid+gr.LabelHeight/2
		var blocked []labels.Span
		block := func(x0, y0, x1, y1 float64) {
			u0, v0 := uv(x0, y0)
			u1, v1 := uv(x1, y1)
			if max(v0, v1) > top+noise && min(v0, v1) < bottom-noise {
				blocked = append(blocked, labels.Span{Lo: min(u0, u1), Hi: max(u0, u1)})
			}
		}
		for _, e := range pg.Edges {
			for k := 0; k+1 < len(e.Points); k++ {
				block(e.Points[k].X, e.Points[k].Y, e.Points[k+1].X, e.Points[k+1].Y)
			}
		}
		for _, n := range pg.Nodes {
			block(n.X-n.Width/2, n.Y-n.Height/2, n.X+n.Width/2, n.Y+n.Height/2)
		}
		skip := map[string]bool{gr.ID: true}
		for p, ok := parent[gr.ID]; ok; p, ok = parent[p] {
			skip[p] = true
		}
		for _, o := range pg.Groups {
			if !skip[o.ID] {
				block(o.X, o.Y, o.X+o.Width, o.Y+o.Height)
			}
		}
		gr.LabelOffset, gr.LabelBlocked = labels.Title(ctx, labels.Band{ID: gr.ID, Width: width, Title: gr.LabelWidth, Blocked: blocked},
			labels.TitleOptions{Inset: cfg.TitleInset, Gap: cfg.LabelGap, Text: cfg.Text})
	}
}

// placeLabels measures every edge label and cardinality and places them in
// the engine's frame (S10), where a label's extents swap under RIGHT and
// LEFT so that it reads horizontally once the frame turns it; in the text
// profile each box is the cells the text renderer draws it in, clear of
// the cells it draws the nodes, the group frames and the wires in.
func placeLabels(ctx context.Context, dir model.Direction, g model.Graph, cfg Config, pg *model.PositionedGraph) {
	measure := func(text string) labels.Size {
		if text == "" {
			return labels.Size{}
		}
		w, h := size.Label(text, cfg.Size)
		if sideways(dir) {
			w, h = h, w
		}
		return labels.Size{W: w, H: h}
	}
	nodes := make([]labels.Rect, len(pg.Nodes), len(pg.Nodes)+len(pg.Groups))
	pinned := make([]bool, len(pg.Nodes), len(pg.Nodes)+len(pg.Groups))
	index := make(map[string]int, len(g.Nodes))
	for i, n := range pg.Nodes {
		nodes[i] = labels.Rect{X: n.X - n.Width/2, Y: n.Y - n.Height/2, W: n.Width, H: n.Height}
		pinned[i] = ports.Pinned(n.Shape, cfg.Text)
		index[n.ID] = i
	}
	// Group titles block labels as node boxes do (S10, C14.1), a text
	// title's blanks included.
	for _, gr := range pg.Groups {
		if t, ok := titleBox(gr, dir, cfg); ok {
			nodes = append(nodes, t)
			pinned = append(pinned, false)
		}
	}
	edges := make([]labels.Edge, len(g.Edges))
	for i, e := range g.Edges {
		edges[i] = labels.Edge{
			ID: e.ID, Points: pg.Edges[i].Points, Label: measure(e.Label),
			From: measure(e.FromCard), To: measure(e.ToCard),
			Adorned: e.Relation.Adorned(), Source: index[e.From], Loop: e.From == e.To,
		}
	}
	for lead, copies := range copyClasses(len(g.Edges), pg.AppliedCongruences) {
		edges[lead].Copies = copies
	}
	groups := make([]labels.Rect, len(pg.Groups))
	for i, gr := range pg.Groups {
		groups[i] = labels.Rect{X: gr.X, Y: gr.Y, W: gr.Width, H: gr.Height}
	}
	if cfg.Text {
		home := homes(pg.Groups)
		for i, e := range g.Edges {
			if k, ok := home(e.From, e.To); ok {
				edges[i].Home = &groups[k]
			}
		}
	}
	placed := labels.Place(ctx, nodes, pinned, groups, edges, labels.Options{
		Gap: cfg.LabelGap, CardStep: cfg.CardStep, EnvDepth: cfg.EnvDepth, EnvHalf: cfg.EnvHalf,
		OwnSlack: cfg.OwnSlack, Pad: cfg.LabelPad, Text: cfg.Text, Dir: dir,
	})
	for i, p := range placed {
		pe := &pg.Edges[i]
		if p.Label != nil {
			pe.LabelPos = &model.Point{X: p.Label.X + p.Label.W/2, Y: p.Label.Y + p.Label.H/2}
			pe.LabelWidth, pe.LabelHeight = p.Label.W, p.Label.H
			pe.LabelUnresolved = p.LabelUnresolved
		}
		for _, c := range []struct {
			card       *model.EndLabel
			rect       *labels.Rect
			unresolved bool
		}{{pe.FromCard, p.From, p.FromUnresolved}, {pe.ToCard, p.To, p.ToUnresolved}} {
			if c.card == nil || c.rect == nil {
				continue
			}
			c.card.Pos = &model.Point{X: c.rect.X + c.rect.W/2, Y: c.rect.Y + c.rect.H/2}
			c.card.Width, c.card.Height, c.card.Unresolved = c.rect.W, c.rect.H, c.unresolved
		}
	}
}

// homes returns, for two nodes, the index of the innermost of groups
// holding both, the deepest, and false when no group holds both: a text
// label stays inside that group's frame (S10).
func homes(groups []model.PositionedGroup) func(a, b string) (int, bool) {
	index := make(map[string]int, len(groups))
	for i, gr := range groups {
		index[gr.ID] = i
	}
	var holds func(i int, n string) bool
	holds = func(i int, n string) bool {
		if slices.Contains(groups[i].Contains, n) {
			return true
		}
		for _, ch := range groups[i].Children {
			if k, ok := index[ch]; ok && holds(k, n) {
				return true
			}
		}
		return false
	}
	return func(a, b string) (int, bool) {
		best := -1
		for i := range groups {
			if (best < 0 || groups[i].Depth > groups[best].Depth) && holds(i, a) && holds(i, b) {
				best = i
			}
		}
		return best, best >= 0
	}
}

// copyClasses joins every congruence's representative edges with their
// counterparts in its members (S12), across congruences too, and returns
// per class its first edge and the others, which place their labels as
// translated copies of its (C17.1).
func copyClasses(n int, acs []model.AppliedCongruence) map[int][]int {
	root := make([]int, n)
	for i := range root {
		root[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		if root[i] != i {
			root[i] = find(root[i])
		}
		return root[i]
	}
	for _, ac := range acs {
		for _, m := range ac.Members {
			for j, r := range ac.RepEdges {
				a, b := find(r), find(m.Edges[j])
				root[max(a, b)] = min(a, b)
			}
		}
	}
	out := map[int][]int{}
	for i := range root {
		if l := find(i); l != i {
			out[l] = append(out[l], i)
		}
	}
	return out
}

// titleBox is group gr's title box in the engine's frame, where gr's box
// already is: LabelOffset from its left side and centered TitleMid below
// its top side in the output frame (S10, C13), grown by TitlePad at both
// ends along the band, turned back by the direction; false for an
// untitled group.
func titleBox(gr model.PositionedGroup, dir model.Direction, cfg Config) (labels.Rect, bool) {
	if gr.LabelWidth <= 0 {
		return labels.Rect{}, false
	}
	w, h, at := gr.LabelWidth+2*cfg.TitlePad, gr.LabelHeight, gr.LabelOffset-cfg.TitlePad
	left, top, bottom := gr.X, gr.Y, gr.Y+gr.Height
	switch dir {
	case model.Up:
		return labels.Rect{X: left + at, Y: bottom - cfg.TitleMid - h/2, W: w, H: h}, true
	case model.Right:
		return labels.Rect{X: left + cfg.TitleMid - h/2, Y: top + at, W: h, H: w}, true
	case model.Left:
		return labels.Rect{X: left + cfg.TitleMid - h/2, Y: bottom - at - w, W: h, H: w}, true
	}
	return labels.Rect{X: left + at, Y: top + cfg.TitleMid - h/2, W: w, H: h}, true
}

// scale is the text adapter (S14): it turns a layout in cells into px, sx
// per column and sy per row, so that the text renderer, which reads px,
// finds every box edge on a cell boundary. A record box's line heights are
// rows. A congruence's translation stays in the engine's frame (S12),
// where, under RIGHT and LEFT (dir), x runs along the output's rows and y
// along its columns, so each scales by the factor of the axis it maps to.
func scale(pg *model.PositionedGraph, dir model.Direction, sx, sy float64) {
	for i := range pg.Nodes {
		n := &pg.Nodes[i]
		n.X, n.Y, n.Width, n.Height = n.X*sx, n.Y*sy, n.Width*sx, n.Height*sy
		if m := n.Members; m != nil {
			m.HeaderLineHeight, m.MemberLineHeight = m.HeaderLineHeight*sy, m.MemberLineHeight*sy
		}
	}
	for i := range pg.Groups {
		gr := &pg.Groups[i]
		gr.X, gr.Y, gr.Width, gr.Height = gr.X*sx, gr.Y*sy, gr.Width*sx, gr.Height*sy
		gr.LabelWidth, gr.LabelHeight, gr.LabelOffset = gr.LabelWidth*sx, gr.LabelHeight*sy, gr.LabelOffset*sx
	}
	for i := range pg.Edges {
		e := &pg.Edges[i]
		for k := range e.Points {
			e.Points[k].X *= sx
			e.Points[k].Y *= sy
		}
		if e.LabelPos != nil {
			e.LabelPos.X, e.LabelPos.Y = e.LabelPos.X*sx, e.LabelPos.Y*sy
			e.LabelWidth, e.LabelHeight = e.LabelWidth*sx, e.LabelHeight*sy
		}
		for _, c := range []*model.EndLabel{e.FromCard, e.ToCard} {
			if c != nil && c.Pos != nil {
				c.Pos.X, c.Pos.Y = c.Pos.X*sx, c.Pos.Y*sy
				c.Width, c.Height = c.Width*sx, c.Height*sy
			}
		}
	}
	ex, ey := sx, sy // the factors of the engine's x and y
	if sideways(dir) {
		ex, ey = sy, sx
	}
	for i := range pg.AppliedCongruences {
		for j := range pg.AppliedCongruences[i].Members {
			m := &pg.AppliedCongruences[i].Members[j]
			m.Dx, m.Dy = m.Dx*ex, m.Dy*ey
		}
	}
	pg.Width, pg.Height = pg.Width*sx, pg.Height*sy
}

// legend is one row per relation kind among g's edges, sorted by kind,
// labelled from g.Legend.Labels (an empty override drops the row) or by
// the kind; nil when g asks for no legend.
func legend(g model.Graph) []model.LegendEntry {
	if g.Legend == nil {
		return nil
	}
	present := map[string]bool{}
	for _, e := range g.Edges {
		if e.Relation != model.RelationNone {
			present[e.Relation.String()] = true
		}
	}
	kinds := make([]string, 0, len(present))
	for k := range present {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	var out []model.LegendEntry
	for _, k := range kinds {
		label := k
		if l, ok := g.Legend.Labels[k]; ok {
			if l == "" {
				continue
			}
			label = l
		}
		out = append(out, model.LegendEntry{Kind: k, Label: label})
	}
	return out
}
