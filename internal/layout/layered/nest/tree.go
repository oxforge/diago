// Package nest builds a graph's containment tree and composes its levels
// (S9): the root and one level per group, each laid out over its direct
// children, where an edge that crosses a container's border meets it at a
// terminal.
package nest

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/oxforge/diago/internal/model"
)

// Container is the root or one group: a level of the tree.
type Container struct {
	Group    int     // index into the graph's Groups; -1 for the root
	Parent   int     // the enclosing container; -1 for the root
	Children []Child // its direct children, in declaration order (S9)
}

// Child is a container's direct child: a node, or a child container.
type Child struct {
	Node      int // index into the graph's Nodes; -1 for a container
	Container int // index into Tree.Containers; -1 for a node
}

// Tree is a graph's containment (S9). Container 0 is the root; the others
// follow the graph's Groups, container i + 1 for group i.
type Tree struct {
	Containers []Container
	In         []int // per node: its container
	Home       []int // per edge: its home container, the nearest that holds both ends
}

// Build returns g's containment tree. A child's declaration order is its
// first-declared node's: a group enters its container where the first
// node inside it, at any depth, was declared. A node or a group that two
// groups claim, a group that holds itself, and a name that is neither a
// node nor a group are errors.
func Build(g model.Graph) (*Tree, error) {
	nodeAt := make(map[string]int, len(g.Nodes))
	for i, n := range g.Nodes {
		nodeAt[n.ID] = i
	}
	groupAt := make(map[string]int, len(g.Groups))
	for i, gr := range g.Groups {
		groupAt[gr.ID] = i
	}
	t := &Tree{Containers: make([]Container, len(g.Groups)+1), In: make([]int, len(g.Nodes))}
	parent := make([]int, len(t.Containers))
	for i := range parent {
		parent[i] = -1
	}
	for i := range t.Containers {
		t.Containers[i].Group = i - 1
	}
	for i, gr := range g.Groups {
		c := i + 1
		for _, id := range gr.Contains {
			n, ok := nodeAt[id]
			if !ok {
				return nil, fmt.Errorf("nest: group %s contains %s, not a node", gr.ID, id)
			}
			if t.In[n] != 0 {
				return nil, fmt.Errorf("nest: node %s is in two groups", id)
			}
			t.In[n] = c
		}
		for _, id := range gr.Children {
			k, ok := groupAt[id]
			if !ok {
				return nil, fmt.Errorf("nest: group %s holds %s, not a group", gr.ID, id)
			}
			if parent[k+1] != -1 {
				return nil, fmt.Errorf("nest: group %s is in two groups", id)
			}
			parent[k+1] = c
		}
	}
	for c := 1; c < len(parent); c++ {
		if parent[c] == -1 {
			parent[c] = 0
		}
		t.Containers[c].Parent = parent[c]
	}
	for c := 1; c < len(parent); c++ {
		for p, hops := parent[c], 0; p != 0; p, hops = parent[p], hops+1 {
			if p == c || hops > len(parent) {
				return nil, fmt.Errorf("nest: group %s holds itself", g.Groups[c-1].ID)
			}
		}
	}
	t.Containers[0].Parent = -1

	first := make([]int, len(t.Containers)) // per container: its first-declared node, len(g.Nodes) when empty
	for c := range first {
		first[c] = len(g.Nodes)
	}
	for n := len(g.Nodes) - 1; n >= 0; n-- {
		for c := t.In[n]; c != -1; c = t.Containers[c].Parent {
			first[c] = n
		}
	}
	key := func(ch Child) int {
		if ch.Node >= 0 {
			return ch.Node
		}
		return first[ch.Container]
	}
	for n := range g.Nodes {
		c := t.In[n]
		t.Containers[c].Children = append(t.Containers[c].Children, Child{Node: n, Container: -1})
	}
	for c := 1; c < len(t.Containers); c++ {
		p := t.Containers[c].Parent
		t.Containers[p].Children = append(t.Containers[p].Children, Child{Node: -1, Container: c})
	}
	for c := range t.Containers {
		slices.SortStableFunc(t.Containers[c].Children, func(a, b Child) int {
			return cmp.Compare(key(a), key(b))
		})
	}

	t.Home = make([]int, len(g.Edges))
	for i, e := range g.Edges {
		from, okFrom := nodeAt[e.From]
		to, okTo := nodeAt[e.To]
		if !okFrom || !okTo {
			return nil, fmt.Errorf("nest: edge %s joins %s and %s, not both nodes", e.ID, e.From, e.To)
		}
		t.Home[i] = t.common(t.In[from], t.In[to])
	}
	return t, nil
}

// common is the nearest container that holds both a and b.
func (t *Tree) common(a, b int) int {
	for x := a; x != -1; x = t.Containers[x].Parent {
		for y := b; y != -1; y = t.Containers[y].Parent {
			if x == y {
				return x
			}
		}
	}
	return 0
}

// Rep is the direct child of container c that holds node n, which must lie
// inside c.
func (t *Tree) Rep(c, n int) Child {
	if t.In[n] == c {
		return Child{Node: n, Container: -1}
	}
	x := t.In[n]
	for t.Containers[x].Parent != c {
		x = t.Containers[x].Parent
	}
	return Child{Node: -1, Container: x}
}

// Path is the containers an edge crosses from its home c down to node n:
// every container that holds n and lies inside c, outermost first.
func (t *Tree) Path(c, n int) []int {
	var path []int
	for x := t.In[n]; x != c; x = t.Containers[x].Parent {
		path = append(path, x)
	}
	slices.Reverse(path)
	return path
}
