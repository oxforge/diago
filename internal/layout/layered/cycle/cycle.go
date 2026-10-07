// Package cycle breaks a level's cycles (S2).
package cycle

import (
	"context"

	"github.com/oxforge/diago/internal/layout/layered/lgraph"
	"github.com/oxforge/diago/internal/layoutdbg"
)

// Break returns, per edge, whether the layering treats it as reversed
// (S2). A depth-first search starts from the true sources, then from every
// other node not yet reached, each in declaration order, and scans a
// node's out-edges in edge order; an edge that closes a cycle onto the
// search stack is reversed, so a cycle is entered at the first of its
// nodes the search reaches: where a search from a true source first
// arrives, else at its first-declared node unless a search begun at an
// earlier-declared node reaches it first. Self-loops are never reversed.
// previous marks the edges a previous layout reversed (S13), nil for none:
// they are flipped before the search, and an edge ends up reversed when
// exactly one of the flip and the search reverses it.
func Break(ctx context.Context, lv *lgraph.Level, previous []bool) []bool {
	n := len(lv.Nodes)
	out := make([][]int, n)
	indeg := make([]int, n)
	for e := range lv.Edges {
		if lv.SelfLoop(e) {
			continue
		}
		from, to := lv.Oriented(e, previous)
		out[from] = append(out[from], e)
		indeg[to]++
	}

	const (
		unseen = iota
		active
		done
	)
	state := make([]int, n)
	closes := make([]bool, len(lv.Edges))
	var visit func(v int)
	visit = func(v int) {
		state[v] = active
		for _, e := range out[v] {
			_, to := lv.Oriented(e, previous)
			switch state[to] {
			case active:
				closes[e] = true
			case unseen:
				visit(to)
			}
		}
		state[v] = done
	}
	for _, sources := range []bool{true, false} {
		for v := range n {
			if (indeg[v] == 0) == sources && state[v] == unseen {
				visit(v)
			}
		}
	}

	reversed := make([]bool, len(lv.Edges))
	for e := range lv.Edges {
		if lv.SelfLoop(e) {
			continue
		}
		flipped := previous != nil && previous[e]
		reversed[e] = flipped != closes[e]
		if reversed[e] {
			layoutdbg.Decision(ctx, "edge_reversed",
				"phase", "cycle", "module", "diago", "spec_ref", "S2",
				"edge", lv.Edges[e].ID, "previous", flipped)
		}
	}
	return reversed
}
