package order

import (
	"context"
	"math"

	"github.com/oxforge/diago/internal/layout/layered/lgraph"
	"github.com/oxforge/diago/internal/layoutdbg"
)

// keepLoops runs S6's loop transposition (Loops kept together) on g's
// order: it returns the ordering and true when it swapped a pair, and g's
// order and false when it swapped none. It records the vertices the loops
// enclosed before and after.
func keepLoops(ctx context.Context, g *lgraph.Graph) ([][]int, bool) {
	work := *g
	work.Layers = snapshot(g.Layers)
	s := newSweeper(&work)
	before := s.enclosed()
	if before == 0 || !s.loops() {
		return g.Layers, false
	}
	layoutdbg.Decision(ctx, "loops_kept",
		"phase", "order", "module", "diago", "spec_ref", "S6",
		"enclosed_before", before, "enclosed", s.enclosed())
	return work.Layers, true
}

// loops swaps adjacent pairs of one band, the last pair of a layer
// included, wherever that strictly reduces the vertices the layers above
// and below enclose in the layer and raises neither its crossings (local)
// nor its side-aware crossings (sided), in passes until one swaps
// nothing: a swap changes only its own layer's enclosure, so each lowers
// the vertices enclosed, and the passes end. It reports whether it
// swapped.
func (s *sweeper) loops() bool {
	swapped := false
	for {
		improved := false
		for l, layer := range s.g.Layers {
			for i := 0; i+1 < len(layer); i++ {
				enclosed := s.encloses(l)
				if enclosed == 0 {
					break
				}
				if s.band[layer[i]] != s.band[layer[i+1]] {
					continue
				}
				// the enclosure is cheap and rules most swaps out, so it
				// goes first, and each crossing count runs only when the
				// tests before it pass: swapped, then back, then swapped
				s.swap(layer, i)
				if s.encloses(l) >= enclosed {
					s.swap(layer, i)
					continue
				}
				after := s.local(l)
				s.swap(layer, i)
				if after > s.local(l) {
					continue
				}
				before := s.sided(l)
				s.swap(layer, i)
				if s.sided(l) <= before {
					improved = true
				} else {
					s.swap(layer, i)
				}
			}
		}
		if !improved {
			return swapped
		}
		swapped = true
	}
}

// enclosed counts the vertices every vertex encloses, above and below it.
func (s *sweeper) enclosed() int {
	n := 0
	for v := range s.g.Vertices {
		n += s.enclosedBy(s.up[v]) + s.enclosedBy(s.down[v])
	}
	return n
}

// encloses counts the vertices of layer l that the vertices of the layers
// above and below it enclose.
func (s *sweeper) encloses(l int) int {
	n := 0
	if l > 0 {
		for _, u := range s.g.Layers[l-1] {
			n += s.enclosedBy(s.down[u])
		}
	}
	if l+1 < len(s.g.Layers) {
		for _, u := range s.g.Layers[l+1] {
			n += s.enclosedBy(s.up[u])
		}
	}
	return n
}

// enclosedBy counts the vertices that a vertex whose neighbors on one side
// are nbrs encloses (S6, Loops kept together): when a hop of a reversed
// chain is among them, the vertices of their layer between the first and
// the last of them that are none of them; otherwise none.
func (s *sweeper) enclosedBy(nbrs []nbr) int {
	loop, lo, hi, distinct := false, math.MaxInt, -1, 0
	for i, n := range nbrs {
		loop = loop || n.loop
		lo, hi = min(lo, s.at[n.w]), max(hi, s.at[n.w])
		if !repeats(nbrs[:i], n.w) {
			distinct++
		}
	}
	if !loop {
		return 0
	}
	return hi - lo + 1 - distinct
}

// repeats reports whether w is among nbrs' neighbors.
func repeats(nbrs []nbr, w int) bool {
	for _, n := range nbrs {
		if n.w == w {
			return true
		}
	}
	return false
}

// sideEnd is an end the loop transposition counts at a side of its vertex
// (lgraph.Graph.CounterFlow): edge e's, whose adjacent vertex along its
// chain is w, in the layer below the vertex when below, else above.
type sideEnd struct {
	e, w  int
	below bool
}

// sided counts the crossings between layer l and the layers above and
// below it as local does, but with every end in sweeper.ends half a slot
// to the side of its vertex it takes, a model of S8's attachment (S6,
// Side-aware crossings; sides). It counts those two gaps only, though a
// swap can change the side a vertex of the layer above or below holds,
// and so the crossings of that vertex's ends in the gap beyond.
func (s *sweeper) sided(l int) int {
	memo := map[int][]float64{}
	total := 0
	for _, nbrs := range [2][][]nbr{s.up, s.down} {
		var spans [][2]float64
		for _, v := range s.g.Layers[l] {
			for _, n := range nbrs[v] {
				self, other := n.self, n.other
				if n.selfSide {
					self = s.sideOf(v, n.e, memo)
				}
				if n.otherSide {
					other = s.sideOf(n.w, n.e, memo)
				}
				spans = append(spans, [2]float64{float64(s.at[v]) + self, float64(s.at[n.w]) + other})
			}
		}
		total += lgraph.Inversions(spans)
	}
	return total
}

// sideOf is the shift of edge e's end at vertex v: -½ or ½ at the side it
// takes, 0 when it takes none (sides).
func (s *sweeper) sideOf(v, e int, memo map[int][]float64) float64 {
	for i, end := range s.ends[v] {
		if end.e == e {
			return s.sides(v, memo)[i]
		}
	}
	return 0
}

// sides returns the shift of each of v's ends in sweeper.ends, in their
// order, memoized in memo for the current order (S6, Side-aware
// crossings): the lower end of a one-hop edge takes its upper end's side
// when that end takes one, every other end the side it faces (facing), and
// a side holds one end, the first in edge order: an end whose side an
// earlier end holds takes none and counts at v's center.
func (s *sweeper) sides(v int, memo map[int][]float64) []float64 {
	if out, ok := memo[v]; ok {
		return out
	}
	out := make([]float64, len(s.ends[v]))
	var held [2]bool
	for i, end := range s.ends[v] {
		side := 0.0
		if chain := s.g.Chains[end.e]; len(chain) == 2 && chain[1] == v {
			side = s.sideOf(chain[0], end.e, memo)
		}
		if side == 0 {
			side = s.facing(v, end)
		}
		k := 0
		if side > 0 {
			k = 1
		}
		if !held[k] {
			held[k], out[i] = true, side
		}
	}
	memo[v] = out
	return out
}

// facing is the side of vertex v that faces end's adjacent vertex: ½, the
// right, when at least as many of v's other neighbors in that vertex's
// layer lie left of it as right of it, else -½, the left.
func (s *sweeper) facing(v int, end sideEnd) float64 {
	nbrs := s.up[v]
	if end.below {
		nbrs = s.down[v]
	}
	left, right := 0, 0
	for i, n := range nbrs {
		if n.w == end.w || repeats(nbrs[:i], n.w) {
			continue
		}
		if s.at[n.w] < s.at[end.w] {
			left++
		} else {
			right++
		}
	}
	if left >= right {
		return 0.5
	}
	return -0.5
}
