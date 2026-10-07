package equalize

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/oxforge/diago/internal/layout/layered/lgraph"
	"github.com/oxforge/diago/internal/layout/layered/lgraph/lgraphtest"
)

func heights(nodes []lgraph.Node) []float64 {
	out := make([]float64, len(nodes))
	for i, n := range nodes {
		out[i] = n.H
	}
	return out
}

func TestApply(t *testing.T) {
	t.Run("a row of boxes shares its tallest height", func(t *testing.T) {
		lv := lgraphtest.Level(t, "a->b", "a->c", "a->d")
		lv.Nodes[1].H = 64
		lv.Nodes[3].H = 52
		out := Apply(context.Background(), lv, []int{0, 1, 1, 1}, 0)
		assert.Equal(t, []float64{40, 64, 64, 64}, heights(out.Nodes))
		assert.Equal(t, 40.0, lv.Nodes[2].H, "the input is not modified")
	})
	t.Run("fixed-ratio shapes and record boxes neither grow nor count", func(t *testing.T) {
		lv := lgraphtest.Level(t, "a->m:diamond", "a->c:circle", "a->h:hexagon", "a->r", "a->x")
		lv.Nodes[1].H = 90
		lv.Nodes[2].H = 80
		lv.Nodes[3].H = 70
		lv.Nodes[4].Record = true
		lv.Nodes[4].H = 100
		out := Apply(context.Background(), lv, []int{0, 1, 1, 1, 1, 1}, 0)
		assert.Equal(t, []float64{40, 90, 80, 70, 100, 40}, heights(out.Nodes))
	})
	t.Run("a stretched parallelogram widens by the slant of its height gain", func(t *testing.T) {
		lv := lgraphtest.Level(t, "a->p:parallelogram", "a->b")
		lv.Nodes[2].H = 60
		out := Apply(context.Background(), lv, []int{0, 1, 1}, 0.3)
		assert.Equal(t, 60.0, out.Nodes[1].H)
		assert.InDelta(t, 86, out.Nodes[1].W, 1e-9, "80 + 0.3 * 20")

		out = Apply(context.Background(), lv, []int{0, 1, 1}, 0)
		assert.Equal(t, 80.0, out.Nodes[1].W, "no slant under RIGHT, LEFT or text")
	})
	t.Run("nodes and groups stretch apart, terminals never", func(t *testing.T) {
		lv := lgraphtest.Level(t, "t", "a", "g", "h", "t->a", "t->g", "t->h", "t->u")
		lv.Nodes[0].Terminal, lv.Nodes[0].H = true, 0
		lv.Nodes[2].Group, lv.Nodes[2].H = true, 200
		lv.Nodes[3].Group, lv.Nodes[3].H = true, 150
		lv.Nodes[4].Terminal, lv.Nodes[4].H = true, 0
		out := Apply(context.Background(), lv, []int{0, 1, 1, 1, 1}, 0)
		assert.Equal(t, []float64{0, 40, 200, 200, 0}, heights(out.Nodes), "the node keeps 40, the groups share 200, the terminal stays 0")
	})
	t.Run("different layers stay apart", func(t *testing.T) {
		lv := lgraphtest.Level(t, "a->b")
		lv.Nodes[0].H = 70
		out := Apply(context.Background(), lv, []int{0, 1}, 0)
		assert.Equal(t, []float64{70, 40}, heights(out.Nodes))
	})
}
