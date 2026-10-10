package text

import (
	"github.com/oxforge/diago/internal/model"
)

// cellPoint is a grid coordinate.
type cellPoint struct{ x, y int }

// cellCard is a cardinality label mapped to the cell its box starts in.
type cellCard struct {
	text       string
	at         cellPoint
	unresolved bool // the labels stage's least-overlap fallback (C14.4): seeded at at, placed by the ladder
}

// cellEdge is an edge mapped to cells, with its side attachments and the
// arrowhead bookkeeping already applied.
type cellEdge struct {
	id            string
	from, to      string
	label         string
	labelAt       *cellPoint // the cell the label's box starts in (nil when no LabelPos)
	unresolved    bool       // the label is the labels stage's least-overlap fallback (C14.4): labelAt seeds the ladder
	dir           model.EdgeDirection
	style         lineStyle
	pts           []cellPoint
	sideArrival   bool // last point snapped onto the target's side face
	sideDeparture bool // first point snapped onto the source's side face
	arrivalHead   rune // head glyph for a side arrival (► when the wire is left of the target)
	departureHead rune // head glyph for a side departure (◄ when the wire is right of the source)
	crossings     []model.Crossing
	srcPts        []model.Point // original pixel points, for crossing interpolation

	// Class relations. relation is RelationNone on a flow edge.
	relation         model.Relation
	directed         bool
	fromCard, toCard *cellCard
}

func gridStyle(s model.EdgeStyle) lineStyle {
	switch s {
	case model.EdgeDashed:
		return styleDashed
	case model.EdgeDotted:
		return styleDotted
	case model.EdgeThick:
		return styleThick
	}
	return styleSolid
}

// toCellEdge maps the polyline to cells, validates axis alignment, and moves
// side-face endpoints one cell outside the border, on the row the layout
// gave them.
func toCellEdge(e model.PositionedEdge, nodes map[string]model.PositionedNode) (cellEdge, error) {
	ce := cellEdge{id: e.ID, from: e.From, to: e.To, label: e.Label, dir: e.Direction, style: gridStyle(e.Style),
		crossings: e.Crossings, srcPts: e.Points, relation: e.Relation, directed: e.Directed, unresolved: e.LabelUnresolved}
	if len(e.Points) < 2 {
		return ce, nil
	}
	ce.pts = make([]cellPoint, len(e.Points))
	for i, p := range e.Points {
		ce.pts[i] = cellPoint{px2col(p.X), px2row(p.Y)}
	}
	last := len(ce.pts) - 1
	from, hasFrom := nodes[e.From]
	to, hasTo := nodes[e.To]
	if hasFrom {
		ce.pts[0] = faceCell(ce.pts[0], e.Points[0], e.Points[1], from)
	}
	if hasTo {
		ce.pts[last] = faceCell(ce.pts[last], e.Points[last], e.Points[last-1], to)
	}
	if i := firstDiagonal(ce.pts); i >= 0 {
		return ce, errNotAxisAligned(e.From, e.To, i)
	}
	if e.LabelPos != nil {
		// The layout's LabelPos is the center of the label's box, whose
		// cells the label is drawn in (C14.5): it starts in the cell holding
		// the box's left side (LabelWidth is 8 × runes under the text
		// profile, 0 when the caller measured nothing) and middle row.
		ce.labelAt = &cellPoint{px2col(e.LabelPos.X - e.LabelWidth/2), px2row(e.LabelPos.Y)}
	}
	ce.fromCard, ce.toCard = cardCell(e.FromCard), cardCell(e.ToCard)
	if hasFrom {
		ce.pts[0], ce.departureHead, ce.sideDeparture = snapSide(ce.pts[0], e.Points[0], e.Points[1], from)
	}
	if hasTo {
		ce.pts[last], ce.arrivalHead, ce.sideArrival = snapSide(ce.pts[last], e.Points[last], e.Points[last-1], to)
	}
	// Collapse any consecutive duplicate the face mapping or snapSide's x
	// move produced (the latter when a side end's next point already sits
	// in the column just outside the face), then check orthogonality again
	// on the cells that will actually be drawn.
	ce.pts = dropRepeats(ce.pts)
	if i := firstDiagonal(ce.pts); i >= 0 {
		return ce, errNotAxisAligned(e.From, e.To, i)
	}
	return ce, nil
}

// faceCell maps an endpoint, the pixel point at in cell p, onto the border
// cell of node's face it sits on, so the wire meets the box and the head
// lands next to it: a point on a node's bottom or right face lies exactly
// on a cell boundary, where floor() would place it in the cell outside the
// node. Only the axis the segment toward adj does not run along moves: a
// point on a corner sits on two faces, and moving it on both axes would
// pull it off its own segment.
func faceCell(p cellPoint, at, adj model.Point, node model.PositionedNode) cellPoint {
	left, right := node.X-node.Width/2, node.X+node.Width/2
	top, bottom := node.Y-node.Height/2, node.Y+node.Height/2
	if px2row(at.Y) != px2row(adj.Y) {
		switch {
		case abs(at.Y-bottom) < 0.5:
			p.y = px2row(bottom) - 1
		case abs(at.Y-top) < 0.5:
			p.y = px2row(top)
		}
		return p
	}
	switch {
	case abs(at.X-right) < 0.5:
		p.x = px2col(right) - 1
	case abs(at.X-left) < 0.5:
		p.x = px2col(left)
	}
	return p
}

// snapSide attaches an endpoint, the pixel point at in cell p, to node's
// left or right face when it sits on one and its wire leaves along its
// row, toward next: it moves one cell outside the border, where its
// arrowhead sits flush against the face. It keeps the row the layout gave
// it, a face's corner row included, so every wire is drawn where the
// labels stage saw it (S10 Text cells). It returns the end's cell, the
// head glyph pointing into the face, and whether the end attached.
func snapSide(p cellPoint, at, next model.Point, node model.PositionedNode) (cellPoint, rune, bool) {
	left, right := node.X-node.Width/2, node.X+node.Width/2
	onLeft := abs(at.X-left) < 0.5
	onRight := abs(at.X-right) < 0.5
	if (!onLeft && !onRight) || px2row(at.Y) != px2row(next.Y) {
		return p, 0, false
	}
	if onRight {
		p.x = px2col(right)
		return p, '◄', true
	}
	p.x = px2col(left) - 1
	return p, '►', true // the wire on the node's left, pointing right into it
}

// cardCell maps a cardinality to the cell its box starts in, its left
// side's column on its middle row; nil when it has no position or no text.
func cardCell(c *model.EndLabel) *cellCard {
	if c == nil || c.Pos == nil || c.Text == "" {
		return nil
	}
	return &cellCard{text: c.Text, at: cellPoint{px2col(c.Pos.X - c.Width/2), px2row(c.Pos.Y)}, unresolved: c.Unresolved}
}

// dropRepeats collapses the consecutive duplicates of pts, in place.
func dropRepeats(pts []cellPoint) []cellPoint {
	out := pts[:1]
	for _, p := range pts[1:] {
		if p != out[len(out)-1] {
			out = append(out, p)
		}
	}
	return out
}

// firstDiagonal is the index of the first segment of pts that is neither
// horizontal nor vertical; -1 when every segment is axis-aligned.
func firstDiagonal(pts []cellPoint) int {
	for i := 1; i < len(pts); i++ {
		if pts[i-1].x != pts[i].x && pts[i-1].y != pts[i].y {
			return i - 1
		}
	}
	return -1
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// drawEdgeRuns paints every segment as an hline/vline run in the edge's style.
func drawEdgeRuns(g *charGrid, ce cellEdge) {
	for i := 1; i < len(ce.pts); i++ {
		a, b := ce.pts[i-1], ce.pts[i]
		if a.y == b.y {
			g.hline(a.x, b.x, a.y, ce.style)
		} else {
			g.vline(a.y, b.y, a.x, ce.style)
		}
	}
}

// stampCrossings sets the vertical wire's glyph, in that wire's style, at
// every crossing edges[i] passes over, so the vertical passes through and
// ┼ never appears there.
func stampCrossings(g *charGrid, edges []cellEdge, i int) {
	ce := edges[i]
	for _, c := range ce.crossings {
		if c.SegmentIndex < 0 || c.SegmentIndex+1 >= len(ce.srcPts) {
			continue
		}
		a, b := ce.srcPts[c.SegmentIndex], ce.srcPts[c.SegmentIndex+1]
		x := a.X + (b.X-a.X)*c.T
		y := a.Y + (b.Y-a.Y)*c.T
		col, row := px2col(x), px2row(y)
		if px2col(a.X) == px2col(b.X) {
			g.setGlyph(col, row, verticalGlyph(ce.style))
			continue
		}
		// This edge is the horizontal one: draw the vertical wire that
		// crosses it, unless that wire already stamped its own glyph.
		if g.glyphAt(col, row) == 0 {
			g.setGlyph(col, row, verticalGlyph(verticalStyleAt(edges, i, col, row)))
		}
	}
}

// verticalGlyph is a vertical run's glyph in style st.
func verticalGlyph(st lineStyle) rune {
	if pair, ok := styledRuns[st]; ok {
		return pair[1]
	}
	return '│'
}

// verticalStyleAt is the style of the vertical run another edge than
// edges[skip] draws through cell (col, row); solid when none does.
func verticalStyleAt(edges []cellEdge, skip, col, row int) lineStyle {
	for j, e := range edges {
		if j == skip {
			continue
		}
		for k := 1; k < len(e.pts); k++ {
			a, b := e.pts[k-1], e.pts[k]
			if a.x == col && b.x == col && min(a.y, b.y) < row && row < max(a.y, b.y) {
				return e.style
			}
		}
	}
	return styleSolid
}

// drawHeads places ▼ ▲ ► ◄ one cell before the terminal point (the terminal
// point is on the node border), flush when the end was side-snapped.
func drawHeads(g *charGrid, ce cellEdge) {
	if len(ce.pts) < 2 {
		return
	}
	place := func(last, prev cellPoint, flush bool, head rune) {
		if flush {
			// A side end sits one cell outside the face it meets; the head
			// points into that face whatever the last segment's direction.
			g.setGlyph(last.x, last.y, head)
			return
		}
		switch {
		case last.y > prev.y:
			g.setGlyph(last.x, last.y-1, '▼')
		case last.y < prev.y:
			g.setGlyph(last.x, last.y+1, '▲')
		case last.x > prev.x:
			g.setGlyph(last.x-1, last.y, '►')
		default:
			g.setGlyph(last.x+1, last.y, '◄')
		}
	}
	n := len(ce.pts)
	if ce.relation != model.RelationNone {
		// Class policy: an arrowhead only at the target of a directed
		// association or dependency; adorned kinds get a glyph instead.
		if ce.directed {
			place(ce.pts[n-1], ce.pts[n-2], ce.sideArrival, ce.arrivalHead)
		}
		return
	}
	switch ce.dir {
	case model.EdgeBackward:
		place(ce.pts[0], ce.pts[1], ce.sideDeparture, ce.departureHead)
	case model.EdgeBoth:
		place(ce.pts[n-1], ce.pts[n-2], ce.sideArrival, ce.arrivalHead)
		place(ce.pts[0], ce.pts[1], ce.sideDeparture, ce.departureHead)
	case model.EdgeNone: // undirected: the wire ends flush, no glyph
		// Don't place any arrowhead
	default:
		place(ce.pts[n-1], ce.pts[n-2], ce.sideArrival, ce.arrivalHead)
	}
}
