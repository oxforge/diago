package diff

import (
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
	d := DiffGraphs(before, after)
	u := UnionGraph{
		Nodes: map[string]Status{}, Edges: map[string]Status{}, Groups: map[string]Status{},
		Diff: d, BeforeLabels: map[string]string{}, Members: map[string]MemberUnion{},
	}
	for _, g := range []*model.Graph{before, after} {
		for _, n := range g.Nodes {
			if n.Members != nil {
				u.Class = true
				break
			}
		}
	}
	for _, n := range before.Nodes {
		u.BeforeLabels[n.ID] = n.Label
	}
	for _, g := range before.Groups {
		u.BeforeLabels[g.ID] = g.Label
	}
	set := func(m map[string]Status, ids []string, s Status) {
		for _, id := range ids {
			m[id] = s
		}
	}

	// Nodes. A changed class node with members on both sides carries the
	// member union instead of after's own list, so removed members keep a
	// line in the record box.
	bn := nodeByID(before)
	changedNode := make(map[string]bool, len(d.ChangedNodes))
	for _, id := range d.ChangedNodes {
		changedNode[id] = true
	}
	nodes := make([]model.Node, 0, len(after.Nodes)+len(d.RemovedNodes))
	for _, n := range after.Nodes {
		if o, ok := bn[n.ID]; ok && changedNode[n.ID] && o.Members != nil && n.Members != nil {
			mu := UnionMembers(o.Members, n.Members)
			u.Members[n.ID] = mu
			members := mu.Members
			n.Members = &members
		}
		nodes = append(nodes, n)
		u.Nodes[n.ID] = Same
	}
	set(u.Nodes, d.AddedNodes, Added)
	set(u.Nodes, d.ChangedNodes, Changed)
	for _, id := range d.RemovedNodes {
		nodes = append(nodes, bn[id])
		u.Nodes[id] = Removed
	}

	// Edges.
	be := edgeByID(before)
	taken := make(map[string]bool, len(after.Edges))
	edges := make([]model.Edge, 0, len(after.Edges)+len(d.RemovedEdges))
	for _, e := range after.Edges {
		edges = append(edges, e)
		taken[e.ID] = true
		u.Edges[e.ID] = Same
	}
	set(u.Edges, d.AddedEdges, Added)
	set(u.Edges, d.ChangedEdges, Changed)
	for _, id := range d.RemovedEdges {
		e := be[id]
		for taken[e.ID] {
			e.ID += "__removed"
		}
		taken[e.ID] = true
		edges = append(edges, e)
		u.Edges[e.ID] = Removed
	}

	// Groups: after's, then removed before-groups; membership rebuilt.
	bg := groupByID(before)
	groups := make([]model.Group, 0, len(after.Groups)+len(d.RemovedGroups))
	for _, g := range after.Groups {
		g.Contains, g.Children, g.Depth = nil, nil, 0
		groups = append(groups, g)
		u.Groups[g.ID] = Same
	}
	set(u.Groups, d.AddedGroups, Added)
	set(u.Groups, d.ChangedGroups, Changed)
	for _, id := range d.RemovedGroups {
		g := bg[id]
		g.Contains, g.Children, g.Depth = nil, nil, 0
		groups = append(groups, g)
		u.Groups[id] = Removed
	}
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
	// Hoist removed groups whose chain would exceed the limit: process
	// shallowest would-be depth first (union order as the tie-break), so an
	// overflowing ancestor is hoisted together with its whole subtree
	// instead of a deeper descendant being torn out from under it first.
	// groupParent cannot contain a cycle: survivors resolve their parent
	// through after only, removed groups through before only, after never
	// references a removed group, and each side's own containment tree is
	// validated acyclic and depth-bounded by the schema before it reaches
	// here. depthOf and subtreeHeight below can therefore walk it without a
	// visited-set guard.
	depthOf := func(id string) int {
		depth := 1
		for p := groupParent[id]; p != ""; p = groupParent[p] {
			depth++
		}
		return depth
	}
	subtreeHeight := func(id string) int {
		h := 1
		var walk func(string, int)
		walk = func(g string, level int) {
			for _, c := range groups {
				if groupParent[c.ID] == g {
					if level+1 > h {
						h = level + 1
					}
					walk(c.ID, level+1)
				}
			}
		}
		walk(id, 1)
		return h
	}
	removed := make([]model.Group, 0, len(d.RemovedGroups))
	initialDepth := make(map[string]int, len(d.RemovedGroups))
	for _, g := range groups {
		if u.Groups[g.ID] == Removed {
			removed = append(removed, g)
			initialDepth[g.ID] = depthOf(g.ID)
		}
	}
	sort.SliceStable(removed, func(i, j int) bool {
		return initialDepth[removed[i].ID] < initialDepth[removed[j].ID]
	})
	for _, g := range removed {
		if groupParent[g.ID] != "" &&
			depthOf(g.ID)+subtreeHeight(g.ID)-1 > maxGroupDepth {
			delete(groupParent, g.ID)
		}
	}
	for _, g := range groups {
		if p, ok := groupParent[g.ID]; ok {
			groups[index[p]].Children = append(groups[index[p]].Children, g.ID)
		}
	}
	for i := range groups {
		groups[i].Depth = depthOf(groups[i].ID)
	}

	u.Graph = model.Graph{
		Direction: after.Direction,
		Nodes:     nodes, Edges: edges, Groups: groups,
	}
	return u
}
