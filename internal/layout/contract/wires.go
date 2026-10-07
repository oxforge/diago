package contract

import (
	"math"

	"github.com/oxforge/diago/internal/model"
)

// checkEdgesPresent verifies C2.1: every spec edge is in the layout with a
// polyline of at least two points.
func (c *checker) checkEdgesPresent() {
	have := make(map[string]bool, len(c.pg.Edges))
	for _, e := range c.pg.Edges {
		have[e.ID] = true
		if !routed(e) {
			c.add("C2.1", e.ID, "edge has %d route points, want at least 2", len(e.Points))
		}
	}
	for _, id := range c.opts.EdgeIDs {
		if !have[id] {
			c.add("C2.1", id, "spec edge is missing from the layout")
		}
	}
}

// checkAttachment verifies C2.2: the first point lies on the source node's
// outline and the last on the target's.
func (c *checker) checkAttachment() {
	for _, e := range c.pg.Edges {
		if !routed(e) {
			continue
		}
		for _, end := range []struct {
			node string
			p    model.Point
		}{{e.From, e.Points[0]}, {e.To, e.Points[len(e.Points)-1]}} {
			n, ok := c.nodes[end.node]
			if !ok {
				c.add("C2.2", e.ID, "endpoint references unknown node %q", end.node)
				continue
			}
			if d := outlineDist(n, end.p); d > attachTol {
				c.add("C2.2", e.ID, "endpoint (%.2f,%.2f) is %.2f px off the %s outline of node %s",
					end.p.X, end.p.Y, d, n.Shape, n.ID)
			}
		}
	}
}

// checkOrthogonal verifies C3: every segment is horizontal or vertical.
func (c *checker) checkOrthogonal() {
	for _, e := range c.pg.Edges {
		for i := 0; i+1 < len(e.Points); i++ {
			p, q := e.Points[i], e.Points[i+1]
			if !horizontal(p, q) && !vertical(p, q) {
				c.add("C3", e.ID, "segment %d (%.2f,%.2f)->(%.2f,%.2f) is diagonal", i, p.X, p.Y, q.X, q.Y)
			}
		}
	}
}

// terminal reports whether segment i of e is the terminal segment at node
// id: the first segment at the source, the last at the target.
func terminal(e model.PositionedEdge, i int, id string) bool {
	return (i == 0 && e.From == id) || (i == len(e.Points)-2 && e.To == id)
}

// checkNodeInteriors verifies C4: no segment passes through a node box. The
// terminal segment at an edge's own endpoint node is exempt: it starts on
// the outline, which can lie inside the box, and C7 sends it outward.
func (c *checker) checkNodeInteriors() {
	for _, e := range c.pg.Edges {
		for _, n := range c.pg.Nodes {
			b := nodeBox(n).inflate(-penetrationTol)
			for i := 0; i+1 < len(e.Points); i++ {
				if terminal(e, i, n.ID) {
					continue
				}
				if pen := penetration(e.Points[i], e.Points[i+1], b); pen > penetrationTol {
					c.add("C4", e.ID, "segment %d runs %.2f px through node %s", i, pen, n.ID)
				}
			}
		}
	}
}

// checkNodeClearance verifies C5: every segment keeps the node clearance from
// every node box but the edge's own endpoints.
func (c *checker) checkNodeClearance() {
	for _, e := range c.pg.Edges {
		for _, n := range c.pg.Nodes {
			if n.ID == e.From || n.ID == e.To {
				continue
			}
			b := nodeBox(n).inflate(c.lim.NodeClearance - slack)
			for i := 0; i+1 < len(e.Points); i++ {
				if pen := penetration(e.Points[i], e.Points[i+1], b); pen > penetrationTol {
					c.add("C5", e.ID, "segment %d comes within %.0f px of node %s", i, c.lim.NodeClearance, n.ID)
				}
			}
		}
	}
}

// checkGroupClearance verifies C6.1: an edge with no endpoint inside a
// group keeps the node clearance from the group's box.
func (c *checker) checkGroupClearance() {
	for _, e := range c.pg.Edges {
		for _, g := range c.pg.Groups {
			if m := c.members[g.ID]; m[e.From] || m[e.To] {
				continue
			}
			b := groupBox(g).inflate(c.lim.NodeClearance - slack)
			for i := 0; i+1 < len(e.Points); i++ {
				if pen := penetration(e.Points[i], e.Points[i+1], b); pen > penetrationTol {
					c.add("C6.1", e.ID, "segment %d comes within %.0f px of group %s", i, c.lim.NodeClearance, g.ID)
				}
			}
		}
	}
}

// checkGroupSides verifies C6.2: no segment of any edge runs along a side
// of a group's box. A segment parallel to a side keeps the track gap from
// it wherever their extents overlap; one that crosses a side is free.
func (c *checker) checkGroupSides() {
	for _, e := range c.pg.Edges {
		for _, g := range c.pg.Groups {
			for i := 0; i+1 < len(e.Points); i++ {
				if side, d, ok := c.alongSide(e.Points[i], e.Points[i+1], g); ok {
					c.add("C6.2", e.ID, "segment %d runs %.2f px from the %s side of group %s, under %.0f", i, d, side, g.ID, c.lim.TrackGap)
				}
			}
		}
	}
}

// alongSide reports the first side of group g that segment p-q runs
// parallel to within the track gap, where their extents overlap, and its
// distance from it (C6.2). In the text profile both are the cells the
// renderer draws them in: the segment's (a point on a cell boundary in the
// cell right of it or below it) and the side's, the box's edge cells.
func (c *checker) alongSide(p, q model.Point, g model.PositionedGroup) (string, float64, bool) {
	vert, horiz := vertical(p, q), horizontal(p, q)
	if vert == horiz {
		return "", 0, false // a point, or a diagonal (C3)
	}
	if c.text() {
		d, f := c.drawnCells(p, q), c.frameCells(g)
		if vert && spanOverlap(d.top, d.bottom, f.top, f.bottom) > 0 {
			return c.nearSide(d.left, f.left, f.right-1, c.lim.CellW, "left", "right")
		}
		if horiz && spanOverlap(d.left, d.right, f.left, f.right) > 0 {
			return c.nearSide(d.top, f.top, f.bottom-1, c.lim.CellH, "top", "bottom")
		}
		return "", 0, false
	}
	b := groupBox(g)
	if vert && spanOverlap(p.Y, q.Y, b.top, b.bottom) > penetrationTol {
		return c.nearSide(p.X, b.left, b.right, 1, "left", "right")
	}
	if horiz && spanOverlap(p.X, q.X, b.left, b.right) > penetrationTol {
		return c.nearSide(p.Y, b.top, b.bottom, 1, "top", "bottom")
	}
	return "", 0, false
}

// nearSide reports which of two parallel sides, at lo and hi, a segment
// on track lies within the track gap of, and how far from it, in px: unit
// px per step of the coordinates, a cell's in the text profile (C6.2).
func (c *checker) nearSide(track, lo, hi, unit float64, loName, hiName string) (string, float64, bool) {
	for _, s := range [...]struct {
		name string
		at   float64
	}{{loName, lo}, {hiName, hi}} {
		if d := math.Abs(track-s.at) * unit; d < c.lim.TrackGap-slack {
			return s.name, d, true
		}
	}
	return "", 0, false
}

// checkStubs verifies C7: the first and last segments leave their node
// perpendicular to the side the endpoint is on, outward, at least StubMin
// long.
func (c *checker) checkStubs() {
	for _, e := range c.pg.Edges {
		if !routed(e) {
			continue
		}
		last := len(e.Points) - 1
		for _, end := range []struct {
			node      string
			at, next  model.Point
			whichEnd  string
			segLength float64
		}{
			{e.From, e.Points[0], e.Points[1], "first", dist(e.Points[0], e.Points[1])},
			{e.To, e.Points[last], e.Points[last-1], "last", dist(e.Points[last], e.Points[last-1])},
		} {
			n, ok := c.nodes[end.node]
			if !ok {
				continue // C2.2 reports the unknown node
			}
			ss := sides(n, end.at)
			if len(ss) == 0 {
				continue // C2.2 reports the detached endpoint
			}
			dx, dy := end.next.X-end.at.X, end.next.Y-end.at.Y
			perpendicular := false
			for _, s := range ss {
				if s.outward(dx, dy) {
					perpendicular = true
				}
			}
			if !perpendicular {
				c.add("C7", e.ID, "%s segment at node %s does not leave its %v side perpendicularly outward", end.whichEnd, n.ID, ss)
				continue
			}
			if end.segLength < c.lim.StubMin-slack {
				c.add("C7", e.ID, "%s segment at node %s is %.2f px, under %.0f", end.whichEnd, n.ID, end.segLength, c.lim.StubMin)
			}
		}
	}
}

// checkSelfRetrace verifies C10: an edge revisits no waypoint and no two of
// its segments overlap.
func (c *checker) checkSelfRetrace() {
	for _, e := range c.pg.Edges {
		pts := e.Points
		for i := range pts {
			for j := i + 2; j < len(pts); j++ {
				if dist(pts[i], pts[j]) <= retraceTol {
					c.add("C10", e.ID, "revisits (%.2f,%.2f) at points %d and %d", pts[i].X, pts[i].Y, i, j)
				}
			}
		}
		for i := 0; i+1 < len(pts); i++ {
			for j := i + 1; j+1 < len(pts); j++ {
				if ov := sharedRun(pts[i], pts[i+1], pts[j], pts[j+1], retraceTol); ov > retraceTol {
					c.add("C10", e.ID, "segments %d and %d retrace each other for %.2f px", i, j, ov)
				}
			}
		}
	}
}

// sharedRun returns how long two axis-parallel segments run on the same line
// (their coordinates on the constant axis within tol). Perpendicular and
// diagonal segments share nothing.
func sharedRun(a1, a2, b1, b2 model.Point, tol float64) float64 {
	switch {
	case horizontal(a1, a2) && horizontal(b1, b2) && math.Abs(a1.Y-b1.Y) <= tol:
		return spanOverlap(a1.X, a2.X, b1.X, b2.X)
	case vertical(a1, a2) && vertical(b1, b2) && math.Abs(a1.X-b1.X) <= tol:
		return spanOverlap(a1.Y, a2.Y, b1.Y, b2.Y)
	}
	return 0
}

// spanOverlap is the overlap of intervals [a1,a2] and [b1,b2], in either
// order; negative when apart.
func spanOverlap(a1, a2, b1, b2 float64) float64 {
	return math.Min(math.Max(a1, a2), math.Max(b1, b2)) - math.Max(math.Min(a1, a2), math.Min(b1, b2))
}
