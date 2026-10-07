package lgraph

import "fmt"

// Vertex is a node of the layered graph: a level node, or a dummy on the
// chain of an edge that spans several layers (S5).
type Vertex struct {
	ID    string // the node's id, or "<edge id>@<layer>" for a dummy
	Node  int    // index into Level.Nodes; -1 for a dummy
	Edge  int    // the edge a dummy belongs to; -1 for a node
	Layer int
	W     float64 // cross-axis extent: the node's W, or the dummy width
}

// Graph is a level expanded into layers (S5). Vertex v < len(Level.Nodes)
// is node v; the dummies follow, by edge and then by layer.
type Graph struct {
	Level    *Level
	Reversed []bool // per edge (S2); nil reverses nothing
	Vertices []Vertex
	Layers   [][]int // vertex indices per layer, in order; order (S6) rearranges them
	Chains   [][]int // per edge: its vertices from the upstream end to the downstream end; nil for a self-loop
	Bands    []int   // per vertex: the band of its terminal row it keeps to (S6, S9), bands in row order; nil, or -1 for a vertex free to move
}

// Build expands lv into layers (S5). layers gives each node's layer (S3),
// reversed each edge's orientation (S2), dummyW a dummy's cross-axis
// extent. Each layer starts with its nodes in declaration order, then its
// dummies in edge order; SeatCorridors completes the declaration seed. An
// edge that does not run at least one layer downstream is an error.
func Build(lv *Level, layers []int, reversed []bool, dummyW float64) (*Graph, error) {
	if len(layers) != len(lv.Nodes) {
		return nil, fmt.Errorf("lgraph: %d layers for %d nodes", len(layers), len(lv.Nodes))
	}
	count := 0
	for i, l := range layers {
		if l < 0 {
			return nil, fmt.Errorf("lgraph: node %s on layer %d", lv.Nodes[i].ID, l)
		}
		count = max(count, l+1)
	}
	g := &Graph{Level: lv, Reversed: reversed, Layers: make([][]int, count), Chains: make([][]int, len(lv.Edges))}
	for i, n := range lv.Nodes {
		g.add(Vertex{ID: n.ID, Node: i, Edge: -1, Layer: layers[i], W: n.W})
	}
	for e, ed := range lv.Edges {
		if lv.SelfLoop(e) {
			continue
		}
		upper, lower := lv.Oriented(e, reversed)
		if layers[lower] <= layers[upper] {
			return nil, fmt.Errorf("lgraph: edge %s runs from layer %d to layer %d", ed.ID, layers[upper], layers[lower])
		}
		chain := []int{upper}
		for l := layers[upper] + 1; l < layers[lower]; l++ {
			chain = append(chain, g.add(Vertex{
				ID: fmt.Sprintf("%s@%d", ed.ID, l), Node: -1, Edge: e, Layer: l, W: dummyW,
			}))
		}
		g.Chains[e] = append(chain, lower)
	}
	return g, nil
}

func (g *Graph) add(v Vertex) int {
	i := len(g.Vertices)
	g.Vertices = append(g.Vertices, v)
	g.Layers[v.Layer] = append(g.Layers[v.Layer], i)
	return i
}

// Dummy reports whether vertex v is a dummy.
func (g *Graph) Dummy(v int) bool { return g.Vertices[v].Node < 0 }

// Index returns each vertex's position within its layer.
func (g *Graph) Index() []int {
	idx := make([]int, len(g.Vertices))
	for _, layer := range g.Layers {
		for i, v := range layer {
			idx[v] = i
		}
	}
	return idx
}

// Hop is one step of an edge's chain, between adjacent layers.
type Hop struct{ Edge, Upper, Lower int }

// Hops returns every chain's hops, in edge order and down each chain.
func (g *Graph) Hops() []Hop {
	var hops []Hop
	for e, chain := range g.Chains {
		for i := 0; i+1 < len(chain); i++ {
			hops = append(hops, Hop{Edge: e, Upper: chain[i], Lower: chain[i+1]})
		}
	}
	return hops
}

// Shift is where edge e's end at vertex v sits on v's face, as a fraction
// of v's width from its center: the anchor of an end anchored on a
// group's face (S9), which S6 counts crossings by; 0 for any other end,
// whose port follows the order.
func (g *Graph) Shift(e, v int) float64 {
	chain := g.Chains[e]
	w := g.Vertices[v].W
	if len(chain) < 2 || w <= 0 {
		return 0
	}
	var a Anchor
	switch v {
	case chain[0]:
		a = g.Level.End(e, 0, g.Reversed)
	case chain[len(chain)-1]:
		a = g.Level.End(e, 1, g.Reversed)
	}
	if !a.On {
		return 0
	}
	return a.At / w
}

// CounterFlow reports whether edge e's end at vertex v is a counter-flow
// end on a node: e is reversed (S2), v ends its chain, and v is a node
// that is neither a group nor a terminal. The router attaches such an end
// at a side of its node (S8), and S6 counts it there when it keeps a loop
// together.
func (g *Graph) CounterFlow(e, v int) bool {
	chain := g.Chains[e]
	if g.Reversed == nil || !g.Reversed[e] || len(chain) < 2 || (v != chain[0] && v != chain[len(chain)-1]) {
		return false
	}
	n := g.Vertices[v].Node
	return n >= 0 && !g.Level.Nodes[n].Group && !g.Level.Nodes[n].Terminal
}

// Crossings counts the pairs of hops that cross between adjacent layers in
// the current order, a hop's ends placed by their vertices' indices and
// their shifts.
func (g *Graph) Crossings() int {
	idx := g.Index()
	spans := make([][][2]float64, len(g.Layers))
	for _, h := range g.Hops() {
		l := g.Vertices[h.Upper].Layer
		spans[l] = append(spans[l], [2]float64{
			float64(idx[h.Upper]) + g.Shift(h.Edge, h.Upper),
			float64(idx[h.Lower]) + g.Shift(h.Edge, h.Lower),
		})
	}
	total := 0
	for _, s := range spans {
		total += Inversions(s)
	}
	return total
}

// Inversions counts the crossing pairs among hops between two layers, each
// given as (position above, position below). Hops that share an end never
// cross.
func Inversions[T int | float64](spans [][2]T) int {
	n := 0
	for i, a := range spans {
		for _, b := range spans[i+1:] {
			if (a[0] < b[0] && a[1] > b[1]) || (a[0] > b[0] && a[1] < b[1]) {
				n++
			}
		}
	}
	return n
}
