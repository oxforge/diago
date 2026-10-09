package layered

import (
	"math"

	"github.com/oxforge/diago/internal/layout/layered/route"
	"github.com/oxforge/diago/internal/layout/layered/size"
	"github.com/oxforge/diago/internal/model"
)

// bounds sets the level's content bounds: every node box and every route
// point, and in the text profile the whole cells around them.
func (l *scope) bounds(text bool) {
	lo := model.Point{X: math.Inf(1), Y: math.Inf(1)}
	hi := model.Point{X: math.Inf(-1), Y: math.Inf(-1)}
	add := func(x0, y0, x1, y1 float64) {
		lo.X, lo.Y = math.Min(lo.X, x0), math.Min(lo.Y, y0)
		hi.X, hi.Y = math.Max(hi.X, x1), math.Max(hi.Y, y1)
	}
	out := l.out
	for v, nd := range out.Level.Nodes {
		if nd.Terminal {
			continue
		}
		top := out.Route.RowTop[out.Layers[v]]
		add(out.X[v]-nd.W/2, top, out.X[v]+nd.W/2, top+nd.H)
	}
	for _, pts := range out.Route.Edges {
		for _, p := range pts {
			add(p.X, p.Y, p.X, p.Y)
		}
	}
	if math.IsInf(lo.X, 1) {
		lo, hi = model.Point{}, model.Point{}
	}
	if text {
		lo.X, lo.Y, hi.X, hi.Y = math.Floor(lo.X), math.Floor(lo.Y), math.Ceil(hi.X), math.Ceil(hi.Y)
	}
	l.lo, l.hi = lo, hi
}

// wrap sizes group container c's box around its content (S9): GroupPadX
// beside it, GroupPadY below it, and above it the larger of GroupPadY and,
// titled, the title band (TitleMid plus half the title's height plus
// TitleGap); all in the output frame, turned into the engine's by the
// direction. A title wider than the content widens the box, the content
// centered: the widest title among the containers that copy one level
// (titleW, S12).
func (n *nesting) wrap(c int) {
	l, cfg := n.levels[c], n.cfg
	gr := n.g.Groups[n.tree.Containers[c].Group]
	top := cfg.GroupPadY
	if gr.Label != "" {
		w, h := size.Title(gr.Label, cfg.Size)
		l.title = [2]float64{w, h}
		top = max(top, cfg.TitleMid+h/2+cfg.TitleGap)
	}
	side, bottom := cfg.GroupPadX, cfg.GroupPadY
	// left, top, right, bottom in the engine's frame
	pad := [4]float64{side, top, side, bottom}
	switch n.dir {
	case model.Up:
		pad = [4]float64{side, bottom, side, top}
	case model.Right, model.Left:
		pad = [4]float64{top, side, bottom, side}
	}
	l.w = l.hi.X - l.lo.X + pad[0] + pad[2]
	l.h = l.hi.Y - l.lo.Y + pad[1] + pad[3]
	l.in = model.Point{X: pad[0], Y: pad[1]}
	need := 2*cfg.TitleInset + n.titleW[c]
	if gr.Label == "" {
		need = 0
	}
	slack := func(have float64) float64 {
		if cfg.Text {
			return math.Floor((need - have) / 2)
		}
		return (need - have) / 2
	}
	if sideways(n.dir) {
		if l.h < need {
			l.in.Y += slack(l.h)
			l.h = need
		}
	} else if l.w < need {
		l.in.X += slack(l.w)
		l.w = need
	}
}

// compose turns container c's level, and every level inside it, into the
// engine's frame (S9): the root's frame is the engine's, and a child
// group's content sits inside its box, centered on the flow axis when S4
// stretched the box.
func (n *nesting) compose(c int) {
	l := n.levels[c]
	for v, ch := range l.stands {
		k := ch.Container
		if k < 0 {
			continue
		}
		nd := l.out.Level.Nodes[v]
		left := l.out.X[v] - nd.W/2 + l.off.X
		top := l.out.Route.RowTop[l.out.Layers[v]] + l.off.Y
		kl := n.levels[k]
		stretch := (nd.H - kl.h) / 2
		if n.cfg.Text {
			stretch = math.Floor(stretch)
		}
		kl.box = [4]float64{left, top, nd.W, nd.H}
		kl.off = model.Point{X: left + kl.in.X - kl.lo.X, Y: top + kl.in.Y + stretch - kl.lo.Y}
		n.compose(k)
	}
}

// assemble builds the positioned graph in the engine's frame: every node
// from its container's level, every group's box, and every edge's route
// joined across the borders it crosses.
func (n *nesting) assemble() *model.PositionedGraph {
	g, t := n.g, n.tree
	pg := &model.PositionedGraph{
		Nodes:  make([]model.PositionedNode, len(g.Nodes)),
		Edges:  make([]model.PositionedEdge, len(g.Edges)),
		Groups: make([]model.PositionedGroup, len(g.Groups)),
	}
	for i, nd := range g.Nodes {
		l := n.levels[t.In[i]]
		v := l.nodeAt[i]
		ln := l.out.Level.Nodes[v]
		pg.Nodes[i] = model.PositionedNode{
			ID: nd.ID, Label: nd.Label, Lines: n.sizes[i].Lines, Shape: nd.Shape, Color: nd.Color,
			X: l.out.X[v] + l.off.X, Y: l.out.Route.RowTop[l.out.Layers[v]] + ln.H/2 + l.off.Y,
			Width: ln.W, Height: ln.H, Members: n.sizes[i].Members,
		}
	}
	for i, gr := range g.Groups {
		l := n.levels[i+1]
		pg.Groups[i] = model.PositionedGroup{
			ID: gr.ID, Label: gr.Label, X: l.box[0], Y: l.box[1], Width: l.box[2], Height: l.box[3],
			Contains: gr.Contains, Children: gr.Children, Depth: gr.Depth, Color: gr.Color,
			LabelWidth: l.title[0], LabelHeight: l.title[1],
		}
		if pg.Groups[i].Contains == nil {
			pg.Groups[i].Contains = []string{}
		}
		if pg.Groups[i].Children == nil {
			pg.Groups[i].Children = []string{}
		}
	}
	for i, e := range g.Edges {
		home := n.levels[t.Home[i]]
		pg.Edges[i] = positionedEdge(e)
		pg.Edges[i].Points = simplify(n.expand(t.Home[i], home.edgeAt[i]))
	}
	return pg
}

// positionedEdge is edge e as the positioned graph carries it, not routed
// yet: its fields, and its cardinalities without their boxes.
func positionedEdge(e model.Edge) model.PositionedEdge {
	pe := model.PositionedEdge{
		ID: e.ID, From: e.From, To: e.To, Label: e.Label, Style: e.Style, Direction: e.Direction,
		Color: e.Color, Relation: e.Relation, Directed: e.Directed,
	}
	if e.FromCard != "" {
		pe.FromCard = &model.EndLabel{Text: e.FromCard}
	}
	if e.ToCard != "" {
		pe.ToCard = &model.EndLabel{Text: e.ToCard}
	}
	return pe
}

// expand is level edge le of container c's route in the engine's frame,
// continued into every child group it anchors on, to the real nodes.
func (n *nesting) expand(c, le int) []model.Point {
	l := n.levels[c]
	ed := l.lv.Edges[le]
	ge := l.carries[le]
	var pts []model.Point
	join := func(more []model.Point) { pts = append(pts, more...) }
	if k := l.stands[ed.From].Container; k >= 0 {
		join(n.expand(k, n.levels[k].edgeAt[ge]))
	}
	own := make([]model.Point, len(l.out.Route.Edges[le]))
	for i, p := range l.out.Route.Edges[le] {
		own[i] = model.Point{X: p.X + l.off.X, Y: p.Y + l.off.Y}
	}
	join(own)
	if k := l.stands[ed.To].Container; k >= 0 {
		join(n.expand(k, n.levels[k].edgeAt[ge]))
	}
	return pts
}

// simplify snaps a coordinate within a millionth of a unit of the previous
// point's to its value, once, then drops repeated points and the middle of
// three collinear ones as the router does within a level (S8): the two
// sides of a border, and two anchored ports on one column, compute one x
// along different sums.
func simplify(pts []model.Point) []model.Point {
	for i := 1; i < len(pts); i++ {
		if math.Abs(pts[i].X-pts[i-1].X) < noise {
			pts[i].X = pts[i-1].X
		}
		if math.Abs(pts[i].Y-pts[i-1].Y) < noise {
			pts[i].Y = pts[i-1].Y
		}
	}
	return route.Simplify(pts)
}
