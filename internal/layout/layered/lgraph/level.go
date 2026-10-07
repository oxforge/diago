// Package lgraph holds the layered engine's graph types (S0): a Level, one
// scope's nodes and edges in declaration order, and the layered Graph built
// from it (S5).
package lgraph

import "github.com/oxforge/diago/internal/model"

// Node is one node of a level. W and H are its extents in the engine's
// top-to-bottom frame: W on the cross axis, H on the flow axis.
type Node struct {
	ID       string
	Shape    model.Shape
	Record   bool // a class record box (S1)
	Group    bool // a laid-out child group, entering its parent level as a box (S9)
	Terminal bool // where an edge that crosses the level's container meets its border (S9)
	W, H     float64
}

// Edge is one edge of a level. From and To index Level.Nodes.
type Edge struct {
	ID       string
	From, To int
	Anchor   [2]Anchor // at the From end and the To end
}

// Anchor fixes an edge end's port on a group's face (S9): At is the
// cross-axis offset from the group's center of the terminal that carries
// the edge on inside the group.
type Anchor struct {
	On bool
	At float64
}

// Level is one scope's nodes and edges, each in declaration order: the
// whole graph when it has no groups, else the root or a group's direct
// children and their terminals (S9).
type Level struct {
	Nodes []Node
	Edges []Edge
}

// SelfLoop reports whether edge e joins a node to itself.
func (lv *Level) SelfLoop(e int) bool {
	return lv.Edges[e].From == lv.Edges[e].To
}

// SelfLoops counts each node's self-loops, per node in node order.
func (lv *Level) SelfLoops() []int {
	out := make([]int, len(lv.Nodes))
	for e, ed := range lv.Edges {
		if lv.SelfLoop(e) {
			out[ed.From]++
		}
	}
	return out
}

// End returns edge e's anchor at chain end which (ports.Head = 0 for the
// upstream end, ports.Tail = 1 for the downstream one), given the
// reversals (S2).
func (lv *Level) End(e, which int, reversed []bool) Anchor {
	upstream := 0
	if reversed != nil && reversed[e] {
		upstream = 1
	}
	if which == 0 {
		return lv.Edges[e].Anchor[upstream]
	}
	return lv.Edges[e].Anchor[1-upstream]
}

// Oriented returns edge e's endpoints as the layering sees them, upstream
// end first: a reversed edge (S2) runs from To to From. A nil reversed
// reverses nothing.
func (lv *Level) Oriented(e int, reversed []bool) (upper, lower int) {
	ed := lv.Edges[e]
	if reversed != nil && reversed[e] {
		return ed.To, ed.From
	}
	return ed.From, ed.To
}
