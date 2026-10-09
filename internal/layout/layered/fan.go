package layered

import (
	"math"

	"github.com/oxforge/diago/internal/layout/layered/route"
	"github.com/oxforge/diago/internal/model"
)

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
