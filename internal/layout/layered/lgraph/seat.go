package lgraph

import (
	"cmp"
	"context"
	"slices"

	"github.com/oxforge/diago/internal/layoutdbg"
)

// SeatCorridors completes the declaration seed Build starts (S5,
// Corridor seats): the last dummy of each chain whose edge is not
// reversed, when the chain's downstream end has two or more parents (the
// nodes with a hop into it, each once), moves to the median of their
// positions in its layer, between the two middle ones for an even count.
// Every key is read from the order before any move; each layer with a
// seated dummy is then sorted by key, stably, so a dummy keyed at a
// parent's position follows it and the columns of one end keep edge
// order. A corridor_seated record names each seat.
func (g *Graph) SeatCorridors(ctx context.Context) {
	idx := g.Index()
	parents := make([][]int, len(g.Vertices))
	for _, h := range g.Hops() {
		if !g.Dummy(h.Upper) && !slices.Contains(parents[h.Lower], h.Upper) {
			parents[h.Lower] = append(parents[h.Lower], h.Upper)
		}
	}
	type seat struct {
		dummy, end int
		key        float64
	}
	var seats []seat
	for e, chain := range g.Chains {
		if len(chain) < 3 || (g.Reversed != nil && g.Reversed[e]) {
			continue
		}
		d, end := chain[len(chain)-2], chain[len(chain)-1]
		ps := parents[end]
		if len(ps) < 2 {
			continue
		}
		pos := make([]int, len(ps))
		for i, p := range ps {
			pos[i] = idx[p]
		}
		slices.Sort(pos)
		m := len(pos) / 2
		key := float64(pos[m])
		if len(pos)%2 == 0 {
			key = float64(pos[m-1]+pos[m]) / 2
		}
		seats = append(seats, seat{d, end, key})
	}
	if len(seats) == 0 {
		return
	}
	key := make([]float64, len(g.Vertices))
	for v := range g.Vertices {
		key[v] = float64(idx[v])
	}
	sorted := make([]bool, len(g.Layers))
	for _, s := range seats {
		key[s.dummy] = s.key
		sorted[g.Vertices[s.dummy].Layer] = true
	}
	for l, layer := range g.Layers {
		if sorted[l] {
			slices.SortStableFunc(layer, func(a, b int) int { return cmp.Compare(key[a], key[b]) })
		}
	}
	at := g.Index()
	for _, s := range seats {
		ps := slices.Clone(parents[s.end])
		slices.SortFunc(ps, func(a, b int) int { return cmp.Compare(idx[a], idx[b]) })
		ids := make([]string, len(ps))
		for i, p := range ps {
			ids[i] = g.Vertices[p].ID
		}
		layoutdbg.Decision(ctx, "corridor_seated",
			"phase", "lgraph", "module", "diago", "spec_ref", "S5",
			"dummy", g.Vertices[s.dummy].ID, "end", g.Vertices[s.end].ID, "parents", ids,
			"layer", g.Vertices[s.dummy].Layer, "key", s.key, "index", at[s.dummy])
	}
}
