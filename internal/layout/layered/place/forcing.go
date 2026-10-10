package place

import (
	"cmp"
	"math"
	"slices"

	"github.com/oxforge/diago/internal/layout/layered/ports"
)

// realized reports whether forced exit f's first hop lands where its
// forcing puts it on x, within realizedTolerance.
func (p *placer) realized(x []float64, f forcedExit) bool {
	c := p.g.Chains[f.edge]
	delta := f.offset - p.offset(c[1], f.edge, ports.Tail)
	return math.Abs(x[c[0]]+delta-x[c[1]]) <= realizedTolerance
}

// primaryClause is where a diamond stands with S7's primary clause: held,
// the clause applies; denied, its row blocks its secondary's column, and
// the exit rule decides; restored, the denial did not clear the row, and
// the clause applies for good.
type primaryClause uint8

const (
	primaryHeld primaryClause = iota
	primaryDenied
	primaryRestored
)

// forcedExit is an exit whose first hop must put its next vertex under a
// side column, or, for a primary exit or a bottom clause's, under the
// bottom vertex: offset is the column's distance from the node's center,
// 0 under the bottom vertex, and round the recheck that forced it, 0 for
// the baseline.
type forcedExit struct {
	edge    int
	offset  float64
	round   int
	primary bool
	bottom  bool
}

// exitRound is one round of S7's side-exit forcing: the forcings kept and
// added, in edge order, and the rooms kept and added, the added ones after
// the kept ones; or the changes of diamonds' primary clauses, in vertex
// order, which restart the rounds once for all of them.
type exitRound struct {
	forced  []forcedExit
	rooms   []room
	changes []clauseChange
}

// clauseChange denies diamond node its primary, or gives it back when
// restore is set, for the exit edge whose column its row blocks (S7).
type clauseChange struct {
	node, edge int
	restore    bool
}

// sideExits predicts, against the layout base, which exits of each pinned
// node the router draws from a side vertex, and returns kept, the
// forcings of earlier rounds, with the exits of this round added, and
// keptRooms with the rooms this round gives added (S7). It reads every
// pinned node's exits off the chains (readExits) and what the kept
// forcings hold (heldBy), and judges each diamond with two or more exits
// (judgeDiamond). The round returns every denial it finds, or, settled
// with no new forcing and no new room, every give-back, as its changes. A
// node with a single exit gets none: the router settles those from the
// realized coordinates; nor does a node with a counter-flow end in the
// first pass, before the router has attached its back edge at a side.
// Nor does a circle: its exits all leave its bottom vertex, a merged
// fan-out (S8).
func (p *placer) sideExits(base []float64, kept []forcedExit, keptRooms []room, round int) exitRound {
	g := p.g
	j := &exitJudge{
		p: p, base: base, round: round, band: max(p.o.Clearance, p.o.Span), exitFacts: p.readExits(base),
		roomed: map[int]bool{}, forced: slices.Clone(kept), rooms: slices.Clone(keptRooms),
	}
	j.held, j.straight = p.heldBy(kept)
	for _, r := range keptRooms {
		j.roomed[r.edge] = true
	}
	for v, ex := range j.exits {
		// a counter-flow end's side vertex is known in the second pass only
		if len(ex) < 2 || (j.counter[v] && p.o.Side == nil) || ports.Merged(g.Level.Nodes[g.Vertices[v].Node].Shape) {
			continue
		}
		j.judgeDiamond(v)
	}
	if len(j.denials) > 0 {
		return exitRound{changes: j.denials}
	}
	if len(j.restores) > 0 && len(j.forced) == len(kept) && len(j.rooms) == len(keptRooms) {
		return exitRound{changes: j.restores}
	}
	slices.SortFunc(j.forced, func(a, b forcedExit) int { return cmp.Compare(a.edge, b.edge) })
	return exitRound{forced: j.forced, rooms: j.rooms}
}

// exitFacts is what a round of side-exit forcing reads off the chains
// (S7), per vertex: whether a counter-flow end lies on it, a pinned node;
// on which sides, left and right, the replay attached one (Options.Side,
// second pass only); whether a forward chain leaves it; and, on a pinned
// node, its forward exits, each heading for the port its first hop ends
// on.
type exitFacts struct {
	counter  []bool
	backSide [][2]bool
	leads    []bool
	exits    [][]ports.Exit
}

// readExits reads the exit facts of every vertex against the layout base.
func (p *placer) readExits(base []float64) exitFacts {
	g := p.g
	f := exitFacts{
		counter:  make([]bool, len(g.Vertices)),
		backSide: make([][2]bool, len(g.Vertices)),
		leads:    make([]bool, len(g.Vertices)),
		exits:    make([][]ports.Exit, len(g.Vertices)),
	}
	for e, chain := range g.Chains {
		if len(chain) < 2 {
			continue
		}
		if g.Reversed != nil && g.Reversed[e] {
			for which, v := range [2]int{chain[0], chain[len(chain)-1]} {
				if !p.pinned(v) {
					continue
				}
				f.counter[v] = true
				if p.o.Side == nil {
					continue
				}
				switch p.o.Side[e][which] {
				case ports.Left:
					f.backSide[v][0] = true
				case ports.Right:
					f.backSide[v][1] = true
				}
			}
			continue
		}
		f.leads[chain[0]] = true
		if p.pinned(chain[0]) {
			next := chain[1]
			// the port its first hop ends on (S8, Shape ports), its group's
			// anchor on a group (S9)
			port := base[next] + p.offset(next, e, ports.Tail)
			f.exits[chain[0]] = append(f.exits[chain[0]], ports.Exit{Edge: e, Toward: port})
		}
	}
	return f
}

// heldBy returns, per vertex, which of its sides, left and right, a kept
// forcing holds, and whether one holds an exit straight under it: its
// primary, or its bottom clause's exit (S7).
func (p *placer) heldBy(kept []forcedExit) (held [][2]bool, straight []bool) {
	held = make([][2]bool, len(p.g.Vertices))
	straight = make([]bool, len(p.g.Vertices))
	for _, f := range kept {
		v := p.g.Chains[f.edge][0]
		switch {
		case f.primary || f.bottom:
			straight[v] = true
		case f.offset > 0:
			held[v][1] = true
		default:
			held[v][0] = true
		}
	}
	return held, straight
}

// exitJudge is one round of side-exit forcing as sideExits judges it,
// diamond by diamond: what the round reads, and what it decides so far,
// in the order it decides it.
type exitJudge struct {
	p     *placer
	base  []float64
	round int
	band  float64 // the band an exit heading must leave (ports.Exits)
	exitFacts
	held     [][2]bool    // per vertex, left and right: a side a kept forcing holds
	straight []bool       // per vertex: a kept forcing holds an exit straight under it
	roomed   map[int]bool // per edge: a kept room holds it
	forced   []forcedExit // the kept forcings, then the round's
	rooms    []room       // the kept rooms, then the round's
	denials  []clauseChange
	restores []clauseChange // denied diamonds whose denials did not clear their rows
}

// judgeDiamond judges the exits of diamond v (S7). Its vertices come from
// the exit rule (ports.Exits), and in the second pass from the router's
// replay (Options.Side): the recorded side vertex, the bottom for the
// others. A diamond with two exits of which exactly one leads on takes
// them by the primary clause instead (primary): the primary on the
// bottom, forced straight under it while its port lies off the axis, and
// the other, its secondary, on the side its port lies of the primary's,
// its sides judged only in a round after the primary's forcing. A diamond
// with a counter-flow end, judged in the second pass only, takes no
// primary clause, and the side vertex its back edge holds forces nothing;
// a diamond denied its primary takes its exits by the exit rule, the exit
// that was its secondary still marked as one. A diamond without a primary
// whose vertices put a lone exit on its bottom vertex, one hop long,
// forces it straight under it, by the bottom clause, unless it was denied
// its primary or the clause gave way. Each side vertex a kept forcing
// does not hold is judged on its own (judgeSide).
func (j *exitJudge) judgeDiamond(v int) {
	p, ex, cx := j.p, j.exits[v], j.base[v]
	sides := ports.Exits(cx, j.band, ex)
	if p.o.Side != nil {
		for i := range ex {
			sides[i] = p.o.Side[ex[i].Edge][ports.Head] // the router's replay (S8)
		}
	}
	prim, sec := p.primary(ex, j.leads, sides), -1
	if j.counter[v] {
		prim = -1 // its back edge claims a side first: no main line
	}
	if prim >= 0 && p.clause[v] == primaryDenied {
		sec, prim = 1-prim, -1 // the exit that was the secondary
	}
	if prim >= 0 {
		sec = 1 - prim
		sides[prim], sides[sec] = ports.Bottom, ports.Left
		if ex[sec].Toward > ex[prim].Toward {
			sides[sec] = ports.Right
		}
		if !j.straight[v] && math.Abs(ex[prim].Toward-cx) > realizedTolerance {
			j.forced = append(j.forced, forcedExit{edge: ex[prim].Edge, round: j.round, primary: true})
			return // its sides wait for the rerun that puts the primary straight
		}
	}
	// the bottom clause: no primary, nor one denied, and a lone exit on
	// the bottom vertex whose first hop ends at its node
	if b := lone(sides, ports.Bottom); prim < 0 && sec < 0 && b >= 0 && len(p.g.Chains[ex[b].Edge]) == 2 &&
		!j.straight[v] && !p.bottomless[v] && math.Abs(ex[b].Toward-cx) > realizedTolerance {
		j.forced = append(j.forced, forcedExit{edge: ex[b].Edge, round: j.round, bottom: true})
		return // its sides wait for the rerun that puts it straight
	}
	for k, side := range [2]ports.Vertex{ports.Left, ports.Right} {
		if j.held[v][k] || j.backSide[v][k] {
			continue // a side a forcing holds, or the back edge's column
		}
		j.judgeSide(v, side, sides, prim, sec)
	}
}

// judgeSide judges side vertex side of diamond v, sides putting each of
// its exits on a vertex, prim and sec its primary and secondary exit, -1
// for none (S7): the exit heading farthest out there is forced when the
// port its first hop ends on (its next vertex's center plus that vertex's
// in-port offset) has not reached the side column, W/2 + Reach from the
// center. A port at or beyond the column has reached it, since the router
// moves a lone column out over it where the row lets it; a side vertex
// that two or more exits leave from is a side rail, which the router
// never moves out, so there only a port on the column, within
// realizedTolerance, has. A secondary alone on its side vertex that has
// reached its column may still need room (judgeSecondary).
func (j *exitJudge) judgeSide(v int, side ports.Vertex, sides []ports.Vertex, prim, sec int) {
	ex := j.exits[v]
	sign := -1.0
	if side == ports.Right {
		sign = 1
	}
	best, rail := -1, 0
	for i := range ex {
		if sides[i] != side {
			continue
		}
		rail++
		if best < 0 || sign*(ex[i].Toward-ex[best].Toward) > 0 {
			best = i
		}
	}
	if best < 0 {
		return
	}
	column := j.p.g.Vertices[v].W/2 + j.p.o.Reach
	beyond := sign * (ex[best].Toward - (j.base[v] + sign*column))
	reached := beyond >= 0
	if rail > 1 {
		reached = math.Abs(beyond) <= realizedTolerance
	}
	if reached && best == sec && rail == 1 {
		j.judgeSecondary(v, sign, prim, sec)
	}
	if !reached {
		j.forced = append(j.forced, forcedExit{edge: ex[best].Edge, offset: sign * column, round: j.round})
	}
}

// judgeSecondary judges secondary exit sec of diamond v, whose port has
// reached its column on the side sign points to, alone on its side vertex
// (S7), when nodes of the row block that column (blocking). Only a
// primary prim that a forcing holds and that was not given back answers
// it: realized, every one of them keeps the node clearance beyond the
// port, which act 2 holds from the next rerun on, among the rooms the
// round returns, once per secondary. In the first pass, a secondary
// still blocked so with its room kept (the room gave way, or another node
// now blocks), or one whose primary's forcing dropped, denies its diamond
// the primary; and a denied diamond (prim -1) whose exit that was the
// secondary still leaves a side the row blocks the same way gets it back
// once the rounds settle with no denial and no new room (sideExits). The
// second pass gives room too, but denies and gives back nothing.
func (j *exitJudge) judgeSecondary(v int, sign float64, prim, sec int) {
	p, ex := j.p, j.exits[v]
	blocking := p.blocking(j.base, v, sign, ex[sec].Toward)
	if len(blocking) == 0 {
		return
	}
	e := ex[sec].Edge
	clauseHeld := prim >= 0 && j.straight[v] && p.clause[v] == primaryHeld
	// a room only beside a realized primary: its next vertex keeps the
	// node gap from the secondary's
	realized := clauseHeld && math.Abs(ex[prim].Toward-j.base[v]) <= realizedTolerance
	switch {
	case realized && !j.roomed[e]:
		next := p.g.Chains[e][1]
		for _, o := range blocking {
			j.rooms = append(j.rooms, room{edge: e, diamond: v, next: next, other: o, sign: sign, port: p.offset(next, e, ports.Tail)})
		}
	case p.o.Side != nil: // the second pass denies nothing
	case clauseHeld:
		j.denials = append(j.denials, clauseChange{node: v, edge: e})
	case prim < 0:
		j.restores = append(j.restores, clauseChange{node: v, edge: e, restore: true})
	}
}

// lone is the index of the only exit sides puts on vertex at, or -1 when
// none or several do.
func lone(sides []ports.Vertex, at ports.Vertex) int {
	found := -1
	for i, side := range sides {
		if side != at {
			continue
		}
		if found >= 0 {
			return -1
		}
		found = i
	}
	return found
}

// blocking lists, in layer order, the nodes of pinned vertex v's row that
// keep the router from moving v's side column, on the side sign points
// to, out over x (S8, Side attachments): the column at x, or its stub from
// v's side vertex, would come within the node clearance of the node's box
// by more than realizedTolerance, its own farthest side column or loop leg
// included as reaches predicts it. In the first pass a pinned node's loops
// run on its right there, though the router puts a loop on the left when
// another end holds the right and the left is free (S8, Self-loops). A
// dummy does not block: a port gives way from its wire and takes the
// column along (S8, Ports).
func (p *placer) blocking(base []float64, v int, sign, x float64) []int {
	g := p.g
	vertex := base[v] + sign*g.Vertices[v].W/2
	lo, hi := math.Min(x, vertex), math.Max(x, vertex)
	var out []int
	for _, o := range g.Layers[g.Vertices[v].Layer] {
		if o == v || g.Dummy(o) {
			continue
		}
		half := g.Vertices[o].W / 2
		left := base[o] - half - p.space.out[o][0] - p.o.Clearance
		right := base[o] + half + p.space.out[o][1] + p.o.Clearance
		if lo < right-realizedTolerance && hi > left+realizedTolerance {
			out = append(out, o)
		}
	}
	return out
}

// primary is the index of a diamond's primary exit among its two exits ex
// (S7, Side-exit forcing), or -1: the one of the two whose edge ends at a
// node that leads on, when exactly one does. In the second pass the
// replay must record it on the bottom vertex and the other on a side;
// otherwise the diamond keeps its replayed vertices and has no primary.
func (p *placer) primary(ex []ports.Exit, leads []bool, sides []ports.Vertex) int {
	if len(ex) != 2 {
		return -1
	}
	on := func(i int) bool {
		c := p.g.Chains[ex[i].Edge]
		return leads[c[len(c)-1]]
	}
	if on(0) == on(1) {
		return -1
	}
	prim := 0
	if on(1) {
		prim = 1
	}
	if p.o.Side != nil && (sides[prim] != ports.Bottom || sides[1-prim] == ports.Bottom) {
		return -1
	}
	return prim
}

// withForced replaces each forced exit's first-hop link with a forced one
// that puts the next vertex under its column.
func (p *placer) withForced(forced []forcedExit) []link {
	skip := map[[3]int]bool{}
	for _, f := range forced {
		c := p.g.Chains[f.edge]
		skip[[3]int{f.edge, c[0], c[1]}] = true
	}
	var out []link
	for _, l := range p.links {
		if !skip[l.key()] {
			out = append(out, l)
		}
	}
	for _, f := range forced {
		c := p.g.Chains[f.edge]
		out = append(out, link{
			edge: f.edge, upper: c[0], lower: c[1],
			delta:  f.offset - p.offset(c[1], f.edge, ports.Tail),
			forced: true,
		})
	}
	return out
}
