// Package equalize gives parallel siblings equal flow-axis extents (S4).
package equalize

import (
	"context"
	"slices"

	"github.com/oxforge/diago/internal/layout/layered/lgraph"
	"github.com/oxforge/diago/internal/layoutdbg"
	"github.com/oxforge/diago/internal/model"
)

// Apply returns a copy of lv in which, on every layer, the stretchable
// nodes take the largest flow-axis extent (H) among them (S4). Diamonds,
// circles, hexagons and record boxes are not stretchable: they neither
// grow nor count. slant widens a stretched parallelogram by slant times
// its height gain: the 0.3 slant in a DOWN or UP screen layout, where the
// parallelogram's sides lean with its height, and 0 otherwise. The edges
// are shared with lv; the nodes are copied.
func Apply(ctx context.Context, lv *lgraph.Level, layers []int, slant float64) *lgraph.Level {
	out := &lgraph.Level{Nodes: slices.Clone(lv.Nodes), Edges: lv.Edges}
	count := 0
	for _, l := range layers {
		count = max(count, l+1)
	}
	// Nodes and groups stretch apart: a row's nodes take its tallest node's
	// extent, its groups its tallest group's (S4, S9).
	rows := make([][]int, 2*count)
	for v, n := range out.Nodes {
		if stretchable(n) {
			l := 2 * layers[v]
			if n.Group {
				l++
			}
			rows[l] = append(rows[l], v)
		}
	}
	for l, row := range rows {
		l /= 2
		if len(row) < 2 {
			continue
		}
		tallest := 0.0
		for _, v := range row {
			tallest = max(tallest, out.Nodes[v].H)
		}
		var grown []string
		for _, v := range row {
			n := &out.Nodes[v]
			gain := tallest - n.H
			if gain <= 0 {
				continue
			}
			n.H = tallest
			if n.Shape == model.ShapeParallelogram {
				n.W += slant * gain
			}
			grown = append(grown, n.ID)
		}
		if len(grown) > 0 {
			layoutdbg.Decision(ctx, "siblings_equalized",
				"phase", "equalize", "module", "diago", "spec_ref", "S4",
				"layer", l, "extent", tallest, "grown", grown)
		}
	}
	return out
}

func stretchable(n lgraph.Node) bool {
	if n.Record || n.Terminal {
		return false
	}
	switch n.Shape {
	case model.ShapeDiamond, model.ShapeCircle, model.ShapeHexagon:
		return false
	}
	return true
}
