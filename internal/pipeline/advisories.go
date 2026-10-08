package pipeline

import (
	"fmt"
	"strings"

	"github.com/oxforge/diago/internal/model"
	textrender "github.com/oxforge/diago/internal/render/text"
	"github.com/oxforge/diago/internal/schema"
)

// checkAdvisories runs schema.Check and appends the text renderer's dropped
// labels as text-label-dropped and the layout's ranked flat edges as
// flat-edge-ranked, honouring the spec's ignore list. diago check (pre-layout,
// schema.Check alone) never sees flatRanked: it runs before layout exists.
func checkAdvisories(input []byte, opts schema.CheckOptions, dropped []textrender.DroppedLabel, flatRanked []flatRankedField) ([]schema.Advisory, error) {
	advs, err := schema.Check(input, opts)
	if err != nil {
		return nil, fmt.Errorf("render: %w", err)
	}
	advs, err = appendDropped(advs, input, dropped)
	if err != nil {
		return nil, fmt.Errorf("render: %w", err)
	}
	advs, err = appendFlatRanked(advs, input, flatRanked, false)
	if err != nil {
		return nil, fmt.Errorf("render: %w", err)
	}
	schema.SortAdvisories(advs)
	return advs, nil
}

// appendDropped adds one text-label-dropped finding per dropped label
// unless the spec at data ignores the rule.
func appendDropped(advs []schema.Advisory, data []byte, dropped []textrender.DroppedLabel) ([]schema.Advisory, error) {
	if len(dropped) == 0 {
		return advs, nil
	}
	ignored, err := schema.Ignored(data)
	if err != nil {
		return nil, err
	}
	if ignored[schema.RuleTextLabelDropped] {
		return advs, nil
	}
	for _, d := range dropped {
		advs = append(advs, textLabelDropped(d))
	}
	return advs, nil
}

// flatRankedField pairs a flat-edge-ranked finding's spec field path with
// whether the layout kept the edge ranked from a previous carrier's Ranked
// (S13, keepRanked) rather than trying its side route this render: a kept
// edge never reaches the router (composed's settle loop skips an edge
// already ranked), so its warning must not claim "no clear side route".
type flatRankedField struct {
	Field string
	Kept  bool
}

// previousRanked is the set of edge ids prev's carrier lists as fallen back
// (model.LayoutHints.Ranked); empty when prev is nil. It mirrors the
// membership test internal/layout/layered/anchor.go's keepRanked applies at
// layout time, so a field's Kept here matches what the layout actually did.
func previousRanked(prev *model.LayoutHints) map[string]bool {
	if prev == nil {
		return nil
	}
	out := make(map[string]bool, len(prev.Ranked))
	for _, id := range prev.Ranked {
		out[id] = true
	}
	return out
}

// appendFlatRanked adds one flat-edge-ranked finding per entry in fields
// unless the spec at data ignores the rule. diffCtx selects the diff
// wording for a kept fallback.
func appendFlatRanked(advs []schema.Advisory, data []byte, fields []flatRankedField, diffCtx bool) ([]schema.Advisory, error) {
	if len(fields) == 0 {
		return advs, nil
	}
	ignored, err := schema.Ignored(data)
	if err != nil {
		return nil, err
	}
	if ignored[schema.RuleFlatEdgeRanked] {
		return advs, nil
	}
	for _, f := range fields {
		advs = append(advs, flatEdgeRanked(f.Field, f.Kept, diffCtx))
	}
	return advs, nil
}

// flatRankedFields returns the field path ("edges[i]") and kept status of
// every edge pg's layout ranked (model.PositionedEdge.FlatRanked). pg's
// edges are in the spec's edge order (nested.go's assemble, flat.go's
// nestedRanked), so i is the edge's index in the spec. prev is the carrier
// the render was anchored on (opts.Previous), or nil for a fresh render.
func flatRankedFields(pg *model.PositionedGraph, prev *model.LayoutHints) []flatRankedField {
	if pg == nil {
		return nil
	}
	kept := previousRanked(prev)
	var fields []flatRankedField
	for i, e := range pg.Edges {
		if e.FlatRanked {
			fields = append(fields, flatRankedField{Field: fmt.Sprintf("edges[%d]", i), Kept: kept[e.ID]})
		}
	}
	return fields
}

// flatEdgeRankedMessage is the flat-edge-ranked finding's message: a fresh
// fallback (the router tried this render and kept no route) keeps the
// original wording; a kept one (S13, keepRanked: the previous carrier
// listed it, so the router never tried) says so, worded for a render
// anchored with -previous or for a diff's union, anchored on the before
// layout.
func flatEdgeRankedMessage(kept, diffCtx bool) string {
	switch {
	case !kept:
		return "no clear side route; laid out as an ordinary edge"
	case diffCtx:
		return "kept as an ordinary edge from the before layout"
	default:
		return "kept as an ordinary edge from the previous layout (-previous); a render without it tries a side route again"
	}
}

func flatEdgeRanked(field string, kept, diffCtx bool) schema.Advisory {
	return schema.Advisory{
		Rule:    schema.RuleFlatEdgeRanked,
		Field:   field,
		Message: flatEdgeRankedMessage(kept, diffCtx),
	}
}

func textLabelDropped(d textrender.DroppedLabel) schema.Advisory {
	var msg string
	if d.End != "" {
		msg = fmt.Sprintf("cardinality %q at the %s end of relation %s->%s", d.Label, d.End, d.From, d.To)
	} else {
		msg = fmt.Sprintf("label %q on edge %s->%s", d.Label, d.From, d.To)
	}
	if d.ID != "" {
		msg += " (" + d.ID + ")"
	}
	return schema.Advisory{Rule: schema.RuleTextLabelDropped, Message: msg + " could not be placed in text art"}
}

// diffAdvisories checks both sides of a diff as anchored, keeps once a
// finding both sides raise alike (unrepeated), prefixes their fields with
// the side, and appends the union render's dropped labels and ranked flat
// edges under the after spec's ignore list. flatRanked is already
// field-shaped ("before.edges[i]" or "after.edges[i]",
// flatRankedDiffFields), so no further side prefixing is needed here.
func diffAdvisories(before, after []byte, dropped []textrender.DroppedLabel, flatRanked []flatRankedField) ([]schema.Advisory, error) {
	b, err := schema.Check(before, schema.CheckOptions{Anchored: true})
	if err != nil {
		return nil, fmt.Errorf("diff: before: %w", err)
	}
	a, err := schema.Check(after, schema.CheckOptions{Anchored: true})
	if err != nil {
		return nil, fmt.Errorf("diff: after: %w", err)
	}
	advs := append(prefixAdvisories(unrepeated(b, a), "before"), prefixAdvisories(a, "after")...)
	// Deliberately the after spec's ignore list only: the union render is
	// the after-shaped picture, so its ignore list is what governs it.
	advs, err = appendDropped(advs, after, dropped)
	if err != nil {
		return nil, fmt.Errorf("diff: %w", err)
	}
	advs, err = appendFlatRanked(advs, after, flatRanked, true)
	if err != nil {
		return nil, fmt.Errorf("diff: %w", err)
	}
	schema.SortAdvisories(advs)
	return advs, nil
}

// flatRankedDiffFields returns the diff field path (before.edges[i] or
// after.edges[i]) and kept status of every union edge the layout ranked
// (model.PositionedEdge.FlatRanked). The union's edges are after.Edges
// followed by the removed before-edges (diff.BuildUnion), so an edge
// present in after (added, changed or same) keeps its after id and names
// the after side; an edge present only in before (removed) may carry a
// "__removed" suffix the union gave it to dodge an id collision
// (diff.UnionGraph.Edges), stripped here to recover the before spec's id
// and name the before side. prev is the before spec's own fresh layout
// carrier, the anchor the union was laid out on (layoutFlowDiff's basePG);
// kept mirrors flatRankedFields, checked on the union edge's own id, the
// same id keepRanked checked at layout time.
func flatRankedDiffFields(pg *model.PositionedGraph, before, after *model.Graph, prev *model.LayoutHints) []flatRankedField {
	if pg == nil {
		return nil
	}
	kept := previousRanked(prev)
	afterIdx := edgeIndexByID(after.Edges)
	beforeIdx := edgeIndexByID(before.Edges)
	var fields []flatRankedField
	for _, e := range pg.Edges {
		if !e.FlatRanked {
			continue
		}
		k := kept[e.ID]
		if i, ok := afterIdx[e.ID]; ok {
			fields = append(fields, flatRankedField{Field: fmt.Sprintf("after.edges[%d]", i), Kept: k})
			continue
		}
		id := e.ID
		for {
			if i, ok := beforeIdx[id]; ok {
				fields = append(fields, flatRankedField{Field: fmt.Sprintf("before.edges[%d]", i), Kept: k})
				break
			}
			trimmed := strings.TrimSuffix(id, "__removed")
			if trimmed == id {
				break // the union edge id is unknown to either spec; should not happen
			}
			id = trimmed
		}
	}
	return fields
}

// edgeIndexByID maps every edge's id to its index in edges.
func edgeIndexByID(edges []model.Edge) map[string]int {
	idx := make(map[string]int, len(edges))
	for i, e := range edges {
		idx[e.ID] = i
	}
	return idx
}

// prefixAdvisories copies advs with side prepended to every field
// ("before.nodes[2]"); an empty field becomes the bare side.
// unrepeated returns the before findings the after side does not repeat
// alike, with the same rule, field and message: a finding both sides raise
// is one finding, reported on the after side, the version drawn. The field
// takes part because a message need not name its element (unknown-field's
// does not), so two elements' findings may read alike.
func unrepeated(before, after []schema.Advisory) []schema.Advisory {
	repeated := make(map[schema.Advisory]bool, len(after))
	for _, a := range after {
		repeated[a] = true
	}
	var out []schema.Advisory
	for _, b := range before {
		if !repeated[b] {
			out = append(out, b)
		}
	}
	return out
}

func prefixAdvisories(advs []schema.Advisory, side string) []schema.Advisory {
	out := make([]schema.Advisory, len(advs))
	for i, a := range advs {
		out[i] = a
		if a.Field == "" {
			out[i].Field = side
		} else {
			out[i].Field = side + "." + a.Field
		}
	}
	return out
}
