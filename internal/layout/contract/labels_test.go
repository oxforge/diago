package contract

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/oxforge/diago/internal/model"
)

// labeled is chain() with a 24 x 14 label on the a->b wire (x=60, y 60..120),
// centered at (x, y).
func labeled(x, y float64) *model.PositionedGraph {
	pg := chain()
	pg.Edges[0].Label = "go"
	pg.Edges[0].LabelPos = &model.Point{X: x, Y: y}
	pg.Edges[0].LabelWidth, pg.Edges[0].LabelHeight = 24, 14
	return pg
}

func TestC14_CleanLabel(t *testing.T) {
	assert.Empty(t, check(labeled(78, 90)), "6 px beside its own wire")
}

func TestC14_1_Overlaps(t *testing.T) {
	t.Run("node", func(t *testing.T) {
		assert.True(t, has(check(labeled(60, 40)), "C14.1", "a->b#0"))
	})
	t.Run("group title", func(t *testing.T) {
		pg := labeled(78, 90)
		pg.Groups = []model.PositionedGroup{{ID: "g", Label: "G", X: 62, Y: 72, Width: 50, Height: 40, LabelWidth: 20, LabelHeight: 14}}
		assert.True(t, has(check(pg), "C14.1", "a->b#0"))
	})
	t.Run("group title placed clear of it", func(t *testing.T) {
		pg := labeled(78, 90)
		pg.Groups = []model.PositionedGroup{{ID: "g", Label: "G", X: 62, Y: 72, Width: 80, Height: 40, LabelWidth: 20, LabelHeight: 14, LabelOffset: 30}}
		assert.False(t, has(check(pg), "C14.1", "a->b#0"), "the title box starts at 92, the label ends at 90")
	})
	t.Run("group title's blank in text", func(t *testing.T) {
		// g's title spans columns 7..11 of row 2, its blanks columns 6
		// and 12; a five-column label on row 2 ends at column 6 or 5.
		text := Options{Limits: TextLimits(model.Down), Direction: model.Down, Text: true}
		scene := func(x float64) []Violation {
			e := wire("p", "q", 200, 0, 200, 120)
			e.Label, e.LabelPos, e.LabelWidth, e.LabelHeight = "HTTPS", &model.Point{X: x, Y: 40}, 40, 16
			pg := layout(240, 160, nil, e)
			pg.Groups = []model.PositionedGroup{{ID: "g", Label: "G", X: 16, Y: 32, Width: 160, Height: 96, Depth: 1,
				LabelWidth: 40, LabelHeight: 16, LabelOffset: 40}}
			return filter(Check(pg, text), "C14.1")
		}
		assert.NotEmpty(t, scene(36), "the label's last column is the blank before the title")
		assert.Empty(t, scene(28), "one column further out")
	})
	t.Run("another label, reported once", func(t *testing.T) {
		pg := labeled(78, 90)
		other := wire("c", "d", 100, 60, 100, 120)
		other.Label, other.LabelPos, other.LabelWidth, other.LabelHeight = "x", &model.Point{X: 84, Y: 92}, 24, 14
		pg.Edges = append(pg.Edges, other)
		assert.Len(t, filter(check(pg), "C14.1"), 1)
	})
	t.Run("cardinality on a node", func(t *testing.T) {
		pg := chain()
		pg.Edges[0].FromCard = &model.EndLabel{Text: "1", Pos: &model.Point{X: 60, Y: 50}, Width: 8, Height: 14}
		assert.True(t, has(check(pg), "C14.1", "a->b#0"))
	})
	t.Run("adornment envelope", func(t *testing.T) {
		pg := labeled(60, 67) // over the inheritance triangle at a's end
		pg.Edges[0].Relation = model.RelationInheritance
		assert.True(t, has(check(pg), "C14.1", "a->b#0"))
	})
}

func TestC14_2_StruckByAnotherEdge(t *testing.T) {
	pg := labeled(78, 90)
	pg.Edges = append(pg.Edges, wire("c", "d", 60, 90, 120, 90))
	assert.True(t, has(check(pg), "C14.2", "a->b#0"))
}

// besideInText is a text layout: p->q's wire runs down column 25, and its
// five-column label sits centered on (x, y), at (244, 56) on columns
// 28..32 of row 3, one blank column right of the wire; more edges follow.
func besideInText(x, y float64, more ...model.PositionedEdge) *model.PositionedGraph {
	e := wire("p", "q", 204, 0, 204, 160)
	e.Label, e.LabelPos, e.LabelWidth, e.LabelHeight = "HTTPS", &model.Point{X: x, Y: y}, 40, 16
	return layout(400, 200, nil, append([]model.PositionedEdge{e}, more...)...)
}

func TestC14_2_TextReadsTheCellsAWireIsDrawnIn(t *testing.T) {
	// another edge's wire along a side of the label box: on screen it
	// runs through nothing; in text it is drawn in the cell right of or
	// below the boundary it lies on, which is the box's own cell along
	// its top and left sides (C14.2)
	text := Options{Limits: TextLimits(model.Down), Direction: model.Down, Text: true}
	for _, tc := range []struct {
		name string
		xy   []float64
		want bool
	}{
		{"along its top side, drawn in its row", []float64{216, 48, 300, 48}, true},
		{"along its bottom side, drawn in the row below", []float64{216, 64, 300, 64}, false},
		{"along its left side, drawn in its first column", []float64{224, 0, 224, 160}, true},
		{"along its right side, drawn in the column right of it", []float64{264, 0, 264, 160}, false},
	} {
		pg := besideInText(244, 56, wire("c", "d", tc.xy...))
		assert.Empty(t, filter(check(pg), "C14.2"), "%s: on screen", tc.name)
		assert.Equal(t, tc.want, has(Check(pg, text), "C14.2", "p->q#0"), "%s: in text", tc.name)
	}
}

func TestC14_5_TextBoxesLieOnWholeCells(t *testing.T) {
	text := Options{Limits: TextLimits(model.Down), Direction: model.Down, Text: true}
	assert.Empty(t, filter(Check(besideInText(244, 56), text), "C14.5"), "columns 28..32 of row 3")
	assert.True(t, has(Check(besideInText(240, 56), text), "C14.5", "p->q#0"), "half a column off")
	assert.True(t, has(Check(besideInText(244, 48), text), "C14.5", "p->q#0"), "half a row off")
	assert.Empty(t, filter(check(besideInText(240, 48)), "C14.5"), "a screen box lies anywhere")

	card := besideInText(244, 56)
	card.Edges[0].ToCard = &model.EndLabel{Text: "1", Pos: &model.Point{X: 228, Y: 136}, Width: 8, Height: 16}
	assert.Empty(t, filter(Check(card, text), "C14.5"), "a cardinality on column 28 of row 8")
	card.Edges[0].ToCard.Pos.Y = 144
	assert.True(t, has(Check(card, text), "C14.5", "p->q#0"), "a cardinality half a row off")

	unresolved := besideInText(240, 48)
	unresolved.Edges[0].LabelUnresolved = true
	assert.Empty(t, filter(Check(unresolved, text), "C14.5"), "exempt (C14.4)")
}

func TestC14_3_OwnEdgeGap(t *testing.T) {
	assert.True(t, has(check(labeled(76, 90)), "C14.3", "a->b#0"), "4 px from its wire")
	assert.True(t, has(check(labeled(60, 90)), "C14.3", "a->b#0"), "on its wire")
	assert.False(t, has(check(labeled(79, 90)), "C14.3", "a->b#0"), "7 px from its wire")
}

func TestC14_4_Exemptions(t *testing.T) {
	unresolved := labeled(60, 40)
	unresolved.Edges[0].LabelUnresolved = true
	assert.Empty(t, filter(check(unresolved), "C14.1"), "the resolver's recorded fallback")

	// A congruence-owned label is checked like any other (C17.4 dropped
	// at the cutover): AppliedCongruences no longer buys an overlapping
	// label an exemption.
	owned := labeled(60, 40)
	owned.AppliedCongruences = []model.AppliedCongruence{{FanOut: []int{0}}}
	assert.True(t, has(check(owned), "C14.1", "a->b#0"), "a congruence-owned label overlapping a node is a violation")
}

func TestEnvelope(t *testing.T) {
	e := wire("a", "b", 60, 60, 60, 120)
	c := newChecker(chain(), Options{Limits: ScreenLimits()})
	_, ok := c.envelope(e)
	assert.False(t, ok, "a flow edge has no adornment")

	e.Relation = model.RelationComposition
	b, ok := c.envelope(e)
	assert.True(t, ok)
	assert.Equal(t, box{53, 60, 67, 74}, b)
}

func TestC14_3_TextReadsTheCellsItsOwnWireIsDrawnIn(t *testing.T) {
	// p->q runs along the boundary above row 4, drawn in row 4. In text
	// a label may take the row next to it on either side, though the
	// box touches the wire on screen; it may not hold a cell the wire is
	// drawn in, nor have one in the cell before or after it along its
	// row (C14.3)
	text := Options{Limits: TextLimits(model.Down), Direction: model.Down, Text: true}
	scene := func(x, y float64, more ...model.PositionedEdge) *model.PositionedGraph {
		e := wire("p", "q", 100, 64, 300, 64)
		e.Label, e.LabelPos, e.LabelWidth, e.LabelHeight = "HTTPS", &model.Point{X: x, Y: y}, 40, 16
		return layout(400, 200, nil, append([]model.PositionedEdge{e}, more...)...)
	}
	assert.True(t, has(check(scene(164, 56)), "C14.3", "p->q#0"), "on screen: 0 px from its wire")
	assert.False(t, has(Check(scene(164, 56), text), "C14.3", "p->q#0"), "row 3, next to row 4")
	assert.False(t, has(Check(scene(164, 88), text), "C14.3", "p->q#0"), "row 5, next to row 4")
	assert.True(t, has(Check(scene(164, 72), text), "C14.3", "p->q#0"), "row 4, on its wire's cells")

	// q->r leaves q down column 17, its own edge too: the label on
	// columns 18..22 has it in the cell before it, columns 19..23 a blank
	down := wire("p", "q", 140, 0, 140, 200)
	besideOwn := func(x float64) []Violation {
		e := down
		e.Label, e.LabelPos, e.LabelWidth, e.LabelHeight = "HTTPS", &model.Point{X: x, Y: 56}, 40, 16
		return Check(layout(400, 200, nil, e), text)
	}
	assert.True(t, has(besideOwn(164), "C14.3", "p->q#0"), "column 17, its wire's, right before it")
	assert.False(t, has(besideOwn(172), "C14.3", "p->q#0"), "a blank column between")
}

func TestC14_5_TextCoversNoFramesTopOrBottomSide(t *testing.T) {
	// g's frame spans columns 16..35 and rows 2..7: its top and bottom
	// sides run along a label's row and its corners lie on them; its left
	// and right sides cross it, and a label may cover them (C14.5)
	text := Options{Limits: TextLimits(model.Down), Direction: model.Down, Text: true}
	scene := func(x, y float64) []Violation {
		e := wire("p", "q", 300, 0, 300, 200)
		e.Label, e.LabelPos, e.LabelWidth, e.LabelHeight = "HTTPS", &model.Point{X: x, Y: y}, 40, 16
		pg := layout(400, 200, nil, e)
		pg.Groups = []model.PositionedGroup{{ID: "g", X: 128, Y: 32, Width: 160, Height: 96, Depth: 1}}
		return filter(Check(pg, text), "C14.5")
	}
	assert.NotEmpty(t, scene(180, 40), "on the top side, row 2")
	assert.NotEmpty(t, scene(132, 120), "on the bottom left corner, column 16 of row 7")
	assert.Empty(t, scene(132, 56), "over the left side, column 16 of row 3")
	assert.Empty(t, scene(180, 56), "inside the frame")
}
