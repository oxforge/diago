package text

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/model"
)

// The title is centered over the art's width on the first
// line, followed by one blank line; no trailing whitespace anywhere.
func TestWithTitle(t *testing.T) {
	art := "┌──────────┐\n│ A        │\n└──────────┘\n"
	none, lines := withTitle(art, "")
	assert.Equal(t, art, none)
	assert.Equal(t, 0, lines)
	got, lines := withTitle(art, "Hi")
	assert.Equal(t, "     Hi\n\n"+art, got)
	assert.Equal(t, 2, lines)
	for _, l := range strings.Split(got, "\n") {
		assert.Equal(t, l, strings.TrimRight(l, " "))
	}
	long, lines := withTitle(art, "a title wider than the art itself")
	assert.Equal(t, "a title wider than the art itself\n\n"+art, long)
	assert.Equal(t, 2, lines)
}

func TestRender_TitleLine(t *testing.T) {
	// Node placed at cell (1,1), 7 cells wide by 3 tall, with a 1-cell
	// margin on every side of the 9x5-cell canvas (cellNode is text_test.go's
	// col/row/w/h-based builder, mirroring class_test.go's approach; the
	// brief's literal X: cellW, Y: cellH left the box clipped off-canvas).
	n := cellNode("a", "A", model.ShapeRect, 1, 1, 7, 3)
	pg := &model.PositionedGraph{Width: 9 * cellW, Height: 5 * cellH, Title: "Top", Nodes: []model.PositionedNode{n}}
	res, err := Render(pg)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(res.Art, "  Top\n\n"), res.Art)
}
