package text

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/model"
)

// classCellNode builds a text-profile node at cell (col,row) with w x h
// cells. Named distinctly from text_test.go's cellNode (which additionally
// takes a label and shape) to avoid a redeclaration in this package.
func classCellNode(id string, col, row, w, h int) model.PositionedNode {
	return model.PositionedNode{ID: id, Label: id, Shape: model.ShapeRect,
		X: float64(col)*cellW + float64(w)*cellW/2, Y: float64(row)*cellH + float64(h)*cellH/2,
		Width: float64(w) * cellW, Height: float64(h) * cellH}
}

func TestRender_ClassBox(t *testing.T) {
	n := classCellNode("order", 0, 0, 22, 8)
	n.Members = &model.PositionedMembers{
		Header:           []model.PositionedLine{{Text: "«entity»", Stereotype: true}, {Text: "Order"}},
		Attributes:       []model.PositionedLine{{Text: "- id: string"}},
		Methods:          []model.PositionedLine{{Text: "+ total(): Money *", Italic: true}},
		HeaderLineHeight: 16, MemberLineHeight: 16,
	}
	pg := &model.PositionedGraph{Width: 22 * cellW, Height: 8 * cellH, Nodes: []model.PositionedNode{n}}
	r, err := Render(pg)
	require.NoError(t, err)
	assert.Equal(t, strings.Join([]string{
		"┌────────────────────┐",
		"│      «entity»      │",
		"│       Order        │",
		"├────────────────────┤",
		"│ - id: string       │",
		"├────────────────────┤",
		"│ + total(): Money * │",
		"└────────────────────┘",
	}, "\n")+"\n", r.Art)
}

func TestRender_ClassBoxNoEmptyCompartments(t *testing.T) {
	n := classCellNode("a", 0, 0, 8, 3)
	n.Members = &model.PositionedMembers{Header: []model.PositionedLine{{Text: "A"}}, HeaderLineHeight: 16, MemberLineHeight: 16}
	pg := &model.PositionedGraph{Width: 8 * cellW, Height: 3 * cellH, Nodes: []model.PositionedNode{n}}
	r, err := Render(pg)
	require.NoError(t, err)
	assert.Equal(t, "┌──────┐\n│  A   │\n└──────┘\n", r.Art)
}

func TestRender_AdornmentGlyphs(t *testing.T) {
	// a at rows 0-2 cols 0-7, b at rows 6-8 cols 0-7; wire down column 4.
	a, b := classCellNode("a", 0, 0, 8, 3), classCellNode("b", 0, 6, 8, 3)
	mk := func(rel model.Relation, pts ...model.Point) model.PositionedEdge {
		return model.PositionedEdge{ID: "e", From: "a", To: "b", Relation: rel, Points: pts}
	}
	down := []model.Point{{X: 4 * cellW, Y: 3 * cellH}, {X: 4 * cellW, Y: 6 * cellH}}
	cases := []struct {
		rel   model.Relation
		glyph rune
	}{{model.RelationInheritance, '△'}, {model.RelationRealization, '△'}, {model.RelationAggregation, '◇'}, {model.RelationComposition, '◆'}}
	for _, c := range cases {
		pg := &model.PositionedGraph{Width: 8 * cellW, Height: 9 * cellH, Nodes: []model.PositionedNode{a, b}, Edges: []model.PositionedEdge{mk(c.rel, down...)}}
		r, err := Render(pg)
		require.NoError(t, err)
		lines := strings.Split(r.Art, "\n")
		assert.Equal(t, c.glyph, []rune(lines[3])[4], "glyph in the cell below the source border (%s)", c.rel)
		assert.NotContains(t, r.Art, "▼", "adorned relations have no arrowhead")
	}
	// Up: b above a. Wire from a's top face up to b.
	up := []model.Point{{X: 4 * cellW, Y: 6 * cellH}, {X: 4 * cellW, Y: 3 * cellH}}
	pg := &model.PositionedGraph{Width: 8 * cellW, Height: 9 * cellH, Nodes: []model.PositionedNode{a, b}, Edges: []model.PositionedEdge{{ID: "e", From: "b", To: "a", Relation: model.RelationInheritance, Points: up}}}
	r, err := Render(pg)
	require.NoError(t, err)
	assert.Equal(t, '▽', []rune(strings.Split(r.Art, "\n")[5])[4])
	// Right: a at cols 0-7, c at cols 12-19, same rows; the wire leaves a's right face.
	c := classCellNode("c", 12, 0, 8, 3)
	right := []model.Point{{X: 8 * cellW, Y: 1*cellH + 8}, {X: 12 * cellW, Y: 1*cellH + 8}}
	pg = &model.PositionedGraph{Width: 20 * cellW, Height: 3 * cellH, Nodes: []model.PositionedNode{a, c}, Edges: []model.PositionedEdge{{ID: "e", From: "a", To: "c", Relation: model.RelationInheritance, Points: right}}}
	r, err = Render(pg)
	require.NoError(t, err)
	assert.Equal(t, '◁', []rune(strings.Split(r.Art, "\n")[1])[8], "side departure: glyph on the snapped cell outside the face, pointing back at the source")
	left := []model.Point{{X: 12 * cellW, Y: 1*cellH + 8}, {X: 8 * cellW, Y: 1*cellH + 8}}
	pg.Edges = []model.PositionedEdge{{ID: "e", From: "c", To: "a", Relation: model.RelationInheritance, Points: left}}
	r, err = Render(pg)
	require.NoError(t, err)
	assert.Equal(t, '▷', []rune(strings.Split(r.Art, "\n")[1])[11])
}

func TestRender_DirectedAssociationAndDependencyHeads(t *testing.T) {
	a, b := classCellNode("a", 0, 0, 8, 3), classCellNode("b", 0, 6, 8, 3)
	down := []model.Point{{X: 4 * cellW, Y: 3 * cellH}, {X: 4 * cellW, Y: 6 * cellH}}
	pg := &model.PositionedGraph{Width: 8 * cellW, Height: 9 * cellH, Nodes: []model.PositionedNode{a, b}, Edges: []model.PositionedEdge{
		{ID: "e", From: "a", To: "b", Relation: model.RelationDependency, Directed: true, Style: model.EdgeDashed, Points: down}}}
	r, err := Render(pg)
	require.NoError(t, err)
	assert.Equal(t, '▼', []rune(strings.Split(r.Art, "\n")[5])[4])
	assert.Equal(t, '╎', []rune(strings.Split(r.Art, "\n")[4])[4])
	pg.Edges[0] = model.PositionedEdge{ID: "e", From: "a", To: "b", Relation: model.RelationAssociation, Points: down}
	r, err = Render(pg)
	require.NoError(t, err)
	assert.NotContains(t, r.Art, "▼")
	assert.NotContains(t, r.Art, "▲")
}

func TestRender_CardsPlacedAndDropped(t *testing.T) {
	a, b := classCellNode("a", 0, 0, 8, 3), classCellNode("b", 0, 7, 8, 3)
	down := []model.Point{{X: 4 * cellW, Y: 3 * cellH}, {X: 4 * cellW, Y: 7 * cellH}}
	e := model.PositionedEdge{ID: "r1", From: "a", To: "b", Relation: model.RelationComposition, Points: down,
		FromCard: &model.EndLabel{Text: "1", Pos: &model.Point{X: 6*cellW + 4, Y: 4*cellH + 8}, Width: 8, Height: 16},
		ToCard:   &model.EndLabel{Text: "*", Pos: &model.Point{X: 6*cellW + 4, Y: 5*cellH + 8}, Width: 8, Height: 16}}
	pg := &model.PositionedGraph{Width: 12 * cellW, Height: 10 * cellH, Nodes: []model.PositionedNode{a, b}, Edges: []model.PositionedEdge{e}}
	r, err := Render(pg)
	require.NoError(t, err)
	lines := strings.Split(r.Art, "\n")
	assert.Equal(t, '1', []rune(lines[4])[6])
	assert.Equal(t, '*', []rune(lines[5])[6])
	assert.Empty(t, r.DroppedLabels)

	// A resolved card whose box holds ink (a node) is dropped with its end,
	// never drawn over it nor moved (S10 Text cells).
	wall := classCellNode("w", 8, 4, 7, 3)
	e.ToCard = &model.EndLabel{Text: "0..*", Pos: &model.Point{X: 9 * cellW, Y: 5*cellH + 8}, Width: 32, Height: 16}
	pg = &model.PositionedGraph{Width: 16 * cellW, Height: 10 * cellH, Nodes: []model.PositionedNode{a, b, wall}, Edges: []model.PositionedEdge{e}}
	r, err = Render(pg)
	require.NoError(t, err)
	require.Len(t, r.DroppedLabels, 1)
	assert.Equal(t, DroppedLabel{ID: "r1", From: "a", To: "b", Label: "0..*", End: "to"}, r.DroppedLabels[0])
	assert.NotContains(t, r.Art, "0..*")
}

func TestRender_LegendRows(t *testing.T) {
	assert.Equal(t, []string{"◁── is-a", "◁╌╌ implements", "◆── composition", "◇── aggregation", "╌╌► depends", "─── association"},
		legendRows([]model.LegendEntry{{Kind: "inheritance", Label: "is-a"}, {Kind: "realization", Label: "implements"}, {Kind: "composition", Label: "composition"}, {Kind: "aggregation", Label: "aggregation"}, {Kind: "dependency", Label: "depends"}, {Kind: "association", Label: "association"}}))
	n := classCellNode("a", 0, 0, 8, 3)
	n.Members = &model.PositionedMembers{Header: []model.PositionedLine{{Text: "A"}}, HeaderLineHeight: 16, MemberLineHeight: 16}
	pg := &model.PositionedGraph{Width: 8 * cellW, Height: 3 * cellH, Nodes: []model.PositionedNode{n}, Legend: []model.LegendEntry{{Kind: "composition", Label: "owns"}}}
	r, err := Render(pg)
	require.NoError(t, err)
	assert.Equal(t, []string{"◆── owns"}, r.Legend)
	assert.NotContains(t, r.Art, "owns", "the legend is not part of the art")
}
