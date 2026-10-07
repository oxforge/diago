package layered

import (
	"context"
	"fmt"
	"math"

	"github.com/oxforge/diago/internal/layout/layered/cycle"
	"github.com/oxforge/diago/internal/layout/layered/lgraph"
	"github.com/oxforge/diago/internal/layout/layered/nest"
	"github.com/oxforge/diago/internal/layout/layered/place"
	"github.com/oxforge/diago/internal/layout/layered/route"
	"github.com/oxforge/diago/internal/layout/layered/size"
	"github.com/oxforge/diago/internal/layoutdbg"
	"github.com/oxforge/diago/internal/model"
)

// nesting is one graph laid out level by level (S9): the root and one
// level per group, each over its direct children.
type nesting struct {
	g      model.Graph
	cfg    Config
	dir    model.Direction
	sizes  []size.Size
	tree   *nest.Tree
	levels []*scope           // per container
	flat   []float64          // per node: its cross-axis position in the whole-graph seed (S9); nil without groups
	index  map[string]int     // per node id: its index
	cgs    []congruence       // the congruent sibling groups (S12)
	copies map[int]copying    // per container whose level copies another's: where from (S12, Rule A)
	titleW []float64          // per container: the width of the title its box makes room for (S9, S12)
	prev   *model.LayoutHints // the previous layout's carrier (S13); nil for a fresh layout
	prevR  map[string]bool    // the edges it reversed that the graph has
}

// seed lays the whole graph out as one level, groups ignored, through
// place (S0 step 3, S9) and returns each node's cross-axis position there.
func (n *nesting) seed(ctx context.Context) ([]float64, error) {
	lv, err := level(n.g, n.sizes, n.dir)
	if err != nil {
		return nil, err
	}
	pl, err := planLevel(ctx, lv, cycle.Break(ctx, lv, nil), n.cfg, n.dir, nil, nil)
	if err != nil {
		return nil, err
	}
	x := place.Place(ctx, pl.Graph, pl.place)
	layoutdbg.Decision(ctx, "order_seeded",
		"phase", "nest", "module", "diago", "spec_ref", "S9",
		"nodes", len(n.g.Nodes), "edges", len(n.g.Edges))
	return x[:len(n.g.Nodes)], nil
}

// keys places container c's level nodes in the whole-graph seed: a key
// and a tie-break each; nil without a seed. A node takes its seed
// position and a group the mean of the nodes inside it. A terminal takes
// the seed position of its edge's outside end's representative at the
// edge's home level; among terminals of one representative, a top
// terminal follows that representative's own terminal for the edge when
// it is a group laid out already, a binding tie-break, so entries line up
// with the exits they come from; every other terminal follows its inside
// end's seed position, which S6 may overturn.
func (n *nesting) keys(c int) *seeding {
	if n.flat == nil {
		return nil
	}
	l, t := n.levels[c], n.tree
	sum := make([]float64, len(t.Containers))
	count := make([]float64, len(t.Containers))
	for node, x := range n.flat {
		for k := t.In[node]; k != -1; k = t.Containers[k].Parent {
			sum[k] += x
			count[k]++
		}
	}
	at := func(ch nest.Child) float64 {
		if ch.Node >= 0 {
			return n.flat[ch.Node]
		}
		if count[ch.Container] == 0 {
			return 0
		}
		return sum[ch.Container] / count[ch.Container]
	}
	key := make([]float64, len(l.lv.Nodes))
	tie := make([]float64, len(l.lv.Nodes))
	hard := make([]bool, len(l.lv.Nodes))
	for v, ch := range l.stands {
		if ch.Node >= 0 || ch.Container >= 0 {
			key[v] = at(ch)
		}
	}
	for le, ed := range l.lv.Edges {
		for end, v := range [2]int{ed.From, ed.To} {
			if !l.lv.Nodes[v].Terminal {
				continue
			}
			e := l.carries[le]
			// The terminal stands for the edge's end outside c: its From
			// end when the terminal is the level edge's From.
			outside, inner := n.index[n.g.Edges[e].From], n.index[n.g.Edges[e].To]
			if end == 1 {
				outside, inner = inner, outside
			}
			rep := t.Rep(t.Home[e], outside)
			key[v], tie[v] = at(rep), n.flat[inner]
			upper, _ := l.lv.Oriented(le, l.reversed)
			if k := rep.Container; upper == v && k >= 0 && n.levels[k].out != nil {
				tie[v], hard[v] = n.levels[k].at[e], true
			}
		}
	}
	return &seeding{key: key, tie: tie, hard: hard}
}

// scope is one container's level: its nodes stand for its direct
// children and its terminals, its edges carry graph edges.
type scope struct {
	lv       *lgraph.Level
	stands   []nest.Child // per level node: the child it stands for; a terminal's has both indices -1
	carries  []int        // per level edge: the graph edge it carries
	edgeAt   map[int]int  // per graph edge the level carries: its level edge
	nodeAt   map[int]int  // per graph node among the children: its level node
	groupAt  map[int]int  // per child container: its level node
	reversed []bool
	out      *levelLayout

	// After layout, in the level's own frame:
	lo, hi model.Point     // the content's bounds: node boxes and routes
	at     map[int]float64 // per graph edge with a terminal here: the terminal's cross-axis position
	// A group's box, around its content (S9):
	w, h  float64     // its extents before S4 stretches it in its parent level
	in    model.Point // the content's top-left corner, from the box's
	title [2]float64  // its title's extents in the output frame; zero when untitled
	off   model.Point // after composition: what turns the level's frame into the engine's
	box   [4]float64  // after composition: the box's left, top, width and height in the engine's frame
}

// nested lays g out level by level (S9), anchored on prev when it is not
// nil (S13), and returns the positioned graph in the engine's top-to-bottom
// frame, labels not yet placed, with its carrier.
func nested(ctx context.Context, g model.Graph, sizes []size.Size, cfg Config, dir model.Direction, prev *model.LayoutHints) (*model.PositionedGraph, error) {
	n, err := arrangeNested(ctx, g, sizes, cfg, dir, prev)
	if err != nil {
		return nil, err
	}
	pg := n.assemble()
	pg.AppliedCongruences = n.applied(ctx, pg)
	pg.LayoutHints = n.carrier()
	return pg, nil
}

// arrangeNested builds g's levels and orients them top-down, seeds them
// when g has groups, lays them out bottom-up and composes them (S9),
// anchored on prev when it is not nil (S13).
func arrangeNested(ctx context.Context, g model.Graph, sizes []size.Size, cfg Config, dir model.Direction, prev *model.LayoutHints) (*nesting, error) {
	t, err := nest.Build(g)
	if err != nil {
		return nil, fmt.Errorf("layered: %w", err)
	}
	n := &nesting{g: g, cfg: cfg, dir: dir, sizes: sizes, tree: t, levels: make([]*scope, len(t.Containers)), index: make(map[string]int, len(g.Nodes)), prev: prev}
	for i, nd := range g.Nodes {
		n.index[nd.ID] = i
	}
	n.prevR = n.readPrevious(ctx)
	if len(g.Groups) > 0 {
		if n.flat, err = n.seed(ctx); err != nil {
			return nil, err
		}
	}
	for _, c := range n.preorder(0, nil) {
		n.build(ctx, c)
	}
	n.cgs = n.congruences(ctx)
	n.titleWidths()
	for _, c := range n.postorder(0, nil) {
		if err := n.arrange(ctx, c); err != nil {
			return nil, err
		}
	}
	n.compose(0)
	return n, nil
}

// preorder lists container c and every container inside it, each before
// the containers it holds, children in declaration order.
func (n *nesting) preorder(c int, out []int) []int {
	out = append(out, c)
	for _, ch := range n.tree.Containers[c].Children {
		if ch.Container >= 0 {
			out = n.preorder(ch.Container, out)
		}
	}
	return out
}

// postorder lists container c and every container inside it, each after
// the containers it holds, children in declaration order.
func (n *nesting) postorder(c int, out []int) []int {
	for _, ch := range n.tree.Containers[c].Children {
		if ch.Container >= 0 {
			out = n.postorder(ch.Container, out)
		}
	}
	return append(out, c)
}

// build makes container c's level and orients its edges (S2, S9): its
// direct children in declaration order, its home edges between their
// representatives, and for every edge that crosses c's border a terminal
// on the first or the last row, the side its wire leaves the edge's home
// representative by. Every container above c is built first.
func (n *nesting) build(ctx context.Context, c int) {
	t := n.tree
	across := sideways(n.dir)
	l := &scope{lv: &lgraph.Level{}, edgeAt: map[int]int{}, nodeAt: map[int]int{}, groupAt: map[int]int{}}
	for _, ch := range t.Containers[c].Children {
		v := len(l.lv.Nodes)
		if ch.Node >= 0 {
			nd := n.g.Nodes[ch.Node]
			w, h := n.sizes[ch.Node].W, n.sizes[ch.Node].H
			if across {
				w, h = h, w
			}
			l.lv.Nodes = append(l.lv.Nodes, lgraph.Node{ID: nd.ID, Shape: nd.Shape, Record: nd.Members != nil, W: w, H: h})
			l.nodeAt[ch.Node] = v
		} else {
			l.lv.Nodes = append(l.lv.Nodes, lgraph.Node{ID: n.g.Groups[t.Containers[ch.Container].Group].ID, Shape: model.ShapeRect, Group: true})
			l.groupAt[ch.Container] = v
		}
		l.stands = append(l.stands, ch)
	}
	rep := func(node int) int {
		r := t.Rep(c, node)
		if r.Node >= 0 {
			return l.nodeAt[r.Node]
		}
		return l.groupAt[r.Container]
	}
	index := n.index
	var flips []bool
	for e, ed := range n.g.Edges {
		if t.Home[e] != c {
			continue
		}
		l.edgeAt[e] = len(l.lv.Edges)
		l.lv.Edges = append(l.lv.Edges, lgraph.Edge{ID: ed.ID, From: rep(index[ed.From]), To: rep(index[ed.To])})
		l.carries = append(l.carries, e)
		flips = append(flips, n.prevR[ed.ID])
	}
	for e, ed := range n.g.Edges {
		home := t.Home[e]
		if home == c {
			continue
		}
		for end, id := range [2]string{ed.From, ed.To} {
			node := index[id]
			if !inside(t, c, home, node) {
				continue
			}
			hl := n.levels[home]
			bottom := (end == 0) != hl.reversed[hl.edgeAt[e]]
			term := len(l.lv.Nodes)
			l.lv.Nodes = append(l.lv.Nodes, lgraph.Node{ID: ed.ID + "@" + n.g.Groups[t.Containers[c].Group].ID, Terminal: true, W: n.cfg.DummyWidth})
			l.stands = append(l.stands, nest.Child{Node: -1, Container: -1})
			le := lgraph.Edge{ID: ed.ID, From: rep(node), To: term}
			if end == 1 {
				le.From, le.To = term, rep(node)
			}
			l.edgeAt[e] = len(l.lv.Edges)
			l.lv.Edges = append(l.lv.Edges, le)
			l.carries = append(l.carries, e)
			flips = append(flips, (bottom && end == 1) || (!bottom && end == 0))
		}
	}
	l.reversed = cycle.Break(ctx, l.lv, flips)
	n.levels[c] = l
}

// inside reports whether node lies inside container c, which lies inside
// the edge's home container but is not it.
func inside(t *nest.Tree, c, home, node int) bool {
	for _, x := range t.Path(home, node) {
		if x == c {
			return true
		}
	}
	return false
}

// arrange lays out container c's level (S2 to S8), or copies the level it
// copies (S12, Rule A): its child groups, laid out already, enter as boxes
// whose ports anchor on their terminals, and a group's box then wraps the
// content in its padding and title band.
func (n *nesting) arrange(ctx context.Context, c int) error {
	l := n.levels[c]
	if cp, ok := n.copies[c]; ok {
		l.out = n.copyLevel(c, cp)
		layoutdbg.Decision(ctx, "level_copied",
			"phase", "congruence", "module", "diago", "spec_ref", "S12",
			"container", n.name(c), "from", n.name(cp.from))
	} else {
		for v, ch := range l.stands {
			if ch.Container >= 0 {
				l.lv.Nodes[v].W, l.lv.Nodes[v].H = n.levels[ch.Container].w, n.levels[ch.Container].h
			}
		}
		for le := range l.lv.Edges {
			ed := &l.lv.Edges[le]
			for end, v := range [2]int{ed.From, ed.To} {
				ch := l.stands[v]
				if ch.Container < 0 {
					continue
				}
				k := n.levels[ch.Container]
				ed.Anchor[end] = lgraph.Anchor{On: true, At: k.at[l.carries[le]] - k.lo.X + k.in.X - k.w/2}
			}
		}
		out, err := arrangeLevel(ctx, l.lv, l.reversed, n.cfg, n.dir, n.keys(c), n.fans(c), n.scopePrevious(c))
		if err != nil {
			return err
		}
		l.out = out
	}
	l.bounds(n.cfg.Text)
	l.at = map[int]float64{}
	for le, ed := range l.lv.Edges {
		pts := l.out.Route.Edges[le]
		switch {
		case l.lv.Nodes[ed.From].Terminal:
			l.at[l.carries[le]] = pts[0].X
		case l.lv.Nodes[ed.To].Terminal:
			l.at[l.carries[le]] = pts[len(pts)-1].X
		}
	}
	if c > 0 {
		n.wrap(c)
	}
	layoutdbg.Decision(ctx, "level_arranged",
		"phase", "nest", "module", "diago", "spec_ref", "S9",
		"container", n.name(c), "nodes", len(l.lv.Nodes), "edges", len(l.lv.Edges), "width", l.w, "height", l.h)
	return nil
}

func (n *nesting) name(c int) string {
	if c == 0 {
		return ""
	}
	return n.g.Groups[n.tree.Containers[c].Group].ID
}

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
