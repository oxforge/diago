package place

import (
	"context"
	"math"

	"github.com/oxforge/diago/internal/layoutdbg"
)

// pack moves the level's parts, the vertices chain hops join, rigidly
// toward the level's weighted center under the minimum separations between
// parts and the rooms between them (S7, *Packing*): act 1's reference
// drifts a part that shares no edge with the rest, and act 2 keeps the
// gap. Each part wants the shift that puts its weighted mean on the
// center, with the sum of its members' weights; parts are numbered by
// their first vertex in layer order, which solve's ties go by. A level
// with one part, or whose parts interleave (no one left-to-right order of
// them agrees with every layer and every room that holds), keeps x.
func (p *placer) pack(ctx context.Context, x []float64, rooms []room) {
	g := p.g
	part, n := p.parts()
	if n < 2 {
		return
	}
	var cons []sepCon
	succ := make([][]int, n)
	for _, layer := range g.Layers {
		for i := 1; i < len(layer); i++ {
			a, b := layer[i-1], layer[i]
			if part[a] == part[b] {
				continue
			}
			cons = append(cons, sepCon{left: part[a], right: part[b], gap: p.sep(a, b) - (x[b] - x[a])})
			succ[part[a]] = append(succ[part[a]], part[b])
		}
	}
	for _, r := range rooms {
		a, b, gap := p.space.roomSep(g, r)
		if part[a] == part[b] || x[b]-x[a] < gap-realizedTolerance {
			continue // a part moves rigidly; a room that gave way is no room
		}
		cons = append(cons, sepCon{left: part[a], right: part[b], gap: gap - (x[b] - x[a])})
		succ[part[a]] = append(succ[part[a]], part[b])
	}
	if !acyclic(succ) {
		layoutdbg.Decision(ctx, "parts_packed",
			"phase", "place", "module", "diago", "spec_ref", "S7",
			"parts", n, "interleaved", true, "moved", 0, "shift", 0.0)
		return
	}
	sum, weight := make([]float64, n), make([]float64, n)
	center, total := 0.0, 0.0
	for v := range g.Vertices {
		w := p.weight[v]
		sum[part[v]] += w * x[v]
		weight[part[v]] += w
		center += w * x[v]
		total += w
	}
	center /= total
	vars := make([]sepVar, n)
	for c := range vars {
		vars[c] = sepVar{desired: center - sum[c]/weight[c], weight: weight[c]}
	}
	shift := solve(vars, cons)
	moved, most := 0, 0.0
	for _, s := range shift {
		if math.Abs(s) > eps {
			moved++
			most = max(most, math.Abs(s))
		}
	}
	for v := range x {
		if s := shift[part[v]]; math.Abs(s) > eps {
			x[v] = clean(x[v] + s)
		}
	}
	layoutdbg.Decision(ctx, "parts_packed",
		"phase", "place", "module", "diago", "spec_ref", "S7",
		"parts", n, "interleaved", false, "moved", moved, "shift", most)
}

// parts returns each vertex's part, the hop-connected component it lies
// in, and the number of parts, numbered by their first vertex, layer by
// layer from the top and each layer from the left (S7, *Packing*).
func (p *placer) parts() ([]int, int) {
	g := p.g
	root := make([]int, len(g.Vertices))
	for v := range root {
		root[v] = v
	}
	find := func(v int) int {
		for root[v] != v {
			root[v] = root[root[v]]
			v = root[v]
		}
		return v
	}
	for _, h := range g.Hops() {
		root[find(h.Upper)] = find(h.Lower)
	}
	number := make([]int, len(g.Vertices))
	for v := range number {
		number[v] = -1
	}
	part := make([]int, len(g.Vertices))
	n := 0
	for _, layer := range g.Layers {
		for _, v := range layer {
			r := find(v)
			if number[r] < 0 {
				number[r] = n
				n++
			}
			part[v] = number[r]
		}
	}
	return part, n
}

// acyclic reports whether the order succ gives, each entry's successors,
// has no cycle.
func acyclic(succ [][]int) bool {
	indeg := make([]int, len(succ))
	for _, next := range succ {
		for _, c := range next {
			indeg[c]++
		}
	}
	var queue []int
	for c, d := range indeg {
		if d == 0 {
			queue = append(queue, c)
		}
	}
	for h := 0; h < len(queue); h++ {
		for _, c := range succ[queue[h]] {
			if indeg[c]--; indeg[c] == 0 {
				queue = append(queue, c)
			}
		}
	}
	return len(queue) == len(succ)
}
