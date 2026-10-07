package place

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSolve(t *testing.T) {
	t.Run("an unconstrained variable lands on its desire", func(t *testing.T) {
		assert.Equal(t, []float64{17.5, -3}, solve([]sepVar{{17.5, 1}, {-3, 4}}, nil))
	})
	t.Run("a squeezed run keeps every gap and stays centered", func(t *testing.T) {
		got := solve([]sepVar{{0, 1}, {0, 1}, {0, 1}}, []sepCon{{0, 1, 10}, {1, 2, 10}})
		assert.InDeltaSlice(t, []float64{-10, 0, 10}, got, 1e-9)
	})
	t.Run("a heavier variable holds its ground", func(t *testing.T) {
		got := solve([]sepVar{{0, 1}, {0, 9}}, []sepCon{{0, 1, 10}})
		assert.InDeltaSlice(t, []float64{-9, 1}, got, 1e-9)
	})
	t.Run("a constraint wider than the hops it spans holds", func(t *testing.T) {
		got := solve([]sepVar{{0, 1}, {0, 1}, {0, 1}}, []sepCon{{0, 1, 2}, {1, 2, 2}, {0, 2, 10}})
		assert.GreaterOrEqual(t, got[1]-got[0], 2-1e-9)
		assert.GreaterOrEqual(t, got[2]-got[1], 2-1e-9)
		assert.GreaterOrEqual(t, got[2]-got[0], 10-1e-9)
	})
	t.Run("a satisfied constraint leaves both alone", func(t *testing.T) {
		assert.Equal(t, []float64{0, 100}, solve([]sepVar{{0, 1}, {100, 1}}, []sepCon{{0, 1, 10}}))
	})
	t.Run("no variables", func(t *testing.T) {
		assert.Empty(t, solve(nil, nil))
	})
}

// TestSolve_ParallelDuplicateConstraints pins S7's separation solver on
// constraints repeated between one pair, as compact hands it a pair that
// several rows separate: the duplicates bind as one, the widest gap of
// the pair holds, and the run stays centered on the desires, however the
// duplicates are ordered and wherever another constraint joins them.
func TestSolve_ParallelDuplicateConstraints(t *testing.T) {
	t.Run("two equal constraints bind as one", func(t *testing.T) {
		got := solve([]sepVar{{0, 1}, {0, 1}}, []sepCon{{0, 1, 10}, {0, 1, 10}})
		assert.InDeltaSlice(t, []float64{-5, 5}, got, 1e-9)
	})
	t.Run("the widest of a pair's gaps holds, in either order", func(t *testing.T) {
		for _, cons := range [][]sepCon{
			{{0, 1, 6}, {0, 1, 10}, {0, 1, 8}},
			{{0, 1, 10}, {0, 1, 6}, {0, 1, 8}},
		} {
			got := solve([]sepVar{{0, 1}, {0, 1}}, cons)
			assert.InDeltaSlice(t, []float64{-5, 5}, got, 1e-9, "%v", cons)
		}
	})
	t.Run("duplicates beside a third variable", func(t *testing.T) {
		got := solve([]sepVar{{0, 1}, {0, 1}, {0, 1}}, []sepCon{{0, 1, 10}, {1, 2, 10}, {0, 1, 10}, {1, 2, 4}})
		assert.InDeltaSlice(t, []float64{-10, 0, 10}, got, 1e-9)
	})
	t.Run("a satisfied pair of duplicates leaves both alone", func(t *testing.T) {
		assert.Equal(t, []float64{0, 100}, solve([]sepVar{{0, 1}, {100, 1}}, []sepCon{{0, 1, 10}, {0, 1, 10}}))
	})
	t.Run("duplicates under unequal weights", func(t *testing.T) {
		got := solve([]sepVar{{0, 1}, {0, 9}}, []sepCon{{0, 1, 10}, {0, 1, 10}})
		assert.InDeltaSlice(t, []float64{-9, 1}, got, 1e-9)
	})
}

func TestSolve_MatchesTheOptimum(t *testing.T) {
	rng := rand.New(rand.NewPCG(0x2b6d1f, 1))
	for trial := range 300 {
		n := 2 + rng.IntN(4)
		vars := make([]sepVar, n)
		for i := range vars {
			vars[i] = sepVar{desired: float64(rng.IntN(101) - 50), weight: float64(1 + rng.IntN(4))}
		}
		// constraints point from a lower index to a higher one: acyclic
		var cons []sepCon
		for i := range n {
			for j := i + 1; j < n; j++ {
				if len(cons) < 8 && rng.Float64() < 0.45 {
					cons = append(cons, sepCon{left: i, right: j, gap: float64(rng.IntN(31))})
				}
			}
		}
		got := solve(vars, cons)
		for _, c := range cons {
			assert.GreaterOrEqual(t, got[c.right]-got[c.left], c.gap-1e-6, "trial %d", trial)
		}
		assert.LessOrEqual(t, cost(got, vars), optimum(vars, cons)+1e-6, "trial %d", trial)
	}
}

func cost(x []float64, vars []sepVar) float64 {
	total := 0.0
	for i, v := range vars {
		total += v.weight * (x[i] - v.desired) * (x[i] - v.desired)
	}
	return total
}

// optimum tries every subset of the constraints as the active set: tight
// constraints fuse variables into groups placed at their weighted mean,
// and the cheapest feasible outcome is the optimum.
func optimum(vars []sepVar, cons []sepCon) float64 {
	n := len(vars)
	best := math.Inf(1)
	for mask := range 1 << len(cons) {
		root := make([]int, n)
		shift := make([]float64, n) // x[i] = x[root] + shift along the path
		for i := range root {
			root[i] = i
		}
		find := func(i int) (int, float64) {
			off := 0.0
			for root[i] != i {
				off += shift[i]
				i = root[i]
			}
			return i, off
		}
		consistent := true
		for ci, c := range cons {
			if mask&(1<<ci) == 0 {
				continue
			}
			ra, oa := find(c.left)
			rb, ob := find(c.right)
			if ra == rb {
				if math.Abs(ob-oa-c.gap) > 1e-9 {
					consistent = false
					break
				}
				continue
			}
			root[rb] = ra
			shift[rb] = oa + c.gap - ob
		}
		if !consistent {
			continue
		}
		w, wd := make([]float64, n), make([]float64, n)
		for i, v := range vars {
			r, off := find(i)
			w[r] += v.weight
			wd[r] += v.weight * (v.desired - off)
		}
		x := make([]float64, n)
		for i := range x {
			r, off := find(i)
			x[i] = wd[r]/w[r] + off
		}
		feasible := true
		for _, c := range cons {
			if x[c.right]-x[c.left] < c.gap-1e-9 {
				feasible = false
			}
		}
		if feasible {
			best = min(best, cost(x, vars))
		}
	}
	return best
}
