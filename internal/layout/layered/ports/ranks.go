package ports

import (
	"cmp"
	"slices"

	"github.com/oxforge/diago/internal/layout/layered/lgraph"
)

// Head and Tail index an edge's two chain ends: the head is the upstream
// end, on its node's out-face; the tail the downstream end, on its node's
// in-face.
const (
	Head = 0
	Tail = 1
)

// Rank is a chain end's slot on its node's face: the At-th (from 0) of Of
// ends there. Of is 0 for an end without a slot: a self-loop's, or one the
// router attached at a side. An anchored end (S9) takes no slot: its
// port sits at Fixed from its group's center.
type Rank struct {
	At, Of   int
	Anchored bool
	Fixed    float64
}

// Offset is the end's port position on span s, its anchor when anchored,
// or 0 without a slot.
func (r Rank) Offset(s Span) float64 {
	if r.Anchored {
		return r.Fixed
	}
	if r.Of == 0 {
		return 0
	}
	return s.At(r.At, r.Of)
}

// Reach is how far a side column sits outside its side (S8): reach, and in
// the text profile half a cell farther, on the middle of a cell, where
// ports and terminals run; a column on a cell's border would sit one Snap
// from them and draw in the next cell.
func Reach(reach float64, text bool) float64 {
	if text {
		return reach + 0.5
	}
	return reach
}

// LoopOut is how far outside its node's right face loop j (from 0, in edge
// order) of the n self-loops on a node with faces runs (S8, Self-loops):
// the loops nest, the first outermost, each inLaneGap inside the one
// before it, the innermost Reach out, so that in the text profile every
// one runs on a cell's middle, as a side column does. Every caller passes
// the profile, to measure the leg the router draws; with stops and side
// columns on cell middles, only place's room (S7) turns on the half cell.
func LoopOut(reach, inLaneGap float64, j, n int, text bool) float64 {
	return Reach(reach, text) + float64(n-1-j)*inLaneGap
}

// Off turns side attachments into the ends they hold off their faces, as
// Ranks takes them; nil stays nil.
func Off(side [][2]Vertex) [][2]bool {
	if side == nil {
		return nil
	}
	out := make([][2]bool, len(side))
	for e, s := range side {
		out[e] = [2]bool{s[0] != Bottom, s[1] != Bottom}
	}
	return out
}

// Ranks derives every chain end's slot from the ordering (S8). The ends on
// a node's out-face sort by the in-layer index of the next vertex along
// their chains, the ends on its in-face by the index of the previous
// vertex, ties by edge order. side marks the ends the router attached at
// a side, nil for none: they take no slot, and the rest spread over the
// face without them. An anchored end (S9) takes its anchor and no slot.
func Ranks(g *lgraph.Graph, side [][2]bool) [][2]Rank {
	type end struct{ edge, key int }
	idx := g.Index()
	faces := make([][2][]end, len(g.Vertices))
	for e, chain := range g.Chains {
		if len(chain) < 2 {
			continue
		}
		head, tail := chain[0], chain[len(chain)-1]
		if side == nil || !side[e][Head] {
			faces[head][Head] = append(faces[head][Head], end{e, idx[chain[1]]})
		}
		if side == nil || !side[e][Tail] {
			faces[tail][Tail] = append(faces[tail][Tail], end{e, idx[chain[len(chain)-2]]})
		}
	}
	ranks := make([][2]Rank, len(g.Chains))
	for e := range g.Chains {
		for which := range 2 {
			if a := g.Level.End(e, which, g.Reversed); a.On {
				ranks[e][which] = Rank{Anchored: true, Fixed: a.At}
			}
		}
	}
	for _, face := range faces {
		for which, ends := range face {
			slices.SortStableFunc(ends, func(a, b end) int {
				return cmp.Or(cmp.Compare(a.key, b.key), cmp.Compare(a.edge, b.edge))
			})
			ends = slices.DeleteFunc(ends, func(en end) bool { return ranks[en.edge][which].Anchored })
			for k, en := range ends {
				ranks[en.edge][which] = Rank{At: k, Of: len(ends)}
			}
		}
	}
	return ranks
}
