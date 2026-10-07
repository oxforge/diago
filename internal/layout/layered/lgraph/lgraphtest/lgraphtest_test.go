package lgraphtest_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/layout/layered/lgraph"
	"github.com/oxforge/diago/internal/layout/layered/lgraph/lgraphtest"
	"github.com/oxforge/diago/internal/model"
)

func TestLevel_Parse(t *testing.T) {
	lv := lgraphtest.Level(t, "a->m:diamond", "m->a", "m->a", "iso")
	require.Len(t, lv.Nodes, 3)
	assert.Equal(t, []string{"a", "m", "iso"}, []string{lv.Nodes[0].ID, lv.Nodes[1].ID, lv.Nodes[2].ID})
	assert.Equal(t, model.ShapeDiamond, lv.Nodes[1].Shape)
	assert.Equal(t, lgraph.Node{ID: "a", Shape: model.ShapeRect, W: 80, H: 40}, lv.Nodes[0])
	assert.Equal(t, []lgraph.Edge{
		{ID: "a->m#0", From: 0, To: 1},
		{ID: "m->a#0", From: 1, To: 0},
		{ID: "m->a#1", From: 1, To: 0},
	}, lv.Edges)
}
