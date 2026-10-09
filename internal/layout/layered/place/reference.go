package place

import (
	"slices"
)

// reference is act 1 (S7): each layer starts packed from 0; then 6 sweeps
// alternate downward and upward, 3 equilibrium rounds pull from both
// sides, and a final upward pass centers parents over their children.
func (p *placer) reference() []float64 {
	g := p.g
	x := make([]float64, len(g.Vertices))
	for _, layer := range g.Layers {
		cursor := 0.0
		for _, v := range layer {
			w := g.Vertices[v].W
			x[v] = cursor + w/2
			cursor += w + p.o.Gap
		}
	}
	for s := range sweeps {
		if s%2 == 0 {
			for l := 1; l < len(g.Layers); l++ {
				p.project(x, l, p.up)
			}
		} else {
			for l := len(g.Layers) - 2; l >= 0; l-- {
				p.project(x, l, p.down)
			}
		}
	}
	both := make([][]pull, len(g.Vertices))
	for v := range both {
		both[v] = slices.Concat(p.up[v], p.down[v])
	}
	for range equilibriumRounds {
		for l := 1; l < len(g.Layers); l++ {
			p.project(x, l, both)
		}
		for l := len(g.Layers) - 2; l >= 0; l-- {
			p.project(x, l, both)
		}
	}
	for l := len(g.Layers) - 2; l >= 0; l-- {
		p.project(x, l, p.down)
	}
	return x
}

// project moves layer l to the weighted isotonic fit of its targets: each
// vertex's target is the mean of x[n] + delta over its pulls in nbrs, and
// a vertex without pulls holds its place with a weight of 0.01. A vertex
// without neighbors on either side takes no part in the fit: it rides
// with the nearest vertex before it that has neighbors, or, with none
// before it, the nearest after it, at the minimum separations from it; a
// layer without such a vertex holds its place.
func (p *placer) project(x []float64, l int, nbrs [][]pull) {
	layer := p.g.Layers[l]
	if len(layer) == 0 {
		return
	}
	var z, w []float64
	var fitted []int // the indices in layer of the vertices the fit places
	off := make([]float64, len(layer))
	for i, v := range layer {
		if i > 0 {
			off[i] = off[i-1] + p.sep(layer[i-1], v)
		}
		if p.alone(v) {
			continue
		}
		target, weight := x[v], idleWeight
		if ns := nbrs[v]; len(ns) > 0 {
			sum := 0.0
			for _, n := range ns {
				sum += x[n.node] + n.delta
			}
			target, weight = sum/float64(len(ns)), p.weight[v]
		}
		z, w, fitted = append(z, target-off[i]), append(w, weight), append(fitted, i)
	}
	if len(fitted) == 0 {
		return
	}
	u := make([]float64, len(layer))
	for k, fit := range isotonic(z, w) {
		u[fitted[k]] = fit
	}
	// each vertex left out takes the fit of the one it rides with
	next := 0
	for i := range layer {
		if next < len(fitted) && fitted[next] == i {
			next++
			continue
		}
		u[i] = u[fitted[max(next-1, 0)]]
	}
	for i, v := range layer {
		x[v] = u[i] + off[i]
	}
}

// isotonic is the weighted nondecreasing fit of values
// (pool-adjacent-violators): a violating run pools at its weighted mean.
func isotonic(values, weights []float64) []float64 {
	type pool struct {
		sum, weight float64
		count       int
	}
	var pools []pool
	for i, v := range values {
		b := pool{v * weights[i], weights[i], 1}
		for len(pools) > 0 {
			prev := pools[len(pools)-1]
			if prev.sum/prev.weight <= b.sum/b.weight {
				break
			}
			pools = pools[:len(pools)-1]
			b = pool{prev.sum + b.sum, prev.weight + b.weight, prev.count + b.count}
		}
		pools = append(pools, b)
	}
	out := make([]float64, 0, len(values))
	for _, b := range pools {
		for range b.count {
			out = append(out, b.sum/b.weight)
		}
	}
	return out
}
