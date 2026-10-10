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
// side face where S8 lets them (S8, Side attachments): the back edge hugs
// the side at mid-height instead of weaving between the forward ports.
// Each end attaches on its own, head first (attachRect). Every column then
// settles in columns, where one that does not keep the row clear stays
// Reach out, the room place kept for it (S7).
func (r *router) attachRects() {
	for e, chain := range r.g.Chains {
		if len(chain) < 2 || !r.reversed(e) {
			continue
		}
		for which := range 2 {
			r.attachRect(e, which)
		}
	}
}

// attachRect attaches counter-flow end which (its head or its tail) of
// edge e at a side face of its node (attachRects), on the side and aiming
// over the x rectSide gives it: none on a pinned node, a group or a
// terminal, nor an end attached already, nor, in replay, an end the
// replay does not record at a side face. A longer chain's end attaches
// only when its aim clears the face by Reach; no end takes a side that is
// not usable; and only a recorded end attaches where the row blocks its
// column (columnFor).
func (r *router) attachRect(e, which int) {
	chain := r.g.Chains[e]
	v, adjacent := chain[0], chain[1]
	if which == ports.Tail {
		v, adjacent = chain[len(chain)-1], chain[len(chain)-2]
	}
	if r.pinned(v) || r.attach[e][which] != nil || r.node(v).Group || r.node(v).Terminal {
		return
	}
	forced := r.sideFace(e, which)
	if r.o.Side != nil && !forced {
		return
	}
	side, ax, partner := r.rectSide(e, which, v, adjacent, forced)
	if !forced && len(chain) > 2 && sign(side)*(ax-r.x[v]) < r.node(v).W/2+r.o.Reach {
		return
	}
	if !r.usable(v, side) {
		return
	}
	vx, blocked := r.columnFor(v, side, ax)
	if blocked && !forced {
		return
	}
	r.taken[v][slot(side)] = e + 1
	r.attach[e][which] = &attachment{side: side, vx: vx, rect: true, aim: ax, partner: partner}
	r.port[e][which] = vx
}

// rectSide is the side face that counter-flow end which of edge e, on
// node v, takes (attachRect), and the x its column aims over: the x of
// adjacent, the next vertex along its chain; on a one-hop edge, its other
// end's anchored port on a group (S9), or, when that end attached first,
// its column (partner). The end takes the side the replay records for it
// (forced), else its partner's side, else the side its aim lies on; a
// one-hop edge's end without a partner takes the other side where that
// one is not usable.
func (r *router) rectSide(e, which, v, adjacent int, forced bool) (side ports.Vertex, ax float64, partner bool) {
	chain := r.g.Chains[e]
	ax = r.x[adjacent]
	if len(chain) == 2 && r.ranks[e][1-which].Anchored {
		ax = r.port[e][1-which] // its anchored port on the group (S9)
	}
	peer := r.attach[e][1-which]
	partner = len(chain) == 2 && peer != nil
	if partner {
		ax = peer.vx
	}
	side = ports.Left
	switch {
	case forced:
		side = r.o.Side[e][which]
	case partner:
		side = peer.side
	case ax > r.x[v]:
		side = ports.Right
	}
	if !forced && len(chain) == 2 && peer == nil && !r.usable(v, side) {
		side = other(side)
	}
	return side, ax, partner
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
