package contract

import (
	"fmt"
	"math"

	"github.com/oxforge/diago/internal/model"
)

func boxAround(p model.Point, w, h float64) box {
	return box{p.X - w/2, p.Y - h/2, p.X + w/2, p.Y + h/2}
}

// segBoxDist is the distance between segment p-q and b; 0 when they touch.
func segBoxDist(p, q model.Point, b box) float64 {
	if pointBoxDist(p, b) == 0 || pointBoxDist(q, b) == 0 || penetration(p, q, b) > 0 {
		return 0
	}
	d := math.Min(pointBoxDist(p, b), pointBoxDist(q, b))
	for _, corner := range [4]model.Point{
		{X: b.left, Y: b.top}, {X: b.right, Y: b.top}, {X: b.right, Y: b.bottom}, {X: b.left, Y: b.bottom},
	} {
		d = math.Min(d, pointSegDist(corner, p, q))
	}
	return d
}

// labelBox is one edge label or cardinality box (C14).
type labelBox struct {
	what       string // `label "x"` or `cardinality "1" (from)`
	box        box
	unresolved bool // the resolver's recorded fallback (C14.4)
}

// labelBoxes returns e's positioned label and cardinality boxes, label first.
func labelBoxes(e model.PositionedEdge) []labelBox {
	var out []labelBox
	if e.LabelPos != nil && e.LabelWidth > 0 {
		out = append(out, labelBox{fmt.Sprintf("label %q", e.Label), boxAround(*e.LabelPos, e.LabelWidth, e.LabelHeight), e.LabelUnresolved})
	}
	for _, card := range []struct {
		l   *model.EndLabel
		end string
	}{{e.FromCard, "from"}, {e.ToCard, "to"}} {
		if card.l != nil && card.l.Pos != nil && card.l.Width > 0 {
			out = append(out, labelBox{fmt.Sprintf("cardinality %q (%s)", card.l.Text, card.end),
				boxAround(*card.l.Pos, card.l.Width, card.l.Height), card.l.Unresolved})
		}
	}
	return out
}

// envelope returns the adornment envelope of an inheritance, realization,
// aggregation or composition edge: the box of p0 ± perp·half and
// (p0 + d·depth) ± perp·half, where p0 is the first point and d the unit
// vector toward the second.
func (c *checker) envelope(e model.PositionedEdge) (box, bool) {
	if !e.Relation.Adorned() || !routed(e) {
		return box{}, false
	}
	p0, p1 := e.Points[0], e.Points[1]
	l := dist(p0, p1)
	if l < orthoTol {
		return box{}, false
	}
	dx, dy := (p1.X-p0.X)/l, (p1.Y-p0.Y)/l
	px, py := -dy*c.lim.EnvelopeHalf, dx*c.lim.EnvelopeHalf
	far := model.Point{X: p0.X + dx*c.lim.EnvelopeDepth, Y: p0.Y + dy*c.lim.EnvelopeDepth}
	b := box{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
	for _, p := range []model.Point{p0, far} {
		for _, s := range [2]float64{1, -1} {
			x, y := p.X+s*px, p.Y+s*py
			b = box{math.Min(b.left, x), math.Min(b.top, y), math.Max(b.right, x), math.Max(b.bottom, y)}
		}
	}
	return b, true
}

// placedLabel is a label box with its edge's index.
type placedLabel struct {
	labelBox
	edge int
}

// cellTol is how far from a cell boundary a text label box's side may lie
// (C14.5), in px: float error only.
const cellTol = 0.01

// text reports whether the check reads the text profile's cells (C6.2,
// C14.2, C14.5, C16, C17.1).
func (c *checker) text() bool { return c.opts.Text && c.lim.CellW > 0 && c.lim.CellH > 0 }

// cellsOf is the cells box b covers, in cells: the columns and rows it
// overlaps.
func (c *checker) cellsOf(b box) box {
	w, h := c.lim.CellW, c.lim.CellH
	return box{math.Floor((b.left + cellTol) / w), math.Floor((b.top + cellTol) / h), math.Ceil((b.right - cellTol) / w), math.Ceil((b.bottom - cellTol) / h)}
}

// drawnCells is the cells segment p-q is drawn in, in cells: every cell it
// passes through, a point on a cell boundary in the cell right of it or
// below it, as the text renderer draws it (C14.2).
func (c *checker) drawnCells(p, q model.Point) box {
	w, h := c.lim.CellW, c.lim.CellH
	return box{math.Floor(math.Min(p.X, q.X) / w), math.Floor(math.Min(p.Y, q.Y) / h), math.Floor(math.Max(p.X, q.X)/w) + 1, math.Floor(math.Max(p.Y, q.Y)/h) + 1}
}

// onCells reports whether every side of b lies on a cell boundary (C14.5).
func (c *checker) onCells(b box) bool {
	on := func(v, cell float64) bool { return math.Abs(v-math.Round(v/cell)*cell) <= cellTol }
	return on(b.left, c.lim.CellW) && on(b.right, c.lim.CellW) && on(b.top, c.lim.CellH) && on(b.bottom, c.lim.CellH)
}

// frameCells is the cells group g's frame is drawn in, in cells: its left
// and right sides in the first and the last column, its top and bottom
// sides in the first and the last row, as the text renderer draws it
// (C6.2, C14.5).
func (c *checker) frameCells(g model.PositionedGroup) box {
	w, h := c.lim.CellW, c.lim.CellH
	return box{math.Floor(g.X / w), math.Floor(g.Y / h), math.Floor((g.X + g.Width) / w), math.Floor((g.Y + g.Height) / h)}
}

// frameRows is the cells of group g's frame sides that run along a text
// label's row, its top and bottom sides with their corners, in cells: the
// cells the text renderer draws them in (C14.5).
func (c *checker) frameRows(g model.PositionedGroup) [2]box {
	f := c.frameCells(g)
	return [2]box{{f.left, f.top, f.right, f.top + 1}, {f.left, f.bottom - 1, f.right, f.bottom}}
}

// checkLabels verifies C14: no label or cardinality box overlaps a node, a
// group title, another label or an adornment envelope (C14.1), is struck by
// another edge (C14.2; in the text profile, holds a cell another edge is
// drawn in), or sits within LabelGap of its own edge (C14.3; in the text
// profile, holds a cell its own edge is drawn in, or has one in the cell
// before or after it along its row); and in the text profile every box
// lies on whole cells and covers no cell of a group frame's top or bottom
// side (C14.5). Only an unresolved label or cardinality is exempt, as
// subject and object (C14.4); a congruence-owned label (C17) is checked
// like any other.
func (c *checker) checkLabels() {
	labels := c.placedLabels()
	envs := c.envelopes()
	for li, l := range labels {
		if c.text() {
			c.checkLabelCells(l)
		}
		c.checkLabelOverlaps(l, labels[li+1:], envs)
		c.checkLabelWires(l)
	}
}

// placedLabels returns every edge's label and cardinality boxes, in edge
// order, but for the unresolved ones, which are exempt (C14.4).
func (c *checker) placedLabels() []placedLabel {
	var labels []placedLabel
	for i, e := range c.pg.Edges {
		for _, l := range labelBoxes(e) {
			if !l.unresolved {
				labels = append(labels, placedLabel{l, i})
			}
		}
	}
	return labels
}

// edgeEnvelope is an adorned edge's adornment envelope.
type edgeEnvelope struct {
	edge string
	box  box
}

// envelopes returns every adorned edge's adornment envelope, in edge order.
func (c *checker) envelopes() []edgeEnvelope {
	var envs []edgeEnvelope
	for _, e := range c.pg.Edges {
		if b, ok := c.envelope(e); ok {
			envs = append(envs, edgeEnvelope{e.ID, b})
		}
	}
	return envs
}

// boxesOverlap reports whether a and b overlap by more than boxOverlapTol
// on both axes.
func boxesOverlap(a, b box) bool {
	w, h := overlap(a, b)
	return w > boxOverlapTol && h > boxOverlapTol
}

// checkLabelCells verifies C14.5 for label l, in the text profile: its box
// lies on whole cells and covers no cell of a group frame's top or bottom
// side.
func (c *checker) checkLabelCells(l placedLabel) {
	id := c.pg.Edges[l.edge].ID
	if !c.onCells(l.box) {
		c.add("C14.5", id, "%s does not lie on whole cells", l.what)
	}
	for _, g := range c.pg.Groups {
		for _, side := range c.frameRows(g) {
			if w, h := overlap(c.cellsOf(l.box), side); w > 0 && h > 0 {
				c.add("C14.5", id, "%s covers the top or bottom side of group %s's frame", l.what, g.ID)
			}
		}
	}
}

// checkLabelOverlaps verifies C14.1 for label l: its box overlaps no node
// box, no group title box, no box of the labels after it (later), so that
// each pair of labels is checked once, and no adornment envelope.
func (c *checker) checkLabelOverlaps(l placedLabel, later []placedLabel, envs []edgeEnvelope) {
	id := c.pg.Edges[l.edge].ID
	for _, n := range c.pg.Nodes {
		if boxesOverlap(l.box, nodeBox(n)) {
			c.add("C14.1", id, "%s overlaps node %s", l.what, n.ID)
		}
	}
	for _, g := range c.pg.Groups {
		if tb, ok := c.titleBox(g); ok && boxesOverlap(l.box, tb) {
			c.add("C14.1", id, "%s overlaps the title of group %s", l.what, g.ID)
		}
	}
	for _, o := range later {
		if boxesOverlap(l.box, o.box) {
			c.add("C14.1", id, "%s overlaps %s of edge %s", l.what, o.what, c.pg.Edges[o.edge].ID)
		}
	}
	for _, v := range envs {
		if boxesOverlap(l.box, v.box) {
			c.add("C14.1", id, "%s overlaps the adornment of edge %s", l.what, v.edge)
		}
	}
}

// checkLabelWires verifies C14.2 and C14.3 for label l against every
// segment of every edge, in edge order: another edge's segment strikes no
// label box (checkLabelStrike), its own edge's keeps clear of it
// (checkLabelGap).
func (c *checker) checkLabelWires(l placedLabel) {
	for oi, o := range c.pg.Edges {
		for i := 0; i+1 < len(o.Points); i++ {
			if oi == l.edge {
				c.checkLabelGap(l, i)
			} else {
				c.checkLabelStrike(l, o, i)
			}
		}
	}
}

// checkLabelStrike verifies C14.2 for label l against segment i of another
// edge, o: the segment runs through the box no deeper than penetrationTol;
// in the text profile, it is drawn in none of the box's cells.
func (c *checker) checkLabelStrike(l placedLabel, o model.PositionedEdge, i int) {
	id, p, q := c.pg.Edges[l.edge].ID, o.Points[i], o.Points[i+1]
	if c.text() {
		if w, h := overlap(c.cellsOf(l.box), c.drawnCells(p, q)); w > 0 && h > 0 {
			c.add("C14.2", id, "%s holds a cell segment %d of edge %s is drawn in", l.what, i, o.ID)
		}
		return
	}
	if pen := penetration(p, q, l.box.inflate(-boxOverlapTol)); pen > penetrationTol {
		c.add("C14.2", id, "%s is struck by segment %d of edge %s", l.what, i, o.ID)
	}
}

// checkLabelGap verifies C14.3 for label l against segment i of its own
// edge: the segment keeps LabelGap from the box; in the text profile, it
// is drawn in none of the box's cells, nor in the cell before or after it
// along its row.
func (c *checker) checkLabelGap(l placedLabel, i int) {
	e := c.pg.Edges[l.edge]
	p, q := e.Points[i], e.Points[i+1]
	if c.text() {
		cells := c.cellsOf(l.box)
		cells.left, cells.right = cells.left-1, cells.right+1 // the blank before and after it along its row
		if w, h := overlap(cells, c.drawnCells(p, q)); w > 0 && h > 0 {
			c.add("C14.3", e.ID, "%s or the blank beside it holds a cell segment %d of its own edge is drawn in", l.what, i)
		}
		return
	}
	if d := segBoxDist(p, q, l.box); d < c.lim.LabelGap-slack {
		c.add("C14.3", e.ID, "%s is %.2f px from segment %d of its own edge, under %.0f", l.what, d, i, c.lim.LabelGap)
	}
}
