package diff

import (
	"strconv"
	"strings"

	"github.com/oxforge/diago/internal/model"
)

// DriftKind is what a Drift is: a flow node or a class, a flow edge or a
// class relation, a flow group or a class package, or a sequence actor.
type DriftKind string

const (
	DriftNode  DriftKind = "node"
	DriftEdge  DriftKind = "edge"
	DriftGroup DriftKind = "group"
	DriftActor DriftKind = "actor"
)

// Drift is an element the diff adds while it removes another of the same
// kind that reads the same: a node, group or actor with the same label, an
// edge with the same ends (by their labels) and the same label. Most likely
// it is one thing whose id changed between the versions, which the diff
// then draws twice, once removed and once added.
type Drift struct {
	Kind     DriftKind
	AfterID  string // the added element's id
	BeforeID string // the removed element's id
	Name     string // the label both carry; for an edge, its ends' labels as "From -> To"
}

// driftSide is one removed or added element: its id, what it reads as (""
// never pairs) and the name a Drift reports.
type driftSide struct{ id, key, name string }

// GraphDrift returns a flow or class diff's drifted nodes, then edges, then
// groups, each in after's spec order. An edge removed and added under one
// id (its ends moved) is not drift, nor is an edge with a derived id: it has
// no id of its own to keep (edge-without-id covers it).
func GraphDrift(before, after *model.Graph) []Drift {
	d := DiffGraphs(before, after)
	bn, an := nodeByID(before), nodeByID(after)
	be, ae := edgeByID(before), edgeByID(after)
	bg, ag := groupByID(before), groupByID(after)

	nodes := func(ids []string, byID map[string]model.Node) []driftSide {
		out := make([]driftSide, len(ids))
		for i, id := range ids {
			out[i] = driftSide{id: id, key: byID[id].Label, name: byID[id].Label}
		}
		return out
	}
	edges := func(ids []string, byID map[string]model.Edge, ends map[string]model.Node) []driftSide {
		out := make([]driftSide, len(ids))
		for i, id := range ids {
			e := byID[id]
			name := ends[e.From].Label + " -> " + ends[e.To].Label
			out[i] = driftSide{id: id, name: name}
			if !derivedID(e) {
				out[i].key = name + "\x00" + e.Label
			}
		}
		return out
	}
	groups := func(ids []string, byID map[string]model.Group) []driftSide {
		out := make([]driftSide, len(ids))
		for i, id := range ids {
			out[i] = driftSide{id: id, key: byID[id].Label, name: byID[id].Label}
		}
		return out
	}

	var out []Drift
	out = append(out, pairDrift(DriftNode, nodes(d.RemovedNodes, bn), nodes(d.AddedNodes, an))...)
	out = append(out, pairDrift(DriftEdge, edges(d.RemovedEdges, be, bn), edges(d.AddedEdges, ae, an))...)
	out = append(out, pairDrift(DriftGroup, groups(d.RemovedGroups, bg), groups(d.AddedGroups, ag))...)
	return out
}

// SequenceDrift returns a sequence diff's drifted actors in after's spec
// order. Messages carry no ids: they follow their actors.
func SequenceDrift(before, after *model.SequenceDiagram) []Drift {
	inBefore := make(map[string]bool, len(before.Actors))
	for _, a := range before.Actors {
		inBefore[a.ID] = true
	}
	inAfter := make(map[string]bool, len(after.Actors))
	for _, a := range after.Actors {
		inAfter[a.ID] = true
	}
	var removed, added []driftSide
	for _, a := range before.Actors {
		if !inAfter[a.ID] {
			removed = append(removed, driftSide{id: a.ID, key: a.Label, name: a.Label})
		}
	}
	for _, a := range after.Actors {
		if !inBefore[a.ID] {
			added = append(added, driftSide{id: a.ID, key: a.Label, name: a.Label})
		}
	}
	return pairDrift(DriftActor, removed, added)
}

// derivedID reports whether e's id has the derived form from->to#n for its
// own ends, the id an edge without one takes.
func derivedID(e model.Edge) bool {
	n, ok := strings.CutPrefix(e.ID, e.From+"->"+e.To+"#")
	if !ok {
		return false
	}
	_, err := strconv.Atoi(n)
	return err == nil
}

// pairDrift pairs every added element with the first unpaired removed one
// that reads the same, both in spec order. An id on both sides is one
// element that moved, never drift.
func pairDrift(kind DriftKind, removed, added []driftSide) []Drift {
	addedIDs := make(map[string]bool, len(added))
	for _, a := range added {
		addedIDs[a.id] = true
	}
	queue := make(map[string][]string, len(removed))
	removedIDs := make(map[string]bool, len(removed))
	for _, r := range removed {
		removedIDs[r.id] = true
		if r.key != "" && !addedIDs[r.id] {
			queue[r.key] = append(queue[r.key], r.id)
		}
	}
	var out []Drift
	for _, a := range added {
		ids := queue[a.key]
		if a.key == "" || removedIDs[a.id] || len(ids) == 0 {
			continue
		}
		queue[a.key] = ids[1:]
		out = append(out, Drift{Kind: kind, AfterID: a.id, BeforeID: ids[0], Name: a.name})
	}
	return out
}
