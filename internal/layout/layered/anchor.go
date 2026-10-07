package layered

import (
	"context"
	"maps"
	"slices"
	"strconv"

	"github.com/oxforge/diago/internal/layout/layered/lgraph"
	"github.com/oxforge/diago/internal/layout/layered/order"
	"github.com/oxforge/diago/internal/layoutdbg"
	"github.com/oxforge/diago/internal/model"
)

// slot is the carrier's key for edge id's vertex on layer l of a scope
// where it has no node of its own: a dummy (S5), or a terminal on the
// scope's border row (S9, C18).
func slot(id string, l int) string { return id + "@" + strconv.Itoa(l) }

// keys gives every vertex of lg its carrier key (C18): a node's or a
// child group's id, and a dummy's or a terminal's slot of its edge on
// layer at[l], l its own layer; "" when at[l] is negative, a layer with
// none.
func keys(lg *lgraph.Graph, at []int) []string {
	lv := lg.Level
	edge := make([]int, len(lv.Nodes)) // per terminal: its one edge
	for e, ed := range lv.Edges {
		if lv.Nodes[ed.From].Terminal {
			edge[ed.From] = e
		}
		if lv.Nodes[ed.To].Terminal {
			edge[ed.To] = e
		}
	}
	out := make([]string, len(lg.Vertices))
	for v, vx := range lg.Vertices {
		e := vx.Edge
		switch {
		case vx.Node >= 0 && !lv.Nodes[vx.Node].Terminal:
			out[v] = vx.ID
			continue
		case vx.Node >= 0:
			e = edge[vx.Node]
		}
		if l := at[vx.Layer]; l >= 0 {
			out[v] = slot(lv.Edges[e].ID, l)
		}
	}
	return out
}

// identity is the layer map of a carrier read off lg itself: each layer
// to its own number.
func identity(lg *lgraph.Graph) []int {
	at := make([]int, len(lg.Layers))
	for l := range at {
		at[l] = l
	}
	return at
}

// readPrevious checks prev against g and records what it cannot use
// (S13, C18): a scope the graph has no container for, and a reversed
// edge the graph lacks. It returns the edges prev reversed; nil when prev
// is nil.
func (n *nesting) readPrevious(ctx context.Context) map[string]bool {
	if n.prev == nil {
		return nil
	}
	scopes := map[string]bool{}
	for c := range n.tree.Containers {
		scopes[n.name(c)] = true
	}
	var vanished []string
	for id := range n.prev.Scopes {
		if !scopes[id] {
			vanished = append(vanished, id)
		}
	}
	slices.Sort(vanished)
	edges := make(map[string]bool, len(n.g.Edges))
	for _, e := range n.g.Edges {
		edges[e.ID] = true
	}
	reversed := make(map[string]bool, len(n.prev.Reversed))
	var unknown []string
	for _, id := range n.prev.Reversed {
		if edges[id] {
			reversed[id] = true
		} else {
			unknown = append(unknown, id)
		}
	}
	layoutdbg.Decision(ctx, "previous_read",
		"phase", "anchoring", "module", "diago", "spec_ref", "S13",
		"scopes", len(n.prev.Scopes), "vanished", vanished,
		"reversed", len(reversed), "unknown_reversed", unknown)
	return reversed
}

// prevScope is what a previous carrier holds for one level (S13): its
// scope's entries, and the lowest and the highest layer among them, which
// the previous level's terminal rows lay just outside of.
type prevScope struct {
	name            string
	sh              model.ScopeHints
	ok              bool // the carrier has the scope
	lowest, highest int  // -1 without a layer
	all             *model.LayoutHints
}

// scopePrevious is container c's slice of the previous carrier; nil
// without one.
func (n *nesting) scopePrevious(c int) *prevScope {
	if n.prev == nil {
		return nil
	}
	name := n.name(c)
	sh, ok := n.prev.Scopes[name]
	p := &prevScope{name: name, sh: sh, ok: ok, lowest: -1, highest: -1, all: n.prev}
	for _, l := range sh.Layers {
		if l >= 0 {
			if p.lowest < 0 || l < p.lowest {
				p.lowest = l
			}
			p.highest = max(p.highest, l)
		}
	}
	return p
}

// layers gives each node of lv its previous layer as the scope holds it,
// or -1: a terminal, a node the scope does not hold, and a negative value
// take none. rank aligns them with the level (S13).
func (p *prevScope) layers(lv *lgraph.Level) []int {
	out := make([]int, len(lv.Nodes))
	for v, nd := range lv.Nodes {
		out[v] = -1
		if l, ok := p.sh.Layers[nd.ID]; ok && !nd.Terminal && l >= 0 {
			out[v] = l
		}
	}
	return out
}

// rows maps each layer of lg to its previous layer, or -1 for none (S13,
// Scopes): a layer to the one that its nodes which kept their previous
// layer (kept, per level node) held, a first terminal row to the row
// above the scope's lowest layer and a last one to the row below its
// highest. Kept nodes of one layer held one layer: rank pins them at
// their previous layers shifted alike and renumbers in order.
func (p *prevScope) rows(lg *lgraph.Graph, kept []bool) []int {
	lv := lg.Level
	at := make([]int, len(lg.Layers))
	for l, layer := range lg.Layers {
		at[l] = -1
		terminals := len(layer) > 0
		for _, v := range layer {
			nd := lg.Vertices[v].Node
			if nd < 0 || !lv.Nodes[nd].Terminal {
				terminals = false
			}
			if nd >= 0 && kept != nil && kept[nd] {
				at[l] = p.sh.Layers[lv.Nodes[nd].ID]
			}
		}
		switch {
		case !terminals || p.lowest < 0:
		case l == 0:
			at[l] = p.lowest - 1
		default:
			at[l] = p.highest + 1
		}
	}
	return at
}

// seed orders lg's layers from the scope's previous order (S13, order.Seed)
// and records what it used: the level's nodes and groups the scope holds
// (known), holds elsewhere (moved) or not at all (new), the entries that
// match nothing in the level (missing) and those with a negative value
// (rejected), and the vertices seeded. A vertex is looked up by its key
// (keys) on its layer's previous layer (rows); kept tells, per level
// node, whether it kept its previous layer (rank.AssignKept). It returns
// how many vertices it seeded.
func (p *prevScope) seed(ctx context.Context, lg *lgraph.Graph, kept []bool) int {
	lv := lg.Level
	prev := make([]int, len(lg.Vertices))
	used := map[string]bool{}
	var known, moved, fresh, rejected []string
	for v, key := range keys(lg, p.rows(lg, kept)) {
		prev[v] = -1
		if key == "" {
			continue
		}
		i, inOrder := p.sh.Order[key]
		_, inLayers := p.sh.Layers[key]
		used[key] = inOrder || inLayers
		switch {
		case inOrder && i >= 0:
			prev[v] = i
		case inOrder:
			rejected = append(rejected, key)
		}
		if vx := lg.Vertices[v]; vx.Node < 0 || lv.Nodes[vx.Node].Terminal {
			continue
		}
		switch {
		case used[key]:
			known = append(known, key)
		case p.elsewhere(key):
			moved = append(moved, key)
		default:
			fresh = append(fresh, key)
		}
	}
	for _, id := range known {
		if l := p.sh.Layers[id]; l < 0 {
			rejected = append(rejected, id)
		}
	}
	var missing []string
	for _, m := range []map[string]int{p.sh.Layers, p.sh.Order} {
		for id := range m {
			if !used[id] {
				missing = append(missing, id)
			}
		}
	}
	slices.Sort(missing)
	missing = slices.Compact(missing)
	slices.Sort(rejected)
	rejected = slices.Compact(rejected)
	seeded := order.Seed(lg, prev)
	layoutdbg.Decision(ctx, "previous_applied",
		"phase", "anchoring", "module", "diago", "spec_ref", "S13",
		"scope", p.name, "scope_known", p.ok,
		"known", known, "moved", moved, "new", fresh,
		"missing", missing, "rejected", rejected, "seeded", seeded)
	return seeded
}

// elsewhere reports whether a scope other than p's holds id.
func (p *prevScope) elsewhere(id string) bool {
	for name, sh := range p.all.Scopes {
		if name == p.name {
			continue
		}
		if _, ok := sh.Layers[id]; ok {
			return true
		}
		if _, ok := sh.Order[id]; ok {
			return true
		}
	}
	return false
}

// carrier reads the anchoring carrier off the final levels (S13, C18):
// per container, each node's and child group's layer and index in its
// layer, each dummy's and terminal's index under its edge's slot on its
// layer, and the edges its layered graph reverses at their home levels,
// sorted: for a copying level, the copy's (S12, Rule A).
func (n *nesting) carrier() *model.LayoutHints {
	h := model.NewLayoutHints()
	for c, l := range n.levels {
		s := h.Scope(n.name(c))
		lg := l.out.Graph
		key := keys(lg, identity(lg))
		for li, layer := range lg.Layers {
			for i, v := range layer {
				s.Order[key[v]] = i
				if nd := lg.Vertices[v].Node; nd >= 0 && !lg.Level.Nodes[nd].Terminal {
					s.Layers[key[v]] = li
				}
			}
		}
		for le, r := range lg.Reversed {
			if r && n.tree.Home[l.carries[le]] == c {
				h.Reversed = append(h.Reversed, l.lv.Edges[le].ID)
			}
		}
	}
	slices.Sort(h.Reversed)
	return h
}

// keepRanked ranks, in ranked (per edge of g), the flat edges of g that
// prev's carrier lists as fallen back (S13, Flat edges), and records them
// with the entries it ignores: an edge g lacks or does not mark flat
// (C18.2). It does nothing when prev lists none.
func keepRanked(ctx context.Context, g model.Graph, prev *model.LayoutHints, ranked []bool) {
	if prev == nil || len(prev.Ranked) == 0 {
		return
	}
	listed := make(map[string]bool, len(prev.Ranked))
	for _, id := range prev.Ranked {
		listed[id] = true
	}
	kept := []string{}
	for i, e := range g.Edges {
		if e.Flat && listed[e.ID] {
			ranked[i] = true
			kept = append(kept, e.ID)
			delete(listed, e.ID)
		}
	}
	stale := append([]string{}, slices.Sorted(maps.Keys(listed))...)
	layoutdbg.Decision(ctx, "previous_ranked",
		"phase", "anchoring", "module", "diago", "spec_ref", "S13",
		"ranked", kept, "stale", stale)
}

// writeRanked lists in h, a carrier of g's layout, the flat edges of g
// that fell back (ranked, per edge of g), sorted (C18, ranked); none
// leaves it nil, so that the carrier omits it.
func writeRanked(h *model.LayoutHints, g model.Graph, ranked []bool) {
	for i, e := range g.Edges {
		if e.Flat && ranked[i] {
			h.Ranked = append(h.Ranked, e.ID)
		}
	}
	slices.Sort(h.Ranked)
}
