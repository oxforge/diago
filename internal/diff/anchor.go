package diff

import (
	"cmp"
	"maps"
	"slices"

	"github.com/oxforge/diago/internal/model"
)

// Anchor is the carrier the union is laid out on (S13, Diff): before's,
// each scope's order completed from after's. Per scope, after's layers are
// aligned with before's (shift), and in each layer before's order stays
// and every entry the before scope lacks goes just before the nearest
// entry to its right in after's layer that the merged layer holds, or at
// its end (merge). A scope only after holds gives its order. Layers,
// Reversed and Ranked are before's alone, and a removed edge the union
// renamed has its slots renamed alike. before and after are left as they
// were.
func (u UnionGraph) Anchor(before, after *model.LayoutHints) *model.LayoutHints {
	out := &model.LayoutHints{
		Version:  before.Version,
		Scopes:   make(map[string]model.ScopeHints, len(before.Scopes)),
		Reversed: slices.Clone(before.Reversed),
		Ranked:   slices.Clone(before.Ranked),
	}
	for id, b := range before.Scopes {
		s := model.ScopeHints{Layers: maps.Clone(b.Layers), Order: u.renamed(b.Order)}
		if a, ok := after.Scopes[id]; ok {
			s.Order = merge(rows(s.Layers, s.Order, 0), rows(a.Layers, a.Order, shift(b.Layers, a.Layers)))
		}
		out.Scopes[id] = s
	}
	for id, a := range after.Scopes {
		if _, ok := before.Scopes[id]; !ok {
			out.Scopes[id] = model.ScopeHints{Layers: map[string]int{}, Order: maps.Clone(a.Order)}
		}
	}
	return out
}

// renamed is order with the slots of every removed edge the union renamed
// keyed by its union id.
func (u UnionGraph) renamed(order map[string]int) map[string]int {
	out := make(map[string]int, len(order))
	for key, i := range order {
		if id, l, ok := model.SplitSlot(key); ok {
			if uid, ok := u.RemovedEdgeIDs[id]; ok {
				key = model.Slot(uid, l)
			}
		}
		out[key] = i
	}
	return out
}

// shift is what turns after's layers into before's: the difference that
// the most nodes both give a layer agree on; on a tie the one nearest
// zero, then the lower one; 0 when they share none.
func shift(before, after map[string]int) int {
	votes := map[int]int{}
	for id, b := range before {
		if a, ok := after[id]; ok {
			votes[b-a]++
		}
	}
	best, most := 0, 0
	for s, n := range votes {
		if n > most || n == most && (abs(s) < abs(best) || abs(s) == abs(best) && s < best) {
			best, most = s, n
		}
	}
	return best
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// rows groups order's keys by layer, by in-layer index, each one's layer
// moved by by: a node's from layers, a slot's from its key, which is
// renumbered alike. A key with neither has no row.
func rows(layers, order map[string]int, by int) map[int][]string {
	type entry struct {
		key string
		i   int
	}
	byLayer := map[int][]entry{}
	for key, i := range order {
		l, ok := layers[key]
		if !ok {
			var id string
			if id, l, ok = model.SplitSlot(key); !ok {
				continue
			}
			key = model.Slot(id, l+by)
		}
		byLayer[l+by] = append(byLayer[l+by], entry{key, i})
	}
	out := make(map[int][]string, len(byLayer))
	for l, es := range byLayer {
		slices.SortFunc(es, func(a, b entry) int { return cmp.Or(cmp.Compare(a.i, b.i), cmp.Compare(a.key, b.key)) })
		for _, e := range es {
			out[l] = append(out[l], e.key)
		}
	}
	return out
}

// merge is the order of before's rows with after's entries that before
// lacks: each goes just before the nearest entry to its right in its
// after row that the merged row holds, or at the row's end, so a run of
// them keeps its after order.
func merge(before, after map[int][]string) map[string]int {
	held := map[string]bool{}
	for _, row := range before {
		for _, key := range row {
			held[key] = true
		}
	}
	layers := maps.Clone(before)
	maps.Copy(layers, after)
	out := map[string]int{}
	for l := range layers {
		row := slices.Clone(before[l])
		at := len(row)
		for _, key := range slices.Backward(after[l]) {
			switch {
			case !held[key]:
				row = slices.Insert(row, at, key)
			case slices.Contains(row, key):
				at = slices.Index(row, key)
			}
		}
		for i, key := range row {
			out[key] = i
		}
	}
	return out
}
