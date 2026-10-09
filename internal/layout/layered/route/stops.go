package route

import (
	"math"

	"github.com/oxforge/diago/internal/layout/layered/ports"
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
