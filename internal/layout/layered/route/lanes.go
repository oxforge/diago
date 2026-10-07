package route

import (
	"cmp"
	"math"
	"slices"

	"github.com/oxforge/diago/internal/layout/layered/ports"
)

// jog is a wire's horizontal run in one channel, from the column it
// enters on to the column it leaves on.
type jog struct {
	edge     int
	from, to float64
	toRail   int // the pinned node whose in-vertex this last jog ends on, else -1
	fromRail int // the pinned node whose bottom vertex this first jog starts on, else -1
	head     int // the chain head this first jog leaves, else -1
}

// loopJog is a self-loop on a pinned node: it leaves side vertex side,
// rises along column sx into the channel above its node and runs across
// it to the in-vertex (S8).
type loopJog struct {
	edge, node int
	side       ports.Vertex
	sx         float64
	lane       int
}

// unit is what one lane holds: a jog, the jogs of one rail (several wires
// into one in-vertex, or out of one bottom vertex, drawn as one run), or a
// self-loop's run.
type unit struct {
	members []int // jogs, by index
	loop    int   // the loopJog's index, or -1
	lo, hi  float64
	entries []float64 // columns of the verticals that reach the lane from above
	exits   []float64 // columns of the verticals that leave the lane downward
	diverge int       // a lone first jog's chain head, for the diverging exemption; else -1
	dir     float64
	lane    int
	key     int // the lowest edge among its members, for a deterministic order
}

// laneEntry is a unit's span in the lane it takes.
type laneEntry struct {
	lo, hi  float64
	diverge int
	dir     float64
}

// laneMap is each jog's lane, keyed by edge and channel.
type laneMap map[[2]int]int

// packLanes gives every jog a lane in its channel (S8). A pair of jogs
// that only one order draws without a crossing gets that order as a
// must-be-above constraint. The units are sorted topologically, a cycle
// releasing the unit with the fewest remaining constraints (an unavoidable
// crossing), and each takes the shallowest lane below its constraints
// where no other unit comes within InLaneGap.
func (r *router) packLanes() {
	g := r.g
	channels := make([][]jog, max(0, len(g.Layers)-1))
	for e, w := range r.wires {
		if len(w.stops) < 2 {
			continue
		}
		chain := g.Chains[e]
		for i := 0; i+1 < len(w.stops); i++ {
			j := jog{edge: e, from: w.stops[i], to: w.stops[i+1], toRail: -1, fromRail: -1, head: -1}
			// settle leaves stops equal, more than Snap apart, or, between two
			// ends that never move (S9), apart by less: every jog but float
			// noise takes a lane
			if math.Abs(j.to-j.from) <= noise {
				continue
			}
			if i == len(w.stops)-2 {
				tail := chain[len(chain)-1]
				if r.pinned(tail) && r.attach[e][1] == nil && math.Abs(j.to-r.x[tail]) <= r.o.Snap {
					j.toRail = tail
				}
			}
			if i == 0 {
				j.head = chain[0]
				if r.pinned(chain[0]) && r.attach[e][0] == nil && math.Abs(j.from-r.x[chain[0]]) <= r.o.Snap {
					j.fromRail = chain[0]
				}
			}
			channels[w.start+i] = append(channels[w.start+i], j)
		}
	}
	loopsIn := make([][]int, len(g.Layers))
	for i, lj := range r.loopAt {
		l := g.Vertices[lj.node].Layer
		loopsIn[l] = append(loopsIn[l], i)
	}
	r.lanes = laneMap{}
	r.nLanes = make([]int, len(channels))
	for c, jogs := range channels {
		var loops []int
		if c+1 < len(loopsIn) {
			loops = loopsIn[c+1]
		}
		r.nLanes[c] = r.pack(c, jogs, loops)
	}
	if len(loopsIn) > 0 {
		r.pack(-1, nil, loopsIn[0]) // above layer 0, where nothing else runs
	}
}

// pack assigns the lanes of channel c to its jogs and to the self-loops
// that rise into it, and returns the channel's lane count.
func (r *router) pack(c int, jogs []jog, loops []int) int {
	units := r.units(jogs, loops)
	above := make([][]int, len(units))  // above[i]: the units that must sit above unit i
	beside := make([][]int, len(units)) // beside[i]: those of above[i] that keep two verticals from running side by side
	column := make([][]int, len(units)) // column[i]: those of beside[i] that keep them off one column
	for i := range units {
		for j := i + 1; j < len(units); j++ {
			// Verticals side by side break the contract (C9), a crossing
			// only costs one (Q1): the first rules out an order outright,
			// and of two orders that both do, the one that puts two
			// verticals on one column, a shared run (C9.1).
			if bi, bj := r.beside(units[j], units[i]), r.beside(units[i], units[j]); bi || bj {
				if bi && bj {
					bi, bj = r.meets(units[j], units[i]), r.meets(units[i], units[j])
				}
				switch {
				case bi && !bj:
					above[i], beside[i] = append(above[i], j), append(beside[i], j)
					if r.meets(units[j], units[i]) {
						column[i] = append(column[i], j)
					}
				case bj && !bi:
					above[j], beside[j] = append(above[j], i), append(beside[j], i)
					if r.meets(units[i], units[j]) {
						column[j] = append(column[j], i)
					}
				}
				continue
			}
			ij, ji := !r.crosses(units[i], units[j]), !r.crosses(units[j], units[i])
			switch {
			case ij && !ji:
				above[j] = append(above[j], i)
			case ji && !ij:
				above[i] = append(above[i], j)
			}
		}
	}
	indegree := make([]int, len(units))
	for i := range units {
		indegree[i] = len(above[i])
	}
	emitted := make([]bool, len(units))
	var topo []int
	for len(topo) < len(units) {
		picked := -1
		for i := range units {
			if !emitted[i] && indegree[i] == 0 {
				picked = i
				break
			}
		}
		if picked < 0 {
			picked = release(units, emitted, indegree, beside, column)
		}
		emitted[picked] = true
		topo = append(topo, picked)
		for i := range units {
			if !emitted[i] && slices.Contains(above[i], picked) {
				indegree[i]--
			}
		}
	}
	// A fan's pair takes one lane (S12, Rule C). A pair still apart once
	// the floors stop cannot agree: the first is released and the channel
	// packs again without it, from no floors. The pairs only shrink.
	pairs := r.fanPairs(jogs, units)
	var lanes [][]laneEntry
	for {
		lanes = r.agree(units, topo, above, pairs)
		apart := slices.IndexFunc(pairs, func(p [2]int) bool { return units[p[0]].lane != units[p[1]].lane })
		if apart < 0 {
			break
		}
		pairs = slices.Delete(pairs, apart, apart+1)
	}
	for idx := range units {
		u := &units[idx]
		for _, m := range u.members {
			r.lanes[[2]int{jogs[m].edge, c}] = u.lane
		}
		if u.loop >= 0 {
			r.loopAt[u.loop].lane = u.lane
		}
	}
	return len(lanes)
}

// release picks the unit a cycle of constraints releases (S8, Lanes): the
// one with the fewest remaining constraints among those no side-by-side
// constraint still holds, an unavoidable crossing; when every unit left is
// held by one, among those no one-column constraint holds, two verticals
// side by side rather than on one column; and only then any of them.
func release(units []unit, emitted []bool, indegree []int, beside, column [][]int) int {
	for _, tier := range [][][]int{beside, column, nil} {
		picked := -1
		for i := range units {
			if emitted[i] || (tier != nil && slices.ContainsFunc(tier[i], func(k int) bool { return !emitted[k] })) {
				continue
			}
			if picked < 0 || indegree[i] < indegree[picked] {
				picked = i
			}
		}
		if picked >= 0 {
			return picked
		}
	}
	return -1 // no unit left: the caller never asks
}

// agree packs the units with a floor per fan pair (S12, Rule C): when a
// pair lands on two lanes, the deeper becomes the floor of both and the
// units pack again, until every pair agrees or a floor would reach the
// unit count. It returns the lanes' spans.
func (r *router) agree(units []unit, topo []int, above [][]int, pairs [][2]int) [][]laneEntry {
	floor := make([]int, len(units))
	for {
		lanes := r.assign(units, topo, above, floor)
		settled := true
		for _, p := range pairs {
			a, b := units[p[0]].lane, units[p[1]].lane
			if a != b && max(a, b) < len(units) {
				floor[p[0]], floor[p[1]] = max(a, b), max(a, b)
				settled = false
			}
		}
		if settled {
			return lanes
		}
	}
}

// assign gives every unit, in topo order, the shallowest lane below the
// units that must sit above it and at or below its floor where it keeps at
// least InLaneGap from every other unit of that lane, within float noise
// (S8), and returns the lanes' spans.
func (r *router) assign(units []unit, topo []int, above [][]int, floor []int) [][]laneEntry {
	for i := range units {
		units[i].lane = -1
	}
	var lanes [][]laneEntry
	for _, idx := range topo {
		u := &units[idx]
		lane := floor[idx]
		for _, a := range above[idx] {
			if units[a].lane >= 0 {
				lane = max(lane, units[a].lane+1)
			}
		}
		gap := func(en laneEntry) float64 {
			if u.diverge >= 0 && en.diverge == u.diverge && en.dir != u.dir {
				return r.o.Snap
			}
			return r.o.InLaneGap
		}
		for ; lane < len(lanes); lane++ {
			fits := true
			for _, en := range lanes[lane] {
				if !(u.hi <= en.lo-gap(en)+noise || u.lo >= en.hi+gap(en)-noise) {
					fits = false
					break
				}
			}
			if fits {
				break
			}
		}
		for lane >= len(lanes) {
			lanes = append(lanes, nil)
		}
		lanes[lane] = append(lanes[lane], laneEntry{u.lo, u.hi, u.diverge, u.dir})
		u.lane = lane
	}
	return lanes
}

// units groups the jogs of one channel: jogs into one in-vertex form a
// rail, then jogs out of one bottom vertex; every other jog and every
// self-loop is a unit of its own. Units sort by their lowest edge.
func (r *router) units(jogs []jog, loops []int) []unit {
	var units []unit
	claimed := make([]bool, len(jogs))
	rail := func(key func(jog) int, ends func([]int) ([]float64, []float64)) {
		var nodes []int
		groups := map[int][]int{}
		for i, j := range jogs {
			k := key(j)
			if claimed[i] || k < 0 {
				continue
			}
			if _, ok := groups[k]; !ok {
				nodes = append(nodes, k)
			}
			groups[k] = append(groups[k], i)
		}
		for _, n := range nodes {
			members := groups[n]
			if len(members) < 2 {
				continue
			}
			for _, m := range members {
				claimed[m] = true
			}
			en, ex := ends(members)
			units = append(units, r.unitOf(jogs, members, en, ex))
		}
	}
	rail(func(j jog) int { return j.toRail },
		func(ms []int) ([]float64, []float64) {
			var en []float64
			for _, m := range ms {
				en = append(en, jogs[m].from)
			}
			return en, []float64{jogs[ms[0]].to}
		})
	rail(func(j jog) int { return j.fromRail },
		func(ms []int) ([]float64, []float64) {
			var ex []float64
			for _, m := range ms {
				ex = append(ex, jogs[m].to)
			}
			return []float64{jogs[ms[0]].from}, ex
		})
	for i, j := range jogs {
		if claimed[i] {
			continue
		}
		u := r.unitOf(jogs, []int{i}, []float64{j.from}, []float64{j.to})
		if j.head >= 0 {
			u.diverge = j.head
			u.dir = math.Copysign(1, j.to-j.from)
		}
		units = append(units, u)
	}
	for _, li := range loops {
		lj := r.loopAt[li]
		cx := r.x[lj.node]
		units = append(units, unit{
			loop: li, lo: math.Min(cx, lj.sx), hi: math.Max(cx, lj.sx),
			exits: []float64{lj.sx, cx}, diverge: -1, lane: -1, key: lj.edge,
		})
	}
	slices.SortStableFunc(units, func(a, b unit) int { return cmp.Compare(a.key, b.key) })
	return units
}

func (r *router) unitOf(jogs []jog, members []int, entries, exits []float64) unit {
	u := unit{members: members, loop: -1, entries: entries, exits: exits, diverge: -1, lane: -1, key: jogs[members[0]].edge}
	u.lo, u.hi = math.Inf(1), math.Inf(-1)
	for _, m := range members {
		u.lo = math.Min(u.lo, math.Min(jogs[m].from, jogs[m].to))
		u.hi = math.Max(u.hi, math.Max(jogs[m].from, jogs[m].to))
		u.key = min(u.key, jogs[m].edge)
	}
	return u
}

// beside reports whether unit a sitting below unit b runs a vertical of
// each side by side: a column reaching a's lane from above within
// InLaneGap of a column leaving b's lane downward. Anchored ports (S9),
// which never move apart, meet this way.
func (r *router) beside(a, b unit) bool {
	for _, x := range a.entries {
		for _, y := range b.exits {
			if math.Abs(x-y) < r.o.InLaneGap {
				return true
			}
		}
	}
	return false
}

// meets reports whether unit a sitting below unit b runs a vertical of
// each on one column: a column reaching a's lane from above within Snap of
// a column leaving b's lane downward, a shared run (C9.1).
func (r *router) meets(a, b unit) bool {
	for _, x := range a.entries {
		for _, y := range b.exits {
			if math.Abs(x-y) <= r.o.Snap {
				return true
			}
		}
	}
	return false
}

// crosses reports whether unit a sitting above unit b makes their wires
// cross: a vertical of b reaching its lane from above passes a's run, or
// a vertical of a leaving its lane downward passes b's run.
func (r *router) crosses(a, b unit) bool {
	inside := func(x float64, u unit) bool { return x > u.lo+r.o.Snap && x < u.hi-r.o.Snap }
	for _, x := range b.entries {
		if inside(x, a) {
			return true
		}
	}
	for _, x := range a.exits {
		if inside(x, b) {
			return true
		}
	}
	return false
}

// fanPairs lists the units of a channel that a congruence fan pairs
// (S12, Rule C): the fan's edges in the order of the columns they descend
// on, ranked from both ends of the row, and for each rank whose two
// edges' spread runs start in this channel as lone jogs in opposite
// directions, their two units. Two jogs heading the same way overlap at
// the source and never share a lane.
func (r *router) fanPairs(jogs []jog, units []unit) [][2]int {
	unitOf := map[int]int{} // per edge whose first jog runs here: its unit
	for u, un := range units {
		for _, m := range un.members {
			if jogs[m].head >= 0 {
				unitOf[jogs[m].edge] = u
			}
		}
	}
	var out [][2]int
	for _, fan := range r.o.Fans {
		edges := slices.Clone(fan)
		col := func(e int) float64 {
			st := r.wires[e].stops
			if len(st) == 0 {
				return 0
			}
			return st[len(st)-1]
		}
		slices.SortStableFunc(edges, func(a, b int) int { return cmp.Or(cmp.Compare(col(a), col(b)), cmp.Compare(a, b)) })
		for i := 0; i < len(edges)/2; i++ {
			a, okA := unitOf[edges[i]]
			b, okB := unitOf[edges[len(edges)-1-i]]
			if okA && okB && units[a].dir*units[b].dir < 0 {
				out = append(out, [2]int{a, b})
			}
		}
	}
	return out
}
