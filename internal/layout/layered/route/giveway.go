package route

import (
	"cmp"
	"context"
	"math"
	"slices"

	"github.com/oxforge/diago/internal/layout/layered/ports"
	"github.com/oxforge/diago/internal/layoutdbg"
)

// deconflict moves a port that lands within InLaneGap of another node's
// port or side column, or of a dummy column, in its channel, where their
// verticals could run side by side (S8): to the spot it gives way to. The
// ports of one node, a rake, may sit closer than the in-lane gap. A spot
// that passes another end of its node inverts two ports, which no lane
// order undoes: when the port takes one, the ports of its face take their
// positions in the order they held. A pinned vertex and an anchored port
// (S9) never move, nor does a side column here: it gave way already, in
// columns, and a port moves off one that found no spot there; but the
// column of the exit whose first hop ends on the port follows it (follow).
func (r *router) deconflict(ctx context.Context) {
	for e, chain := range r.g.Chains {
		if len(chain) < 2 {
			continue
		}
		w := r.wires[e]
		for _, which := range [2]int{ports.Head, ports.Tail} {
			i, v := 0, chain[0]
			if which == ports.Tail {
				i, v = len(w.stops)-1, chain[len(chain)-1]
			}
			if r.pinned(v) || r.attach[e][which] != nil || r.ranks[e][which].Anchored {
				continue
			}
			c, x0 := w.channels(i)[0], w.stops[i]
			if r.clear(c, x0, e, v, 0) {
				continue
			}
			x, passes, ok := r.spot(c, e, v, which, x0)
			if !ok {
				continue
			}
			r.move(c, e, i, x)
			reordered := 0
			if passes {
				reordered = r.orderFace(c, v, which)
			}
			followed := which == ports.Tail && r.follow(c, e)
			layoutdbg.Decision(ctx, "port_gave_way",
				"phase", "route", "module", "diago", "spec_ref", "S8",
				"edge", r.g.Level.Edges[e].ID, "end", which, "from", x0, "to", x, "passes", passes, "reordered", reordered,
				"column_followed", followed)
		}
	}
}

// follow moves the column of edge e, a one-hop exit that leaves a pinned
// node's side vertex alone (no side rail), over its in-port, which gave
// way in channel c, or Reach out when the port lands inside Reach, when
// the column keeps its row clear there (S8, Ports):
// the column aims over the in-port its first hop ends on, where that port
// lands. It reports whether the column moved.
func (r *router) follow(c, e int) bool {
	chain := r.g.Chains[e]
	a := r.attach[e][ports.Head]
	if len(chain) != 2 || a == nil || !r.pinned(chain[0]) || r.reversed(e) || r.railed(e, ports.Head) {
		return false
	}
	w := r.wires[e]
	vx := r.column(chain[0], a.side, w.stops[len(w.stops)-1])
	if vx == a.vx || !r.rowClear(e, ports.Head, vx) {
		return false
	}
	a.vx = vx
	r.port[e][ports.Head] = vx
	r.move(c, e, 0, vx)
	return true
}

// spot returns the spot that a port of edge e on node v (which: its head
// or its tail), at x0 in channel c, gives way to (S8, Ports): of the
// spots the in-lane gap apart, up to four times, right before left, that
// lie over the middle nine tenths of its face's usable span, are clear of
// other nodes' ports and side columns and of dummy columns and keep half
// the in-lane gap from v's other ends, the first that passes no other end
// of v, else the first, which passes one. ok is false when no spot is
// clear.
func (r *router) spot(c, e, v, which int, x0 float64) (x float64, passes, ok bool) {
	for k := 1; k <= 4; k++ {
		for _, dir := range [2]float64{1, -1} {
			at := x0 + dir*float64(k)*r.o.InLaneGap
			if !r.onSpan(v, which, at) || !r.clear(c, at, e, v, r.o.InLaneGap/2) {
				continue
			}
			if !r.passes(c, e, v, x0, at) {
				return at, false, true
			}
			if !ok {
				x, passes, ok = at, true, true
			}
		}
	}
	return x, passes, ok
}

// passes reports whether a port of edge e on node v that moves from x0 to
// x in channel c passes another end of v there, or lands on one (S8,
// Stops).
func (r *router) passes(c, e, v int, x0, x float64) bool {
	for _, cl := range r.claims[c] {
		if cl.edge != e && cl.node == v && (cl.x-x0)*(cl.x-x) <= 0 {
			return true
		}
	}
	return false
}

// orderFace gives the ports on node v's face in channel c (which: its
// out-face for heads, its in-face for tails) their positions in the order
// they held before any gave way (S8, Ports): the positions sorted, handed
// out in the order of the ports' slots after the bundles, ties by edge
// order. An end attached at a side holds no port, and an anchored port
// never moves. It returns how many ports took another position.
func (r *router) orderFace(c, v, which int) int {
	type end struct {
		edge, stop int
		x          float64
	}
	var ends []end
	for _, cl := range r.claims[c] {
		if cl.node == v && r.attach[cl.edge][which] == nil && !r.ranks[cl.edge][which].Anchored {
			ends = append(ends, end{cl.edge, cl.stop, cl.x})
		}
	}
	xs := make([]float64, len(ends))
	for k, en := range ends {
		xs[k] = en.x
	}
	slices.Sort(xs)
	slices.SortStableFunc(ends, func(a, b end) int {
		return cmp.Or(cmp.Compare(r.port[a.edge][which], r.port[b.edge][which]), cmp.Compare(a.edge, b.edge))
	})
	moved := 0
	for k, en := range ends {
		if en.x != xs[k] {
			r.move(c, en.edge, en.stop, xs[k])
			moved++
		}
	}
	return moved
}

// clear reports whether no other edge holds an x in channel c within
// InLaneGap of x at a port or side column of a node other than v or at a
// dummy column, or within rake at another end on v.
func (r *router) clear(c int, x float64, e, v int, rake float64) bool {
	for _, cl := range r.claims[c] {
		if cl.edge == e {
			continue
		}
		gap := r.o.InLaneGap
		if cl.node == v {
			gap = rake
		}
		if math.Abs(cl.x-x) < gap {
			return false
		}
	}
	return true
}

// move sets stop i of edge e, a port holding channel c, to x.
func (r *router) move(c, e, i int, x float64) {
	r.wires[e].stops[i] = x
	for k := range r.claims[c] {
		if cl := &r.claims[c][k]; cl.edge == e && cl.stop == i {
			cl.x = x
		}
	}
}

// free reports whether stop i of edge e's wire w may move to x (S8,
// Stops): no other edge holds an x within InLaneGap of it in the stop's
// channels, and it keeps InLaneGap from every self-loop leg of its row,
// which no claim records.
func (r *router) free(w wire, i int, x float64, e int) bool {
	for _, c := range w.channels(i) {
		for _, cl := range r.claims[c] {
			if cl.edge != e && math.Abs(cl.x-x) < r.o.InLaneGap {
				return false
			}
		}
	}
	return !r.nearLoop(w.start+i, x)
}

// nearLoop reports whether x lies within InLaneGap of a self-loop leg in
// layer l's row: a leg of a node with faces, ports.LoopOut right of its
// right face, or the column a pinned node's loop rises along (S8).
func (r *router) nearLoop(l int, x float64) bool {
	for _, v := range r.g.Layers[l] {
		if r.g.Dummy(v) || r.pinned(v) {
			continue
		}
		n := len(r.loops[v])
		for j := range n {
			leg := r.vertexX(v, ports.Right) + ports.LoopOut(r.o.Reach, r.o.InLaneGap, j, n, r.o.Text)
			if math.Abs(leg-x) < r.o.InLaneGap {
				return true
			}
		}
	}
	for _, lj := range r.loopAt {
		if r.g.Vertices[lj.node].Layer == l && math.Abs(lj.sx-x) < r.o.InLaneGap {
			return true
		}
	}
	return false
}

// withinFace reports whether x lies over the middle nine tenths of node
// v's face.
func (r *router) withinFace(v int, x float64) bool {
	return math.Abs(x-r.x[v]) <= 0.9*r.node(v).W/2
}

// onSpan reports whether x lies over the middle nine tenths of the usable
// span (S8, Shape ports) of the face of node v that a chain end takes: the
// out-face for its head, the in-face for its tail. Off the span a port
// would sit on a cylinder's cap or a hexagon's or parallelogram's angled
// side, where no stub along the flow axis leaves its side (C7).
func (r *router) onSpan(v, which int, x float64) bool {
	s, out := r.faces(v)
	if which == ports.Head {
		s = out
	}
	margin := (s.Hi - s.Lo) / 20 // a twentieth at each end
	off := x - r.x[v]
	return off >= s.Lo+margin && off <= s.Hi-margin
}
