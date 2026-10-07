package svg

import (
	"strings"
	"testing"

	"github.com/oxforge/diago/internal/model"
	"github.com/oxforge/diago/internal/theme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An undirected edge (direction none) references no marker at either end.
func TestRenderEdge_NoneHasNoMarkers(t *testing.T) {
	th, err := theme.Load("default")
	require.NoError(t, err)
	pg := &model.PositionedGraph{Width: 200, Height: 200,
		Nodes: []model.PositionedNode{
			{ID: "a", Label: "A", Lines: []string{"A"}, X: 20, Y: 20, Width: 60, Height: 40},
			{ID: "b", Label: "B", Lines: []string{"B"}, X: 20, Y: 120, Width: 60, Height: 40},
		},
		Edges: []model.PositionedEdge{{ID: "a->b#0", From: "a", To: "b", Direction: model.EdgeNone,
			Points: []model.Point{{X: 50, Y: 60}, {X: 50, Y: 120}}}},
	}
	got := Render(pg, th)
	edge := got[strings.Index(got, `id="edge-a-b"`):] // edgeElementID: derived id #0 renders as edge-<from>-<to>
	edge = edge[:strings.Index(edge, "</g>")]
	assert.NotContains(t, edge, "marker-end")
	assert.NotContains(t, edge, "marker-start")

	pg.Edges[0].Direction = model.EdgeForward
	got = Render(pg, th)
	assert.Contains(t, got, "marker-end")
}
