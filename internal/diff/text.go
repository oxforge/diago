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

// FlowChanges lists every change, one line each, in the fixed order
// added / removed / changed × node / edge / group. Removed elements are
// named by their before labels; edges by their endpoints' labels, in the
// orientation the spec wrote them (a swapped relation is named back). A
// changed line names the fields that differ. A class union names its kinds
// class / relation / package (a moved class's parent field is package) and
// appends one changed line per changed member, in node id order.
func FlowChanges(u UnionGraph) []Change {
	label := func(id string, removed bool) string {
		if removed {
			if l, ok := u.BeforeLabels[id]; ok {
				return oneLine(l)
			}
		}
		for _, n := range u.Graph.Nodes {
			if n.ID == id {
				return oneLine(n.Label)
			}
		}
		for _, g := range u.Graph.Groups {
			if g.ID == id {
				return oneLine(g.Label)
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
	nodeFields := func(id string) []FieldChange {
		fs := u.Diff.NodeFields[id]
		if !u.Class {
			return fs
		}
		out := make([]FieldChange, len(fs))
		for i, f := range fs {
			if f.Field == "group" {
				f.Field = "package"
			}
			out[i] = f
		}
		return out
	}
	var out []Change
	add := func(s Status, wire bool, kind, name, suffix string) {
		out = append(out, Change{Status: s, Wire: wire, Text: string(s) + ": " + kind + " " + name + suffix})
	}
	d := u.Diff
	for _, id := range d.AddedNodes {
		add(Added, false, kindNode, label(id, false), "")
	}
	for _, id := range d.AddedEdges {
		add(Added, true, kindEdge, edgeName(id, false), "")
	}
	for _, id := range d.AddedGroups {
		add(Added, false, kindGroup, label(id, false), "")
	}
	for _, id := range d.RemovedNodes {
		add(Removed, false, kindNode, label(id, true), "")
	}
	for _, id := range d.RemovedEdges {
		add(Removed, true, kindEdge, edgeName(removedUnionID(u, id), true), "")
	}
	for _, id := range d.RemovedGroups {
		add(Removed, false, kindGroup, label(id, true), "")
	}
	for _, id := range d.ChangedNodes {
		fields := nodeFields(id)
		if len(fields) == 0 && len(u.Members[id].Records) > 0 {
			continue // a members-only change: its member lines name the class
		}
		add(Changed, false, kindNode, label(id, false), changedSuffix(fields))
	}
	for _, id := range d.ChangedEdges {
		add(Changed, true, kindEdge, edgeName(id, false), changedSuffix(d.EdgeFields[id]))
	}
	for _, id := range d.ChangedGroups {
		add(Changed, false, kindGroup, label(id, false), changedSuffix(d.GroupFields[id]))
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
				add(Changed, false, kindNode, label(id, false)+", "+string(r.Status)+" "+noun(r.Compartment)+" "+memberText(r.Member), "")
			}
		}
	}
	return out
}

// FlowLegend is the text diff's footer: FlowChanges' lines.
func FlowLegend(u UnionGraph) []string { return Texts(FlowChanges(u)) }

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
