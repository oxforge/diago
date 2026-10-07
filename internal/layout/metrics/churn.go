package metrics

import (
	"fmt"
	"math"
	"slices"

	"github.com/oxforge/diago/internal/model"
)

// Churn is how far a layout anchored on a previous one moved from it (Q6).
// A survivor is a node of both layouts; in the carriers, an id of one
// scope's layers in both.
type Churn struct {
	Across, Along float64 // the worst survivor's center move across and along the flow, after the median move on that axis, in px
	Relayered     int     // survivors whose layer moved against the other survivors of their scope (Relayered; C18.4 allows none)
	Swapped       int     // pairs of survivors that share a layer before and after, in the other order
}

// MeasureChurn compares after, laid out in dir anchored on before's
// carrier (C18), with before (Q6).
func MeasureChurn(before, after *model.PositionedGraph, dir model.Direction) Churn {
	was := make(map[string]model.PositionedNode, len(before.Nodes))
	for _, n := range before.Nodes {
		was[n.ID] = n
	}
	var dx, dy []float64
	for _, n := range after.Nodes {
		if b, ok := was[n.ID]; ok {
			dx, dy = append(dx, n.X-b.X), append(dy, n.Y-b.Y)
		}
	}
	c := Churn{Across: worstMove(dx), Along: worstMove(dy)}
	if dir == model.Right || dir == model.Left {
		c.Across, c.Along = c.Along, c.Across
	}
	if before.LayoutHints == nil || after.LayoutHints == nil {
		return c
	}
	c.Relayered = len(Relayered(before.LayoutHints, after.LayoutHints))
	for scope, b := range before.LayoutHints.Scopes {
		a := after.LayoutHints.Scopes[scope]
		for id, l := range b.Layers {
			al, ok := a.Layers[id]
			if !ok {
				continue
			}
			for id2, l2 := range b.Layers {
				if al2, ok := a.Layers[id2]; ok && l2 == l && al2 == al && b.Order[id] < b.Order[id2] && a.Order[id] > a.Order[id2] {
					c.Swapped++
				}
			}
		}
	}
	return c
}

// worstMove is the largest distance of a move in d from their median; 0
// for none.
func worstMove(d []float64) float64 {
	if len(d) == 0 {
		return 0
	}
	m := slices.Sorted(slices.Values(d))[len(d)/2]
	w := 0.0
	for _, v := range d {
		w = math.Max(w, math.Abs(v-m))
	}
	return w
}

// String renders c as space-separated key=value pairs, px values rounded
// to whole px.
func (c Churn) String() string {
	return fmt.Sprintf("across=%.0f along=%.0f relayered=%d swapped=%d", c.Across, c.Along, c.Relayered, c.Swapped)
}

// Relayered lists the survivors of before's carrier whose layer moved in
// after's, a layout anchored on it (C18), against the other survivors of
// their scope (C18.4): in each scope, those whose move differs from its
// most common one, the smallest on a tie, so that a layer added above
// them all moves none. Each is "<scope>/<id>", sorted; nil for none.
func Relayered(before, after *model.LayoutHints) []string {
	if before == nil || after == nil {
		return nil
	}
	var out []string
	for scope, b := range before.Scopes {
		a := after.Scopes[scope]
		count := map[int]int{}
		for id, l := range b.Layers {
			if al, ok := a.Layers[id]; ok {
				count[al-l]++
			}
		}
		// The most common move, the smallest on a tie: no map order
		// reaches the choice.
		common, most := 0, 0
		for move, n := range count {
			if n > most || (n == most && move < common) {
				common, most = move, n
			}
		}
		for id, l := range b.Layers {
			if al, ok := a.Layers[id]; ok && al-l != common {
				out = append(out, scope+"/"+id)
			}
		}
	}
	slices.Sort(out)
	return out
}

// NewlyRanked lists the flat edges that after's carrier ranks (C18,
// ranked) and before's layout routed flat: the ids among flat, the flat
// edges of before's graph, that after's carrier ranks and before's does
// not. C18.3 and C18.4 do not bind a layout for which it is not empty.
// Sorted; nil for none.
func NewlyRanked(before, after *model.LayoutHints, flat []string) []string {
	if after == nil {
		return nil
	}
	var was []string
	if before != nil {
		was = before.Ranked
	}
	var out []string
	for _, id := range after.Ranked {
		if slices.Contains(flat, id) && !slices.Contains(was, id) {
			out = append(out, id)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}
