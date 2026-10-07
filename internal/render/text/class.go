package text

import (
	"strings"

	"github.com/oxforge/diago/internal/model"
)

// drawClassBox paints a UML record box: the plain frame,
// header lines centered from the row under the top border, a separator row
// before each non-empty compartment, member lines at left + 2. Lines are the
// composed PositionedMembers lines, so suffixes and diff marks are present.
func drawClassBox(g *charGrid, n model.PositionedNode, left, top, w, h int) {
	g.box(left, top, w, h, nil, boxFrame{})
	m := n.Members
	row := top + 1
	bottom := top + h - 1
	for _, line := range m.Header {
		if row >= bottom {
			return
		}
		rs := []rune(line.Text)
		g.writeText(left+max(1, (w-len(rs))/2), row, line.Text)
		row++
	}
	for _, comp := range [][]model.PositionedLine{m.Attributes, m.Methods} {
		if len(comp) == 0 {
			continue
		}
		if row >= bottom {
			return
		}
		g.setGlyph(left, row, '├')
		g.writeText(left+1, row, strings.Repeat("─", w-2))
		g.setGlyph(left+w-1, row, '┤')
		row++
		for _, line := range comp {
			if row >= bottom {
				return
			}
			g.writeText(left+2, row, line.Text)
			row++
		}
	}
}

// drawAdornment stamps the relation glyph in the cell adjacent to pts[0]
// along the first segment (never on the border cell), or on pts[0] itself
// after a side-face snap (the snapped point is already outside the face),
// mirroring drawHeads. The triangle points back at the source box.
func drawAdornment(g *charGrid, ce cellEdge) {
	if !ce.relation.Adorned() || len(ce.pts) < 2 {
		return
	}
	first, next := ce.pts[0], ce.pts[1]
	var glyph rune
	dx, dy := 0, 0
	switch {
	case next.y > first.y:
		dy, glyph = 1, '△'
	case next.y < first.y:
		dy, glyph = -1, '▽'
	case next.x > first.x:
		dx, glyph = 1, '◁'
	default:
		dx, glyph = -1, '▷'
	}
	switch ce.relation {
	case model.RelationAggregation:
		glyph = '◇'
	case model.RelationComposition:
		glyph = '◆'
	}
	if ce.sideDeparture {
		g.setGlyph(first.x, first.y, glyph)
		return
	}
	g.setGlyph(first.x+dx, first.y+dy, glyph)
}

// legendSamples are the three-cell relation samples of the text legend.
var legendSamples = map[string]string{
	"inheritance": "◁──",
	"realization": "◁╌╌",
	"composition": "◆──",
	"aggregation": "◇──",
	"dependency":  "╌╌►",
	"association": "───",
}

// legendRows renders one "<sample> <label>" row per legend entry.
func legendRows(entries []model.LegendEntry) []string {
	if len(entries) == 0 {
		return nil
	}
	rows := make([]string, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, legendSamples[e.Kind]+" "+e.Label)
	}
	return rows
}
