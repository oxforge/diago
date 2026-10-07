// Package diff computes structural diffs of diagram specs and the union
// graphs a diff render lays out once (Phase 2b design). It depends on model
// only (schema appears in its tests): layout and rendering happen in the
// pipeline.
package diff

import "github.com/oxforge/diago/internal/model"

// Status is an element's state in the union of before and after.
type Status string

const (
	Added   Status = "added"
	Removed Status = "removed"
	Changed Status = "changed"
	Same    Status = "same"
)

// GraphDiff lists ids by change, each slice in the spec order of the side
// it comes from (after for added/changed, before for removed). NodeFields,
// EdgeFields and GroupFields hold, per changed id, the fields that differ in
// the change list's order; a node changed only in its members has an empty
// entry.
type GraphDiff struct {
	AddedNodes, RemovedNodes, ChangedNodes    []string
	AddedEdges, RemovedEdges, ChangedEdges    []string
	AddedGroups, RemovedGroups, ChangedGroups []string
	NodeFields, EdgeFields, GroupFields       map[string][]FieldChange
}

// Empty reports whether nothing changed.
func (d GraphDiff) Empty() bool {
	return len(d.AddedNodes)+len(d.RemovedNodes)+len(d.ChangedNodes)+
		len(d.AddedEdges)+len(d.RemovedEdges)+len(d.ChangedEdges)+
		len(d.AddedGroups)+len(d.RemovedGroups)+len(d.ChangedGroups) == 0
}

// parentOf maps every node and group id to its parent group id ("" = top).
func parentOf(g *model.Graph) map[string]string {
	p := make(map[string]string, len(g.Nodes)+len(g.Groups))
	for _, grp := range g.Groups {
		for _, id := range grp.Contains {
			p[id] = grp.ID
		}
		for _, id := range grp.Children {
			p[id] = grp.ID
		}
	}
	return p
}

func nodeByID(g *model.Graph) map[string]model.Node {
	m := make(map[string]model.Node, len(g.Nodes))
	for _, n := range g.Nodes {
		m[n.ID] = n
	}
	return m
}

func edgeByID(g *model.Graph) map[string]model.Edge {
	m := make(map[string]model.Edge, len(g.Edges))
	for _, e := range g.Edges {
		m[e.ID] = e
	}
	return m
}

func groupByID(g *model.Graph) map[string]model.Group {
	m := make(map[string]model.Group, len(g.Groups))
	for _, grp := range g.Groups {
		m[grp.ID] = grp
	}
	return m
}

// DiffGraphs computes the structural diff by stable ids. A node is changed
// when label, shape, color, parent or members differ; a group when label or
// color differ; an edge when label, style, direction, color, relation kind,
// cardinality, arrow direction or flat differ under the same id and
// endpoints. An edge whose endpoints moved under the same id is removed +
// added: the union needs one geometry per edge. Every changed element also
// records the fields that differ.
func DiffGraphs(before, after *model.Graph) GraphDiff {
	d := GraphDiff{
		NodeFields: map[string][]FieldChange{}, EdgeFields: map[string][]FieldChange{}, GroupFields: map[string][]FieldChange{},
	}
	bn, an := nodeByID(before), nodeByID(after)
	bp, ap := parentOf(before), parentOf(after)
	bg, ag := groupByID(before), groupByID(after)
	parent := func(parents map[string]string, groups map[string]model.Group, id string) parentRef {
		pid := parents[id]
		return parentRef{id: pid, label: groups[pid].Label}
	}
	for _, n := range after.Nodes {
		o, ok := bn[n.ID]
		switch {
		case !ok:
			d.AddedNodes = append(d.AddedNodes, n.ID)
		case o.Label != n.Label || o.Shape != n.Shape || o.Color != n.Color || bp[n.ID] != ap[n.ID] ||
			membersChanged(o.Members, n.Members):
			d.ChangedNodes = append(d.ChangedNodes, n.ID)
			d.NodeFields[n.ID] = nodeFields(o, n, parent(bp, bg, n.ID), parent(ap, ag, n.ID))
		}
	}
	for _, n := range before.Nodes {
		if _, ok := an[n.ID]; !ok {
			d.RemovedNodes = append(d.RemovedNodes, n.ID)
		}
	}

	be, ae := edgeByID(before), edgeByID(after)
	moved := func(id string) bool {
		o, ok1 := be[id]
		n, ok2 := ae[id]
		return ok1 && ok2 && (o.From != n.From || o.To != n.To)
	}
	for _, e := range after.Edges {
		o, ok := be[e.ID]
		switch {
		case !ok || moved(e.ID):
			d.AddedEdges = append(d.AddedEdges, e.ID)
		case o.Label != e.Label || o.Style != e.Style || o.Direction != e.Direction || o.Color != e.Color ||
			o.Relation != e.Relation || o.FromCard != e.FromCard || o.ToCard != e.ToCard || o.Directed != e.Directed ||
			o.Flat != e.Flat:
			d.ChangedEdges = append(d.ChangedEdges, e.ID)
			d.EdgeFields[e.ID] = edgeFields(o, e)
		}
	}
	for _, e := range before.Edges {
		if _, ok := ae[e.ID]; !ok || moved(e.ID) {
			d.RemovedEdges = append(d.RemovedEdges, e.ID)
		}
	}

	for _, g := range after.Groups {
		o, ok := bg[g.ID]
		switch {
		case !ok:
			d.AddedGroups = append(d.AddedGroups, g.ID)
		case o.Label != g.Label || o.Color != g.Color:
			d.ChangedGroups = append(d.ChangedGroups, g.ID)
			d.GroupFields[g.ID] = groupFields(o, g)
		}
	}
	for _, g := range before.Groups {
		if _, ok := ag[g.ID]; !ok {
			d.RemovedGroups = append(d.RemovedGroups, g.ID)
		}
	}
	return d
}
