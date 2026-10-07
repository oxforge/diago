package flat

import (
	"math"
	"slices"

	"github.com/oxforge/diago/internal/layout/layered/ports"
	"github.com/oxforge/diago/internal/model"
)

// check returns the first rule candidate c breaks against the scene, as
// the rule's id and what it runs into, or "" when it meets them all (S9,
// Flat edges): C3, C10, C4, C5, C6.1, C6.2 against the sides of a group
// holding an end, C7, C8, and C9 against every wire drawn so far. Each
// floor is the Config's distance, with no tolerance beyond float noise,
// so a kept route meets the contract's (Part 1).
func (s *scene) check(c candidate) string {
	pts := c.pts
	last := len(pts) - 2
	for i := 0; i <= last; i++ {
		if !horizontal(pts[i], pts[i+1]) && !vertical(pts[i], pts[i+1]) {
			return "C3"
		}
	}
	if retraces(pts) {
		return "C10"
	}
	for n, nd := range s.pg.Nodes {
		b := s.box(n)
		for i := 0; i <= last; i++ {
			// the terminal segment at its own end starts on the outline,
			// inside the box for some shapes, and C7 sends it outward
			if (i == 0 && n == s.a) || (i == last && n == s.b) {
				continue
			}
			if through(pts[i], pts[i+1], b) {
				return "C4 node " + nd.ID
			}
		}
	}
	for n, nd := range s.pg.Nodes {
		if n == s.a || n == s.b {
			continue
		}
		b := s.box(n).grow(s.o.Clearance)
		for i := 0; i <= last; i++ {
			if through(pts[i], pts[i+1], b) {
				return "C5 node " + nd.ID
			}
		}
	}
	for g, gr := range s.pg.Groups {
		if !s.free[g] {
			continue // a group holding an end is crossed freely
		}
		b := bounds{gr.X, gr.Y, gr.X + gr.Width, gr.Y + gr.Height}.grow(s.o.Clearance)
		for i := 0; i <= last; i++ {
			if through(pts[i], pts[i+1], b) {
				return "C6.1 group " + gr.ID
			}
		}
	}
	// C6.1 keeps a route farther from a group that holds neither end
	for g, gr := range s.pg.Groups {
		if !s.free[g] && s.alongSide(pts, bounds{gr.X, gr.Y, gr.X + gr.Width, gr.Y + gr.Height}) {
			return "C6.2 group " + gr.ID
		}
	}
	if !s.stub(pts[0], pts[1], c.from) || !s.stub(pts[last+1], pts[last], c.to) {
		return "C7"
	}
	if r := s.shapePorts(c); r != "" {
		return r
	}
	f := model.PositionedEdge{ID: s.id, From: s.pg.Nodes[s.a].ID, To: s.pg.Nodes[s.b].ID, Points: pts}
	for _, j := range s.wires {
		if r := s.spacing(f, s.pg.Edges[j]); r != "" {
			return r
		}
	}
	return ""
}

// alongSide reports whether a segment of the route pts runs parallel to a
// side of the group box b within that axis's track gap, where their
// extents overlap by more than float noise (C6.2; S9, Flat edges,
// Checks): a run along the flow beside the box's left or right side, a run
// across it beside its top or bottom side. C6.1 lets a route cross the
// border of a group that holds one of its ends; C6.2 keeps it off the
// border line, on which the wire would vanish. Crossing a side
// perpendicularly is free. In the text profile a side is its border's own
// cell, the box's edge cell the text art draws it in, so the gap is
// measured from that cell's middle, half a cell inside the box's edge
// line.
func (s *scene) alongSide(pts []model.Point, b bounds) bool {
	if s.o.Text {
		b = bounds{b.l + 0.5, b.t + 0.5, b.r - 0.5, b.b - 0.5}
	}
	near := func(v, side, gap float64) bool { return math.Abs(v-side) < gap-noise }
	for i := 0; i+1 < len(pts); i++ {
		p, q := pts[i], pts[i+1]
		switch {
		case vertical(p, q) && overlap(p.Y, q.Y, b.t, b.b) > noise:
			if near(p.X, b.l, s.o.TrackGap.X) || near(p.X, b.r, s.o.TrackGap.X) {
				return true
			}
		case horizontal(p, q) && overlap(p.X, q.X, b.l, b.r) > noise:
			if near(p.Y, b.t, s.o.TrackGap.Y) || near(p.Y, b.b, s.o.TrackGap.Y) {
				return true
			}
		}
	}
	return false
}

// stub reports whether the terminal segment from end at to next leaves
// face f perpendicular to it, outward, and at least the stub long on its
// axis (C7).
func (s *scene) stub(at, next model.Point, f ports.Face) bool {
	dx, dy := next.X-at.X, next.Y-at.Y
	switch f {
	case ports.FaceLeft:
		return math.Abs(dy) <= noise && -dx >= s.o.Stub.X-noise
	case ports.FaceRight:
		return math.Abs(dy) <= noise && dx >= s.o.Stub.X-noise
	case ports.FaceIn:
		return math.Abs(dx) <= noise && -dy >= s.o.Stub.Y-noise
	}
	return math.Abs(dx) <= noise && dy >= s.o.Stub.Y-noise
}

// shapePorts checks C8 on candidate c's ends at a diamond or a circle,
// whose faces are its vertices: never leaving from the in-vertex, the top
// in the engine's frame (C8.3); by the edge's kind (C0), a forward edge
// entering at the in-vertex (C8.2), a back edge attaching at side
// vertices (C8.4), and a forward edge leaving apart from the node's other
// forward exits (C8.5).
func (s *scene) shapePorts(c candidate) string {
	pa, pb := s.pinned(s.a), s.pinned(s.b)
	if pa && c.from == ports.FaceIn {
		return "C8.3"
	}
	side := func(f ports.Face) bool { return f == ports.FaceLeft || f == ports.FaceRight }
	switch s.kind {
	case forward:
		if pb && c.to != ports.FaceIn {
			return "C8.2"
		}
		if pa {
			return s.exitsApart(c.from)
		}
	case back:
		if (pa && !side(c.from)) || (pb && !side(c.to)) {
			return "C8.4"
		}
	}
	return ""
}

// exitsApart checks C8.5 on A's forward exits, the flat edge leaving from
// vertex v among them: two or three take a vertex each, unless a circle's
// all leave one. It refuses v only where the exits met C8.5 without it,
// naming the exit already on v.
func (s *scene) exitsApart(v ports.Face) string {
	shape := s.pg.Nodes[s.a].Shape
	if apart(shape, append(slices.Clone(s.exits), exit{edge: s.id, at: v})) || !apart(shape, s.exits) {
		return ""
	}
	for _, x := range s.exits {
		if x.at == v {
			return "C8.5 edge " + x.edge
		}
	}
	return "C8.5"
}

// apart reports whether exits meet C8.5 at a node of shape shape.
func apart(shape model.Shape, exits []exit) bool {
	if len(exits) < 2 || len(exits) > 3 {
		return true
	}
	if shape == model.ShapeCircle && !slices.ContainsFunc(exits, func(x exit) bool { return x.at != exits[0].at }) {
		return true // a circle's merged fan-out
	}
	for i := range exits {
		for j := i + 1; j < len(exits); j++ {
			if exits[i].at == exits[j].at {
				return false
			}
		}
	}
	return true
}

// spacing checks C9 between the flat route f and wire w, as C9 states it:
// no shared run (C9.1) and the track gap between parallel segments,
// across their tracks side by side and as the closest-point distance end
// to end (C9.2); exempt a shared-vertex bundle, a rail on C9.1, and a
// comb end to end, and the port gap for a rake. On each axis the gaps are
// that axis's, so the distance is measured in track gaps.
func (s *scene) spacing(f, w model.PositionedEdge) string {
	for i := 0; i+1 < len(f.Points); i++ {
		p1, p2 := f.Points[i], f.Points[i+1]
		for j := 0; j+1 < len(w.Points); j++ {
			q1, q2 := w.Points[j], w.Points[j+1]
			var across, along, track, port, trackAlong float64
			switch {
			case horizontal(p1, p2) && horizontal(q1, q2):
				across, along = p1.Y-q1.Y, overlap(p1.X, p2.X, q1.X, q2.X)
				track, port, trackAlong = s.o.TrackGap.Y, s.o.PortGap.Y, s.o.TrackGap.X
			case vertical(p1, p2) && vertical(q1, q2):
				across, along = p1.X-q1.X, overlap(p1.Y, p2.Y, q1.Y, q2.Y)
				track, port, trackAlong = s.o.TrackGap.X, s.o.PortGap.X, s.o.TrackGap.Y
			default:
				continue // perpendicular: a crossing (C15)
			}
			if s.oneVertex(ends(f, i, terminal), ends(w, j, terminal)) {
				continue // a shared-vertex bundle
			}
			if math.Abs(across) <= noise && along > noise {
				if s.oneVertex(ends(f, i, near), ends(w, j, near)) {
					continue // a rail
				}
				return "C9.1 edge " + w.ID
			}
			if along <= noise && shared(f, i, w, j, near) {
				continue // a comb
			}
			d := math.Abs(across) / track
			switch {
			case along > noise && shared(f, i, w, j, terminal):
				d = math.Abs(across) / port // a rake
			case along < -noise:
				d = math.Hypot(across/track, along/trackAlong)
			}
			if d < 1-noise {
				return "C9.2 edge " + w.ID
			}
		}
	}
	return ""
}

// terminal reports whether segment i of e is its terminal segment at node
// id: the first at its From node, the last at its To node.
func terminal(e model.PositionedEdge, i int, id string) bool {
	return (i == 0 && e.From == id) || (i == len(e.Points)-2 && e.To == id)
}

// near reports whether segment i of e is one of the two segments nearest
// its end at node id.
func near(e model.PositionedEdge, i int, id string) bool {
	last := len(e.Points) - 2
	return (e.From == id && i <= 1) || (e.To == id && i >= last-1)
}

// shared reports whether segment i of a and segment j of b are both at
// their ends at one node, by at (terminal: a rake; near: a comb).
func shared(a model.PositionedEdge, i int, b model.PositionedEdge, j int, at func(model.PositionedEdge, int, string) bool) bool {
	for _, id := range []string{a.From, a.To} {
		if at(a, i, id) && at(b, j, id) {
			return true
		}
	}
	return false
}

// nodeEnd is an edge's end on a node.
type nodeEnd struct {
	node string
	p    model.Point
}

// ends are e's ends that segment i lies at, by at.
func ends(e model.PositionedEdge, i int, at func(model.PositionedEdge, int, string) bool) []nodeEnd {
	var out []nodeEnd
	if at(e, i, e.From) {
		out = append(out, nodeEnd{e.From, e.Points[0]})
	}
	if at(e, i, e.To) {
		out = append(out, nodeEnd{e.To, e.Points[len(e.Points)-1]})
	}
	return out
}

// oneVertex reports whether an end in as and one in bs lie at one vertex
// of one diamond or circle.
func (s *scene) oneVertex(as, bs []nodeEnd) bool {
	for _, a := range as {
		if n, ok := s.index[a.node]; !ok || !s.pinned(n) {
			continue
		}
		for _, b := range bs {
			if b.node == a.node && math.Hypot(a.p.X-b.p.X, a.p.Y-b.p.Y) <= noise {
				return true
			}
		}
	}
	return false
}

func horizontal(p, q model.Point) bool { return math.Abs(p.Y-q.Y) <= noise }
func vertical(p, q model.Point) bool   { return math.Abs(p.X-q.X) <= noise }

// overlap is the overlap of the intervals [a1, a2] and [b1, b2], in either
// order; negative when they lie apart.
func overlap(a1, a2, b1, b2 float64) float64 {
	return min(max(a1, a2), max(b1, b2)) - max(min(a1, a2), min(b1, b2))
}

// through reports whether the horizontal or vertical segment p-q runs
// inside the open box b for more than float noise.
func through(p, q model.Point, b bounds) bool {
	if horizontal(p, q) {
		return p.Y > b.t+noise && p.Y < b.b-noise && overlap(p.X, q.X, b.l, b.r) > noise
	}
	return p.X > b.l+noise && p.X < b.r-noise && overlap(p.Y, q.Y, b.t, b.b) > noise
}

// retraces reports whether a route revisits a point or runs back over
// one of its own segments (C10).
func retraces(pts []model.Point) bool {
	for i := range pts {
		for j := i + 1; j < len(pts); j++ {
			if math.Hypot(pts[i].X-pts[j].X, pts[i].Y-pts[j].Y) <= noise {
				return true
			}
		}
	}
	for i := 0; i+1 < len(pts); i++ {
		for j := i + 1; j+1 < len(pts); j++ {
			p1, p2, q1, q2 := pts[i], pts[i+1], pts[j], pts[j+1]
			switch {
			case horizontal(p1, p2) && horizontal(q1, q2) && math.Abs(p1.Y-q1.Y) <= noise:
				if overlap(p1.X, p2.X, q1.X, q2.X) > noise {
					return true
				}
			case vertical(p1, p2) && vertical(q1, q2) && math.Abs(p1.X-q1.X) <= noise:
				if overlap(p1.Y, p2.Y, q1.Y, q2.Y) > noise {
					return true
				}
			}
		}
	}
	return false
}
