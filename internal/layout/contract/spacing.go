package contract

import (
	"math"

	"github.com/oxforge/diago/internal/model"
)

const (
	trackTol   = 1.0 // C9: same track, and the track-gap tolerance
	overlapTol = 1.0 // C9.1: shared run length
)

// checkSpacing verifies C9 for every pair of distinct edges: no shared run
// (C9.1) and the track gap between parallel segments (C9.2).
func (c *checker) checkSpacing() {
	edges := c.pg.Edges
	for a := range edges {
		for b := a + 1; b < len(edges); b++ {
			c.checkPair(edges[a], edges[b])
		}
	}
}

func (c *checker) checkPair(ea, eb model.PositionedEdge) {
	minGap := c.lim.TrackGap - trackTol
	for i := 0; i+1 < len(ea.Points); i++ {
		p1, p2 := ea.Points[i], ea.Points[i+1]
		for j := 0; j+1 < len(eb.Points); j++ {
			q1, q2 := eb.Points[j], eb.Points[j+1]
			var across, along float64 // offset between the tracks; overlap along them
			switch {
			case horizontal(p1, p2) && horizontal(q1, q2):
				across, along = p1.Y-q1.Y, spanOverlap(p1.X, p2.X, q1.X, q2.X)
			case vertical(p1, p2) && vertical(q1, q2):
				across, along = p1.X-q1.X, spanOverlap(p1.Y, p2.Y, q1.Y, q2.Y)
			default:
				continue // perpendicular or diagonal: C15 and C3 own those
			}
			if c.vertexBundle(ea, i, eb, j) {
				continue
			}
			if math.Abs(across) <= trackTol && along > overlapTol {
				if c.rail(ea, i, eb, j) {
					continue
				}
				c.add("C9.1", ea.ID, "segment %d shares a %.2f px run with segment %d of edge %s", i, along, j, eb.ID)
				continue
			}
			d := math.Abs(across)
			if along < 0 {
				d = math.Hypot(across, along)
			}
			floor := minGap
			switch {
			case along <= 0 && comb(ea, i, eb, j):
				continue // no floor: a same-face comb meets end to end on one track
			case along > 0 && rake(ea, i, eb, j):
				floor = c.lim.PortGap - slack // a plain distance floor, no track tolerance
			}
			if d >= floor {
				continue
			}
			c.add("C9.2", ea.ID, "segment %d is %.2f px from segment %d of edge %s, under the %.0f px track gap",
				i, d, j, eb.ID, c.lim.TrackGap)
		}
	}
}

// vertexBundle reports whether segment i of ea and segment j of eb are both
// terminal segments at the same diamond or circle vertex: a shared-vertex
// bundle, exempt from C9.
func (c *checker) vertexBundle(ea model.PositionedEdge, i int, eb model.PositionedEdge, j int) bool {
	return c.oneVertex(terminalEnds(ea, i), terminalEnds(eb, j))
}

// rail reports whether segment i of ea and segment j of eb both lie within
// two segments of their ends at the same diamond or circle vertex: wires
// converging on the vertex, or diverging from it, along one run (a rail,
// S8), exempt from C9.1.
func (c *checker) rail(ea model.PositionedEdge, i int, eb model.PositionedEdge, j int) bool {
	return c.oneVertex(nearEnds(ea, i), nearEnds(eb, j))
}

// oneVertex reports whether an end in as and an end in bs lie at the same
// vertex of one diamond or circle.
func (c *checker) oneVertex(as, bs []nodeEnd) bool {
	for _, a := range as {
		if n, ok := c.nodes[a.node]; !ok || !pinned(n) {
			continue
		}
		for _, b := range bs {
			if b.node == a.node && dist(a.p, b.p) <= attachTol {
				return true
			}
		}
	}
	return false
}

type nodeEnd struct {
	node string
	p    model.Point
}

// terminalEnds returns the node ends segment i of e is terminal at.
func terminalEnds(e model.PositionedEdge, i int) []nodeEnd {
	var out []nodeEnd
	if i == 0 {
		out = append(out, nodeEnd{e.From, e.Points[0]})
	}
	if i == len(e.Points)-2 {
		out = append(out, nodeEnd{e.To, e.Points[len(e.Points)-1]})
	}
	return out
}

// nearEnds returns the node ends segment i of e lies within two segments
// of.
func nearEnds(e model.PositionedEdge, i int) []nodeEnd {
	var out []nodeEnd
	if nearEnd(e, i, e.From) {
		out = append(out, nodeEnd{e.From, e.Points[0]})
	}
	if nearEnd(e, i, e.To) {
		out = append(out, nodeEnd{e.To, e.Points[len(e.Points)-1]})
	}
	return out
}

// rake reports whether segment i of ea and segment j of eb are both the
// terminal segment (segment 0 at the source, the last segment at the
// target) at a shared endpoint node: two wires leaving or entering the same
// face side by side need only the port gap (C9.2).
func rake(ea model.PositionedEdge, i int, eb model.PositionedEdge, j int) bool {
	for _, n := range []string{ea.From, ea.To} {
		if terminal(ea, i, n) && terminal(eb, j, n) {
			return true
		}
	}
	return false
}

// comb reports whether segment i of ea and segment j of eb share an
// endpoint node and both lie within two segments of their end at it: wires
// converging on one column or vertex may meet end to end with no floor
// (C9.2).
func comb(ea model.PositionedEdge, i int, eb model.PositionedEdge, j int) bool {
	for _, n := range []string{ea.From, ea.To} {
		if nearEnd(ea, i, n) && nearEnd(eb, j, n) {
			return true
		}
	}
	return false
}

// nearEnd reports whether segment i of e is one of the two segments nearest
// e's end at node n.
func nearEnd(e model.PositionedEdge, i int, n string) bool {
	last := len(e.Points) - 2
	return (e.From == n && i <= 1) || (e.To == n && i >= last-1)
}
