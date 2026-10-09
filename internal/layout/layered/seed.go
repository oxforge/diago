package layered

import (
	"context"

	"github.com/oxforge/diago/internal/layout/layered/cycle"
	"github.com/oxforge/diago/internal/layout/layered/nest"
	"github.com/oxforge/diago/internal/layout/layered/place"
	"github.com/oxforge/diago/internal/layoutdbg"
)

// seed lays the whole graph out as one level, groups ignored, through
// place (S0 step 3, S9) and returns each node's cross-axis position there.
func (n *nesting) seed(ctx context.Context) ([]float64, error) {
	lv, err := level(n.g, n.sizes, n.dir)
	if err != nil {
		return nil, err
	}
	pl, err := planLevel(ctx, lv, cycle.Break(ctx, lv, nil), n.cfg, n.dir, nil, nil)
	if err != nil {
		return nil, err
	}
	x := place.Place(ctx, pl.Graph, pl.place)
	layoutdbg.Decision(ctx, "order_seeded",
		"phase", "nest", "module", "diago", "spec_ref", "S9",
		"nodes", len(n.g.Nodes), "edges", len(n.g.Edges))
	return x[:len(n.g.Nodes)], nil
}

// keys places container c's level nodes in the whole-graph seed: a key
// and a tie-break each; nil without a seed. A node takes its seed
// position and a group the mean of the nodes inside it. A terminal takes
// the seed position of its edge's outside end's representative at the
// edge's home level; among terminals of one representative, a top
// terminal follows that representative's own terminal for the edge when
// it is a group laid out already, a binding tie-break, so entries line up
// with the exits they come from; every other terminal follows its inside
// end's seed position, which S6 may overturn.
func (n *nesting) keys(c int) *seeding {
	if n.flat == nil {
		return nil
	}
	l, t := n.levels[c], n.tree
	sum := make([]float64, len(t.Containers))
	count := make([]float64, len(t.Containers))
	for node, x := range n.flat {
		for k := t.In[node]; k != -1; k = t.Containers[k].Parent {
			sum[k] += x
			count[k]++
		}
	}
	at := func(ch nest.Child) float64 {
		if ch.Node >= 0 {
			return n.flat[ch.Node]
		}
		if count[ch.Container] == 0 {
			return 0
		}
		return sum[ch.Container] / count[ch.Container]
	}
	key := make([]float64, len(l.lv.Nodes))
	tie := make([]float64, len(l.lv.Nodes))
	hard := make([]bool, len(l.lv.Nodes))
	for v, ch := range l.stands {
		if ch.Node >= 0 || ch.Container >= 0 {
			key[v] = at(ch)
		}
	}
	for le, ed := range l.lv.Edges {
		for end, v := range [2]int{ed.From, ed.To} {
			if !l.lv.Nodes[v].Terminal {
				continue
			}
			e := l.carries[le]
			// The terminal stands for the edge's end outside c: its From
			// end when the terminal is the level edge's From.
			outside, inner := n.index[n.g.Edges[e].From], n.index[n.g.Edges[e].To]
			if end == 1 {
				outside, inner = inner, outside
			}
			rep := t.Rep(t.Home[e], outside)
			key[v], tie[v] = at(rep), n.flat[inner]
			upper, _ := l.lv.Oriented(le, l.reversed)
			if k := rep.Container; upper == v && k >= 0 && n.levels[k].out != nil {
				tie[v], hard[v] = n.levels[k].at[e], true
			}
		}
	}
	return &seeding{key: key, tie: tie, hard: hard}
}
