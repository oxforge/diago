package route

import (
	"math"
	"slices"

	"github.com/oxforge/diago/internal/layout/layered/ports"
)

// straighten closes the jogs a wire does not need (S8): a reversed edge
// whose end attaches at a pinned node's side vertex runs straight on that
// vertex's column, however wide its stops spread, when every other stop
// may move there, a dummy stop only where its row stays clear (dummyRoom);
// a wire whose stops spread no wider than Span runs straight on its
// in-port's x, or else on its out-port's, when every other stop may move
// there: a dummy stop when it is free to, a side column (shifts), a port
// that slides along its face (slides). Each runs straight through unify.
// Otherwise, walking up from the in-port, each dummy stop moves onto the
// stop below it when that is within Span and free (walkUp).
func (r *router) straighten() {
	for e, chain := range r.g.Chains {
		if len(chain) < 2 {
			continue
		}
		w := r.wires[e]
		stops := w.stops
		lo, hi := slices.Min(stops), slices.Max(stops)
		if hi-lo <= r.o.Snap {
			continue
		}
		if column, ok := r.backColumn(e); ok && r.unify(e, w, column, true) {
			continue
		}
		if hi-lo <= r.o.Span && (r.unify(e, w, stops[len(stops)-1], false) || r.unify(e, w, stops[0], false)) {
			continue
		}
		r.walkUp(e, w)
	}
}

// unify runs edge e's wire w straight on target when every stop may move
// there (straighten), and reports whether it did: each end as endMoves
// allows, and each dummy stop farther than Snap from target when it is
// free to; when far, where target may lie beyond Span, a dummy stop only
// where its row stays clear at target too (dummyRoom).
func (r *router) unify(e int, w wire, target float64, far bool) bool {
	stops, last := w.stops, len(w.stops)-1
	var moved [2]bool
	for which, i := range [2]int{0, last} {
		moves, ok := r.endMoves(e, w, which, i, target)
		if !ok {
			return false
		}
		moved[which] = moves
	}
	chain := r.g.Chains[e]
	for i := 1; i < last; i++ {
		if math.Abs(stops[i]-target) > r.o.Snap && !r.free(w, i, target, e) {
			return false
		}
		if far && !r.dummyRoom(chain[i], target) {
			return false
		}
	}
	for which, i := range [2]int{0, last} {
		if moved[which] && r.attach[e][which] == nil {
			r.move(w.channels(i)[0], e, i, target)
		} else if a := r.attach[e][which]; a != nil {
			a.vx = target
		}
	}
	for i := range stops {
		stops[i] = target
		r.hold(e, i, target)
	}
	return true
}

// endMoves reports whether end stop i of edge e's wire w (which, its head
// or its tail) may run on target (unify), and whether it moves to get
// there rather than snap: a side column moves only where it shifts,
// however close the x, never inward of Reach nor into its row's
// clearance; a port farther than Snap from target moves only where it
// slides along its face; and a port within Snap snaps where target lies
// over the middle nine tenths of its face's usable span (onSpan), an
// anchored one only when it lies on target already.
func (r *router) endMoves(e int, w wire, which, i int, target float64) (moves, ok bool) {
	stops := w.stops
	switch {
	case r.attach[e][which] != nil:
		if stops[i] == target {
			return false, true
		}
		return true, r.shifts(w, e, i, which, target)
	case math.Abs(stops[i]-target) > r.o.Snap:
		return true, r.slides(w, e, i, which, target)
	case !r.onSpan(r.end(e, which), which, target):
		return false, false
	case r.ranks[e][which].Anchored && stops[i] != target:
		return false, false
	}
	return false, true
}

// walkUp walks edge e's wire w up from its in-port (straighten): each
// dummy stop moves onto the stop below it when that lies more than Snap
// and no more than Span away and the stop is free to move there.
func (r *router) walkUp(e int, w wire) {
	stops := w.stops
	for i := len(stops) - 2; i >= 1; i-- {
		next := stops[i+1]
		d := math.Abs(stops[i] - next)
		if d <= r.o.Snap || d > r.o.Span || !r.free(w, i, next, e) {
			continue
		}
		stops[i] = next
		r.hold(e, i, next)
	}
}

// backColumn is the column reversed edge e runs along beside the pinned
// node whose side vertex one of its ends attaches at (S8, Stops), if any.
func (r *router) backColumn(e int) (float64, bool) {
	if !r.reversed(e) {
		return 0, false
	}
	for which, a := range r.attach[e] {
		if a != nil && r.pinned(r.end(e, which)) {
			return a.vx, true
		}
	}
	return 0, false
}

// dummyRoom reports whether dummy d's column may move to x beyond Span
// (S8, Stops): at x it keeps the node clearance from every node box of its
// row and InLaneGap from every other dummy of the row (blocked), a side
// column's room beside a pinned node and a loop's beside a node with faces
// included, and InLaneGap from the side columns and stubs of the row's
// nodes (crowded). What lies between its column and x does not matter.
func (r *router) dummyRoom(d int, x float64) bool {
	return !r.blocked(d, x, x, 0) && !r.crowded(d, x, x)
}

// shifts reports whether end stop i of edge e's wire w (which, its head
// or its tail) is a side column that may move to x (S8, Stops): where it
// stays Reach outside its side, keeps its row clear and is free to move,
// and not on a side rail, whose ends share their column.
func (r *router) shifts(w wire, e, i, which int, x float64) bool {
	a := r.attach[e][which]
	if a == nil || r.railed(e, which) {
		return false
	}
	vertex := r.vertexX(r.end(e, which), a.side)
	return sign(a.side)*(x-vertex) >= r.o.Reach && r.rowClear(e, which, x) && r.free(w, i, x, e)
}

// slides reports whether end stop i of edge e's wire w (which, its head or
// its tail) is a port that may slide along its face to x (S8, Stops): x
// lies over the middle nine tenths of the face's usable span, in the text
// profile on a cell inside the box's corners (Span.Inside); the port
// passes no other end of its node and keeps InLaneGap / 2 from each; and
// no other node's port or side column, nor a dummy column, holds an x
// within InLaneGap of it in its channel. A pinned node's vertex, an
// anchored port and a terminal's port (where the parent level anchors the
// wire, S9) never slide, nor does a side column.
func (r *router) slides(w wire, e, i, which int, x float64) bool {
	v := r.end(e, which)
	if r.attach[e][which] != nil || r.pinned(v) || r.ranks[e][which].Anchored || r.node(v).Terminal {
		return false
	}
	lo, hi := r.span(v, which).Inside()
	if off := x - r.x[v]; off < lo || off > hi {
		return false
	}
	c := w.channels(i)[0]
	return !r.passes(c, e, v, w.stops[i], x) && r.clear(c, x, e, v, r.o.InLaneGap/2)
}

// slideEnds closes a jog within Span next to a side attachment by sliding
// its column onto the adjacent stop, when that stays at least Reach
// outside the side, keeps the row clear and is free to move there (S8): a
// column's distance from its side is a floor, not a position. A side
// rail's column never slides: its ends share it. When the column may not
// slide, the run of dummy stops next to it, on one x, moves onto the
// column instead, when each is free to and the stop past the run lies
// more than Span from the column.
func (r *router) slideEnds() {
	g := r.g
	for e, chain := range g.Chains {
		if len(chain) < 2 {
			continue
		}
		w := r.wires[e]
		stops := w.stops
		last := len(stops) - 1
		for _, end := range [2][3]int{{0, 1, ports.Head}, {last, last - 1, ports.Tail}} {
			i, j, which := end[0], end[1], end[2]
			a := r.attach[e][which]
			if a == nil {
				continue
			}
			target := stops[j]
			d := math.Abs(target - stops[i])
			if d <= r.o.Snap || d > r.o.Span {
				continue
			}
			if r.shifts(w, e, i, which, target) {
				stops[i] = target
				r.hold(e, i, target)
				a.vx = target
				r.port[e][which] = target
				continue
			}
			r.pullRun(w, e, i, j)
		}
	}
}

// pullRun moves the run of dummy stops of edge e's wire w that starts at
// stop j, next to end stop i, and holds one x onto stop i's x, when each
// stop of the run is free to move there and the stop past the run lies
// more than Span from it (S8, Stops).
func (r *router) pullRun(w wire, e, i, j int) {
	stops, last := w.stops, len(w.stops)-1
	step := 1
	if j < i {
		step = -1
	}
	k := j
	for k > 0 && k < last && stops[k] == stops[j] {
		if !r.free(w, k, stops[i], e) {
			return
		}
		k += step
	}
	if k == j || math.Abs(stops[k]-stops[i]) <= r.o.Span {
		return
	}
	for m := j; m != k; m += step {
		stops[m] = stops[i]
		r.hold(e, m, stops[i])
	}
}
