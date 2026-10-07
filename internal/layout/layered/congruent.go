package layered

import (
	"cmp"
	"context"
	"math"
	"slices"

	"github.com/oxforge/diago/internal/layout/layered/lgraph"
	"github.com/oxforge/diago/internal/layout/layered/route"
	"github.com/oxforge/diago/internal/layout/layered/size"
	"github.com/oxforge/diago/internal/layoutdbg"
	"github.com/oxforge/diago/internal/model"
)

// congruentTol is how far two sizes may differ and still match (S12): the
// route tolerance of C17.1.
const congruentTol = 0.01

// congruence is one set of congruent sibling groups (S12): the node that
// feeds them, the container whose children they are, the representative
// and the other members, containers all, and per member the
// correspondence of the representative's subtree to the member's.
type congruence struct {
	source  int
	parent  int
	rep     int
	members []int
	maps    []correspondence
}

// groups is the congruence's groups: the representative, then the members.
func (cg congruence) groups() []int { return append([]int{cg.rep}, cg.members...) }

// correspondence maps a representative's subtree onto a member's (S12):
// every graph node inside it, every container, the representative's
// itself included, and every graph edge with an end inside it.
type correspondence struct {
	nodes      map[int]int
	containers map[int]int
	edges      map[int]int
}

// copying is where a container's level comes from when it copies another
// (S12, Rule A): the container it copies and the correspondence that maps
// that container's subtree onto its own.
type copying struct {
	from int
	m    *correspondence
}

// congruences finds every set of congruent sibling groups (S12), container
// by container from the root down: for each node in declaration order,
// the child groups of the container that it feeds, with an edge from it to
// a node inside each, and that no set found earlier holds. With at least
// two, the first in the level's order is the representative, and every
// other must correspond to it and, when a set found earlier already makes
// it copy, trace its copy back to the representative's origin (Rule A);
// one that does not degrades the whole set. Each set kept makes its
// members copy (n.copies) before the next is sought.
func (n *nesting) congruences(ctx context.Context) []congruence {
	var out []congruence
	held := map[int]bool{}
	n.copies = map[int]copying{}
	for _, c := range n.preorder(0, nil) {
		for s := range n.g.Nodes {
			fed := n.fed(c, s, held)
			if len(fed) < 2 {
				continue
			}
			cg := congruence{source: s, parent: c, rep: fed[0]}
			for _, k := range fed[1:] {
				m, why := n.correspond(fed[0], k)
				if _, copied := n.copies[k]; why == "" && copied && n.origin(k) != n.origin(fed[0]) {
					why = "copied_apart"
				}
				if why != "" {
					layoutdbg.Decision(ctx, "congruence_degraded",
						"phase", "congruence", "module", "diago", "spec_ref", "S12",
						"source", n.g.Nodes[s].ID, "representative", n.name(fed[0]), "member", n.name(k), "reason", why)
					cg.members = nil
					break
				}
				cg.members = append(cg.members, k)
				cg.maps = append(cg.maps, m)
			}
			if cg.members == nil {
				continue
			}
			for _, k := range fed {
				held[k] = true
			}
			n.copyMembers(&cg)
			out = append(out, cg)
			names := make([]string, len(cg.members))
			for i, k := range cg.members {
				names[i] = n.name(k)
			}
			layoutdbg.Decision(ctx, "congruence_detected",
				"phase", "congruence", "module", "diago", "spec_ref", "S12",
				"source", n.g.Nodes[s].ID, "representative", n.name(cg.rep), "members", names)
		}
	}
	return out
}

// fed is the child groups of container c, in the level's order, that node
// s feeds and no set holds yet: s lies outside each, and an edge runs
// from s to a node inside it.
func (n *nesting) fed(c, s int, held map[int]bool) []int {
	var out []int
	for _, ch := range n.tree.Containers[c].Children {
		k := ch.Container
		if k < 0 || held[k] || n.within(s, k) {
			continue
		}
		for _, e := range n.g.Edges {
			if n.index[e.From] == s && n.within(n.index[e.To], k) {
				out = append(out, k)
				break
			}
		}
	}
	return out
}

// within reports whether graph node v lies inside container k, at any
// depth.
func (n *nesting) within(v, k int) bool {
	for x := n.tree.In[v]; x != -1; x = n.tree.Containers[x].Parent {
		if x == k {
			return true
		}
	}
	return false
}

// nodesIn lists the graph nodes inside container k, at any depth, in
// declaration order.
func (n *nesting) nodesIn(k int) []int {
	var out []int
	for v := range n.g.Nodes {
		if n.within(v, k) {
			out = append(out, v)
		}
	}
	return out
}

// correspond maps container r's subtree onto container k's (S12), or
// says why it cannot: the containers pair up in preorder and the nodes in
// declaration order, each pair in corresponding containers with one
// shape and sizes within congruentTol, both titled or both not; the
// edges pair up by their ends, an edge inside r with the edge
// of k between the corresponding nodes and one that crosses r's border
// with the edge of k that crosses at the corresponding node, in the same
// direction and on the same row (S9), each with a label and
// cardinalities where its partner has them. Parallel edges pair in
// declaration order.
func (n *nesting) correspond(r, k int) (correspondence, string) {
	t := n.tree
	m := correspondence{nodes: map[int]int{}, containers: map[int]int{}, edges: map[int]int{}}
	rc, kc := n.preorder(r, nil), n.preorder(k, nil)
	if len(rc) != len(kc) {
		return m, "group_count"
	}
	for i := range rc {
		m.containers[rc[i]] = kc[i]
	}
	for i := range rc {
		a, b := rc[i], kc[i]
		if i > 0 && m.containers[t.Containers[a].Parent] != t.Containers[b].Parent {
			return m, "group_nesting"
		}
		if (n.g.Groups[t.Containers[a].Group].Label == "") != (n.g.Groups[t.Containers[b].Group].Label == "") {
			return m, "title"
		}
	}
	rn, kn := n.nodesIn(r), n.nodesIn(k)
	if len(rn) != len(kn) {
		return m, "node_count"
	}
	for i := range rn {
		a, b := rn[i], kn[i]
		na, nb := n.g.Nodes[a], n.g.Nodes[b]
		sa, sb := n.sizes[a], n.sizes[b]
		switch {
		case m.containers[t.In[a]] != t.In[b]:
			return m, "node_nesting"
		case na.Shape != nb.Shape || (na.Members == nil) != (nb.Members == nil):
			return m, "node_shape"
		case math.Abs(sa.W-sb.W) > congruentTol || math.Abs(sa.H-sb.H) > congruentTol:
			return m, "node_size"
		}
		m.nodes[a] = b
	}
	// An edge's key: its ends inside, by node, and for an end outside, the
	// row its terminals sit on.
	type key struct{ from, to, row int }
	keyOf := func(c, e int, nodes func(int) int) (key, bool) {
		ed := n.g.Edges[e]
		f, to := n.index[ed.From], n.index[ed.To]
		in := [2]bool{n.within(f, c), n.within(to, c)}
		if !in[0] && !in[1] {
			return key{}, false
		}
		kk := key{from: -1, to: -1, row: -1}
		if in[0] {
			kk.from = nodes(f)
		}
		if in[1] {
			kk.to = nodes(to)
		}
		if in[0] != in[1] {
			end := 0
			if in[1] {
				end = 1
			}
			hl := n.levels[t.Home[e]]
			kk.row = 0
			if (end == 0) != hl.reversed[hl.edgeAt[e]] {
				kk.row = 1
			}
		}
		return kk, true
	}
	free := map[key][]int{}
	for e := range n.g.Edges {
		if kk, ok := keyOf(k, e, func(v int) int { return v }); ok {
			free[kk] = append(free[kk], e)
		}
	}
	for e := range n.g.Edges {
		kk, ok := keyOf(r, e, func(v int) int { return m.nodes[v] })
		if !ok {
			continue
		}
		if len(free[kk]) == 0 {
			return m, "edges"
		}
		f := free[kk][0]
		free[kk] = free[kk][1:]
		ea, eb := n.g.Edges[e], n.g.Edges[f]
		if (ea.Label == "") != (eb.Label == "") || (ea.FromCard == "") != (eb.FromCard == "") || (ea.ToCard == "") != (eb.ToCard == "") {
			return m, "edge_labels"
		}
		m.edges[e] = f
	}
	for _, rest := range free {
		if len(rest) > 0 {
			return m, "edges"
		}
	}
	return m, ""
}

// copyMembers makes every container of cg's members' subtrees copy its
// counterpart in the representative's (S12, Rule A), reversals included,
// unless a set found earlier already makes it copy.
func (n *nesting) copyMembers(cg *congruence) {
	for j := range cg.maps {
		m := &cg.maps[j]
		// Each write reads a container of the representative's subtree and
		// writes one of a member's, so the map's order reaches nothing.
		for from, to := range m.containers {
			if _, ok := n.copies[to]; !ok {
				n.copies[to] = copying{from: from, m: m}
				n.copyReversals(from, to, m)
			}
		}
	}
}

// copyReversals gives container to's level the reversals of from's, the
// level it copies (S12, Rule A): each edge reversed exactly when its
// counterpart is, whatever to's own search or a carrier's flips made of it
// (S2, S13). Every later reader, the rows of a set found among to's
// groups and the carrier, then reads the reversals the copy draws.
func (n *nesting) copyReversals(from, to int, m *correspondence) {
	rl, ml := n.levels[from], n.levels[to]
	for le, e := range rl.carries {
		ml.reversed[ml.edgeAt[m.edges[e]]] = rl.reversed[le]
	}
}

// origin is the container whose level container c's traces its copy back
// to (S12, Rule A): c itself when it copies none. A container copies one
// before it in preorder, a representative's subtree preceding its
// members', so the walk ends.
func (n *nesting) origin(c int) int {
	for {
		cp, ok := n.copies[c]
		if !ok {
			return c
		}
		c = cp.from
	}
}

// copyLevel is container c's level laid out as a copy of the level it
// copies (S12, Rule A): every node where its counterpart is, with its
// extents, and every edge along its counterpart's route.
func (n *nesting) copyLevel(c int, cp copying) *levelLayout {
	rl, ml := n.levels[cp.from], n.levels[c]
	m := cp.m
	nodeOf := make([]int, len(rl.lv.Nodes))
	for v, ch := range rl.stands {
		switch {
		case ch.Node >= 0:
			nodeOf[v] = ml.nodeAt[m.nodes[ch.Node]]
		case ch.Container >= 0:
			nodeOf[v] = ml.groupAt[m.containers[ch.Container]]
		}
	}
	edgeOf := make([]int, len(rl.lv.Edges))
	for le, e := range rl.carries {
		me := ml.edgeAt[m.edges[e]]
		edgeOf[le] = me
		re, mm := rl.lv.Edges[le], ml.lv.Edges[me]
		if rl.lv.Nodes[re.From].Terminal {
			nodeOf[re.From] = mm.From
		}
		if rl.lv.Nodes[re.To].Terminal {
			nodeOf[re.To] = mm.To
		}
	}
	ro := rl.out
	lv := &lgraph.Level{Nodes: slices.Clone(ml.lv.Nodes), Edges: ml.lv.Edges}
	x := make([]float64, len(lv.Nodes))
	layers := make([]int, len(lv.Nodes))
	for v, mv := range nodeOf {
		x[mv], layers[mv] = ro.X[v], ro.Layers[v]
		lv.Nodes[mv].W, lv.Nodes[mv].H = ro.Level.Nodes[v].W, ro.Level.Nodes[v].H
	}
	edges := make([][]model.Point, len(lv.Edges))
	for le, me := range edgeOf {
		edges[me] = slices.Clone(ro.Route.Edges[le])
	}
	return &levelLayout{Level: lv, Layers: layers, X: x, Graph: copyGraph(ro.Graph, lv, nodeOf, edgeOf), Route: &route.Result{
		RowTop: ro.Route.RowTop, RowH: ro.Route.RowH, Lanes: ro.Route.Lanes, Edges: edges,
	}}
}

// copyGraph is the layered graph rg of the level a container copies,
// turned onto the copying level lv (S12, Rule A): node v of rg becomes
// node nodeOf[v] and edge e becomes edge edgeOf[e], each vertex on its
// layer and in its place, so that the carrier reads the copy's order
// (S13).
func copyGraph(rg *lgraph.Graph, lv *lgraph.Level, nodeOf, edgeOf []int) *lgraph.Graph {
	at := make([]int, len(rg.Vertices))
	for v := range at {
		at[v] = v
		if v < len(nodeOf) {
			at[v] = nodeOf[v]
		}
	}
	g := &lgraph.Graph{Level: lv, Reversed: make([]bool, len(lv.Edges)), Vertices: make([]lgraph.Vertex, len(rg.Vertices)),
		Layers: make([][]int, len(rg.Layers)), Chains: make([][]int, len(lv.Edges))}
	for v, vx := range rg.Vertices {
		w := vx
		if vx.Node >= 0 {
			w.ID, w.Node = lv.Nodes[at[v]].ID, at[v]
		} else {
			w.Edge = edgeOf[vx.Edge]
			w.ID = slot(lv.Edges[w.Edge].ID, vx.Layer)
		}
		g.Vertices[at[v]] = w
	}
	for l, layer := range rg.Layers {
		for _, v := range layer {
			g.Layers[l] = append(g.Layers[l], at[v])
		}
	}
	for e, chain := range rg.Chains {
		for _, v := range chain {
			g.Chains[edgeOf[e]] = append(g.Chains[edgeOf[e]], at[v])
		}
		if rg.Reversed != nil {
			g.Reversed[edgeOf[e]] = rg.Reversed[e]
		}
	}
	return g
}

// titleWidths gives every container the width of the title its box makes
// room for (S9): its own title's, or, when it copies another's level or
// another copies its own, the widest title among every container that
// traces its copy back to the same one (S12), so that their boxes match.
func (n *nesting) titleWidths() {
	n.titleW = make([]float64, len(n.tree.Containers))
	widest := map[int]float64{}
	for c := 1; c < len(n.tree.Containers); c++ {
		if label := n.g.Groups[n.tree.Containers[c].Group].Label; label != "" {
			w, _ := size.Title(label, n.cfg.Size)
			n.titleW[c] = w
			widest[n.origin(c)] = max(widest[n.origin(c)], w)
		}
	}
	for c := 1; c < len(n.tree.Containers); c++ {
		n.titleW[c] = max(n.titleW[c], widest[n.origin(c)])
	}
}

// applied is every congruence as the layout reports it (C17, S12), in the
// engine's frame: the representative's internal edges in edge order, each
// member's counterparts and its translation from the representative, and
// the fan-out: the source's edges into the members, in cross-axis order
// of their last point, when the source is a node of the members' level,
// they sit in one of its layers, and each member's edges from the source
// are the counterparts of the representative's.
func (n *nesting) applied(ctx context.Context, pg *model.PositionedGraph) []model.AppliedCongruence {
	var out []model.AppliedCongruence
	for _, cg := range n.cgs {
		ac := model.AppliedCongruence{}
		for e, ed := range n.g.Edges {
			if n.within(n.index[ed.From], cg.rep) && n.within(n.index[ed.To], cg.rep) {
				ac.RepEdges = append(ac.RepEdges, e)
			}
		}
		rb := pg.Groups[n.tree.Containers[cg.rep].Group]
		for i, k := range cg.members {
			m := cg.maps[i]
			edges := make([]int, len(ac.RepEdges))
			for j, e := range ac.RepEdges {
				edges[j] = m.edges[e]
			}
			mb := pg.Groups[n.tree.Containers[k].Group]
			ac.Members = append(ac.Members, model.AppliedCongruenceMember{Edges: edges, Dx: mb.X - rb.X, Dy: mb.Y - rb.Y})
		}
		if why := n.fanOut(cg); why != "" {
			layoutdbg.Decision(ctx, "congruence_fanout_skipped",
				"phase", "congruence", "module", "diago", "spec_ref", "S12",
				"source", n.g.Nodes[cg.source].ID, "representative", n.name(cg.rep), "reason", why)
		} else {
			ac.FanOut = n.fanEdges(cg, cg.groups()...)
			slices.SortStableFunc(ac.FanOut, func(a, b int) int {
				pa, pb := pg.Edges[a].Points, pg.Edges[b].Points
				return cmp.Compare(pa[len(pa)-1].X, pb[len(pb)-1].X)
			})
			if why := fanShape(pg, n.index, ac.FanOut); why != "" {
				layoutdbg.Decision(ctx, "congruence_fanout_skipped",
					"phase", "congruence", "module", "diago", "spec_ref", "S12",
					"source", n.g.Nodes[cg.source].ID, "representative", n.name(cg.rep), "reason", why)
				ac.FanOut = nil
			}
		}
		out = append(out, ac)
	}
	return out
}

// fanOut says why congruence cg reports no fan-out (S12), or "": its
// source is a node of the members' level, they sit in one of its layers,
// and every member's edges from the source are the counterparts of the
// representative's.
func (n *nesting) fanOut(cg congruence) string {
	if n.tree.In[cg.source] != cg.parent {
		return "source_outside"
	}
	l := n.levels[cg.parent]
	layer := l.out.Layers[l.groupAt[cg.rep]]
	own := n.fanEdges(cg, cg.rep)
	for i, k := range cg.members {
		if l.out.Layers[l.groupAt[k]] != layer {
			return "layers"
		}
		if len(n.fanEdges(cg, k)) != len(own) {
			return "fan_edges"
		}
		for _, e := range own {
			if n.index[n.g.Edges[cg.maps[i].edges[e]].From] != cg.source {
				return "fan_edges"
			}
		}
	}
	return ""
}

// fanEdges lists the edges from cg's source into groups ks, in edge order.
func (n *nesting) fanEdges(cg congruence, ks ...int) []int {
	var out []int
	for e, ed := range n.g.Edges {
		if n.index[ed.From] != cg.source {
			continue
		}
		for _, k := range ks {
			if n.within(n.index[ed.To], k) {
				out = append(out, e)
			}
		}
	}
	return out
}

// fans lists, per congruence whose members are container c's children
// and whose source is a node of c's level, the level edges from the
// source into the members (S12, Rule C).
func (n *nesting) fans(c int) [][]int {
	l := n.levels[c]
	var out [][]int
	for _, cg := range n.cgs {
		if cg.parent != c || n.tree.In[cg.source] != c {
			continue
		}
		var fan []int
		for _, e := range n.fanEdges(cg, cg.groups()...) {
			fan = append(fan, l.edgeAt[e])
		}
		out = append(out, fan)
	}
	return out
}

// fanShape says why a fan, its edges in row order, does not take Rule C's
// shape (S12, C17.3), or "": the two edges of every rank from both ends
// of the row spread on one track, and no two of its edges cross. An
// edge's spread run is its first segment across the flow axis beyond its
// source's box on the flow axis, in the engine's frame a horizontal one
// above or below it.
func fanShape(pg *model.PositionedGraph, index map[string]int, fan []int) string {
	spread := func(e int) (float64, bool) {
		ed := pg.Edges[e]
		src := pg.Nodes[index[ed.From]]
		for i := 0; i+1 < len(ed.Points); i++ {
			p, q := ed.Points[i], ed.Points[i+1]
			if math.Abs(p.Y-q.Y) <= noise && math.Abs(p.X-q.X) > noise &&
				(p.Y < src.Y-src.Height/2-noise || p.Y > src.Y+src.Height/2+noise) {
				return p.Y, true
			}
		}
		return 0, false
	}
	for i := 0; i < len(fan)/2; i++ {
		a, okA := spread(fan[i])
		b, okB := spread(fan[len(fan)-1-i])
		if okA && okB && math.Abs(a-b) > noise {
			return "lanes"
		}
	}
	routes := make([][]model.Point, len(fan))
	for i, e := range fan {
		routes[i] = pg.Edges[e].Points
	}
	for _, cs := range route.Crossings(routes, 0) {
		if len(cs) > 0 {
			return "crossing"
		}
	}
	return ""
}
