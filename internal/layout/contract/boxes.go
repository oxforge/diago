package contract

import (
	"math"

	"github.com/oxforge/diago/internal/model"
)

// boxOverlapTol is the overlap, on both axes, or the wire length inside a
// box that C13 and C14 report.
const boxOverlapTol = 0.5

// overlap returns the overlap of two boxes on each axis (negative when
// apart).
func overlap(a, b box) (w, h float64) {
	return math.Min(a.right, b.right) - math.Max(a.left, b.left),
		math.Min(a.bottom, b.bottom) - math.Max(a.top, b.top)
}

// gap is the distance between two boxes along the axis where they are
// furthest apart: negative or zero when they overlap.
func gap(a, b box) float64 {
	gx := math.Max(a.left-b.right, b.left-a.right)
	gy := math.Max(a.top-b.bottom, b.top-a.bottom)
	return math.Max(gx, gy)
}

// inside reports whether a lies within b, allowing tol.
func inside(a, b box, tol float64) bool {
	return a.left >= b.left-tol && a.top >= b.top-tol && a.right <= b.right+tol && a.bottom <= b.bottom+tol
}

// checkNodeSeparation verifies C11: any two node boxes are at least NodeGap
// apart on one axis.
func (c *checker) checkNodeSeparation() {
	ns := c.pg.Nodes
	for i := range ns {
		for j := i + 1; j < len(ns); j++ {
			if g := gap(nodeBox(ns[i]), nodeBox(ns[j])); g < c.lim.NodeGap-slack {
				c.add("C11", ns[i].ID, "node %s is %.2f px from node %s, under %.0f", ns[i].ID, g, ns[j].ID, c.lim.NodeGap)
			}
		}
	}
}

// checkGroupContainment verifies C12: a group box holds its member nodes and
// child groups (C12.1), overlaps no other node or unrelated group (C12.2),
// and keeps the node clearance from them along one axis (C12.3).
func (c *checker) checkGroupContainment() {
	byID := make(map[string]model.PositionedGroup, len(c.pg.Groups))
	for _, g := range c.pg.Groups {
		byID[g.ID] = g
	}
	for _, g := range c.pg.Groups {
		gb := groupBox(g)
		for _, n := range c.pg.Nodes {
			nb := nodeBox(n)
			if c.members[g.ID][n.ID] {
				if !inside(nb, gb, slack) {
					c.add("C12.1", g.ID, "member node %s sticks out of group %s", n.ID, g.ID)
				}
				continue
			}
			if w, h := overlap(nb, gb); w > slack && h > slack {
				c.add("C12.2", g.ID, "node %s overlaps group %s but is not its member", n.ID, g.ID)
			} else if d := gap(nb, gb); d < c.lim.NodeClearance-slack {
				c.add("C12.3", g.ID, "node %s is %.2f px from group %s, under %.0f", n.ID, d, g.ID, c.lim.NodeClearance)
			}
		}
		for _, ch := range g.Children {
			if child, ok := byID[ch]; ok && !inside(groupBox(child), gb, slack) {
				c.add("C12.1", g.ID, "child group %s sticks out of group %s", ch, g.ID)
			}
		}
	}
	gs := c.pg.Groups
	for i := range gs {
		for j := i + 1; j < len(gs); j++ {
			if c.nested(gs[i], gs[j]) || c.nested(gs[j], gs[i]) {
				continue
			}
			a, b := groupBox(gs[i]), groupBox(gs[j])
			if w, h := overlap(a, b); w > slack && h > slack {
				c.add("C12.2", gs[i].ID, "group %s overlaps unrelated group %s", gs[i].ID, gs[j].ID)
			} else if d := gap(a, b); d < c.lim.NodeClearance-slack {
				c.add("C12.3", gs[i].ID, "group %s is %.2f px from unrelated group %s, under %.0f", gs[i].ID, d, gs[j].ID, c.lim.NodeClearance)
			}
		}
	}
}

// nested reports whether inner is outer or one of its descendants.
func (c *checker) nested(outer, inner model.PositionedGroup) bool {
	if outer.ID == inner.ID {
		return true
	}
	seen := map[string]bool{}
	var walk func(g model.PositionedGroup) bool
	walk = func(g model.PositionedGroup) bool {
		if seen[g.ID] {
			return false
		}
		seen[g.ID] = true
		for _, ch := range g.Children {
			if ch == inner.ID {
				return true
			}
			for _, cand := range c.pg.Groups {
				if cand.ID == ch && walk(cand) {
					return true
				}
			}
		}
		return false
	}
	return walk(outer)
}

// titleBox returns the box where the renderer draws g's title, or false for
// an untitled group: LabelOffset from its left side, or TitleInsetX when
// the layout left the title to the renderer, and TitlePad wider on each
// side, over the text renderer's blanks.
func (c *checker) titleBox(g model.PositionedGroup) (box, bool) {
	if g.Label == "" || g.LabelWidth <= 0 {
		return box{}, false
	}
	x := g.X + c.lim.TitleInsetX
	if g.LabelOffset > 0 {
		x = g.X + g.LabelOffset
	}
	cy := g.Y + c.lim.TitleCenterY
	pad := c.lim.TitlePad
	return box{x - pad, cy - g.LabelHeight/2, x + g.LabelWidth + pad, cy + g.LabelHeight/2}, true
}

// checkTitles verifies C13: a titled group carries its title's extents,
// and no wire, no node box and no group box but its own and its
// ancestors' crosses its title. A wire may cross a title marked
// LabelBlocked, a sanctioned degradation (C0).
func (c *checker) checkTitles() {
	for _, g := range c.pg.Groups {
		if g.Label != "" && g.LabelWidth <= 0 {
			c.add("C13", g.ID, "group %s is titled but carries no title extents", g.ID)
			continue
		}
		tb, ok := c.titleBox(g)
		if !ok {
			continue
		}
		edges := c.pg.Edges
		if g.LabelBlocked {
			edges = nil // drawn in front of its wires, each gapped under it (C0)
		}
		for _, e := range edges {
			for i := 0; i+1 < len(e.Points); i++ {
				if pen := penetration(e.Points[i], e.Points[i+1], tb); pen > boxOverlapTol {
					c.add("C13", e.ID, "segment %d crosses the title of group %s for %.2f px", i, g.ID, pen)
				}
			}
		}
		for _, n := range c.pg.Nodes {
			if w, h := overlap(nodeBox(n), tb); w > boxOverlapTol && h > boxOverlapTol {
				c.add("C13", n.ID, "node %s covers the title of group %s", n.ID, g.ID)
			}
		}
		for _, o := range c.pg.Groups {
			if c.nested(o, g) {
				continue
			}
			if w, h := overlap(groupBox(o), tb); w > boxOverlapTol && h > boxOverlapTol {
				c.add("C13", o.ID, "group %s covers the title of group %s", o.ID, g.ID)
			}
		}
	}
}

// checkMargins verifies C16: all geometry lies at least Margin inside the
// canvas (C16.1), and the canvas is tight: some geometry touches the margin
// on every side (C16.2). In the text profile the geometry's extent rounds
// out to whole cells, where it is drawn.
func (c *checker) checkMargins() {
	ext := box{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
	grow := func(subject string, b box) {
		ext = box{math.Min(ext.left, b.left), math.Min(ext.top, b.top), math.Max(ext.right, b.right), math.Max(ext.bottom, b.bottom)}
		m := c.lim.Margin - slack
		if b.left < m || b.top < m || b.right > c.pg.Width-m || b.bottom > c.pg.Height-m {
			c.add("C16.1", subject, "(%.2f,%.2f)-(%.2f,%.2f) is within %.0f px of the %gx%g canvas edge",
				b.left, b.top, b.right, b.bottom, c.lim.Margin, c.pg.Width, c.pg.Height)
		}
	}
	for _, n := range c.pg.Nodes {
		grow(n.ID, nodeBox(n))
	}
	for _, g := range c.pg.Groups {
		grow(g.ID, groupBox(g))
	}
	for _, e := range c.pg.Edges {
		if len(e.Points) > 0 {
			route := box{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
			for _, p := range e.Points {
				route = box{math.Min(route.left, p.X), math.Min(route.top, p.Y), math.Max(route.right, p.X), math.Max(route.bottom, p.Y)}
			}
			grow(e.ID, route)
		}
		for _, l := range labelBoxes(e) {
			grow(e.ID, l.box)
		}
	}
	if math.IsInf(ext.left, 1) {
		return // an empty layout
	}
	if w, h := c.lim.CellW, c.lim.CellH; w > 0 && h > 0 {
		ext = box{math.Floor(ext.left/w) * w, math.Floor(ext.top/h) * h, math.Ceil(ext.right/w) * w, math.Ceil(ext.bottom/h) * h}
	}
	m := c.lim.Margin
	for _, s := range []struct {
		name string
		gap  float64
	}{
		{"left", ext.left}, {"top", ext.top}, {"right", c.pg.Width - ext.right}, {"bottom", c.pg.Height - ext.bottom},
	} {
		if s.gap > m+slack {
			c.add("C16.2", "", "the %s margin is %.2f px, not %.0f", s.name, s.gap, m)
		}
	}
}
