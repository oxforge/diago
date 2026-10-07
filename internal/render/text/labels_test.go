package text

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/model"
)

// boxAt is the center of a one-row label box of n cells whose first cell
// is (col, row), with its width: where the layout puts a text label.
func boxAt(col, row, n int) (*model.Point, float64) {
	w := float64(n) * cellW
	return &model.Point{X: float64(col)*cellW + w/2, Y: float64(row)*cellH + cellH/2}, w
}

// cellAt is the rune the art shows in cell (col, row), a space past the
// end of a row.
func cellAt(t *testing.T, r Result, col, row int) rune {
	t.Helper()
	lines := strings.Split(r.Art, "\n")
	require.Less(t, row-r.TopRow, len(lines))
	rs := []rune(lines[row-r.TopRow])
	if col >= len(rs) {
		return ' '
	}
	return rs[col]
}

// textAt is the text the art shows on n cells from cell (col, row).
func textAt(t *testing.T, r Result, col, row, n int) string {
	t.Helper()
	var sb strings.Builder
	for i := range n {
		sb.WriteRune(cellAt(t, r, col+i, row))
	}
	return sb.String()
}

// turnGraph is a above b, right of it: a's wire leaves a's bottom face
// down column 3, runs right along row 5 to column 15 and down into b's
// top face. Row 5 right of column 16 is clear.
func turnGraph() *model.PositionedGraph {
	return &model.PositionedGraph{
		Nodes: []model.PositionedNode{
			cellNode("a", "A", model.ShapeRect, 0, 0, 7, 3),
			cellNode("b", "B", model.ShapeRect, 12, 8, 7, 3),
		},
		Edges: []model.PositionedEdge{{ID: "e", From: "a", To: "b", Direction: model.EdgeForward,
			Points: []model.Point{{X: 28, Y: 48}, {X: 28, Y: 88}, {X: 124, Y: 88}, {X: 124, Y: 128}}}},
		Width: 26 * cellW, Height: 11 * cellH,
	}
}

func TestRender_AResolvedLabelIsDrawnAtItsBox(t *testing.T) {
	// the label's box and the cardinality's share row 5 with a run of
	// their own wire, a blank column past its end: a ladder refuses that
	// row, but a label S10 resolved is drawn at its box (S10 Text cells)
	pg := turnGraph()
	e := &pg.Edges[0]
	e.Label = "yes"
	e.LabelPos, e.LabelWidth = boxAt(19, 5, 3)
	e.LabelHeight = cellH
	pos, w := boxAt(17, 5, 1)
	e.FromCard = &model.EndLabel{Text: "1", Pos: pos, Width: w, Height: cellH}
	r := mustRender(t, pg)
	assert.Empty(t, r.DroppedLabels)
	assert.Equal(t, "1 yes", textAt(t, r, 17, 5, 5), "the cardinality at column 17, the label at columns 19..21 of row 5:\n%s", r.Art)
	assert.NotContains(t, strings.Split(r.Art, "\n")[4-r.TopRow], "yes", "not a row up")
}

func TestRender_AnUnresolvedLabelCannotMoveAResolvedOne(t *testing.T) {
	// e, first in edge order, has an unresolved label seeded on columns
	// 5..6 of row 4, which its ladder would take at once; f's label,
	// resolved on columns 5..7 of row 4, is drawn first, at its box, and
	// e's moves along its ladder instead
	pg := &model.PositionedGraph{
		Nodes: []model.PositionedNode{
			cellNode("a", "A", model.ShapeRect, 0, 0, 7, 3), cellNode("b", "B", model.ShapeRect, 0, 7, 7, 3),
			cellNode("c", "C", model.ShapeRect, 12, 0, 7, 3), cellNode("d", "D", model.ShapeRect, 12, 7, 7, 3),
		},
		Edges: []model.PositionedEdge{
			{ID: "e", From: "a", To: "b", Direction: model.EdgeForward, Label: "no", LabelUnresolved: true,
				Points: []model.Point{{X: 28, Y: 48}, {X: 28, Y: 112}}},
			{ID: "f", From: "c", To: "d", Direction: model.EdgeForward, Label: "yes",
				Points: []model.Point{{X: 124, Y: 48}, {X: 124, Y: 112}}},
		},
		Width: 20 * cellW, Height: 10 * cellH,
	}
	pg.Edges[0].LabelPos, pg.Edges[0].LabelWidth = boxAt(5, 4, 2)
	pg.Edges[1].LabelPos, pg.Edges[1].LabelWidth = boxAt(5, 4, 3)
	r := mustRender(t, pg)
	assert.Empty(t, r.DroppedLabels)
	assert.Equal(t, "yes", textAt(t, r, 5, 4, 3), "f's label on its box:\n%s", r.Art)
	assert.Equal(t, 1, strings.Count(r.Art, "no"), "e's label drawn elsewhere, on its ladder:\n%s", r.Art)
}

func TestRender_AResolvedLabelOnInkIsDropped(t *testing.T) {
	// a resolved label or cardinality whose box holds ink is never drawn
	// over it, nor moved: it is dropped and reported (S10 Text cells)
	pg := chainGraph()
	e := &pg.Edges[0]
	e.ID, e.Label = "e", "yes"
	e.LabelPos, e.LabelWidth = boxAt(2, 3, 3) // column 3 of row 3 holds the wire
	pos, w := boxAt(1, 4, 5)                  // column 3 of row 4 holds the arrowhead
	e.ToCard = &model.EndLabel{Text: "0..*", Pos: pos, Width: w, Height: cellH}
	r := mustRender(t, pg)
	assert.Equal(t, []DroppedLabel{
		{ID: "e", From: "a", To: "b", Label: "0..*", End: "to"},
		{ID: "e", From: "a", To: "b", Label: "yes"},
	}, r.DroppedLabels)
	assert.NotContains(t, r.Art, "yes")
	assert.NotContains(t, r.Art, "0..*")
	assert.Equal(t, '│', cellAt(t, r, 3, 3), "the wire intact")
	assert.Equal(t, '▼', cellAt(t, r, 3, 4), "the arrowhead intact")
}

func TestRender_ASideEndKeepsItsRow(t *testing.T) {
	// a's wire leaves a's right face on its middle row, rises along
	// column 9 and enters b's left face on b's top border row; c's wire
	// rises along column 10 and enters b's left face on its middle row.
	// Each end is drawn where the layout puts it, its arrowhead flush
	// beside b: a's on b's corner row, c's on b's middle row, each run on
	// its own row. Moved one row in off the border row, a's last run would
	// share c's on the middle row, and the wires would merge in a junction
	// (S10 Text cells).
	pg := &model.PositionedGraph{
		Nodes: []model.PositionedNode{
			cellNode("a", "A", model.ShapeRect, 0, 0, 7, 5),
			cellNode("b", "B", model.ShapeRect, 12, 0, 7, 3),
			cellNode("c", "C", model.ShapeRect, 8, 6, 5, 3),
		},
		Edges: []model.PositionedEdge{
			{From: "a", To: "b", Direction: model.EdgeForward,
				Points: []model.Point{{X: 56, Y: 40}, {X: 76, Y: 40}, {X: 76, Y: 8}, {X: 96, Y: 8}}},
			{From: "c", To: "b", Direction: model.EdgeForward,
				Points: []model.Point{{X: 84, Y: 96}, {X: 84, Y: 24}, {X: 96, Y: 24}}},
		},
		Width: 20 * cellW, Height: 9 * cellH,
	}
	r := mustRender(t, pg)
	assert.Equal(t, "┌─►┌", textAt(t, r, 9, 0, 4), "a's wire along row 0 into b's top border row:\n%s", r.Art)
	assert.Equal(t, "│┌►│", textAt(t, r, 9, 1, 4), "c's wire along row 1 into b's middle row, a's rising beside it:\n%s", r.Art)
	assert.Equal(t, "──┘│", textAt(t, r, 7, 2, 4), "a's end on its middle row, c's wire rising:\n%s", r.Art)
	for row := 3; row <= 5; row++ {
		assert.Equal(t, '│', cellAt(t, r, 10, row), "c's wire in column 10, row %d:\n%s", row, r.Art)
	}
	assert.False(t, strings.ContainsAny(r.Art, "┬┴┼├┤"), "no junction: the two wires never merge:\n%s", r.Art)
}

func TestRender_AResolvedLabelOverAFrameSideReplacesIt(t *testing.T) {
	// g's frame spans columns 20..25 and rows 2..9. A label the labels
	// stage resolved over its left side, which crosses the label's row,
	// is written over the side; one over a corner or along the top side
	// is dropped and reported, the frame intact (S10 Text cells)
	scene := func(col, row int) Result {
		pg := turnGraph()
		pg.Groups = []model.PositionedGroup{{ID: "g", X: 20 * cellW, Y: 2 * cellH, Width: 6 * cellW, Height: 8 * cellH, Depth: 1}}
		e := &pg.Edges[0]
		e.Label = "yes"
		e.LabelPos, e.LabelWidth = boxAt(col, row, 3)
		e.LabelHeight = cellH
		return mustRender(t, pg)
	}
	r := scene(19, 5)
	assert.Empty(t, r.DroppedLabels)
	assert.Equal(t, "yes", textAt(t, r, 19, 5, 3), "over the left side, column 20 of row 5:\n%s", r.Art)
	assert.Equal(t, '│', cellAt(t, r, 20, 4), "the side intact above")

	dropped := []DroppedLabel{{ID: "e", From: "a", To: "b", Label: "yes"}}
	r = scene(19, 2)
	assert.Equal(t, dropped, r.DroppedLabels, "the top left corner")
	assert.Equal(t, '┌', cellAt(t, r, 20, 2))

	r = scene(21, 9)
	assert.Equal(t, dropped, r.DroppedLabels, "the bottom side")
	assert.Equal(t, '─', cellAt(t, r, 22, 9))
}

func TestGrid_AFrameSideCrossedByAWireIsNoLongerOnlyASide(t *testing.T) {
	// g's frame spans columns 2..7 and rows 1..6; a wire crosses both its
	// sides along row 4. A label may replace a cell of the left side, but
	// not where the wire crosses it, nor a corner or the top side (S10
	// Text cells)
	g := newCharGrid(12, 8)
	g.frame(2, 1, 6, 6, "", 5)
	g.keepFrames()
	g.hline(0, 10, 4, styleSolid)
	assert.True(t, g.frameSide(2, 3), "the left side")
	assert.False(t, g.frameSide(2, 4), "the left side where the wire crosses it")
	assert.False(t, g.frameSide(2, 1), "the top left corner")
	assert.False(t, g.frameSide(4, 1), "the top side")
	assert.False(t, drawAt(g, cellPoint{x: 2, y: 4}, "y"), "a label over the crossing is not drawn")
	assert.True(t, drawAt(g, cellPoint{x: 1, y: 3}, "yes"), "a label over the plain side is")
	assert.Equal(t, 'e', g.runeAt(2, 3))
}
