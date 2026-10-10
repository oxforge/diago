package layered

import (
	"context"
	"math"
	"testing"

	"github.com/oxforge/diago/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLayout_TextAdapterScalesCellsToPixels(t *testing.T) {
	g := graph(t, nil, "start->check:diamond", "check->ok", "check->retry", "retry->start", "ok->ok")
	g.Edges[1].Label = "yes"
	for _, dir := range []model.Direction{model.Down, model.Right} {
		g.Direction = dir
		cfg := TextConfig(dir)
		pg, err := Layout(context.Background(), g, cfg, nil)
		require.NoError(t, err)
		for _, n := range pg.Nodes {
			for _, v := range []float64{n.X - n.Width/2, n.X + n.Width/2} {
				assert.Equal(t, 0.0, math.Mod(v, 8), "%s %s: box edges on column boundaries", dir, n.ID)
			}
			for _, v := range []float64{n.Y - n.Height/2, n.Y + n.Height/2} {
				assert.Equal(t, 0.0, math.Mod(v, 16), "%s %s: box edges on row boundaries", dir, n.ID)
			}
		}
		yes := edge(t, pg, "check->ok#0")
		assert.Equal(t, [2]float64{24, 16}, [2]float64{yes.LabelWidth, yes.LabelHeight}, "three runes in one row")
		assert.Equal(t, 0.0, math.Mod(pg.Width, 8))
		assert.Equal(t, 0.0, math.Mod(pg.Height, 16))

		// S8: a self-loop keeps at least LoopMinH rows on the flow axis.
		// Under DOWN "ok" has no other reason to be this tall (its label
		// alone needs 3 cells); under RIGHT the flow axis is "ok"'s own
		// label width, which already exceeds LoopMinH, so the rule does
		// not bind there and is not asserted.
		if dir == model.Down {
			var ok model.PositionedNode
			for _, n := range pg.Nodes {
				if n.ID == "ok" {
					ok = n
				}
			}
			assert.Equal(t, cfg.LoopMinH*cellH, ok.Height, "%s: self-loop room (S8), scaled to px (S14)", dir)
		}

		// S14: the label's own position scales along with everything
		// else, so its box (LabelPos +/- half its now-scaled extents)
		// still sits inside the now-scaled canvas.
		require.NotNil(t, yes.LabelPos, "%s", dir)
		lowX, highX := yes.LabelPos.X-yes.LabelWidth/2, yes.LabelPos.X+yes.LabelWidth/2
		lowY, highY := yes.LabelPos.Y-yes.LabelHeight/2, yes.LabelPos.Y+yes.LabelHeight/2
		assert.GreaterOrEqual(t, lowX, 0.0, "%s: label box left edge inside the canvas", dir)
		assert.LessOrEqual(t, highX, pg.Width, "%s: label box right edge inside the canvas", dir)
		assert.GreaterOrEqual(t, lowY, 0.0, "%s: label box top edge inside the canvas", dir)
		assert.LessOrEqual(t, highY, pg.Height, "%s: label box bottom edge inside the canvas", dir)
	}
}

// TestScale_TheTextAdapterScalesEveryField is a direct test of the text
// adapter (S14) on a synthetic positioned graph: every geometry field it
// owns (a node's box, a record box's line heights, an edge's points, its
// label's position and extents, and both cardinalities' positions and
// extents, plus the canvas) scales by sx and sy. Going through Layout on a real fixture cannot pin this
// precisely, because wire-derived positions (ports, labels placed off a
// jog) are not themselves grid-aligned, only quantities the placement
// stage owns are (S7); this test isolates the adapter from that noise.
func TestScale_TheTextAdapterScalesEveryField(t *testing.T) {
	pg := &model.PositionedGraph{
		Nodes: []model.PositionedNode{
			{X: 1, Y: 2, Width: 3, Height: 4},
			{Members: &model.PositionedMembers{HeaderLineHeight: 1, MemberLineHeight: 2}},
		},
		Edges: []model.PositionedEdge{{
			Points:      []model.Point{{X: 1, Y: 1}, {X: 2, Y: 3}},
			LabelPos:    &model.Point{X: 4, Y: 5},
			LabelWidth:  6,
			LabelHeight: 7,
			FromCard:    &model.EndLabel{Pos: &model.Point{X: 8, Y: 9}, Width: 10, Height: 11},
			ToCard:      &model.EndLabel{Pos: &model.Point{X: 12, Y: 13}, Width: 14, Height: 15},
		}},
		Width:  16,
		Height: 17,
	}
	scale(pg, model.Down, 8, 16)

	assert.Equal(t, model.PositionedNode{X: 8, Y: 32, Width: 24, Height: 64}, pg.Nodes[0], "a node's box scales")
	m := pg.Nodes[1].Members
	assert.Equal(t, [2]float64{16, 32}, [2]float64{m.HeaderLineHeight, m.MemberLineHeight}, "a record box's line heights scale by the row")
	e := pg.Edges[0]
	assert.Equal(t, []model.Point{{X: 8, Y: 16}, {X: 16, Y: 48}}, e.Points, "an edge's points scale")
	assert.Equal(t, &model.Point{X: 32, Y: 80}, e.LabelPos, "a label's position scales")
	assert.Equal(t, [2]float64{48, 112}, [2]float64{e.LabelWidth, e.LabelHeight}, "a label's own extents scale")
	assert.Equal(t, &model.Point{X: 64, Y: 144}, e.FromCard.Pos, "a cardinality's position scales too, not just its extents")
	assert.Equal(t, [2]float64{80, 176}, [2]float64{e.FromCard.Width, e.FromCard.Height}, "a cardinality's extents scale")
	assert.Equal(t, &model.Point{X: 96, Y: 208}, e.ToCard.Pos, "the other cardinality's position scales")
	assert.Equal(t, [2]float64{112, 240}, [2]float64{e.ToCard.Width, e.ToCard.Height}, "the other cardinality's extents scale")
	assert.Equal(t, [2]float64{128, 272}, [2]float64{pg.Width, pg.Height}, "the canvas scales")
}

// TestLayout_TextAdapterScalesCardinalities confirms the S14 adapter's
// cardinality scaling (TestScale_TheTextAdapterScalesEveryField's rule)
// through the real pipeline, on a fixture where every wire the
// cardinalities sit on is straight, so their positions land on the
// half-cell grid rather than the thirds a slanted jog can produce (see
// TestLayout_TextAdapterScalesCellsToPixels's label-position check for a
// fixture where that is not true).
func TestLayout_TextAdapterScalesCardinalities(t *testing.T) {
	g := model.Graph{Direction: model.Down, Legend: &model.Legend{},
		Nodes: []model.Node{
			{ID: "animal", Label: "Animal", Members: &model.Members{Methods: []model.Member{{Visibility: "+", Text: "speak(): string"}}}},
			{ID: "dog", Label: "Dog", Members: &model.Members{}},
			{ID: "owner", Label: "Owner", Members: &model.Members{}},
		},
		Edges: []model.Edge{
			{ID: "dog->animal#0", From: "animal", To: "dog", Relation: model.RelationInheritance},
			{ID: "owner->dog#0", From: "owner", To: "dog", Relation: model.RelationComposition, Label: "owns", FromCard: "1", ToCard: "*"},
			{ID: "dog->owner#0", From: "dog", To: "owner", Relation: model.RelationAssociation, FromCard: "0..*", ToCard: "1"},
		},
	}
	pg, err := Layout(context.Background(), g, TextConfig(model.Down), nil)
	require.NoError(t, err)
	for _, id := range []string{"owner->dog#0", "dog->owner#0"} {
		e := edge(t, pg, id)
		for _, c := range []*model.EndLabel{e.FromCard, e.ToCard} {
			require.NotNil(t, c, "%s", id)
			require.NotNil(t, c.Pos, "%s", id)
			assert.GreaterOrEqual(t, c.Pos.X, 0.0, "%s: inside the canvas", id)
			assert.LessOrEqual(t, c.Pos.X, pg.Width, "%s: inside the canvas", id)
			assert.GreaterOrEqual(t, c.Pos.Y, 0.0, "%s: inside the canvas", id)
			assert.LessOrEqual(t, c.Pos.Y, pg.Height, "%s: inside the canvas", id)
			assert.Equal(t, 0.0, math.Mod(c.Pos.X, cellW/2), "%s: cardinality position on the half-cell grid (S14)", id)
			assert.Equal(t, 0.0, math.Mod(c.Pos.Y, cellH/2), "%s: cardinality position on the half-cell grid (S14)", id)
		}
	}
}

// TestScale_Groups pins the text adapter (S14) for groups: a group's box,
// its title extents and its title offset scale from cells to px like
// every other box.
func TestScale_Groups(t *testing.T) {
	pg := &model.PositionedGraph{Groups: []model.PositionedGroup{{ID: "g", X: 2, Y: 1, Width: 20, Height: 6, LabelWidth: 5, LabelHeight: 1, LabelOffset: 3}}}
	scale(pg, model.Down, 8, 16)
	g := pg.Groups[0]
	assert.Equal(t, [7]float64{16, 16, 160, 96, 40, 16, 24}, [7]float64{g.X, g.Y, g.Width, g.Height, g.LabelWidth, g.LabelHeight, g.LabelOffset})
}
