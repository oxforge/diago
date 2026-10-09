package layered

import (
	"context"
	"fmt"

	"github.com/oxforge/diago/internal/layout/layered/cycle"
	"github.com/oxforge/diago/internal/layout/layered/lgraph"
	"github.com/oxforge/diago/internal/layout/layered/nest"
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
