package place

import (
	"cmp"
	"math"
	"slices"

	"github.com/oxforge/diago/internal/layout/layered/lgraph"
)

const (
	// maxRounds caps the alignment rounds. Each round only adds
	// alignments, so the loop ends on its own once nothing new lands in
	// the span; the cap bounds a graph that keeps finding candidates.
	maxRounds = 12
)

// link is one chain hop in port-adjusted form: the wire runs straight when
// x[lower] = x[upper] + delta.
type link struct {
	edge, upper, lower int
	delta              float64
	forced             bool // a side-exit column (S7): a candidate at any residual, sorted first
	exempt             bool // never a deliberate detour: a candidate at any residual
}

func (l link) key() [3]int { return [3]int{l.edge, l.upper, l.lower} }

// alignInput is act 2's input: the ordered graph, the minimum
// separations, the straighten span, the reference positions, the links,
// the vertex weights and the rooms S7's forcing keeps.
type alignInput struct {
	g      *lgraph.Graph
	space  spacing
	span   float64
	ref    []float64
	links  []link
	weight []float64
	rooms  []room
}

// roomSep is room r as a separation: the vertex it keeps on the left, the
// one on the right, and the least distance between their centers. The
// other node keeps the node clearance beyond the next vertex's port,
// counted from its box plus its own farthest side column or loop leg on
// its side facing the port (S7).
func (s spacing) roomSep(g *lgraph.Graph, r room) (left, right int, gap float64) {
	o := r.other
	half := g.Vertices[o].W / 2
	if r.sign > 0 {
		return r.next, o, r.port + half + s.out[o][0] + s.clearance
	}
	return o, r.next, -r.port + half + s.out[o][1] + s.clearance
}

// sep is the minimum distance between the centers of neighbors a and b,
// a left of b.
func (in alignInput) sep(a, b int) float64 {
	return in.space.sep(in.g, a, b)
}

// spacing holds what S7's minimum separations take from the Options and
// from the room each vertex needs beside it.
type spacing struct {
	gap       float64      // the node gap
	terminal  float64      // the distance between the centers of two terminals (S9)
	clearance float64      // kept beyond a side column or a loop leg
	inLane    float64      // kept between two facing side columns or loop legs
	text      bool         // the text profile, where every gap is whole cells
	out       [][2]float64 // per vertex: how far outside its left and right side its farthest side column or loop leg runs, 0 for none
}

// sep is the minimum distance between the centers of neighbors a and b of
// graph g, a left of b: the gap between their boxes, or more where a side
// column or a loop leg runs beside one of them: its distance out plus the
// clearance, and when a's right side and b's left side both hold one,
// both distances out plus the in-lane gap between them, in the text
// profile each rounded up to whole cells (S7). Two terminals, mere wire
// columns, keep only the terminal gap between their centers (S9, S14),
// whatever their widths: every terminal is a dummy column, DummyWidth
// wide (nested.go).
func (s spacing) sep(g *lgraph.Graph, a, b int) float64 {
	if terminal(g, a) && terminal(g, b) {
		return s.terminal
	}
	half := (g.Vertices[a].W + g.Vertices[b].W) / 2
	need := s.gap
	right, left := s.out[a][1], s.out[b][0]
	if right > 0 {
		need = max(need, s.cells(right+s.clearance))
	}
	if left > 0 {
		need = max(need, s.cells(left+s.clearance))
	}
	if right > 0 && left > 0 {
		need = max(need, s.cells(right+left+s.inLane))
	}
	return half + need
}

// cells rounds room up to whole cells in the text profile, where every
// separation is a whole number of cells.
func (s spacing) cells(room float64) float64 {
	if s.text {
		return math.Ceil(room)
	}
	return room
}

// terminal reports whether vertex v is a terminal (S9).
func terminal(g *lgraph.Graph, v int) bool {
	n := g.Vertices[v].Node
	return n >= 0 && g.Level.Nodes[n].Terminal
}

// blocks joins vertices into rigid blocks: x[v] = x[root[v]] + offset[v].
type blocks struct {
	root   []int
	offset []float64
}

func identity(n int) blocks {
	b := blocks{root: make([]int, n), offset: make([]float64, n)}
	for i := range b.root {
		b.root[i] = i
	}
	return b
}

type candidate struct {
	link
	residual float64
}

// align turns near-straight hops into rigid blocks and compacts the blocks
// (S7 act 2). Each round measures every open link's residual on the
// current layout; the links within the span, the span-exempt ones and the
// forced ones are candidates, sorted by residual, then edge, then upper
// vertex, and tried in tie batches (equal residuals sharing an endpoint),
// each accepted all or nothing when it stays consistent and feasible with
// the rooms kept so far. A forced batch outranks the rooms: one that only
// they make infeasible is accepted, and the rooms its order contradicts
// give way. After a round that accepts a batch, the blocks are compacted
// toward the reference under the minimum separations and the kept rooms.
func align(in alignInput) []float64 {
	g := in.g
	prefix := make([][]float64, len(g.Layers))
	for l, layer := range g.Layers {
		prefix[l] = make([]float64, len(layer))
		for i := 1; i < len(layer); i++ {
			prefix[l][i] = prefix[l][i-1] + in.sep(layer[i-1], layer[i])
		}
	}
	var accepted []link
	taken := map[[3]int]bool{}
	current := identity(len(g.Vertices))
	rooms := keptRooms(in, current, in.rooms)
	x := compact(in, current, rooms)
	for range maxRounds {
		var cands []candidate
		for _, l := range in.links {
			if taken[l.key()] {
				continue
			}
			residual := -1.0
			if !l.forced {
				residual = math.Abs(x[l.upper] + l.delta - x[l.lower])
			}
			if residual <= in.span || l.exempt {
				cands = append(cands, candidate{l, residual})
			}
		}
		if len(cands) == 0 {
			break
		}
		slices.SortStableFunc(cands, func(a, b candidate) int {
			return cmp.Or(cmp.Compare(a.residual, b.residual), cmp.Compare(a.edge, b.edge), cmp.Compare(a.upper, b.upper))
		})
		added := false
		for _, batch := range tieBatches(cands) {
			b, ok := build(g, slices.Concat(accepted, batch))
			if !ok {
				continue
			}
			switch {
			case feasible(in, prefix, b, rooms):
			case batch[0].forced && feasible(in, prefix, b, nil):
				// a forcing outranks a room: those it contradicts give way
				rooms = keptRooms(in, b, rooms)
			default:
				continue
			}
			accepted = append(accepted, batch...)
			for _, l := range batch {
				taken[l.key()] = true
			}
			current, added = b, true
		}
		if !added {
			break
		}
		x = compact(in, current, rooms)
	}
	return x
}

// keptRooms returns the rooms, in order, that agree with the blocks b and
// with the rooms kept before them (S7): a room whose two blocks the order
// of b and of the kept rooms already puts the other way round gives way.
// A room whose two vertices share a block is kept, though it orders no
// two blocks: its offsets meet it or not, and S7 finds out on the result.
func keptRooms(in alignInput, b blocks, rooms []room) []room {
	g := in.g
	succ := make([][]int, len(g.Vertices))
	for _, layer := range g.Layers {
		for i := 1; i < len(layer); i++ {
			if a, c := b.root[layer[i-1]], b.root[layer[i]]; a != c {
				succ[a] = append(succ[a], c)
			}
		}
	}
	var kept []room
	for _, r := range rooms {
		left, right, _ := in.space.roomSep(g, r)
		a, c := b.root[left], b.root[right]
		if a == c {
			kept = append(kept, r)
			continue
		}
		if leadsTo(succ, c, a) {
			continue // the order already puts a right of c
		}
		succ[a] = append(succ[a], c)
		kept = append(kept, r)
	}
	return kept
}

// leadsTo reports whether the order succ leads from block from to block to.
func leadsTo(succ [][]int, from, to int) bool {
	seen := map[int]bool{from: true}
	stack := []int{from}
	for len(stack) > 0 {
		v := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if v == to {
			return true
		}
		for _, w := range succ[v] {
			if !seen[w] {
				seen[w] = true
				stack = append(stack, w)
			}
		}
	}
	return false
}

// tieBatches groups sorted candidates that must be decided together: equal
// residuals (within eps) joined through shared endpoints. Batches come in
// residual order, and within one residual in order of their first member.
func tieBatches(cands []candidate) [][]link {
	var batches [][]link
	for i := 0; i < len(cands); {
		j := i
		for j < len(cands) && cands[j].residual <= cands[i].residual+eps {
			j++
		}
		group := cands[i:j]
		parent := make([]int, len(group))
		for k := range parent {
			parent[k] = k
		}
		find := func(k int) int {
			for parent[k] != k {
				k = parent[k]
			}
			return k
		}
		owner := map[int]int{}
		for k, c := range group {
			for _, v := range [2]int{c.upper, c.lower} {
				if seen, ok := owner[v]; ok {
					parent[find(k)] = find(seen)
				} else {
					owner[v] = k
				}
			}
		}
		slot := map[int]int{}
		for k, c := range group {
			r := find(k)
			s, ok := slot[r]
			if !ok {
				s = len(batches)
				slot[r] = s
				batches = append(batches, nil)
			}
			batches[s] = append(batches[s], c.link)
		}
		i = j
	}
	return batches
}

// build joins the chosen links into blocks, rooting each block at its
// first vertex in layer order, and fails when two links disagree on a
// member's offset.
func build(g *lgraph.Graph, chosen []link) (blocks, bool) {
	type arc struct {
		to    int
		delta float64
	}
	b := identity(len(g.Vertices))
	adj := make([][]arc, len(g.Vertices))
	for _, l := range chosen {
		adj[l.upper] = append(adj[l.upper], arc{l.lower, l.delta})
		adj[l.lower] = append(adj[l.lower], arc{l.upper, -l.delta})
	}
	visited := make([]bool, len(g.Vertices))
	for _, layer := range g.Layers {
		for _, start := range layer {
			if visited[start] || len(adj[start]) == 0 {
				continue
			}
			visited[start] = true
			queue := []int{start}
			for h := 0; h < len(queue); h++ {
				v := queue[h]
				for _, a := range adj[v] {
					want := b.offset[v] + a.delta
					if visited[a.to] {
						if math.Abs(b.offset[a.to]-want) > eps {
							return blocks{}, false
						}
						continue
					}
					visited[a.to] = true
					b.root[a.to] = start
					b.offset[a.to] = want
					queue = append(queue, a.to)
				}
			}
		}
	}
	return b, true
}

// feasible reports whether a block placement is realizable: two members
// of one block in a layer leave room for everything ordered between them,
// and the blocks' left-to-right order is acyclic, the two blocks of each
// of rooms in its order included. A block that straddles another block's
// vertex in one layer, however much space there is, two blocks that swap
// sides between layers, and a room the order contradicts all make it
// cyclic, and with it the separation constraints. A room whose two
// vertices share a block is no order between blocks: its offsets keep it
// or not, and S7 finds out on the result.
func feasible(in alignInput, prefix [][]float64, b blocks, rooms []room) bool {
	g := in.g
	for l, layer := range g.Layers {
		last := map[int]int{}
		for i, v := range layer {
			r := b.root[v]
			if prev, ok := last[r]; ok {
				room := prefix[l][i] - prefix[l][prev]
				if b.offset[v]-b.offset[layer[prev]] < room-eps {
					return false
				}
			}
			last[r] = i
		}
	}
	n := len(g.Vertices)
	succ := make([][]int, n)
	indeg := make([]int, n)
	joined := map[[2]int]bool{}
	for _, layer := range g.Layers {
		for i := 1; i < len(layer); i++ {
			a, c := b.root[layer[i-1]], b.root[layer[i]]
			if a == c || joined[[2]int{a, c}] {
				continue
			}
			joined[[2]int{a, c}] = true
			succ[a] = append(succ[a], c)
			indeg[c]++
		}
	}
	for _, r := range rooms {
		left, right, _ := in.space.roomSep(g, r)
		a, c := b.root[left], b.root[right]
		if a == c || joined[[2]int{a, c}] {
			continue
		}
		joined[[2]int{a, c}] = true
		succ[a] = append(succ[a], c)
		indeg[c]++
	}
	roots := 0
	var queue []int
	for v := range n {
		if b.root[v] != v {
			continue
		}
		roots++
		if indeg[v] == 0 {
			queue = append(queue, v)
		}
	}
	for h := 0; h < len(queue); h++ {
		for _, c := range succ[queue[h]] {
			if indeg[c]--; indeg[c] == 0 {
				queue = append(queue, c)
			}
		}
	}
	return len(queue) == roots
}

// compact places the blocks: each pulls toward its members' weighted mean
// reference, under every layer's minimum separations and every one of
// rooms between two blocks.
func compact(in alignInput, b blocks, rooms []room) []float64 {
	g := in.g
	slot := make([]int, len(g.Vertices))
	for i := range slot {
		slot[i] = -1
	}
	var vars []sepVar
	var pull []float64
	for _, layer := range g.Layers {
		for _, v := range layer {
			r := b.root[v]
			if slot[r] < 0 {
				slot[r] = len(vars)
				vars = append(vars, sepVar{})
				pull = append(pull, 0)
			}
			s, w := slot[r], in.weight[v]
			vars[s].weight += w
			pull[s] += w * (in.ref[v] - b.offset[v])
		}
	}
	for s := range vars {
		if vars[s].weight > 0 {
			vars[s].desired = pull[s] / vars[s].weight
		}
	}
	var cons []sepCon
	for _, layer := range g.Layers {
		for i := 1; i < len(layer); i++ {
			a, c := layer[i-1], layer[i]
			left, right := slot[b.root[a]], slot[b.root[c]]
			if left == right {
				continue
			}
			cons = append(cons, sepCon{left: left, right: right, gap: in.sep(a, c) + b.offset[a] - b.offset[c]})
		}
	}
	for _, r := range rooms {
		a, c, gap := in.space.roomSep(g, r)
		left, right := slot[b.root[a]], slot[b.root[c]]
		if left == right {
			continue
		}
		cons = append(cons, sepCon{left: left, right: right, gap: gap + b.offset[a] - b.offset[c]})
	}
	solved := solve(vars, cons)
	x := make([]float64, len(g.Vertices))
	for _, layer := range g.Layers {
		for _, v := range layer {
			x[v] = clean(solved[slot[b.root[v]]] + b.offset[v])
		}
	}
	return x
}

// clean drops arithmetic noise below a billionth of a unit, so that a
// block at its members' mean still meets exact identities such as an
// arrowhead's x equal to a node's center.
func clean(v float64) float64 { return math.Round(v*1e9) / 1e9 }
