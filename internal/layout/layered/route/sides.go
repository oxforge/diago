package route

import (
	"math"
	"slices"

	"github.com/oxforge/diago/internal/layout/layered/ports"
)

// columns settles every side column once every end has attached and every
// self-loop has its side (S8, Side attachments): each column first runs
// Reach out; then, in edge order, each moves out over the x it aims at
// when that column keeps the row clear, an end aiming over its one-hop
// partner's column after that partner, over where the partner's column
// landed. So an end attached early never takes a column that a later
// end's column, Reach out, leaves no room for. The ends of a side rail
// land Reach out and move only as one. Last, in edge order, every column
// that has landed beside another edge's vertical in its channel gives way
// (S8, Ports).
func (r *router) columns() {
	for e := range r.attach {
		for which, a := range r.attach[e] {
			if a != nil {
				a.vx = r.vertexX(r.end(e, which), a.side) + sign(a.side)*r.reach()
				r.port[e][which] = a.vx
			}
		}
	}
	for e := range r.attach {
		for _, which := range [2]int{ports.Head, ports.Tail} {
			if a := r.attach[e][which]; a != nil && !a.partner {
				r.moveOut(e, which)
			}
		}
		for _, which := range [2]int{ports.Head, ports.Tail} {
			if a := r.attach[e][which]; a != nil && a.partner {
				r.moveOut(e, which)
			}
		}
	}
	for e := range r.attach {
		for which, a := range r.attach[e] {
			if a != nil {
				r.giveWay(e, which)
			}
		}
	}
}

// moveOut moves edge e's side column over the x it aims at, when that is
// farther out than its column and keeps the row clear. An end on a side
// rail never moves out on its own.
func (r *router) moveOut(e, which int) {
	a := r.attach[e][which]
	aim := a.aim
	if a.partner {
		aim = r.attach[e][1-which].vx
	}
	v := r.end(e, which)
	vx := r.column(v, a.side, aim)
	if r.railed(e, which) || sign(a.side)*(vx-a.vx) <= 0 || !r.rowClearOut(e, which, vx) {
		return
	}
	a.vx = vx
	r.port[e][which] = vx
}

// railed reports whether edge e's side attachment is on a side rail (S8):
// it shares its pinned node's side vertex with another end, a self-loop's
// among them. Those ends land on one column Reach out, which C9.1 lets
// them share, and move only as one.
func (r *router) railed(e, which int) bool {
	ends, loop := r.onColumn(e, which)
	return len(ends) > 1 || loop
}

// onColumn returns the ends on edge e's side column: that end and, at a
// pinned node's side vertex, every other end attached there, in edge
// order; loop reports whether a self-loop leaves that vertex too.
func (r *router) onColumn(e, which int) (ends []endRef, loop bool) {
	a, v := r.attach[e][which], r.end(e, which)
	ends = []endRef{{e, which}}
	if !r.pinned(v) {
		return ends, false
	}
	for f, fe := range r.attach {
		for w, b := range fe {
			if b != nil && f != e && b.side == a.side && r.end(f, w) == v {
				ends = append(ends, endRef{f, w})
			}
		}
	}
	for _, lj := range r.loopAt {
		if lj.node == v && lj.side == a.side {
			loop = true
		}
	}
	return ends, loop
}

// giveWay moves edge e's side column outward when it lies within
// InLaneGap of another edge's port or dummy column in the channel it
// enters (S8, Ports): by InLaneGap, up to four times, to the first spot
// clear of them that keeps its row clear, and nowhere when there is none.
// A side rail moves as one, clear of them in every channel its ends
// enter, and not at all when a self-loop rises along it, Reach out.
func (r *router) giveWay(e, which int) {
	a := r.attach[e][which]
	ends, loop := r.onColumn(e, which)
	if loop || !r.hemmed(ends, a.vx) {
		return
	}
	for k := 1; k <= 4; k++ {
		x := a.vx + sign(a.side)*float64(k)*r.o.InLaneGap
		if r.rowClear(e, which, x) && !r.hemmed(ends, x) {
			for _, en := range ends {
				r.attach[en.edge][en.which].vx = x
				r.port[en.edge][en.which] = x
			}
			return
		}
	}
}

// hemmed reports whether x lies within InLaneGap of another edge's port or
// dummy column in a channel one of ends enters (S8, Ports): the channel
// below the end's node for a chain's head, above it for its tail. Those
// are another chain's stops in the two layers the channel joins: its port
// where the chain ends, unless that end is a side attachment, which holds
// none, and its dummy's column elsewhere. On a side rail, a stop within
// Snap of x on the wire of another of ends is the rail's own run, not
// another edge's.
func (r *router) hemmed(ends []endRef, x float64) bool {
	g := r.g
	onRail := func(f int) bool {
		return slices.ContainsFunc(ends, func(en endRef) bool { return en.edge == f })
	}
	for _, en := range ends {
		c := g.Vertices[r.end(en.edge, en.which)].Layer
		if en.which == ports.Tail {
			c--
		}
		for f, chain := range g.Chains {
			if f == en.edge || len(chain) < 2 {
				continue
			}
			top := g.Vertices[chain[0]].Layer
			if c < top || c >= top+len(chain)-1 {
				continue // the chain does not run through channel c
			}
			for i := c - top; i <= c-top+1; i++ {
				stop, fw := r.x[chain[i]], -1
				switch i {
				case 0:
					fw = ports.Head
				case len(chain) - 1:
					fw = ports.Tail
				}
				if fw >= 0 {
					if r.attach[f][fw] != nil {
						continue
					}
					stop = r.port[f][fw]
				}
				if onRail(f) && math.Abs(stop-x) <= r.o.Snap {
					continue
				}
				if math.Abs(stop-x) < r.o.InLaneGap {
					return true
				}
			}
		}
	}
	return false
}

// rowClear reports whether edge e's side column at vx and its stub keep
// the row clear (S8): the node clearance from every other vertex of the
// row, and InLaneGap from every side column and stub of the row's other
// nodes, crossing none.
func (r *router) rowClear(e, which int, vx float64) bool {
	v := r.end(e, which)
	vertex := r.vertexX(v, r.attach[e][which].side)
	return !r.blocked(v, vx, vertex, 0) && !r.crowded(v, vx, vertex)
}

// rowClearOut is rowClear for a column moving out over the x it aims at,
// where a vertex exactly at the node clearance, within noise, keeps the
// row clear: a room S7 kept is solved to exactly the clearance (S8, Side
// attachments).
func (r *router) rowClearOut(e, which int, vx float64) bool {
	v := r.end(e, which)
	vertex := r.vertexX(v, r.attach[e][which].side)
	return !r.blocked(v, vx, vertex, noise) && !r.crowded(v, vx, vertex)
}

// crowded reports whether a side attachment of node v, its stub from
// vertex to the column vx and the column, would come within InLaneGap of
// the stub or the column of a side attachment of another node of v's row,
// or cross it. Two attachments meet only between two neighbors, facing
// each other, so it is enough that their stubs keep InLaneGap apart.
func (r *router) crowded(v int, vx, vertex float64) bool {
	lo, hi := math.Min(vx, vertex), math.Max(vx, vertex)
	layer := r.g.Vertices[v].Layer
	for e, ends := range r.attach {
		for which, a := range ends {
			if a == nil {
				continue
			}
			o := r.end(e, which)
			if o == v || r.g.Vertices[o].Layer != layer {
				continue
			}
			ov := r.vertexX(o, a.side)
			if lo < math.Max(a.vx, ov)+r.o.InLaneGap && hi > math.Min(a.vx, ov)-r.o.InLaneGap {
				return true
			}
		}
	}
	return false
}

// usable reports whether a side attachment may take node v's side: no
// other end holds it, and it is not the right face of a node whose
// self-loop runs there.
func (r *router) usable(v int, side ports.Vertex) bool {
	return r.taken[v][slot(side)] == 0 && (side != ports.Right || len(r.loops[v]) == 0)
}

// vertexX is the x of node v's side vertex or side face.
func (r *router) vertexX(v int, side ports.Vertex) float64 {
	return r.x[v] + sign(side)*r.node(v).W/2
}

// reach is how far a side column sits outside its side (S8).
func (r *router) reach() float64 { return ports.Reach(r.o.Reach, r.o.Text) }

// columnFor is where an end attached at node v's side first puts its
// column: over aim (column), or the column Reach out when that one and its
// stub would not keep the node clearance from the row's other vertices;
// blocked reports that fallback.
func (r *router) columnFor(v int, side ports.Vertex, aim float64) (vx float64, blocked bool) {
	vertex := r.vertexX(v, side)
	vx = r.column(v, side, aim)
	if r.blocked(v, vx, vertex, 0) {
		return vertex + sign(side)*r.reach(), true
	}
	return vx, false
}

// column is the approach column on node v's side toward aim: reach outside
// the side, or at aim when that is farther out.
func (r *router) column(v int, side ports.Vertex, aim float64) float64 {
	clear := r.vertexX(v, side) + sign(side)*r.reach()
	if side == ports.Right {
		return math.Max(aim, clear)
	}
	return math.Min(aim, clear)
}

// blocked reports whether a side attachment of node v, a stub from vertex
// to the column vx at mid-height and the column itself, would come within
// the node clearance of another vertex of v's row by more than slack: 0
// takes the clearance as computed, noise lets a vertex exactly at it,
// where S7 solves a room to, keep the row clear. A node with self-loops
// reaches as far out as its outermost loop leg: a node with faces that
// far right of its right face (ports.LoopOut), a pinned node Reach beyond
// the side vertex its loop leaves from (S8, one or both sides when it
// loops both ways). At a pinned node, whose side attachments always
// happen, and for a dummy's own column (dummyRoom), a dummy of the row is
// a wire: the stub and the column keep InLaneGap from its column, not the
// node clearance from its box.
func (r *router) blocked(v int, vx, vertex, slack float64) bool {
	g := r.g
	lo, hi := math.Min(vx, vertex), math.Max(vx, vertex)
	wires := r.pinned(v) || g.Dummy(v)
	for _, o := range g.Layers[g.Vertices[v].Layer] {
		if o == v {
			continue
		}
		half := g.Vertices[o].W / 2
		left, right := r.x[o]-half-r.o.Clearance, r.x[o]+half+r.o.Clearance
		switch {
		case wires && g.Dummy(o):
			left, right = r.x[o]-r.o.InLaneGap, r.x[o]+r.o.InLaneGap
		case r.pinned(o):
			if r.loopWide[o][slot(ports.Right)] {
				right += r.o.Reach
			}
			if r.loopWide[o][slot(ports.Left)] {
				left -= r.o.Reach
			}
		case !g.Dummy(o) && len(r.loops[o]) > 0:
			right += ports.LoopOut(r.o.Reach, r.o.InLaneGap, 0, len(r.loops[o]), r.o.Text)
		}
		if (vx > left+slack && vx < right-slack) || (lo < right-slack && hi > left+slack) {
			return true
		}
	}
	return false
}
