package diff

import (
	"sort"

	"github.com/oxforge/diago/internal/model"
)

// TextPrefix is the marker text art stamps on a non-same label.
func TextPrefix(s Status) string {
	switch s {
	case Added:
		return "+ "
	case Removed:
		return "- "
	case Changed:
		return "~ "
	}
	return ""
}

// StampLabels returns a copy of the union graph whose node and group labels
// carry their status prefix. It runs before layout so text boxes are
// measured wide enough; edges carry no marker (a wire cannot hold one) and
// are named by the legend instead. For every class node it also sets
// Member.Mark on every member: the status column of the class text diff,
// "  " for same.
func StampLabels(u UnionGraph) model.Graph {
	g := u.Graph
	g.Nodes = append([]model.Node(nil), u.Graph.Nodes...)
	for i := range g.Nodes {
		n := &g.Nodes[i]
		n.Label = TextPrefix(u.Nodes[n.ID]) + n.Label
		if n.Members == nil {
			continue
		}
		mu, hasUnion := u.Members[n.ID]
		mark := func(comp []Status, k int) string {
			if hasUnion && k < len(comp) && comp[k] != Same {
				return TextPrefix(comp[k])
			}
			return "  "
		}
		members := *n.Members
		members.Attributes = append([]model.Member(nil), n.Members.Attributes...)
		members.Methods = append([]model.Member(nil), n.Members.Methods...)
		for k := range members.Attributes {
			members.Attributes[k].Mark = mark(mu.AttrStatus, k)
		}
		for k := range members.Methods {
			members.Methods[k].Mark = mark(mu.MethodStatus, k)
		}
		n.Members = &members
	}
	g.Groups = append([]model.Group(nil), u.Graph.Groups...)
	for i := range g.Groups {
		g.Groups[i].Label = TextPrefix(u.Groups[g.Groups[i].ID]) + g.Groups[i].Label
	}
	return g
}

// FlowLegend lists every change, one line each, in the fixed order
// added / removed / changed × node / edge / group. Removed elements are
// named by their before labels; edges by their endpoints' labels, in the
// orientation the spec wrote them (a swapped relation is named back). A
// class union names its kinds class / relation / package and appends one
// line per changed member, in node id order.
func FlowLegend(u UnionGraph) []string {
	label := func(id string, removed bool) string {
		if removed {
			if l, ok := u.BeforeLabels[id]; ok {
				return l
			}
		}
		for _, n := range u.Graph.Nodes {
			if n.ID == id {
				return n.Label
			}
		}
		for _, g := range u.Graph.Groups {
			if g.ID == id {
				return g.Label
			}
		}
		return id
	}
	edgeName := func(id string, removed bool) string {
		for _, e := range u.Graph.Edges {
			if e.ID == id {
				from, to := e.From, e.To
				if e.Relation.Swapped() {
					from, to = to, from
				}
				return label(from, removed) + " -> " + label(to, removed)
			}
		}
		return id
	}
	kindNode, kindEdge, kindGroup := "node", "edge", "group"
	if u.Class {
		kindNode, kindEdge, kindGroup = "class", "relation", "package"
	}
	var out []string
	add := func(verb, kind, name string) { out = append(out, verb+": "+kind+" "+name) }
	d := u.Diff
	for _, id := range d.AddedNodes {
		add("added", kindNode, label(id, false))
	}
	for _, id := range d.AddedEdges {
		add("added", kindEdge, edgeName(id, false))
	}
	for _, id := range d.AddedGroups {
		add("added", kindGroup, label(id, false))
	}
	for _, id := range d.RemovedNodes {
		add("removed", kindNode, label(id, true))
	}
	for _, id := range d.RemovedEdges {
		add("removed", kindEdge, edgeName(removedUnionID(u, id), true))
	}
	for _, id := range d.RemovedGroups {
		add("removed", kindGroup, label(id, true))
	}
	for _, id := range d.ChangedNodes {
		add("changed", kindNode, label(id, false))
	}
	for _, id := range d.ChangedEdges {
		add("changed", kindEdge, edgeName(id, false))
	}
	for _, id := range d.ChangedGroups {
		add("changed", kindGroup, label(id, false))
	}
	if u.Class {
		ids := make([]string, 0, len(u.Members))
		for id := range u.Members {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		noun := func(compartment string) string {
			if compartment == "methods" {
				return "method"
			}
			return "attribute"
		}
		for _, id := range ids {
			for _, r := range u.Members[id].Records {
				out = append(out, "changed: "+kindNode+" "+label(id, false)+
					", "+string(r.Status)+" "+noun(r.Compartment)+" "+memberText(r.Member))
			}
		}
	}
	return out
}

// removedUnionID maps a removed before-edge id to the id it holds in the
// union (suffixed when after reused the id).
func removedUnionID(u UnionGraph, id string) string {
	if s, ok := u.Edges[id]; ok && s == Removed {
		return id
	}
	for candidate := id + "__removed"; ; candidate += "__removed" {
		if s, ok := u.Edges[candidate]; ok && s == Removed {
			return candidate
		}
		if _, ok := u.Edges[candidate]; !ok {
			return id
		}
	}
}
