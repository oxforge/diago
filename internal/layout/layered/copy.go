package layered

import (
	"slices"

	"github.com/oxforge/diago/internal/layout/layered/lgraph"
	"github.com/oxforge/diago/internal/layout/layered/route"
	"github.com/oxforge/diago/internal/layout/layered/size"
	"github.com/oxforge/diago/internal/model"
)

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
			w.ID = model.Slot(lv.Edges[w.Edge].ID, vx.Layer)
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
