package frame

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/oxforge/diago/internal/model"
)

func resolveCases() []struct {
	name string
	g    model.Graph
	want model.Direction
} {
	nodes := func(n int) []model.Node { return make([]model.Node, n) }
	edges := func(n int) []model.Edge { return make([]model.Edge, n) }
	return []struct {
		name string
		g    model.Graph
		want model.Direction
	}{
		{"an explicit direction stays", model.Graph{Direction: model.Left, Nodes: nodes(5)}, model.Left},
		{"AUTO with more nodes than edges plus one", model.Graph{Nodes: nodes(4), Edges: edges(2)}, model.Right},
		{"AUTO with enough edges", model.Graph{Nodes: nodes(4), Edges: edges(3)}, model.Down},
		{"AUTO on an empty graph", model.Graph{}, model.Down},
		// a flat edge implies no flow (S9): three nodes and one ordinary
		// edge lay out RIGHT, though with the flat one they have two
		{"AUTO counts no flat edge", model.Graph{Nodes: nodes(3), Edges: []model.Edge{{}, {Flat: true}}}, model.Right},
	}
}

func TestResolve(t *testing.T) {
	for _, tt := range resolveCases() {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Resolve(context.Background(), tt.g))
		})
	}
}

// TestResolveQuiet checks that ResolveQuiet agrees with Resolve on every
// case. ResolveQuiet takes no context, so it cannot emit a
// "direction_resolved" record; that guarantee (a --debug render resolving
// AUTO emits exactly one) is pinned end-to-end by
// TestRender_DebugResolvesAutoOnce in internal/pipeline.
func TestResolveQuiet(t *testing.T) {
	for _, tt := range resolveCases() {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ResolveQuiet(tt.g))
		})
	}
}

// engine is a small layout in the engine's frame: a above b, a wire from
// a's bottom to b's top, a 30 x 10 label and a 10 x 10 cardinality.
func engine() *model.PositionedGraph {
	return &model.PositionedGraph{
		Nodes: []model.PositionedNode{
			{ID: "a", X: 100, Y: 20, Width: 80, Height: 40},
			{ID: "b", X: 100, Y: 100, Width: 80, Height: 40},
		},
		Edges: []model.PositionedEdge{{
			ID: "a->b#0", Points: []model.Point{{X: 100, Y: 40}, {X: 100, Y: 80}},
			LabelPos: &model.Point{X: 130, Y: 60}, LabelWidth: 30, LabelHeight: 10,
			ToCard: &model.EndLabel{Text: "1", Pos: &model.Point{X: 110, Y: 70}, Width: 10, Height: 10},
		}},
	}
}

func TestApply(t *testing.T) {
	for _, tt := range []struct {
		dir        model.Direction
		a, b       model.Point // node centers
		w, h       float64     // a's extents
		first      model.Point // the wire's first point
		label      model.Point
		lw, lh     float64
		card       model.Point
		canvasW, H float64
	}{
		// DOWN keeps the frame: geometry spans x 60..145, y 0..120
		{model.Down, model.Point{X: 60, Y: 40}, model.Point{X: 60, Y: 120}, 80, 40,
			model.Point{X: 60, Y: 60}, model.Point{X: 90, Y: 80}, 30, 10, model.Point{X: 70, Y: 90}, 125, 160},
		// UP mirrors it vertically
		{model.Up, model.Point{X: 60, Y: 120}, model.Point{X: 60, Y: 40}, 80, 40,
			model.Point{X: 60, Y: 100}, model.Point{X: 90, Y: 80}, 30, 10, model.Point{X: 70, Y: 70}, 125, 160},
		// RIGHT transposes it: every box swaps its extents
		{model.Right, model.Point{X: 40, Y: 60}, model.Point{X: 120, Y: 60}, 40, 80,
			model.Point{X: 60, Y: 60}, model.Point{X: 80, Y: 90}, 10, 30, model.Point{X: 90, Y: 70}, 160, 125},
		// LEFT mirrors RIGHT horizontally
		{model.Left, model.Point{X: 120, Y: 60}, model.Point{X: 40, Y: 60}, 40, 80,
			model.Point{X: 100, Y: 60}, model.Point{X: 80, Y: 90}, 10, 30, model.Point{X: 70, Y: 70}, 160, 125},
	} {
		t.Run(tt.dir.String(), func(t *testing.T) {
			pg := engine()
			Apply(context.Background(), pg, tt.dir, 20, 20, false)
			assert.Equal(t, tt.a, model.Point{X: pg.Nodes[0].X, Y: pg.Nodes[0].Y})
			assert.Equal(t, tt.b, model.Point{X: pg.Nodes[1].X, Y: pg.Nodes[1].Y})
			assert.Equal(t, [2]float64{tt.w, tt.h}, [2]float64{pg.Nodes[0].Width, pg.Nodes[0].Height})
			e := pg.Edges[0]
			assert.Equal(t, tt.first, e.Points[0])
			assert.Equal(t, tt.label, *e.LabelPos)
			assert.Equal(t, [2]float64{tt.lw, tt.lh}, [2]float64{e.LabelWidth, e.LabelHeight})
			assert.Equal(t, tt.card, *e.ToCard.Pos)
			assert.Equal(t, [2]float64{tt.canvasW, tt.H}, [2]float64{pg.Width, pg.Height}, "the extent plus a margin on every side")
		})
	}
}

func TestApply_MarginsArePerAxis(t *testing.T) {
	pg := engine()
	Apply(context.Background(), pg, model.Down, 16, 8, false)
	assert.Equal(t, 16.0, pg.Nodes[0].X-pg.Nodes[0].Width/2, "the leftmost box starts at the x margin")
	assert.Equal(t, 8.0, pg.Nodes[0].Y-pg.Nodes[0].Height/2, "the topmost box starts at the y margin")
	assert.Equal(t, 85.0+32, pg.Width)
	assert.Equal(t, 120.0+16, pg.Height)
}

// TestApply_MapsGroups pins S11 for groups: a group's box maps with the
// direction and swaps its extents under RIGHT and LEFT, its title extents
// stay, and the canvas holds it.
func TestApply_MapsGroups(t *testing.T) {
	for _, tt := range []struct {
		dir        model.Direction
		x, y, w, h float64
		canvasW, H float64
	}{
		{model.Down, 20, 20, 120, 80, 160, 120},
		{model.Up, 20, 20, 120, 80, 160, 120},
		{model.Right, 20, 20, 80, 120, 120, 160},
		{model.Left, 20, 20, 80, 120, 120, 160},
	} {
		t.Run(tt.dir.String(), func(t *testing.T) {
			pg := &model.PositionedGraph{Groups: []model.PositionedGroup{{ID: "g", X: -10, Y: 5, Width: 120, Height: 80, LabelWidth: 30, LabelHeight: 14}}}
			Apply(context.Background(), pg, tt.dir, 20, 20, false)
			g := pg.Groups[0]
			assert.Equal(t, [4]float64{tt.x, tt.y, tt.w, tt.h}, [4]float64{g.X, g.Y, g.Width, g.Height})
			assert.Equal(t, [2]float64{30, 14}, [2]float64{g.LabelWidth, g.LabelHeight})
			assert.Equal(t, [2]float64{tt.canvasW, tt.H}, [2]float64{pg.Width, pg.Height})
		})
	}
}

// TestApply_TextKeepsTheCellGrid pins S11 in the text profile: a route
// point half a cell outside every box (a side column on a cell's middle,
// S8) still moves the layout by whole cells, so every box stays on the
// cell grid (S7), and the canvas spans whole cells.
func TestApply_TextKeepsTheCellGrid(t *testing.T) {
	pg := &model.PositionedGraph{
		Nodes: []model.PositionedNode{{ID: "a", X: 4, Y: 1.5, Width: 8, Height: 3}},
		Edges: []model.PositionedEdge{{ID: "a->a#0", Points: []model.Point{{X: 0, Y: 1.5}, {X: -3.5, Y: 1.5}, {X: -3.5, Y: 5.5}}}},
	}
	Apply(context.Background(), pg, model.Down, 2, 1, true)
	a := pg.Nodes[0]
	assert.Equal(t, 6.0, a.X-a.Width/2, "the box starts on a cell boundary: 2 + 4 cells to the column's cell")
	assert.Equal(t, 2.5, pg.Edges[0].Points[1].X, "the column on its cell's middle, 2 from the canvas's left")
	assert.Equal(t, 16.0, pg.Width, "whole cells: from the column's cell to the box, and the margins")
}
