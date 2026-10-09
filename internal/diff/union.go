package diff

import (
	"slices"
	"sort"

	"github.com/oxforge/diago/internal/model"
)

// maxGroupDepth mirrors schema's nesting limit.
const maxGroupDepth = 3

// UnionGraph is after ∪ removed-from-before with a status per element,
// laid out once by the pipeline so removed elements keep real space.
type UnionGraph struct {
	Graph        model.Graph
	Nodes        map[string]Status
	Edges        map[string]Status // keyed by the union edge id, which is <id>__removed (repeated until free) for a removed before-edge whose id after reused; Diff.RemovedEdges keeps the original before ids, and removedUnionID (text.go) bridges the two.
	Groups       map[string]Status
	Diff         GraphDiff
	BeforeLabels map[string]string      // every before node's and group's label, by id
	Class        bool                   // either side is a class graph (some node carries Members)
	Members      map[string]MemberUnion // per changed node with members on both sides
}

// BuildUnion merges before and after. Element order: after's, then removed
// before-elements in before order. Presentation (direction, style,
// alignment, hints) comes from after. Group membership is rebuilt over the
// merged set: after's for survivors, before's for removed nodes and groups;
// a removed group whose re-attached depth would exceed the nesting limit is
// hoisted to the top level.
func BuildUnion(before, after *model.Graph) UnionGraph {
	u := UnionGraph{
		Nodes: map[string]Status{}, Edges: map[string]Status{}, Groups: map[string]Status{},
		Diff: DiffGraphs(before, after), BeforeLabels: map[string]string{}, Members: map[string]MemberUnion{},
		Class: hasMembers(before) || hasMembers(after),
	}
	for _, n := range before.Nodes {
		u.BeforeLabels[n.ID] = n.Label
	}
	for _, g := range before.Groups {
		u.BeforeLabels[g.ID] = g.Label
	}
	nodes := u.unionNodes(before, after)
	edges := u.unionEdges(before, after)
	groups := u.unionGroups(before, after)
	u.nestGroups(groups, nodes, before, after)
	u.Graph = model.Graph{
		Direction: after.Direction,
		Nodes:     nodes, Edges: edges, Groups: groups,
	}
	return u
}

// hasMembers reports whether some node of g carries members, which makes g
// a class graph.
func hasMembers(g *model.Graph) bool {
	return slices.ContainsFunc(g.Nodes, func(n model.Node) bool { return n.Members != nil })
}

// setStatus sets the status of every id in ids to s.
func setStatus(m map[string]Status, ids []string, s Status) {
	for _, id := range ids {
		m[id] = s
	}
}

// unionNodes returns after's nodes, then before's removed ones, and records
// each one's status. A changed class node with members on both sides
// carries the member union instead of after's own list, so removed members
// keep a line in the record box.
func (u *UnionGraph) unionNodes(before, after *model.Graph) []model.Node {
	d := u.Diff
	bn := nodeByID(before)
	changed := make(map[string]bool, len(d.ChangedNodes))
	for _, id := range d.ChangedNodes {
		changed[id] = true
	}
	nodes := make([]model.Node, 0, len(after.Nodes)+len(d.RemovedNodes))
	for _, n := range after.Nodes {
		if o, ok := bn[n.ID]; ok && changed[n.ID] && o.Members != nil && n.Members != nil {
			mu := unionMembers(o.Members, n.Members)
			u.Members[n.ID] = mu
			members := mu.Members
			n.Members = &members
		}
		nodes = append(nodes, n)
		u.Nodes[n.ID] = Same
	}
	setStatus(u.Nodes, d.AddedNodes, Added)
	setStatus(u.Nodes, d.ChangedNodes, Changed)
	for _, id := range d.RemovedNodes {
		nodes = append(nodes, bn[id])
		u.Nodes[id] = Removed
	}
	return nodes
}

// unionEdges returns after's edges, then before's removed ones, and records
// each one's status. A removed edge whose id after reuses takes "__removed"
// suffixes until its id is free.
func (u *UnionGraph) unionEdges(before, after *model.Graph) []model.Edge {
	d := u.Diff
	be := edgeByID(before)
	taken := make(map[string]bool, len(after.Edges))
	edges := make([]model.Edge, 0, len(after.Edges)+len(d.RemovedEdges))
	for _, e := range after.Edges {
		edges = append(edges, e)
		taken[e.ID] = true
		u.Edges[e.ID] = Same
	}
	setStatus(u.Edges, d.AddedEdges, Added)
	setStatus(u.Edges, d.ChangedEdges, Changed)
	for _, id := range d.RemovedEdges {
		e := be[id]
		for taken[e.ID] {
			e.ID += "__removed"
		}
		taken[e.ID] = true
		edges = append(edges, e)
		u.Edges[e.ID] = Removed
	}
	return edges
}

// unionGroups returns after's groups, then before's removed ones, each with
// its membership cleared for nestGroups to rebuild, and records each one's
// status.
func (u *UnionGraph) unionGroups(before, after *model.Graph) []model.Group {
	d := u.Diff
	bg := groupByID(before)
	groups := make([]model.Group, 0, len(after.Groups)+len(d.RemovedGroups))
	for _, g := range after.Groups {
		g.Contains, g.Children, g.Depth = nil, nil, 0
		groups = append(groups, g)
		u.Groups[g.ID] = Same
	}
	setStatus(u.Groups, d.AddedGroups, Added)
	setStatus(u.Groups, d.ChangedGroups, Changed)
	for _, id := range d.RemovedGroups {
		g := bg[id]
		g.Contains, g.Children, g.Depth = nil, nil, 0
		groups = append(groups, g)
		u.Groups[id] = Removed
	}
	return groups
}

// nestGroups rebuilds the membership of the union's groups in place: a
// node or group joins the group its own side names as its parent, after
// for a survivor and before for a removed one, when the union has that
// group. A removed group whose chain would nest too deep goes to the top
// level instead (hoistOverflowing); every group's Depth follows.
func (u *UnionGraph) nestGroups(groups []model.Group, nodes []model.Node, before, after *model.Graph) {
	index := make(map[string]int, len(groups))
	for i, g := range groups {
		index[g.ID] = i
	}
	bp, ap := parentOf(before), parentOf(after)
	parent := func(id string, removed bool) string {
		if removed {
			return bp[id]
		}
		return ap[id]
	}
	for _, n := range nodes {
		if p := parent(n.ID, u.Nodes[n.ID] == Removed); p != "" {
			if i, ok := index[p]; ok {
				groups[i].Contains = append(groups[i].Contains, n.ID)
			}
		}
	}
	groupParent := make(map[string]string, len(groups))
	for _, g := range groups {
		if p := parent(g.ID, u.Groups[g.ID] == Removed); p != "" {
			if _, ok := index[p]; ok {
				groupParent[g.ID] = p
			}
		}
	}
	hoistOverflowing(groups, groupParent, u.Groups)
	for _, g := range groups {
		if p, ok := groupParent[g.ID]; ok {
			groups[index[p]].Children = append(groups[index[p]].Children, g.ID)
		}
	}
	for i := range groups {
		groups[i].Depth = chainDepth(groups[i].ID, groupParent)
	}
}

// hoistOverflowing deletes the parent link of every removed group whose
// chain, its own subtree included, would nest deeper than maxGroupDepth.
// It goes shallowest would-be depth first, union order breaking ties, so
// an overflowing ancestor is hoisted together with its whole subtree
// instead of a deeper descendant being torn out from under it first.
func hoistOverflowing(groups []model.Group, parent map[string]string, status map[string]Status) {
	var removed []string
	initialDepth := make(map[string]int)
	for _, g := range groups {
		if status[g.ID] == Removed {
			removed = append(removed, g.ID)
			initialDepth[g.ID] = chainDepth(g.ID, parent)
		}
	}
	sort.SliceStable(removed, func(i, j int) bool {
		return initialDepth[removed[i]] < initialDepth[removed[j]]
	})
	for _, id := range removed {
		if parent[id] != "" && chainDepth(id, parent)+subtreeHeight(id, groups, parent)-1 > maxGroupDepth {
			delete(parent, id)
		}
	}
}

// chainDepth is group id's nesting depth along parent, 1 at the top level.
// parent cannot contain a cycle: survivors resolve their parent through
// after only, removed groups through before only, after never references
// a removed group, and each side's own containment tree is validated
// acyclic and depth-bounded by the schema before it reaches here.
// chainDepth and subtreeHeight therefore walk it without a visited-set
// guard.
func chainDepth(id string, parent map[string]string) int {
	depth := 1
	for p := parent[id]; p != ""; p = parent[p] {
		depth++
	}
	return depth
}

// subtreeHeight is the number of levels group id's subtree spans along
// parent, its own included: 1 for a group without child groups.
func subtreeHeight(id string, groups []model.Group, parent map[string]string) int {
	h := 1
	for _, c := range groups {
		if parent[c.ID] == id {
			h = max(h, 1+subtreeHeight(c.ID, groups, parent))
		}
	}
	return h
}
