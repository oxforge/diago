// Package order orders the layers of a layered graph to reduce crossings
// (S6).
package order

import (
	"cmp"
	"context"
	"slices"

	"github.com/oxforge/diago/internal/layout/layered/lgraph"
	"github.com/oxforge/diago/internal/layoutdbg"
)

// Sweeps is a fresh layout's sweep budget (S6), AnchoredSweeps an
// anchored one's (S13).
const (
	Sweeps         = 8
	AnchoredSweeps = 2
)

// transposeRounds caps the passes of one crossing transposition; the
// loop transposition (keepLoops) runs to its own fixed point.
const transposeRounds = 4

// maxReplacements caps the orderings that replace Minimize's best: each
// has fewer crossings, or as many and fewer enclosed vertices, so the
// loop ends long before (no corpus or probe layout takes more than 4);
// the cap guards against a change that breaks this (S6).
const maxReplacements = 100

// Minimize returns an ordering of g's layers that reduces the hops that
// cross (S6); g and its Layers are left untouched (S0: every stage is a
// pure function of its inputs). The order of g.Layers on entry is the
// seed, and anchored tells that a previous layout seeded it (S13). Up to
// Sweeps median sweeps, AnchoredSweeps when anchored, alternate downward
// and upward, each followed by a transposition. The ordering with the
// fewest crossings wins, ties going to the one nearest the seed and then
// to the earliest; the seed competes, and the sweeps stop once the best
// ordering has no crossing. When crossings remain, the sweeps run again
// from the seed, the first one upward, and that run's best replaces the
// first's only with fewer crossings. The best ordering then seeds a run
// of AnchoredSweeps, and each run's best the next; when a run finds no
// ordering with fewer crossings, the counter-flow side moves (sides) try
// to, and when neither does, the loop transposition (keepLoops) tries to
// enclose fewer vertices, unless anchored, where the previous order wins
// that tie. Minimize stops when none of them moves anything, so the
// ordering is its own anchored ordering's (S13), or, as a safeguard,
// after maxReplacements orderings replaced the best. A vertex in a band
// (lgraph.Graph.Bands) stays in it: bands keep their order, and only
// vertices of one band change places (S9).
func Minimize(ctx context.Context, g *lgraph.Graph, anchored bool) [][]int {
	return settle(ctx, g, anchored, maxReplacements)
}

// settle is Minimize with at most most orderings replacing the best.
func settle(ctx context.Context, g *lgraph.Graph, anchored bool, most int) [][]int {
	sweeps := Sweeps
	if anchored {
		sweeps = AnchoredSweeps
	}
	best, crossings, ran := run(g, sweeps)
	restarts, moves, loops := 0, 0, 0
	for {
		if restarts+moves+loops == most {
			layoutdbg.Decision(ctx, "order_capped",
				"phase", "order", "module", "diago", "spec_ref", "S6",
				"replacements", most, "crossings", crossings)
			break
		}
		work := *g
		work.Layers = best
		if crossings > 0 {
			next, c, r := run(&work, AnchoredSweeps)
			ran += r
			if c < crossings {
				restarts++
				best, crossings = next, c
				continue
			}
			next, c, r = sides(&work, crossings)
			ran += r
			if c < crossings {
				moves++
				best, crossings = next, c
				continue
			}
		}
		if anchored {
			break
		}
		next, swapped := keepLoops(ctx, &work)
		if !swapped {
			break
		}
		loops++
		work.Layers = next
		best, crossings = next, work.Crossings()
	}
	layoutdbg.Decision(ctx, "order_chosen",
		"phase", "order", "module", "diago", "spec_ref", "S6",
		"crossings", crossings, "sweeps", ran, "restarts", restarts, "side_moves", moves, "loop_moves", loops,
		"seed_distance", distance(best, g.Index()))
	return best
}

// sides tries S6's counter-flow side moves on g's order: each chain that
// movers names, in turn, has its dummies moved to the left ends of their
// layers, then to the right ends, and each ordering seeds a run of
// AnchoredSweeps. It returns the first run's best with fewer crossings
// than crossings, with its crossings and the sweeps run, or g's order
// when none has.
func sides(g *lgraph.Graph, crossings int) ([][]int, int, int) {
	ran := 0
	for _, e := range movers(g) {
		for _, right := range []bool{false, true} {
			work := *g
			work.Layers = moveChain(g, e, right)
			next, c, r := run(&work, AnchoredSweeps)
			ran += r
			if c < crossings {
				return next, c, ran
			}
		}
	}
	return g.Layers, crossings, ran
}

// movers returns the chains S6's counter-flow sides move, in edge order
// of their pairs' earlier chains: of every two reversed chains whose
// layer spans interleave (one starts strictly inside the other's span and
// ends outside it, interleave one way or the other) and whose hops cross
// each other and no other hop, the shorter, or of two equal spans the
// later.
func movers(g *lgraph.Graph) []int {
	if g.Reversed == nil {
		return nil
	}
	partner := crossedBy(g)
	var out []int
	for a, b := range partner {
		if b <= a || partner[b] != a || !g.Reversed[a] || !g.Reversed[b] {
			continue
		}
		a0, a1 := span(g, a)
		b0, b1 := span(g, b)
		if !interleave(a0, a1, b0, b1) && !interleave(b0, b1, a0, a1) {
			continue
		}
		if a1-a0 < b1-b0 {
			out = append(out, a)
		} else {
			out = append(out, b)
		}
	}
	return out
}

// interleave reports whether a0 < b0 < a1 < b1: the span from b0 to b1
// starts strictly inside the one from a0 to a1 and ends outside it.
func interleave(a0, a1, b0, b1 int) bool { return a0 < b0 && b0 < a1 && a1 < b1 }

// span returns the layers of edge e's chain's upstream and downstream
// ends.
func span(g *lgraph.Graph, e int) (int, int) {
	chain := g.Chains[e]
	return g.Vertices[chain[0]].Layer, g.Vertices[chain[len(chain)-1]].Layer
}

// Values of crossedBy's entries other than an edge.
const (
	crossedByNone    = -1
	crossedBySeveral = -2
)

// crossedBy returns, per edge of g, the one other edge whose hops cross
// its hops in g's order, a hop's ends placed as lgraph.Graph.Crossings
// places them: crossedByNone when no hop crosses them, crossedBySeveral
// when the hops of more than one edge do.
func crossedBy(g *lgraph.Graph) []int {
	type hop struct {
		edge int
		ends [2]float64
	}
	idx := g.Index()
	gaps := make([][]hop, len(g.Layers))
	for _, h := range g.Hops() {
		l := g.Vertices[h.Upper].Layer
		gaps[l] = append(gaps[l], hop{h.Edge, [2]float64{
			float64(idx[h.Upper]) + g.Shift(h.Edge, h.Upper),
			float64(idx[h.Lower]) + g.Shift(h.Edge, h.Lower),
		}})
	}
	partner := make([]int, len(g.Chains))
	for e := range partner {
		partner[e] = crossedByNone
	}
	note := func(e, f int) {
		switch partner[e] {
		case crossedByNone:
			partner[e] = f
		case f:
		default:
			partner[e] = crossedBySeveral
		}
	}
	for _, hops := range gaps {
		for i, a := range hops {
			for _, b := range hops[i+1:] {
				if lgraph.Inversions([][2]float64{a.ends, b.ends}) > 0 {
					note(a.edge, b.edge)
					note(b.edge, a.edge)
				}
			}
		}
	}
	return partner
}

// moveChain returns g's layers with edge e's dummies moved to the left
// ends of their layers, or to the right ends when right.
func moveChain(g *lgraph.Graph, e int, right bool) [][]int {
	layers := snapshot(g.Layers)
	chain := g.Chains[e]
	for _, v := range chain[1 : len(chain)-1] {
		l := g.Vertices[v].Layer
		rest := slices.DeleteFunc(layers[l], func(w int) bool { return w == v })
		if right {
			layers[l] = append(rest, v)
		} else {
			layers[l] = append([]int{v}, rest...)
		}
	}
	return layers
}

// run runs the sweeps from g's seed, the first one downward, and when
// crossings remain again from the seed, the first one upward, whose best
// replaces the first run's only with fewer crossings. It returns the best
// ordering, its crossings and the sweeps run.
func run(g *lgraph.Graph, sweeps int) ([][]int, int, int) {
	best, crossings, _, ran := minimize(g, sweeps, false)
	if crossings > 0 {
		up, c, _, r := minimize(g, sweeps, true)
		ran += r
		if c < crossings {
			best, crossings = up, c
		}
	}
	return best, crossings, ran
}

// Seed reorders every layer of g from a previous layout's order (S13): the
// vertices with a previous index (prev[v] >= 0) keep the slots they hold
// and are sorted among themselves by that index, ties in their current
// order; a vertex of a band (lgraph.Graph.Bands) only among its band's.
// The others keep their slots. It returns how many vertices it seeded.
func Seed(g *lgraph.Graph, prev []int) int {
	seeded := 0
	for _, layer := range g.Layers {
		var bands []int
		slots := map[int][]int{}
		for i, v := range layer {
			if prev[v] < 0 {
				continue
			}
			b := -1
			if g.Bands != nil {
				b = g.Bands[v]
			}
			if _, ok := slots[b]; !ok {
				bands = append(bands, b)
			}
			slots[b] = append(slots[b], i)
		}
		for _, b := range bands {
			vs := make([]int, len(slots[b]))
			for k, i := range slots[b] {
				vs[k] = layer[i]
			}
			slices.SortStableFunc(vs, func(a, c int) int { return cmp.Compare(prev[a], prev[c]) })
			for k, i := range slots[b] {
				layer[i] = vs[k]
			}
			seeded += len(vs)
		}
	}
	return seeded
}

// minimize runs the sweeps from g's seed, the first one downward, or
// upward when upFirst, and returns the best ordering, its crossings, its
// distance from the seed and the sweeps run.
func minimize(g *lgraph.Graph, sweeps int, upFirst bool) ([][]int, int, int, int) {
	work := *g
	work.Layers = snapshot(g.Layers)
	s := newSweeper(&work)
	seed := slices.Clone(s.at)
	best, bestCrossings, bestDistance := snapshot(work.Layers), work.Crossings(), 0
	ran := 0
	for ; ran < sweeps && bestCrossings > 0; ran++ {
		if (ran%2 == 0) != upFirst {
			for l := 1; l < len(work.Layers); l++ {
				s.median(l, s.up, seed)
			}
		} else {
			for l := len(work.Layers) - 2; l >= 0; l-- {
				s.median(l, s.down, seed)
			}
		}
		s.transpose()
		crossings, dist := work.Crossings(), distance(work.Layers, seed)
		if crossings < bestCrossings || (crossings == bestCrossings && dist < bestDistance) {
			best, bestCrossings, bestDistance = snapshot(work.Layers), crossings, dist
		}
	}
	return best, bestCrossings, bestDistance, ran
}

// sweeper holds the state of one minimization.
type sweeper struct {
	g        *lgraph.Graph
	up, down [][]nbr     // each vertex's chain neighbors in the layer above and below
	at       []int       // each vertex's index in its layer
	band     []int       // per vertex: its band (lgraph.Graph.Bands), -1 when free
	ends     [][]sideEnd // per vertex: its ends the loop transposition counts at a side, in edge order
}

// nbr is a chain neighbor w along edge e, with the shifts of e's ends at
// the vertex and at w (lgraph.Graph.Shift); loop marks a hop of a
// reversed chain (S2), and selfSide and otherSide an end at the vertex and
// at w that the loop transposition counts at a side
// (lgraph.Graph.CounterFlow).
type nbr struct {
	w, e                int
	self, other         float64
	loop                bool
	selfSide, otherSide bool
}

func newSweeper(g *lgraph.Graph) *sweeper {
	s := &sweeper{
		g: g, up: make([][]nbr, len(g.Vertices)), down: make([][]nbr, len(g.Vertices)),
		ends: make([][]sideEnd, len(g.Vertices)),
	}
	for _, h := range g.Hops() {
		upper, lower := g.Shift(h.Edge, h.Upper), g.Shift(h.Edge, h.Lower)
		loop := g.Reversed != nil && g.Reversed[h.Edge]
		su, sl := g.CounterFlow(h.Edge, h.Upper), g.CounterFlow(h.Edge, h.Lower)
		s.up[h.Lower] = append(s.up[h.Lower], nbr{h.Upper, h.Edge, lower, upper, loop, sl, su})
		s.down[h.Upper] = append(s.down[h.Upper], nbr{h.Lower, h.Edge, upper, lower, loop, su, sl})
		if su {
			s.ends[h.Upper] = append(s.ends[h.Upper], sideEnd{e: h.Edge, w: h.Lower, below: true})
		}
		if sl {
			s.ends[h.Lower] = append(s.ends[h.Lower], sideEnd{e: h.Edge, w: h.Upper})
		}
	}
	s.at = g.Index()
	s.band = make([]int, len(g.Vertices))
	for v := range s.band {
		s.band[v] = -1
		if g.Bands != nil {
			s.band[v] = g.Bands[v]
		}
	}
	return s
}

// median sorts layer l by the median position of each vertex's neighbors
// in nbrs, an index plus the shift of the neighbor's end (S9). A vertex
// without neighbors there keeps its own index; ties follow the seed.
func (s *sweeper) median(l int, nbrs [][]nbr, seed []int) {
	layer := s.g.Layers[l]
	if len(layer) < 2 {
		return
	}
	want := make(map[int]float64, len(layer))
	for i, v := range layer {
		positions := make([]float64, 0, len(nbrs[v]))
		for _, n := range nbrs[v] {
			positions = append(positions, float64(s.at[n.w])+n.other)
		}
		want[v] = float64(i)
		if len(positions) > 0 {
			slices.Sort(positions)
			want[v] = median(positions)
		}
	}
	slices.SortStableFunc(layer, func(a, b int) int {
		return cmp.Or(cmp.Compare(s.band[a], s.band[b]), cmp.Compare(want[a], want[b]), cmp.Compare(seed[a], seed[b]))
	})
	for i, v := range layer {
		s.at[v] = i
	}
}

// median is the median of sorted positions: the middle one for an odd
// count, the mean for two, and dot's weighted median above that, which
// leans toward the side where the positions pack tighter.
func median[T int | float64](positions []T) float64 {
	m := len(positions) / 2
	switch {
	case len(positions)%2 == 1:
		return float64(positions[m])
	case len(positions) == 2:
		return float64(positions[0]+positions[1]) / 2
	}
	left := float64(positions[m-1] - positions[0])
	right := float64(positions[len(positions)-1] - positions[m])
	if left+right == 0 {
		return float64(positions[m-1]+positions[m]) / 2
	}
	return (float64(positions[m-1])*right + float64(positions[m])*left) / (left + right)
}

// transpose swaps adjacent pairs, the last pair of a layer included,
// wherever that strictly reduces the crossings around the layer; up to
// transposeRounds passes, stopping after one that swaps nothing.
func (s *sweeper) transpose() {
	for range transposeRounds {
		improved := false
		for l, layer := range s.g.Layers {
			for i := 0; i+1 < len(layer); i++ {
				if s.band[layer[i]] != s.band[layer[i+1]] {
					continue
				}
				before := s.local(l)
				s.swap(layer, i)
				if s.local(l) < before {
					improved = true
				} else {
					s.swap(layer, i)
				}
			}
		}
		if !improved {
			return
		}
	}
}

func (s *sweeper) swap(layer []int, i int) {
	layer[i], layer[i+1] = layer[i+1], layer[i]
	s.at[layer[i]], s.at[layer[i+1]] = i, i+1
}

// local counts the crossings between layer l and the layers above and
// below it.
func (s *sweeper) local(l int) int {
	total := 0
	for _, nbrs := range [2][][]nbr{s.up, s.down} {
		var spans [][2]float64
		for _, v := range s.g.Layers[l] {
			for _, n := range nbrs[v] {
				spans = append(spans, [2]float64{float64(s.at[v]) + n.self, float64(s.at[n.w]) + n.other})
			}
		}
		total += lgraph.Inversions(spans)
	}
	return total
}

// distance counts the pairs of vertices that layers order opposite to the
// seed, whose entry for a vertex is its seed index in its layer.
func distance(layers [][]int, seed []int) int {
	d := 0
	for _, layer := range layers {
		for i, a := range layer {
			for _, b := range layer[i+1:] {
				if seed[a] > seed[b] {
					d++
				}
			}
		}
	}
	return d
}

func snapshot(layers [][]int) [][]int {
	out := make([][]int, len(layers))
	for l, layer := range layers {
		out[l] = slices.Clone(layer)
	}
	return out
}
