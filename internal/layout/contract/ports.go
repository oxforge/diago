package contract

import (
	"github.com/oxforge/diago/internal/model"
)

// kind is an edge's direction relative to the flow (C0).
type kind int

const (
	forward kind = iota // the target lies wholly downstream of the source
	back                // the target lies wholly upstream of the source
	lateral             // the two overlap on the flow axis
)

// edgeKind classifies an edge by its endpoint boxes along the flow axis of
// dir.
func edgeKind(from, to model.PositionedNode, dir model.Direction) kind {
	f, t := nodeBox(from), nodeBox(to)
	// Map each box to its extent in downstream coordinates u.
	span := func(b box) (lo, hi float64) {
		switch dir {
		case model.Up:
			return -b.bottom, -b.top
		case model.Right:
			return b.left, b.right
		case model.Left:
			return -b.right, -b.left
		default: // Down
			return b.top, b.bottom
		}
	}
	fLo, fHi := span(f)
	tLo, tHi := span(t)
	switch {
	case tLo >= fHi-slack:
		return forward
	case tHi <= fLo+slack:
		return back
	}
	return lateral
}

// inVertex is the vertex that faces upstream: where forward edges enter.
func inVertex(dir model.Direction) side {
	switch dir {
	case model.Up:
		return bottom
	case model.Right:
		return left
	case model.Left:
		return right
	default:
		return top
	}
}

// crossVertex reports whether s is a vertex on the cross axis of dir.
func crossVertex(s side, dir model.Direction) bool {
	if dir == model.Right || dir == model.Left {
		return s == top || s == bottom
	}
	return s == left || s == right
}

func pinned(n model.PositionedNode) bool {
	return n.Shape == model.ShapeDiamond || n.Shape == model.ShapeCircle
}

// checkShapePorts verifies C8: diamonds and circles connect only at their
// four vertices (C8.1); forward edges enter at the in-vertex (C8.2); no edge
// leaves from it (C8.3); back edges use a cross-axis vertex (C8.4); up to
// three outgoing forward edges each leave from their own vertex, except a
// circle's fan-out that all heads to one side, while back edges keep to C8.4
// alone (C8.5). Self-loops are exempt from C8.2–C8.5. Direction Auto, which
// knows no forward edge, skips C8.2–C8.5.
func (c *checker) checkShapePorts() {
	oriented := c.opts.Direction != model.Auto
	// out[node] collects the start vertex of each outgoing forward edge,
	// for C8.5.
	type start struct {
		edge string
		at   side
	}
	out := make(map[string][]start)
	for _, e := range c.pg.Edges {
		if !routed(e) {
			continue
		}
		from, okF := c.nodes[e.From]
		to, okT := c.nodes[e.To]
		if !okF || !okT {
			continue // C2.2 reports the unknown node
		}
		self := e.From == e.To
		k := lateral
		if oriented {
			k = edgeKind(from, to, c.opts.Direction)
		}
		ends := []struct {
			n        model.PositionedNode
			p        model.Point
			outgoing bool
		}{
			{from, e.Points[0], true},
			{to, e.Points[len(e.Points)-1], false},
		}
		for _, end := range ends {
			if !pinned(end.n) {
				continue
			}
			v, d := nearestVertex(end.n, end.p)
			if d > attachTol {
				c.add("C8.1", e.ID, "endpoint (%.2f,%.2f) on %s %s is %.2f px from its nearest vertex",
					end.p.X, end.p.Y, end.n.Shape, end.n.ID, d)
				continue
			}
			if self {
				continue
			}
			if end.outgoing && k == forward {
				out[end.n.ID] = append(out[end.n.ID], start{e.ID, v})
			}
			if !oriented {
				continue
			}
			in := inVertex(c.opts.Direction)
			if !end.outgoing && k == forward && v != in {
				c.add("C8.2", e.ID, "forward edge enters %s %s at its %v vertex, not the %v in-vertex",
					end.n.Shape, end.n.ID, v, in)
			}
			if end.outgoing && v == in {
				c.add("C8.3", e.ID, "edge leaves %s %s from its %v in-vertex", end.n.Shape, end.n.ID, v)
			}
			if k == back && !crossVertex(v, c.opts.Direction) {
				c.add("C8.4", e.ID, "back edge attaches to %s %s at its %v vertex, not a cross-axis vertex",
					end.n.Shape, end.n.ID, v)
			}
		}
	}
	for _, n := range c.pg.Nodes {
		starts := out[n.ID]
		if len(starts) < 2 || len(starts) > 3 {
			continue
		}
		if n.Shape == model.ShapeCircle {
			same := true
			for _, s := range starts[1:] {
				same = same && s.at == starts[0].at
			}
			if same {
				continue // a circle's merged fan-out
			}
		}
		for i := range starts {
			for j := i + 1; j < len(starts); j++ {
				if starts[i].at == starts[j].at {
					c.add("C8.5", starts[j].edge, "leaves %s %s from the %v vertex that edge %s already uses",
						n.Shape, n.ID, starts[j].at, starts[i].edge)
				}
			}
		}
	}
}
