package cycle

import (
	"context"
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/oxforge/diago/internal/layout/layered/lgraph"
	"github.com/oxforge/diago/internal/layout/layered/lgraph/lgraphtest"
)

func reversedIDs(lv *lgraph.Level, reversed []bool) []string {
	var ids []string
	for e, r := range reversed {
		if r {
			ids = append(ids, lv.Edges[e].ID)
		}
	}
	return ids
}

func TestBreak(t *testing.T) {
	tests := []struct {
		name  string
		specs []string
		want  []string
	}{
		{"a DAG stays as it is", []string{"a->b", "b->c", "a->c"}, nil},
		{"one edge of a 2-cycle", []string{"a->b", "b->a"}, []string{"b->a#0"}},
		{"the back edge of a 3-cycle", []string{"a->b", "b->c", "c->a"}, []string{"c->a#0"}},
		{"a retry loop keeps the forward flow",
			[]string{"start->gate", "gate->fix", "fix->gate", "gate->commit"}, []string{"fix->gate#0"}},
		{"self-loops are never reversed", []string{"a->b", "a->a"}, nil},
		{"the first-declared node of a cycle is its entry", []string{"b", "a->b", "b->a"}, []string{"a->b#0"}},
		// b is declared first, but s is the only true source: the search
		// starts from s and enters the cycle at a, so b->a closes it. A
		// search from b first would reverse a->b instead.
		{"the true sources are searched first", []string{"b->a", "a->b", "s->a"}, []string{"b->a#0"}},
		// no true source: the search from p crosses into the r-s cycle at
		// r, although s is declared before r
		{"an earlier search enters a cycle where it arrives",
			[]string{"p->q", "q->p", "s->r", "r->s", "q->r"}, []string{"q->p#0", "s->r#0"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lv := lgraphtest.Level(t, tt.specs...)
			assert.Equal(t, tt.want, reversedIDs(lv, Break(context.Background(), lv, nil)))
		})
	}
}

func TestBreak_PreviousReversalSticks(t *testing.T) {
	lv := lgraphtest.Level(t, "a->b", "b->a")
	// Fresh, b->a is the back edge; a previous layout that reversed a->b
	// keeps that decision, since the flipped graph has no cycle left.
	assert.Equal(t, []bool{true, false}, Break(context.Background(), lv, []bool{true, false}))
}

func TestBreak_AlwaysAcyclic(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 11))
	for trial := range 200 {
		n := 2 + rng.IntN(12)
		specs := make([]string, 0, 3*n)
		for i := range n {
			specs = append(specs, fmt.Sprintf("n%d", i))
		}
		for range rng.IntN(3 * n) {
			specs = append(specs, fmt.Sprintf("n%d->n%d", rng.IntN(n), rng.IntN(n)))
		}
		lv := lgraphtest.Level(t, specs...)
		reversed := Break(context.Background(), lv, nil)
		assert.True(t, acyclic(lv, reversed), "trial %d: %v", trial, specs)
		for e := range lv.Edges {
			if lv.SelfLoop(e) {
				assert.False(t, reversed[e], "trial %d: self-loop %s reversed", trial, lv.Edges[e].ID)
			}
		}
	}
}

// acyclic runs Kahn's algorithm over the oriented edges.
func acyclic(lv *lgraph.Level, reversed []bool) bool {
	indeg := make([]int, len(lv.Nodes))
	succ := make([][]int, len(lv.Nodes))
	for e := range lv.Edges {
		if lv.SelfLoop(e) {
			continue
		}
		u, w := lv.Oriented(e, reversed)
		succ[u] = append(succ[u], w)
		indeg[w]++
	}
	var queue []int
	for v, d := range indeg {
		if d == 0 {
			queue = append(queue, v)
		}
	}
	for h := 0; h < len(queue); h++ {
		for _, w := range succ[queue[h]] {
			if indeg[w]--; indeg[w] == 0 {
				queue = append(queue, w)
			}
		}
	}
	return len(queue) == len(lv.Nodes)
}
