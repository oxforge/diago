package route

import (
	"cmp"
	"context"
	"math"
	"slices"

	"github.com/oxforge/diago/internal/layout/layered/ports"
	"github.com/oxforge/diago/internal/layoutdbg"
)

// assignPorts gives every chain end its x: the out-ends of every node
// first, then the in-ends, so that a bundle of in-ends can follow the
// order of its far ends (S8, Shape ports).
func (r *router) assignPorts(ctx context.Context) {
	g := r.g
	for v := range g.Level.Nodes {
		if r.pinned(v) {
			r.pinnedPorts(ctx, v)
		}
	}
	for e, chain := range g.Chains {
		if len(chain) < 2 || r.pinned(chain[0]) {
			continue
		}
		_, out := r.faces(chain[0])
		r.port[e][ports.Head] = r.x[chain[0]] + r.ranks[e][ports.Head].Offset(out)
	}
	for e, chain := range g.Chains {
		if len(chain) < 2 || r.pinned(chain[len(chain)-1]) {
			continue
		}
		tail := chain[len(chain)-1]
		in, _ := r.faces(tail)
		r.port[e][ports.Tail] = r.x[tail] + r.ranks[e][ports.Tail].Offset(in)
	}
	r.bundles()
}

// bundles reorders the in-ports of every group of one-hop edges that run
// between the same two nodes so that they follow their out-ports, left to
// right: a two-cycle or a set of parallel edges never crosses itself. When
// the in-ports are anchored on a group's face (S9), the out-ports follow
// them instead, unless those are anchored too or sit on a diamond's or a
// circle's vertices. An edge the replay attaches at a side face is no
// member: that end holds no port, so only real slots are shared out.
func (r *router) bundles() {
	g := r.g
	type pair struct{ head, tail int }
	var keys []pair
	members := map[pair][]int{}
	for e, chain := range g.Chains {
		if len(chain) != 2 || r.pinned(chain[1]) || r.sideFace(e, ports.Head) || r.sideFace(e, ports.Tail) {
			continue
		}
		k := pair{chain[0], chain[1]}
		if _, ok := members[k]; !ok {
			keys = append(keys, k)
		}
		members[k] = append(members[k], e)
	}
	for _, k := range keys {
		es := members[k]
		if len(es) < 2 {
			continue
		}
		moved, fixed := ports.Tail, ports.Head
		if r.ranks[es[0]][ports.Tail].Anchored {
			moved, fixed = ports.Head, ports.Tail
			if r.ranks[es[0]][ports.Head].Anchored || r.pinned(k.head) {
				continue
			}
		}
		here := make([]float64, len(es))
		for i, e := range es {
			here[i] = r.port[e][moved]
		}
		slices.Sort(here)
		byFar := slices.Clone(es)
		slices.SortStableFunc(byFar, func(a, b int) int {
			return cmp.Or(cmp.Compare(r.port[a][fixed], r.port[b][fixed]), cmp.Compare(a, b))
		})
		for i, e := range byFar {
			r.port[e][moved] = here[i]
		}
	}
}

// pinnedPorts settles every chain end on a diamond or circle v (C8): the
// counter-flow ends claim its side vertices first; a circle's two or more
// forward exits then all take its bottom vertex, a merged fan-out, while
// a diamond's spread over the vertices left, the arriving ends giving way
// where the exits need a side (exitVertices); and every other end takes
// the vertex on its face: the in-vertex for an arrival, the bottom for an
// exit. In replay (Options.Side), every end the first route attached
// takes the side vertex recorded for it, and every other exit the bottom
// (S8).
func (r *router) pinnedPorts(ctx context.Context, v int) {
	g := r.g
	cx := r.x[v]
	var counter []counterEnd
	var exits []ports.Exit
	for e, chain := range g.Chains {
		if len(chain) < 2 {
			continue
		}
		if chain[0] == v {
			r.port[e][ports.Head] = cx
			if r.reversed(e) {
				counter = append(counter, counterEnd{endRef: endRef{e, ports.Head}, adjacent: r.x[chain[1]]})
			} else {
				// the port its first hop ends on, its group's anchor on a
				// group (S9), where its column aims
				exits = append(exits, ports.Exit{Edge: e, Toward: r.aim(e)})
			}
		}
		if chain[len(chain)-1] == v {
			r.port[e][ports.Tail] = cx
			if r.reversed(e) {
				counter = append(counter, counterEnd{endRef: endRef{e, ports.Tail}, adjacent: r.x[chain[len(chain)-2]]})
			}
		}
	}
	var held [2]bool
	for i := range counter {
		c := &counter[i]
		c.side = r.recorded(c.edge, c.which)
		if c.side == ports.Bottom {
			c.side = ports.Left
			if c.adjacent >= cx {
				c.side = ports.Right
			}
			if held[slot(c.side)] && !held[slot(other(c.side))] {
				c.side = other(c.side)
			}
		}
		held[slot(c.side)] = true
	}
	var vertices []ports.Vertex
	switch {
	case r.o.Side != nil:
		vertices = make([]ports.Vertex, len(exits))
		for i, ex := range exits {
			vertices[i] = r.recorded(ex.Edge, ports.Head)
		}
	case len(exits) >= 2 && ports.Merged(r.node(v).Shape):
		vertices = make([]ports.Vertex, len(exits)) // every exit on the bottom vertex
		layoutdbg.Decision(ctx, "exits_merged",
			"phase", "route", "module", "diago", "spec_ref", "S8",
			"node", r.node(v).ID, "exits", len(exits))
	default:
		vertices = r.exitVertices(v, exits, counter)
	}
	for _, c := range counter {
		r.claim(v, c.edge, c.which, c.side, c.adjacent, false)
	}
	for i, ex := range exits {
		if vertices[i] == ports.Bottom {
			continue
		}
		r.claim(v, ex.Edge, ports.Head, vertices[i], r.aim(ex.Edge), false)
	}
}

// endRef names a chain end: edge's head or tail (ports.Head, ports.Tail).
type endRef struct{ edge, which int }

// counterEnd is a reversed edge's chain end on a pinned node: its head
// where the edge arrives at the node (the node is its To), its tail where
// it leaves; adjacent is the x of the next vertex along its chain, and
// side the side vertex it takes.
type counterEnd struct {
	endRef
	adjacent float64
	side     ports.Vertex
}

// exitVertices decides which vertex each forward exit of diamond v, or
// the lone exit of circle v, leaves from once its counter-flow ends hold
// their sides (S8, Diamonds and circles): the vertices left (exitsAmong),
// unless that leaves two of its two or three exits on one vertex (C8.5).
// Then two exits clear the side the farther-heading exit heads for (the
// right at the center), even where a leaving end holds it and the other
// side holds none: its arriving ends move to the other side, and the exit
// heading farthest toward it takes it. Three take left, bottom and right,
// sharing each side with the ends there. It moves arriving ends in
// counter.
func (r *router) exitVertices(v int, exits []ports.Exit, counter []counterEnd) []ports.Vertex {
	cx := r.x[v]
	band := math.Max(r.o.Clearance, r.o.Span)
	var held [2]bool
	for _, c := range counter {
		held[slot(c.side)] = true
	}
	vertices := exitsAmong(cx, band, exits, !held[0], !held[1])
	bottom := 0
	for _, vx := range vertices {
		if vx == ports.Bottom {
			bottom++
		}
	}
	switch {
	case len(exits) < 2 || len(exits) > 3 || bottom < 2:
		return vertices
	case len(exits) == 3:
		return ports.Exits(cx, band, exits)
	}
	freed := ports.Left
	if exits[ports.Farther(cx, exits)].Toward >= cx {
		freed = ports.Right
	}
	for i := range counter {
		if counter[i].which == ports.Head && counter[i].side == freed {
			counter[i].side = other(freed)
		}
	}
	return exitsAmong(cx, band, exits, freed == ports.Left, freed == ports.Right)
}

// aim is where exit e's descent wants to run: the in-port of its next
// node when its first hop ends on one, else the next dummy's column.
func (r *router) aim(e int) float64 {
	chain := r.g.Chains[e]
	next := chain[1]
	if len(chain) != 2 || r.pinned(next) {
		return r.x[next]
	}
	in, _ := r.faces(next)
	return r.x[next] + r.ranks[e][ports.Tail].Offset(in)
}

// claim attaches edge e's end at node v's side vertex or face, its column
// aiming over aim (S8). A pinned node's attachment always happens. Until
// columns settles every column, this one sits where columnFor puts it,
// where the bundles and the ends attached after it read it.
func (r *router) claim(v, e, which int, side ports.Vertex, aim float64, rect bool) {
	vx, _ := r.columnFor(v, side, aim)
	if r.taken[v][slot(side)] == 0 {
		r.taken[v][slot(side)] = e + 1
	}
	r.attach[e][which] = &attachment{side: side, vx: vx, rect: rect, aim: aim}
	r.port[e][which] = vx
}

// exitsAmong decides which vertex each forward exit of a diamond or circle
// leaves from (C8.5) when counter-flow ends may already hold its side
// vertices: with both sides free it is ports.Exits; with one, the exit
// heading farthest toward it takes it (of four or more, every exit heading
// beyond band on its side) and the rest share the bottom; with none, every
// exit leaves from the bottom.
func exitsAmong(cx, band float64, exits []ports.Exit, freeLeft, freeRight bool) []ports.Vertex {
	out := make([]ports.Vertex, len(exits))
	switch {
	case freeLeft && freeRight:
		return ports.Exits(cx, band, exits)
	case !freeLeft && !freeRight, len(exits) < 2:
		return out
	}
	side := ports.Left
	if freeRight {
		side = ports.Right
	}
	if len(exits) >= 4 {
		for i, ex := range exits {
			if ports.Heading(cx, band, ex.Toward) == side {
				out[i] = side
			}
		}
		return out
	}
	best := 0
	for i := 1; i < len(exits); i++ {
		if sign(side)*(exits[i].Toward-exits[best].Toward) > 0 {
			best = i
		}
	}
	out[best] = side
	return out
}

// attachRects moves counter-flow ends on nodes that are not pinned to a
// side face (S8): the back edge hugs the side at mid-height instead of
// weaving between the forward ports. A one-hop edge puts both of its ends
// on one side; a longer chain attaches only when its adjacent dummy's
// column already clears the face by Reach. A row that blocks the column,
// a side already held, or a self-loop on the right face keeps the face
// port. In replay (Options.Side), exactly the recorded ends attach, each
// on its recorded side. Every column then settles in columns, where one
// that does not keep the row clear stays Reach out, the room place kept
// for it (S7).
func (r *router) attachRects() {
	g := r.g
	for e, chain := range g.Chains {
		if len(chain) < 2 || !r.reversed(e) {
			continue
		}
		for which := range 2 {
			v, adjacent := chain[0], chain[1]
			if which == ports.Tail {
				v, adjacent = chain[len(chain)-1], chain[len(chain)-2]
			}
			if r.pinned(v) || r.attach[e][which] != nil || r.node(v).Group || r.node(v).Terminal {
				continue
			}
			forced := r.sideFace(e, which)
			if r.o.Side != nil && !forced {
				continue
			}
			cx, half := r.x[v], r.node(v).W/2
			ax := r.x[adjacent]
			if len(chain) == 2 && r.ranks[e][1-which].Anchored {
				ax = r.port[e][1-which] // its anchored port on the group (S9)
			}
			partner := r.attach[e][1-which]
			if len(chain) == 2 && partner != nil {
				ax = partner.vx
			}
			side := ports.Left
			switch {
			case forced:
				side = r.o.Side[e][which]
			case len(chain) == 2 && partner != nil:
				side = partner.side
			case ax > cx:
				side = ports.Right
			}
			if !forced && len(chain) == 2 && partner == nil && !r.usable(v, side) {
				side = other(side)
			}
			if !forced && len(chain) > 2 && sign(side)*(ax-cx) < half+r.o.Reach {
				continue
			}
			if !r.usable(v, side) {
				continue
			}
			vx, blocked := r.columnFor(v, side, ax)
			if blocked && !forced {
				continue
			}
			r.taken[v][slot(side)] = e + 1
			r.attach[e][which] = &attachment{side: side, vx: vx, rect: true, aim: ax, partner: len(chain) == 2 && partner != nil}
			r.port[e][which] = vx
		}
	}
}

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

// loopSides picks the side vertex of every self-loop on a diamond or
// circle: the right one, unless another end holds it and the left one is
// free (S8).
func (r *router) loopSides() {
	for v, es := range r.loops {
		if !r.pinned(v) {
			continue
		}
		for _, e := range es {
			side := ports.LoopSide(r.taken[v][0] != 0, r.taken[v][1] != 0)
			if r.taken[v][slot(side)] == 0 {
				r.taken[v][slot(side)] = e + 1
			}
			r.loopWide[v][slot(side)] = true
			vertex := r.vertexX(v, side)
			r.loopAt = append(r.loopAt, loopJog{edge: e, node: v, side: side, sx: vertex + sign(side)*r.o.Reach})
		}
	}
}

// sideFace reports whether the replay attaches edge e's end at a side face
// of a node with faces (Options.Side): the end takes no slot there, so it
// holds no port (S8).
func (r *router) sideFace(e, which int) bool {
	return r.recorded(e, which) != ports.Bottom && !r.pinned(r.end(e, which))
}

// recorded is the side the replay attaches edge e's end at (Options.Side),
// ports.Bottom for none or outside a replay.
func (r *router) recorded(e, which int) ports.Vertex {
	if r.o.Side == nil {
		return ports.Bottom
	}
	return r.o.Side[e][which]
}

// end is the vertex edge e's chain end which sits on: its head or its tail.
func (r *router) end(e, which int) int {
	chain := r.g.Chains[e]
	if which == ports.Tail {
		return chain[len(chain)-1]
	}
	return chain[0]
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
// happen, a dummy of the row is a wire: the stub and the column keep
// InLaneGap from its column, not the node clearance from its box.
func (r *router) blocked(v int, vx, vertex, slack float64) bool {
	g := r.g
	lo, hi := math.Min(vx, vertex), math.Max(vx, vertex)
	wires := r.pinned(v)
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

func slot(side ports.Vertex) int {
	if side == ports.Right {
		return 1
	}
	return 0
}

func other(side ports.Vertex) ports.Vertex {
	if side == ports.Right {
		return ports.Left
	}
	return ports.Right
}

func sign(side ports.Vertex) float64 {
	if side == ports.Left {
		return -1
	}
	return 1
}
