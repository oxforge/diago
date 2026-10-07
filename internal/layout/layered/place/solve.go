package place

import "math"

// sepVar is one variable of a separation problem: where it would sit
// unconstrained, and how strongly it resists moving (weight > 0).
type sepVar struct{ desired, weight float64 }

// sepCon asks x[right] ≥ x[left] + gap.
type sepCon struct {
	left, right int
	gap         float64
}

// eps is the float noise below which a violation or an improvement does
// not count.
const eps = 1e-9

// solve minimizes Σ wᵢ(xᵢ − dᵢ)² subject to every constraint, by the VPSC
// active-set method of Dwyer, Marriott and Stuckey (S7). Variables joined
// by tight (active) constraints form blocks that move as one, each at its
// members' weighted mean desire. satisfy makes the most violated
// constraint tight, merging two blocks, and when both ends already share a
// block it first cuts that block at its weakest link; splitBlocks releases
// an active constraint whose Lagrange multiplier went negative. The result
// is the optimum of a feasible, acyclic system, and a final sweep in
// topological order keeps every constraint whatever happens. Choices go by
// violation size, ties to the lower index.
func solve(vars []sepVar, cons []sepCon) []float64 {
	n := len(vars)
	if n == 0 {
		return nil
	}
	s := &solver{
		vars: vars, cons: cons,
		blockOf: make([]int, n), offset: make([]float64, n), posn: make([]float64, n),
		members: make([][]int, n), incident: make([][]int, n),
		active: make([]bool, len(cons)), dead: make([]bool, len(cons)),
	}
	for i, v := range vars {
		s.blockOf[i] = i
		s.members[i] = []int{i}
		s.posn[i] = v.desired
	}
	s.satisfy()
	last := s.cost()
	for range 2*n + 8 {
		if !s.splitBlocks() {
			break
		}
		s.satisfy()
		now := s.cost()
		if now >= last-eps {
			break
		}
		last = now
	}
	x := make([]float64, n)
	for i := range x {
		x[i] = s.x(i)
	}
	sweep(x, cons)
	return x
}

// solver is the state of one solve. A block is named by its head
// variable; an absorbed block's member list is empty.
type solver struct {
	vars     []sepVar
	cons     []sepCon
	blockOf  []int     // each variable's block
	offset   []float64 // x[i] = posn[blockOf[i]] + offset[i]
	posn     []float64 // each block's position, by head
	members  [][]int   // each block's variables, by head
	incident [][]int   // the constraints each variable has been joined by
	active   []bool    // the constraints holding blocks together
	dead     []bool    // violated constraints satisfy gave up on this pass
}

func (s *solver) x(i int) float64 { return s.posn[s.blockOf[i]] + s.offset[i] }

// other is constraint ci's end that is not v.
func (s *solver) other(ci, v int) int {
	c := s.cons[ci]
	if c.left == v {
		return c.right
	}
	return c.left
}

// reposition moves block b to its members' weighted mean desire.
func (s *solver) reposition(b int) {
	w, wd := 0.0, 0.0
	for _, m := range s.members[b] {
		v := s.vars[m]
		w += v.weight
		wd += v.weight * (v.desired - s.offset[m])
	}
	s.posn[b] = 0
	if w > 0 {
		s.posn[b] = wd / w
	}
}

// span walks from start along active constraints, depth first, returning
// the visit order and each visited variable's constraint to its parent.
func (s *solver) span(start int) ([]int, map[int]int) {
	var order []int
	parent := map[int]int{}
	seen := map[int]bool{start: true}
	stack := []int{start}
	for len(stack) > 0 {
		v := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		order = append(order, v)
		for _, ci := range s.incident[v] {
			if !s.active[ci] {
				continue
			}
			next := s.other(ci, v)
			if seen[next] {
				continue
			}
			seen[next] = true
			parent[next] = ci
			stack = append(stack, next)
		}
	}
	return order, parent
}

// multipliers returns the Lagrange multiplier of every active constraint
// of block b: the net pull of the sub-block hanging below it, negative
// when that side is held against its will. The constraints come in
// discovery order beside the values.
func (s *solver) multipliers(b int) ([]int, map[int]float64) {
	order, parent := s.span(s.members[b][0])
	force := map[int]float64{}
	lm := map[int]float64{}
	var cons []int
	for i := len(order) - 1; i >= 0; i-- {
		v := order[i]
		sub := s.vars[v].weight*(s.x(v)-s.vars[v].desired) + force[v]
		force[v] = sub
		ci, ok := parent[v]
		if !ok {
			continue
		}
		force[s.other(ci, v)] += sub
		lm[ci] = -sub
		if s.cons[ci].right == v {
			lm[ci] = sub
		}
		cons = append(cons, ci)
	}
	return cons, lm
}

// cuttable returns the active constraints on the tree path from `from` to
// `to` whose right end lies on the `to` side: cutting one of those and
// pushing that side right leaves it more slack, while cutting a
// backward-pointing one would violate it at once.
func (s *solver) cuttable(from, to int) []int {
	_, parent := s.span(from)
	var path []int
	for cursor := to; cursor != from; {
		ci, ok := parent[cursor]
		if !ok {
			return nil
		}
		if s.cons[ci].right == cursor {
			path = append(path, ci)
		}
		cursor = s.other(ci, cursor)
	}
	return path
}

// splitAt releases active constraint ci of block b, leaving two blocks.
func (s *solver) splitAt(b, ci int) {
	s.active[ci] = false
	c := s.cons[ci]
	for _, start := range []int{c.left, c.right} {
		group, _ := s.span(start)
		head := group[0]
		for _, m := range group {
			if m == b {
				head = b
			}
		}
		base := s.offset[head]
		for _, m := range group {
			s.offset[m] -= base
			s.blockOf[m] = head
		}
		s.members[head] = group
		s.reposition(head)
	}
}

// merge makes constraint ci tight, absorbing its right block into its left
// one.
func (s *solver) merge(ci int) {
	c := s.cons[ci]
	bl, br := s.blockOf[c.left], s.blockOf[c.right]
	shift := s.posn[br] + (s.x(c.left) + c.gap - s.x(c.right)) - s.posn[bl]
	for _, m := range s.members[br] {
		s.offset[m] += shift
		s.blockOf[m] = bl
		s.members[bl] = append(s.members[bl], m)
	}
	s.members[br] = nil
	s.reposition(bl)
	s.active[ci] = true
	s.incident[c.left] = append(s.incident[c.left], ci)
	s.incident[c.right] = append(s.incident[c.right], ci)
}

func (s *solver) satisfy() {
	clear(s.dead)
	for range 4*(len(s.cons)+len(s.vars)) + 1 {
		pick, worst := -1, eps
		for ci, c := range s.cons {
			if s.active[ci] || s.dead[ci] {
				continue
			}
			if v := s.x(c.left) + c.gap - s.x(c.right); v > worst {
				pick, worst = ci, v
			}
		}
		if pick < 0 {
			return
		}
		c := s.cons[pick]
		if b := s.blockOf[c.left]; b == s.blockOf[c.right] {
			path := s.cuttable(c.left, c.right)
			_, lm := s.multipliers(b)
			weakest, weakestLM := -1, math.Inf(1)
			for _, ci := range path {
				if v := lm[ci]; v < weakestLM {
					weakest, weakestLM = ci, v
				}
			}
			if weakest < 0 {
				s.dead[pick] = true
				continue
			}
			s.splitAt(b, weakest)
			if s.blockOf[c.left] == s.blockOf[c.right] {
				s.dead[pick] = true
				continue
			}
		}
		s.merge(pick)
	}
}

// splitBlocks releases, in every block, the active constraint with the
// most negative multiplier, and reports whether it released any.
func (s *solver) splitBlocks() bool {
	split := false
	for b := range s.vars {
		if len(s.members[b]) < 2 {
			continue
		}
		cons, lm := s.multipliers(b)
		weakest, weakestLM := -1, -eps
		for _, ci := range cons {
			if v := lm[ci]; v < weakestLM {
				weakest, weakestLM = ci, v
			}
		}
		if weakest < 0 {
			continue
		}
		s.splitAt(b, weakest)
		split = true
	}
	return split
}

func (s *solver) cost() float64 {
	total := 0.0
	for i, v := range s.vars {
		d := s.x(i) - v.desired
		total += v.weight * d * d
	}
	return total
}

// sweep pushes every constraint's right end clear of its left end, in
// topological order: float drift and any constraint the loop gave up on
// are cleaned here, so an acyclic system always comes out feasible.
func sweep(x []float64, cons []sepCon) {
	indeg := make([]int, len(x))
	out := make([][]int, len(x))
	for ci, c := range cons {
		out[c.left] = append(out[c.left], ci)
		indeg[c.right]++
	}
	var queue []int
	for i, d := range indeg {
		if d == 0 {
			queue = append(queue, i)
		}
	}
	for h := 0; h < len(queue); h++ {
		for _, ci := range out[queue[h]] {
			c := cons[ci]
			x[c.right] = max(x[c.right], x[c.left]+c.gap)
			if indeg[c.right]--; indeg[c.right] == 0 {
				queue = append(queue, c.right)
			}
		}
	}
}
