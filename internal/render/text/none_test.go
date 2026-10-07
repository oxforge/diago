package text

import (
	"testing"

	"github.com/oxforge/diago/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// No arrowhead glyph at either end of an undirected edge.
func TestRender_NoneHasNoArrowhead(t *testing.T) {
	a := model.PositionedNode{ID: "a", Label: "A", Lines: []string{"A"}, X: cellW, Y: cellH, Width: 7 * cellW, Height: 3 * cellH}
	b := model.PositionedNode{ID: "b", Label: "B", Lines: []string{"B"}, X: cellW, Y: 7 * cellH, Width: 7 * cellW, Height: 3 * cellH}
	mk := func(d model.EdgeDirection) *model.PositionedGraph {
		e := model.PositionedEdge{ID: "a->b#0", From: "a", To: "b", Direction: d,
			Points: []model.Point{{X: 4.5 * cellW, Y: 4 * cellH}, {X: 4.5 * cellW, Y: 7 * cellH}}}
		return &model.PositionedGraph{Width: 9 * cellW, Height: 11 * cellH, Nodes: []model.PositionedNode{a, b}, Edges: []model.PositionedEdge{e}}
	}
	res, err := Render(mk(model.EdgeNone))
	require.NoError(t, err)
	for _, head := range []string{"▼", "▲", "►", "◄"} {
		assert.NotContains(t, res.Art, head)
	}
	res, err = Render(mk(model.EdgeForward))
	require.NoError(t, err)
	assert.Contains(t, res.Art, "▼")
}
