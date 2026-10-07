// Package rank assigns every node of an acyclic level to a layer (S3).
package rank

import (
	"cmp"
	"context"
	"math"
	"slices"

	"github.com/oxforge/diago/internal/layout/layered/lgraph"
	"github.com/oxforge/diago/internal/layoutdbg"
)

// Assign returns each node's layer (S3): longest-path layering, then pull
// tightening. Edges marked in reversed point the other way (S2);
// self-loops are ignored. Every other edge spans at least one layer
// downstream, and the layers form a contiguous range from 0. Terminals
// (S9) take rows of their own: the other nodes are layered without them,
// a terminal whose edge leaves it takes a first row above them and one
// whose edge enters it a last row below them. previous gives each node's
// layer in a previous layout (S13), -1 for none, and nil gives none: a
// node adopts it, aligned with the level, where the level still allows it
// (Adopt) and then never moves.
func Assign(ctx context.Context, lv *lgraph.Level, reversed []bool, previous []int) []int {
	layers, _ := AssignKept(ctx, lv, reversed, previous)
	return layers
}

// AssignKept is Assign, and which nodes kept their previous layer: those
// that adopted it (Adopt); nil when previous is nil.
func AssignKept(ctx context.Context, lv *lgraph.Level, reversed []bool, previous []int) ([]int, []bool) {
	inner, keep := withoutTerminals(lv)
	longest := LongestPath(inner, reversed)
	start, adopted := longest, []bool(nil)
	if previous != nil {
		start, adopted = Adopt(ctx, inner, reversed, longest, previous)
	}
	layers := Tighten(inner, reversed, start, adopted)
	if inner != lv {
		layers = withTerminals(lv, reversed, keep, layers)
	}
	count := 0
	for _, l := range layers {
		count = max(count, l+1)
	}
	layoutdbg.Decision(ctx, "layers_assigned",
		"phase", "rank", "module", "diago", "spec_ref", "S3",
		"layers", count,
		"span_longest_path", Span(lv, reversed, longest),
		"span", Span(lv, reversed, layers))
	return layers, adopted
}

// Adopt returns layers, the longest-path layering, with the previous
// layers adopted wherever the level still allows them (S13), and which
// nodes adopted theirs; layers is not modified. The previous layers are
// first aligned with layers: shifted alike, by the amount under which the
// most of them are adopted, among those that put one of them exactly on
// its node's layer in layers, the highest the node can take; on a tie,
// the one nearest zero, and of two as near the upward one. The nodes
// previous gives a layer (previous[v] >= 0) are visited by descending
// layer in layers, then in declaration order, so a node's successors are
// settled before it. A node adopts its aligned previous layer when every
// predecessor sits at least one layer above it and every successor can
// sit at least one layer below it: a successor with a previous layer
// where it now is, and one without as deep as its own successors let it
// go. Then every node is pushed below its predecessors, which moves only
// nodes without a previous layer, since adopting left them the room.
// Tighten renumbers the result. An aligned layer past the int range
// saturates at its end.
func Adopt(ctx context.Context, lv *lgraph.Level, reversed []bool, layers, previous []int) ([]int, []bool) {
	adj := adjacent(lv, reversed)
	var visit, shifts []int
	for v := range layers {
		if previous[v] >= 0 {
			visit = append(visit, v)
			shifts = append(shifts, layers[v]-previous[v])
		}
	}
	if visit == nil {
		return slices.Clone(layers), make([]bool, len(layers))
	}
	slices.SortStableFunc(visit, func(a, b int) int { return cmp.Compare(layers[b], layers[a]) })
	// Nearest zero first, the upward of two as near: the first to adopt
	// the most wins.
	slices.SortFunc(shifts, func(a, b int) int { return cmp.Or(cmp.Compare(abs(a), abs(b)), cmp.Compare(a, b)) })
	shifts = slices.Compact(shifts)
	shift, most := shifts[0], -1
	for _, s := range shifts {
		_, adopted := adoptAt(ctx, lv, adj, layers, previous, visit, s, false)
		if n := count(adopted); n > most {
			shift, most = s, n
		}
	}
	layoutdbg.Decision(ctx, "layers_aligned",
		"phase", "anchoring", "module", "diago", "spec_ref", "S13",
		"shift", shift, "adopted", most, "nodes", len(visit), "shifts", shifts)
	out, adopted := adoptAt(ctx, lv, adj, layers, previous, visit, shift, true)
	for range max(1, len(out)) {
		pushed := false
		for v := range out {
			for _, s := range adj.succs[v] {
				if out[s] <= out[v] {
					out[s], pushed = out[v]+1, true
					layoutdbg.Decision(ctx, "layer_pushed",
						"phase", "anchoring", "module", "diago", "spec_ref", "S13",
						"node", lv.Nodes[s].ID, "layer", out[s], "by", lv.Nodes[v].ID)
				}
			}
		}
		if !pushed {
			break
		}
	}
	return out, adopted
}

// adoptAt visits the nodes of visit in order and adopts each one's
// previous layer plus shift where the level allows it (Adopt), recording
// each adoption and rejection when record is true. It returns layers with
// the adopted layers, and which nodes adopted theirs.
func adoptAt(ctx context.Context, lv *lgraph.Level, adj adjacency, layers, previous, visit []int, shift int, record bool) ([]int, []bool) {
	out := slices.Clone(layers)
	adopted := make([]bool, len(out))
	// deepest is the deepest layer node v can end up on: its own when it
	// has a previous layer, visited already, since it lies deeper; else
	// one above the shallowest that its successors allow.
	deepest := make([]int, len(out))
	known := make([]bool, len(out))
	var reach func(v int) int
	reach = func(v int) int {
		if previous[v] >= 0 {
			return out[v]
		}
		if !known[v] {
			d := math.MaxInt
			for _, s := range adj.succs[v] {
				d = min(d, reach(s)-1)
			}
			deepest[v], known[v] = d, true
		}
		return deepest[v]
	}
	// The checks compare rather than subtract: a carrier's layers are
	// unbounded, so an aligned layer can lie near either end of the int
	// range, and reach is math.MaxInt below a new sink.
	for _, v := range visit {
		want, reason := shifted(previous[v], shift), ""
		for _, p := range adj.preds[v] {
			if want <= out[p] {
				reason = "predecessor:" + lv.Nodes[p].ID
				break
			}
		}
		for _, s := range adj.succs[v] {
			if reason == "" && reach(s) <= want {
				reason = "successor:" + lv.Nodes[s].ID
			}
		}
		if reason != "" {
			if record {
				layoutdbg.Decision(ctx, "layer_rejected",
					"phase", "anchoring", "module", "diago", "spec_ref", "S13",
					"node", lv.Nodes[v].ID, "previous", want, "fresh", layers[v], "reason", reason)
			}
			continue
		}
		out[v], adopted[v] = want, true
		if record {
			layoutdbg.Decision(ctx, "layer_adopted",
				"phase", "anchoring", "module", "diago", "spec_ref", "S13",
				"node", lv.Nodes[v].ID, "layer", want, "fresh", layers[v])
		}
	}
	return out, adopted
}

// shifted is layer l shifted by shift, saturated at the ends of the int
// range, so that a layer aligned past them stays beyond every other.
func shifted(l, shift int) int {
	switch {
	case shift > 0 && l > math.MaxInt-shift:
		return math.MaxInt
	case shift < 0 && l < math.MinInt-shift:
		return math.MinInt
	}
	return l + shift
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// count is how many of flags are set.
func count(flags []bool) int {
	n := 0
	for _, f := range flags {
		if f {
			n++
		}
	}
	return n
}

// adjacency lists each node's predecessors and successors along the
// oriented edges, in edge order, self-loops left out.
type adjacency struct{ preds, succs [][]int }

func adjacent(lv *lgraph.Level, reversed []bool) adjacency {
	a := adjacency{preds: make([][]int, len(lv.Nodes)), succs: make([][]int, len(lv.Nodes))}
	for e := range lv.Edges {
		if lv.SelfLoop(e) {
			continue
		}
		upper, lower := lv.Oriented(e, reversed)
		a.preds[lower] = append(a.preds[lower], upper)
		a.succs[upper] = append(a.succs[upper], lower)
	}
	return a
}

// LongestPath puts every source on layer 0 and every other node one layer
// below its lowest predecessor (Kahn's algorithm, in declaration order).
func LongestPath(lv *lgraph.Level, reversed []bool) []int {
	adj := adjacent(lv, reversed)
	layers := make([]int, len(lv.Nodes))
	remaining := make([]int, len(lv.Nodes))
	var queue []int
	for v := range lv.Nodes {
		remaining[v] = len(adj.preds[v])
		if remaining[v] == 0 {
			queue = append(queue, v)
		}
	}
	for h := 0; h < len(queue); h++ {
		v := queue[h]
		for _, p := range adj.preds[v] {
			layers[v] = max(layers[v], layers[p]+1)
		}
		for _, s := range adj.succs[v] {
			if remaining[s]--; remaining[s] == 0 {
				queue = append(queue, s)
			}
		}
	}
	return layers
}

// Tighten shortens the total edge span (S3). A node with more out-edges
// than in-edges moves down to one layer above its nearest successor, and a
// node with more in-edges than out-edges moves up to one layer below its
// nearest predecessor, each only when every incident edge keeps a length
// of at least 1. Sweeps visit the nodes in declaration order until nothing
// moves, at most one sweep per node; anchored nodes (S13) never move, and
// a nil anchored anchors none. The result is renumbered to a contiguous
// range from 0. layers is not modified.
func Tighten(lv *lgraph.Level, reversed []bool, layers []int, anchored []bool) []int {
	adj := adjacent(lv, reversed)
	out := slices.Clone(layers)
	down := func(v int) bool {
		succs, preds := adj.succs[v], adj.preds[v]
		if len(succs) == 0 || len(preds) >= len(succs) {
			return false
		}
		target := math.MaxInt
		for _, s := range succs {
			target = min(target, out[s]-1)
		}
		if target <= out[v] {
			return false
		}
		for _, p := range preds {
			if target-out[p] < 1 {
				return false
			}
		}
		out[v] = target
		return true
	}
	up := func(v int) bool {
		succs, preds := adj.succs[v], adj.preds[v]
		if len(preds) == 0 || len(succs) >= len(preds) {
			return false
		}
		target := math.MinInt
		for _, p := range preds {
			target = max(target, out[p]+1)
		}
		if target >= out[v] {
			return false
		}
		for _, s := range succs {
			if out[s]-target < 1 {
				return false
			}
		}
		out[v] = target
		return true
	}
	for range max(1, len(out)) {
		moved := false
		for v := range out {
			if anchored != nil && anchored[v] {
				continue
			}
			if down(v) || up(v) {
				moved = true
			}
		}
		if !moved {
			break
		}
	}

	used := slices.Clone(out)
	slices.Sort(used)
	used = slices.Compact(used)
	for v := range out {
		out[v], _ = slices.BinarySearch(used, out[v])
	}
	return out
}

// Span is the total edge span: the sum, over the edges that are not
// self-loops, of the layer distance from the upstream end to the
// downstream one.
func Span(lv *lgraph.Level, reversed []bool, layers []int) int {
	span := 0
	for e := range lv.Edges {
		if lv.SelfLoop(e) {
			continue
		}
		upper, lower := lv.Oriented(e, reversed)
		span += layers[lower] - layers[upper]
	}
	return span
}

// withoutTerminals returns lv without its terminals and their edges, and
// the edges it kept; lv itself, and a nil keep, when it has no terminal.
func withoutTerminals(lv *lgraph.Level) (*lgraph.Level, []bool) {
	has := false
	for _, n := range lv.Nodes {
		has = has || n.Terminal
	}
	if !has {
		return lv, nil
	}
	keep := make([]bool, len(lv.Edges))
	inner := &lgraph.Level{Nodes: lv.Nodes, Edges: make([]lgraph.Edge, 0, len(lv.Edges))}
	for e, ed := range lv.Edges {
		if lv.Nodes[ed.From].Terminal || lv.Nodes[ed.To].Terminal {
			// A self-loop stands in for the edge: ignored by every pass,
			// it keeps the edge indices of reversed valid.
			inner.Edges = append(inner.Edges, lgraph.Edge{ID: ed.ID, From: ed.From, To: ed.From})
			continue
		}
		keep[e] = true
		inner.Edges = append(inner.Edges, ed)
	}
	return inner, keep
}

// withTerminals puts lv's terminals on their rows around the other nodes'
// layers, which it first renumbers to a contiguous range from 0 without
// the terminals: a first row when some terminal's edge leaves it, and a
// last row below every other node for those whose edges enter them.
func withTerminals(lv *lgraph.Level, reversed []bool, keep []bool, layers []int) []int {
	out := slices.Clone(layers)
	var used []int
	for v, n := range lv.Nodes {
		if !n.Terminal {
			used = append(used, out[v])
		}
	}
	slices.Sort(used)
	used = slices.Compact(used)
	for v, n := range lv.Nodes {
		if !n.Terminal {
			out[v], _ = slices.BinarySearch(used, out[v])
		}
	}
	first := false
	for e := range lv.Edges {
		if keep[e] || lv.SelfLoop(e) {
			continue
		}
		if upper, _ := lv.Oriented(e, reversed); lv.Nodes[upper].Terminal {
			first = true
		}
	}
	shift, last := 0, 0
	if first {
		shift = 1
	}
	for v, n := range lv.Nodes {
		if !n.Terminal {
			out[v] += shift
			last = max(last, out[v]+1)
		}
	}
	for e := range lv.Edges {
		if keep[e] || lv.SelfLoop(e) {
			continue
		}
		upper, lower := lv.Oriented(e, reversed)
		if lv.Nodes[upper].Terminal {
			out[upper] = 0
		} else {
			out[lower] = last
		}
	}
	return out
}

// Room carries what S3 measures a row's extent with, from the Config
// (S14): the node gap between two of its vertices and a dummy's width.
type Room struct {
	Gap   float64 // the node gap
	Dummy float64 // a dummy's cross-axis extent
}

// eps absorbs float error in the room comparisons.
const eps = 1e-9

// Isolated places the isolated nodes of lv, those no edge touches, not
// even a self-loop, in the rows with the most room, so as not to enlarge
// the level by S3's estimate, which packs each row before placement
// spreads it (S3). layers is
// the level's layering (AssignKept), kept marks the nodes that adopted a
// previous layer (S13), which keep it; nil marks none. Each other isolated
// node, in declaration order, moves to the row with the most room where
// it fits: a row is a layer holding a node that is neither a terminal nor
// isolated, or an isolated node placed already; its extent is its nodes'
// cross-axis extents and room.Dummy for each edge passing it, with
// room.Gap between each two; its room is the widest row's extent minus
// its own. A node fits a row whose room is at least its extent plus the
// gap and whose tallest node is at least as tall; of equal rooms the
// earliest row wins, and a node that fits none keeps its layer. The
// layers are then renumbered to a contiguous range from 0. layers is not
// modified.
func Isolated(ctx context.Context, lv *lgraph.Level, reversed []bool, layers []int, kept []bool, room Room) []int {
	touched := make([]bool, len(lv.Nodes))
	for _, ed := range lv.Edges {
		touched[ed.From], touched[ed.To] = true, true
	}
	if !slices.Contains(touched, false) {
		return layers
	}
	out := slices.Clone(layers)
	count := 0
	for _, l := range out {
		count = max(count, l+1)
	}
	r := rows{extent: make([]float64, count), vertices: make([]int, count), tallest: make([]float64, count), row: make([]bool, count), gap: room.Gap}
	placed := func(v int) bool { return touched[v] || (kept != nil && kept[v]) }
	for v, nd := range lv.Nodes {
		if placed(v) && !nd.Terminal {
			r.add(out[v], nd.W, nd.H, true)
		}
	}
	for e := range lv.Edges {
		if lv.SelfLoop(e) {
			continue
		}
		upper, lower := lv.Oriented(e, reversed)
		for l := out[upper] + 1; l < out[lower]; l++ {
			r.add(l, room.Dummy, 0, false)
		}
	}
	for v, nd := range lv.Nodes {
		if placed(v) {
			continue
		}
		best, most := -1, 0.0
		for l := range count {
			spare := r.widest - r.extent[l]
			if r.row[l] && spare >= nd.W+room.Gap-eps && r.tallest[l] >= nd.H-eps && (best < 0 || spare > most+eps) {
				best, most = l, spare
			}
		}
		if best >= 0 {
			out[v] = best
		}
		r.add(out[v], nd.W, nd.H, true)
		layoutdbg.Decision(ctx, "isolated_placed",
			"phase", "rank", "module", "diago", "spec_ref", "S3",
			"node", nd.ID, "layer", out[v], "fits", best >= 0, "room", most)
	}
	used := slices.Clone(out)
	slices.Sort(used)
	used = slices.Compact(used)
	for v := range out {
		out[v], _ = slices.BinarySearch(used, out[v])
	}
	return out
}

// rows tracks each layer's extent for Isolated: its vertices' cross-axis
// extents with the gap between each two, its tallest node, whether it is a
// row, and the widest row's extent.
type rows struct {
	extent   []float64
	vertices []int
	tallest  []float64
	row      []bool
	widest   float64
	gap      float64
}

// add counts a vertex w wide and h tall on layer l, a node when node is
// true (a node makes its layer a row), else a dummy.
func (r *rows) add(l int, w, h float64, node bool) {
	if r.vertices[l] > 0 {
		r.extent[l] += r.gap
	}
	r.extent[l] += w
	r.vertices[l]++
	r.tallest[l] = max(r.tallest[l], h)
	r.row[l] = r.row[l] || node
	if r.row[l] {
		r.widest = max(r.widest, r.extent[l])
	}
}
