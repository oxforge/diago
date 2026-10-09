package layered

import (
	"cmp"
	"context"
	"math"
	"slices"

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
// declaration order. The containers pair first (pairContainers), then the
// nodes (pairNodes), then the edges (pairEdges), and the first that
// cannot says why.
func (n *nesting) correspond(r, k int) (correspondence, string) {
	m := correspondence{nodes: map[int]int{}, containers: map[int]int{}, edges: map[int]int{}}
	if why := n.pairContainers(r, k, m); why != "" {
		return m, why
	}
	if why := n.pairNodes(r, k, m); why != "" {
		return m, why
	}
	return m, n.pairEdges(r, k, m)
}

// pairContainers maps the containers of r's subtree onto k's, in
// preorder, into m (correspond), or says why it cannot: their counts
// differ (group_count), a pair's parents are no pair (group_nesting), or
// one of a pair is titled and the other not (title).
func (n *nesting) pairContainers(r, k int, m correspondence) string {
	t := n.tree
	rc, kc := n.preorder(r, nil), n.preorder(k, nil)
	if len(rc) != len(kc) {
		return "group_count"
	}
	for i := range rc {
		m.containers[rc[i]] = kc[i]
	}
	for i := range rc {
		a, b := rc[i], kc[i]
		if i > 0 && m.containers[t.Containers[a].Parent] != t.Containers[b].Parent {
			return "group_nesting"
		}
		if (n.g.Groups[t.Containers[a].Group].Label == "") != (n.g.Groups[t.Containers[b].Group].Label == "") {
			return "title"
		}
	}
	return ""
}

// pairNodes maps the graph nodes inside r onto those inside k, in
// declaration order, into m (correspond), its containers mapped already,
// or says why it cannot: their counts differ (node_count), a pair lies in
// containers that are no pair (node_nesting), its shapes differ or only
// one is a record (node_shape), or its sizes differ by more than
// congruentTol (node_size).
func (n *nesting) pairNodes(r, k int, m correspondence) string {
	t := n.tree
	rn, kn := n.nodesIn(r), n.nodesIn(k)
	if len(rn) != len(kn) {
		return "node_count"
	}
	for i := range rn {
		a, b := rn[i], kn[i]
		na, nb := n.g.Nodes[a], n.g.Nodes[b]
		sa, sb := n.sizes[a], n.sizes[b]
		switch {
		case m.containers[t.In[a]] != t.In[b]:
			return "node_nesting"
		case na.Shape != nb.Shape || (na.Members == nil) != (nb.Members == nil):
			return "node_shape"
		case math.Abs(sa.W-sb.W) > congruentTol || math.Abs(sa.H-sb.H) > congruentTol:
			return "node_size"
		}
		m.nodes[a] = b
	}
	return ""
}

// pairEdges maps every graph edge with an end inside r onto one with an
// end inside k, into m (correspond), its nodes mapped already, or says
// why it cannot: each takes, in declaration order, the first edge of k
// left whose key is its own with its ends inside mapped (keyOf), and none
// is left for it or one is left over (edges), or a pair differs in
// whether it has a label or each cardinality (edge_labels).
func (n *nesting) pairEdges(r, k int, m correspondence) string {
	free := map[edgeKey][]int{}
	for e := range n.g.Edges {
		if kk, ok := n.keyOf(k, e, func(v int) int { return v }); ok {
			free[kk] = append(free[kk], e)
		}
	}
	for e := range n.g.Edges {
		kk, ok := n.keyOf(r, e, func(v int) int { return m.nodes[v] })
		if !ok {
			continue
		}
		if len(free[kk]) == 0 {
			return "edges"
		}
		f := free[kk][0]
		free[kk] = free[kk][1:]
		ea, eb := n.g.Edges[e], n.g.Edges[f]
		if (ea.Label == "") != (eb.Label == "") || (ea.FromCard == "") != (eb.FromCard == "") || (ea.ToCard == "") != (eb.ToCard == "") {
			return "edge_labels"
		}
		m.edges[e] = f
	}
	for _, rest := range free {
		if len(rest) > 0 {
			return "edges"
		}
	}
	return ""
}

// edgeKey is an edge's key in a container's subtree (keyOf): its ends
// inside, by node, -1 for an end outside, and for an edge that crosses
// the border the row its terminals sit on, 0 the first and 1 the last, -1
// for none.
type edgeKey struct{ from, to, row int }

// keyOf is edge e's key in container c's subtree, an end inside named by
// nodes of its node; false when neither end lies inside c. The row is the
// one the inside end's terminal takes (build): the last when that end is
// the edge's From, unless its home level reverses it.
func (n *nesting) keyOf(c, e int, nodes func(int) int) (edgeKey, bool) {
	t := n.tree
	ed := n.g.Edges[e]
	f, to := n.index[ed.From], n.index[ed.To]
	in := [2]bool{n.within(f, c), n.within(to, c)}
	if !in[0] && !in[1] {
		return edgeKey{}, false
	}
	kk := edgeKey{from: -1, to: -1, row: -1}
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
