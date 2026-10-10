package route

import (
	"math"

	"github.com/oxforge/diago/internal/layout/layered/ports"
)

// settle makes consecutive stops either equal or more than Snap apart, the
// last word before the jogs are collected: a jog within Snap earns no
// lane. Ports and the stops already snapped onto them are locked; when two
// locked stops meet, a side column moves only outward and only where it
// keeps its row clear: when exactly one side may move, it gives way, and
// otherwise the one that does not carry the arrowhead. An anchored port
// never gives way (S9): the stop it meets moves onto it, except a side
// column inward; there, and against another end that never moves, the
// jog stays, beyond float noise, and takes a lane. Each wire settles on
// its own (settleWire, meet); its ports and side columns then take its
// end stops' x.
func (r *router) settle() {
	for e, w := range r.wires {
		stops := w.stops
		if len(stops) < 2 {
			continue
		}
		r.settleWire(e, stops)
		last := len(stops) - 1
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

// settleWire settles the stops of edge e's wire (settle), its two ends
// locked: pass after pass, at most one per stop and one more, until a
// pass changes nothing, every two consecutive stops apart by no more than
// Snap meet, and one gives way to the other (meet). The run of stops on
// the upper one's x, which ends there, moves onto the lower one's and
// locks; or the lower one moves onto the upper one's and locks when that
// one is locked.
func (r *router) settleWire(e int, stops []float64) {
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
			moveRun, ok := r.meet(e, stops, locked, i, arrowhead)
			if !ok {
				continue
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
}

// meet decides which of stops i and i+1 of edge e's wire, apart by no
// more than Snap, gives way (settle): moveRun moves the run of stops on
// stop i's x, which ends at i, onto stop i+1's x, else stop i+1 moves onto
// stop i's; ok is false where the jog stays. An unlocked stop i+1 moves.
// A locked one and the run meet as their ends allow: where either holds
// an anchored end, the other side moves onto it, unless the jog stays
// (anchoredJog); else the side that may move gives way when exactly one
// may (yields); and else the run, unless it holds the stop that carries
// the arrowhead, locked.
func (r *router) meet(e int, stops []float64, locked []bool, i, arrowhead int) (moveRun, ok bool) {
	if !locked[i+1] {
		return false, true
	}
	a, b := stops[i], stops[i+1]
	last := len(stops) - 1
	runStart, runEnd := i, i+1
	for runStart > 0 && stops[runStart-1] == a {
		runStart--
	}
	for runEnd < last && stops[runEnd+1] == b {
		runEnd++
	}
	runAnchored, bAnchored, stays := r.anchoredJog(e, stops, runStart, i, runEnd)
	switch {
	case stays:
		return false, false
	case runAnchored:
		return false, true
	case bAnchored:
		return true, true
	}
	if runYields, bYields := r.yields(e, stops, runStart, i, b), r.yields(e, stops, i+1, runEnd, a); runYields != bYields {
		return runYields, true
	}
	keepsArrowhead := locked[arrowhead] && arrowhead >= runStart && arrowhead <= i
	return !keepsArrowhead, true
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
