package route

import (
	"cmp"
	"context"
	"math"
	"slices"

	"github.com/oxforge/diago/internal/layout/layered/ports"
	"github.com/oxforge/diago/internal/layoutdbg"
)

// wire is an edge's x stops, one per layer from its chain's head: the
// out-port, the dummy columns, the in-port.
type wire struct {
	start int // the layer of the chain's head
	stops []float64
}

// claim is an x that a wire's vertical holds in a channel: a port's, on
// its node, or a dummy column's (node -1).
type claim struct {
	x          float64
	edge, stop int
	node       int
}

// channels returns the channels stop i of w runs through: the one above
// it and the one below it, where they exist.
func (w wire) channels(i int) []int {
	var out []int
	if i > 0 {
		out = append(out, w.start+i-1)
	}
	if i < len(w.stops)-1 {
		out = append(out, w.start+i)
	}
	return out
}

// pinnedEnd reports whether edge e's end is a vertex of a pinned node that
// is not a side attachment: the in-vertex or the bottom, which never moves.
func (r *router) pinnedEnd(e, which int) bool {
	chain := r.g.Chains[e]
	v := chain[0]
	if which == ports.Tail {
		v = chain[len(chain)-1]
	}
	return r.pinned(v) && r.attach[e][which] == nil
}

// fixedEnd reports whether edge e's end keeps its x when a stop within
// Snap meets it (S8): a pinned node's vertex, which never moves, a side
// column, which keeps Reach outside its side, or an anchored port on a
// group's face, which meets its terminal inside the group (S9).
func (r *router) fixedEnd(e, which int) bool {
	return r.pinnedEnd(e, which) || r.attach[e][which] != nil || r.ranks[e][which].Anchored
}

// noise is the router's float-noise tolerance, no distance. Two stops
// within a millionth of a unit are one x, whatever holds them, since two
// anchored ports on one column compute that x along different sums (S9);
// a jog that small takes no lane; and a vertex within it of the node
// clearance keeps the row clear for a column moving out over its port (S8,
// Side attachments).
const noise = 1e-6

// anchoredIn reports whether stops lo..hi of edge e's wire hold an end
// anchored on a group's face (S9), which never gives way.
func (r *router) anchoredIn(e, lo, hi, last int) bool {
	return (lo == 0 && r.ranks[e][ports.Head].Anchored) || (hi == last && r.ranks[e][ports.Tail].Anchored)
}

// stuckIn reports whether stops lo..hi of edge e's wire hold an end that
// never moves: an anchored port or a pinned node's vertex.
func (r *router) stuckIn(e, lo, hi, last int) bool {
	return r.anchoredIn(e, lo, hi, last) || (lo == 0 && r.pinnedEnd(e, ports.Head)) || (hi == last && r.pinnedEnd(e, ports.Tail))
}

// buildWires lays every edge's stops and snaps consecutive stops within
// Snap onto one x, toward a fixed end: the head's x runs forward, unless
// the tail is fixed and the head is not (or the edge is reversed, whose
// arrowhead is its head). An anchored end's x always runs (S9), but never
// moves a side column inward; there, and between two ends that never
// move, the jog stays, beyond float noise.
func (r *router) buildWires() {
	g := r.g
	for e, chain := range g.Chains {
		if len(chain) < 2 {
			continue
		}
		stops := []float64{r.port[e][ports.Head]}
		for _, d := range chain[1 : len(chain)-1] {
			stops = append(stops, r.x[d])
		}
		stops = append(stops, r.port[e][ports.Tail])
		headFixed, tailFixed := r.fixedEnd(e, ports.Head), r.fixedEnd(e, ports.Tail)
		last := len(stops) - 1
		for i := 0; i < last; i++ {
			d := math.Abs(stops[i] - stops[i+1])
			if d > r.o.Snap {
				continue
			}
			upper, lower := r.anchoredIn(e, i, i, last), r.anchoredIn(e, i+1, i+1, last)
			if d > noise && ((upper && (r.stuckIn(e, i+1, i+1, last) || r.inward(e, stops, i+1, stops[i]))) ||
				(lower && (r.stuckIn(e, i, i, last) || r.inward(e, stops, i, stops[i+1])))) {
				continue
			}
			tailWins := i+1 == last && tailFixed && (!r.reversed(e) || i != 0 || !headFixed)
			if lower {
				tailWins = true
			} else if upper {
				tailWins = false
			}
			if tailWins {
				stops[i] = stops[i+1]
			} else {
				stops[i+1] = stops[i]
			}
		}
		r.wires[e] = wire{start: g.Vertices[chain[0]].Layer, stops: stops}
		for i := range stops {
			r.hold(e, i, stops[i])
		}
	}
}

// hold records that stop i of edge e's wire holds x in its channels.
func (r *router) hold(e, i int, x float64) {
	w := r.wires[e]
	node := -1
	switch i {
	case 0:
		node = r.g.Chains[e][0]
	case len(w.stops) - 1:
		node = r.g.Chains[e][len(r.g.Chains[e])-1]
	}
	for _, c := range w.channels(i) {
		r.claims[c] = append(r.claims[c], claim{x: x, edge: e, stop: i, node: node})
	}
}

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

// straighten closes the jogs a wire does not need (S8): a reversed edge
// whose end attaches at a pinned node's side vertex runs straight on that
// vertex's column, however wide its stops spread, when every other stop
// may move there, a dummy stop only where its row stays clear (dummyRoom);
// a wire whose stops spread no wider than Span runs straight on its
// in-port's x, or else on its out-port's, when every other stop may move
// there: a dummy stop when it is free to, a side column (shifts), a port
// that slides along its face (slides). Otherwise, walking up from the
// in-port, each dummy stop moves onto the stop below it when that is
// within Span and free.
func (r *router) straighten() {
	g := r.g
	for e, chain := range g.Chains {
		if len(chain) < 2 {
			continue
		}
		w := r.wires[e]
		stops := w.stops
		last := len(stops) - 1
		lo, hi := slices.Min(stops), slices.Max(stops)
		if hi-lo <= r.o.Snap {
			continue
		}
		// far: target may lie beyond Span, where a dummy stop moves only
		// where its row stays clear
		unify := func(target float64, far bool) bool {
			var moved [2]bool
			for which, i := range [2]int{0, last} {
				switch {
				case r.attach[e][which] != nil:
					// a side column moves only where it may, however
					// close the x: never inward of Reach, nor into its
					// row's clearance
					if stops[i] != target {
						if !r.shifts(w, e, i, which, target) {
							return false
						}
						moved[which] = true
					}
				case math.Abs(stops[i]-target) > r.o.Snap:
					if !r.slides(w, e, i, which, target) {
						return false
					}
					moved[which] = true
				case !r.withinFace(r.end(e, which), target):
					return false
				case r.ranks[e][which].Anchored && stops[i] != target:
					return false
				}
			}
			for i := 1; i < last; i++ {
				if math.Abs(stops[i]-target) > r.o.Snap && !r.free(w, i, target, e) {
					return false
				}
				if far && !r.dummyRoom(chain[i], stops[i], target) {
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
		if column, ok := r.backColumn(e); ok && unify(column, true) {
			continue
		}
		if hi-lo <= r.o.Span && (unify(stops[len(stops)-1], false) || unify(stops[0], false)) {
			continue
		}
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

// dummyRoom reports whether dummy d's column may move from x0 to x
// beyond Span (S8, Stops): its row stays clear, the node clearance from
// every node box between and at the two, a side column's room beside a
// pinned node and a loop's beside a node with faces included (blocked),
// and InLaneGap from the side columns and stubs of the row's nodes
// (crowded).
func (r *router) dummyRoom(d int, x0, x float64) bool {
	return !r.blocked(d, x, x0, 0) && !r.crowded(d, x, x0)
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
// profile on a cell inside the box's corners; the port passes no other
// end of its node and keeps InLaneGap / 2 from each; and no other node's
// port or side column, nor a dummy column, holds an x within InLaneGap of
// it in its channel. A pinned node's vertex, an anchored port and a
// terminal's port (where the parent level anchors the wire, S9) never
// slide, nor does a side column.
func (r *router) slides(w wire, e, i, which int, x float64) bool {
	v := r.end(e, which)
	if r.attach[e][which] != nil || r.pinned(v) || r.ranks[e][which].Anchored || r.node(v).Terminal || !r.onSpan(v, which, x) {
		return false
	}
	if r.o.Text && math.Abs(x-r.x[v]) >= r.node(v).W/2-1 {
		return false // on the box's border cell: its corner
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

// settle makes consecutive stops either equal or more than Snap apart, the
// last word before the jogs are collected: a jog within Snap earns no
// lane. Ports and the stops already snapped onto them are locked; when two
// locked stops meet, a side column moves only outward and only where it
// keeps its row clear: when exactly one side may move, it gives way, and
// otherwise the one that does not carry the arrowhead. An anchored port
// never gives way (S9): the stop it meets moves onto it, except a side
// column inward; there, and against another end that never moves, the
// jog stays, beyond float noise, and takes a lane.
func (r *router) settle() {
	for e, w := range r.wires {
		stops := w.stops
		if len(stops) < 2 {
			continue
		}
		last := len(stops) - 1
		locked := make([]bool, len(stops))
		locked[0], locked[last] = true, true
		arrowhead := last
		if r.reversed(e) {
			arrowhead = 0
		}
		for range len(stops) + 1 {
			changed := false
			for i := range last {
				a, b := stops[i], stops[i+1]
				if a == b || math.Abs(a-b) > r.o.Snap {
					continue
				}
				runStart := i
				for runStart > 0 && stops[runStart-1] == a {
					runStart--
				}
				keepsArrowhead := locked[arrowhead] && arrowhead >= runStart && arrowhead <= i
				moveRun := locked[i+1] && !keepsArrowhead
				if locked[i+1] {
					runEnd := i + 1
					for runEnd < last && stops[runEnd+1] == b {
						runEnd++
					}
					runAnchored, bAnchored := r.anchoredIn(e, runStart, i, last), r.anchoredIn(e, i+1, runEnd, last)
					switch {
					case math.Abs(a-b) > noise && ((runAnchored && (r.stuckIn(e, i+1, runEnd, last) || r.inward(e, stops, runEnd, a))) ||
						(bAnchored && (r.stuckIn(e, runStart, i, last) || r.inward(e, stops, runStart, b)))):
						continue
					case runAnchored:
						moveRun = false
					case bAnchored:
						moveRun = true
					default:
						if runYields, bYields := r.yields(e, stops, runStart, i, b), r.yields(e, stops, i+1, runEnd, a); runYields != bYields {
							moveRun = runYields
						}
					}
				}
				if moveRun {
					for k := i; k >= 0 && stops[k] == a; k-- {
						stops[k] = b
						locked[k] = true
					}
				} else {
					stops[i+1] = a
					if locked[i] {
						locked[i+1] = true
					}
				}
				changed = true
			}
			if !changed {
				break
			}
		}
		for _, which := range [2]int{ports.Head, ports.Tail} {
			i := 0
			if which == ports.Tail {
				i = last
			}
			r.port[e][which] = stops[i]
			if a := r.attach[e][which]; a != nil {
				a.vx = stops[i]
			}
		}
	}
}

// inward reports whether moving stop i of edge e's wire to x would move a
// side column toward its node, which it never does (S8): from Reach out,
// the least a column sits, any inward move would cut C7's stub.
func (r *router) inward(e int, stops []float64, i int, x float64) bool {
	var a *attachment
	switch i {
	case 0:
		a = r.attach[e][ports.Head]
	case len(stops) - 1:
		a = r.attach[e][ports.Tail]
	}
	return a != nil && sign(a.side)*(x-stops[i]) < 0
}

// yields reports whether stops lo..hi of edge e's wire may all move to x:
// a side column among them moves only outward, and only where it keeps its
// row clear (S8).
func (r *router) yields(e int, stops []float64, lo, hi int, x float64) bool {
	last := len(stops) - 1
	for _, end := range [2][2]int{{0, ports.Head}, {last, ports.Tail}} {
		i, which := end[0], end[1]
		a := r.attach[e][which]
		if i < lo || i > hi || a == nil || stops[i] == x {
			continue
		}
		if sign(a.side)*(x-stops[i]) < 0 || !r.rowClear(e, which, x) {
			return false
		}
	}
	return true
}
