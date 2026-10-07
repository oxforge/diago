package pipeline

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/oxforge/diago/internal/model"
	textrender "github.com/oxforge/diago/internal/render/text"
	"github.com/oxforge/diago/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const vagueSpec = `{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":[{"from":"a","to":"b","label":"uses"}]}`

func advKeys(advs []schema.Advisory) []string {
	out := make([]string, len(advs))
	for i, a := range advs {
		out[i] = a.Rule + " " + a.Field
	}
	return out
}

func TestRenderWithOptions_AdvisorySink(t *testing.T) {
	var advs []schema.Advisory
	out, err := RenderWithOptions(context.Background(), []byte(vagueSpec), Options{Advisories: &advs})
	require.NoError(t, err)
	assert.Equal(t, []string{"vague-edge-label edges[0]"}, advKeys(advs))

	plain, err := RenderWithOptions(context.Background(), []byte(vagueSpec), Options{})
	require.NoError(t, err)
	assert.Equal(t, plain, out, "the sink never changes the bytes")

	// Not anchored: no edge-without-id. Anchored via Previous: it appears.
	assert.NotContains(t, advKeys(advs), "edge-without-id edges[0]")
	var carrier model.LayoutHints
	_, err = RenderWithOptions(context.Background(), []byte(vagueSpec), Options{LayoutHints: &carrier})
	require.NoError(t, err)
	var anchored []schema.Advisory
	_, err = RenderWithOptions(context.Background(), []byte(vagueSpec), Options{Previous: &carrier, Advisories: &anchored})
	require.NoError(t, err)
	assert.Contains(t, advKeys(anchored), "edge-without-id edges[0]")

	// A failed render leaves the sink untouched.
	untouched := []schema.Advisory{{Rule: "sentinel"}}
	_, err = RenderWithOptions(context.Background(), []byte(`{"type":"flow","nodes":[],"edges":[{"from":"x","to":"y"}]}`), Options{Advisories: &untouched})
	require.Error(t, err)
	assert.Equal(t, "sentinel", untouched[0].Rule)
}

func TestRenderWithOptions_TextLabelDropped(t *testing.T) {
	// four labelled edges from a to b and two back, laid out DOWN, are
	// known to drop the "four" label in text mode: S10 leaves it
	// unresolved, and the text renderer finds no room for it on its own
	// ladder. (cicd-pipeline DOWN dropped "on failure" until a text label
	// could leave a group too narrow for it, S10.) This end-to-end check
	// rests on a layout that drops a label; the wiring itself is pinned on
	// hand-built input by TestCheckAdvisories_DroppedLabels here and
	// TestRender_LabelThatFitsNowhereIsReported in internal/render/text.
	data := []byte(`{"type":"flow","direction":"DOWN","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],
		"edges":[{"from":"a","to":"b","label":"one"},{"from":"a","to":"b","label":"two"},{"from":"a","to":"b","label":"three"},
		{"from":"a","to":"b","label":"four"},{"from":"b","to":"a","label":"five"},{"from":"b","to":"a","label":"six"}]}`)
	var advs []schema.Advisory
	var warnings []string
	_, err := RenderWithOptions(context.Background(), data, Options{Format: "text", Advisories: &advs, Warnings: &warnings})
	require.NoError(t, err)
	var dropped []schema.Advisory
	for _, a := range advs {
		if a.Rule == schema.RuleTextLabelDropped {
			dropped = append(dropped, a)
		}
	}
	require.NotEmpty(t, dropped, "this spec drops a label in text mode; if the text profile improved, pick another spec that still drops one")
	assert.Equal(t, "", dropped[0].Field)
	assert.Regexp(t, `^label "[^"]+" on edge [^ ]+->[^ ]+ \([^)]+\) could not be placed in text art$`, dropped[0].Message)
	assert.Empty(t, warnings, "dropped labels are advisories now, not string warnings")

	// The spec's ignore list silences it.
	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))
	m["ignore"] = []string{"text-label-dropped"}
	ignored, err := json.Marshal(m)
	require.NoError(t, err)
	advs = nil
	_, err = RenderWithOptions(context.Background(), ignored, Options{Format: "text", Advisories: &advs})
	require.NoError(t, err)
	assert.NotContains(t, advKeys(advs), "text-label-dropped ")
}

func TestRenderDiff_AdvisorySink(t *testing.T) {
	before := []byte(`{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":[{"from":"a","to":"b","label":"x"}]}`)
	after := []byte(`{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"},{"id":"c","label":"C"}],"edges":[{"id":"e","from":"a","to":"b","label":"uses"}]}`)
	var advs []schema.Advisory
	_, err := RenderDiff(context.Background(), before, after, DiffOptions{Format: "text", Advisories: &advs})
	require.NoError(t, err)
	assert.Equal(t, []string{"edge-without-id before.edges[0]", "isolated-node after.nodes[2]", "vague-edge-label after.edges[0]"}, advKeys(advs))

	// A whole-diagram finding gets the bare side as its field.
	big := `{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":[{"from":"a","to":"b"},{"from":"b","to":"a"}]}`
	advs = nil
	_, err = RenderDiff(context.Background(), []byte(big), []byte(big), DiffOptions{Advisories: &advs})
	require.NoError(t, err)
	assert.Equal(t, []string{"no-entry after", "no-entry before"}, advKeys(advs))
}

// blockedFlatSpec is P3's fallback fixture (flat_test.go's blocked graph):
// m sits directly between a and b, so every candidate route for the flat
// edge a->b (edges[2]) fails and the layout ranks it.
const blockedFlatSpec = `{"type":"flow","direction":"DOWN","nodes":[{"id":"a","label":"A"},{"id":"m","label":"M"},{"id":"b","label":"B"}],"edges":[{"from":"a","to":"m"},{"from":"m","to":"b"},{"from":"a","to":"b","flat":true}]}`

// cleanFlatSpec has nothing in the flat edge's way: the router keeps its
// straight route, and the layout never falls back.
const cleanFlatSpec = `{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":[{"from":"a","to":"b","flat":true}]}`

func TestRenderWithOptions_FlatEdgeRanked(t *testing.T) {
	var advs []schema.Advisory
	_, err := RenderWithOptions(context.Background(), []byte(blockedFlatSpec), Options{Advisories: &advs})
	require.NoError(t, err)
	require.Equal(t, []string{"flat-edge-ranked edges[2]"}, advKeys(advs))
	assert.Equal(t, "no clear side route; laid out as an ordinary edge", advs[0].Message)

	// The spec's ignore list silences it.
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(blockedFlatSpec), &m))
	m["ignore"] = []string{"flat-edge-ranked"}
	ignored, err := json.Marshal(m)
	require.NoError(t, err)
	advs = nil
	_, err = RenderWithOptions(context.Background(), ignored, Options{Advisories: &advs})
	require.NoError(t, err)
	assert.Empty(t, advs)

	// diago check (pre-layout, schema.Check alone) never reports it: the
	// layout that ranks the edge has not run yet.
	checked, err := schema.Check([]byte(blockedFlatSpec), schema.CheckOptions{})
	require.NoError(t, err)
	assert.NotContains(t, advKeys(checked), "flat-edge-ranked edges[2]")

	// A flat edge that routes cleanly reports nothing.
	advs = nil
	_, err = RenderWithOptions(context.Background(), []byte(cleanFlatSpec), Options{Advisories: &advs})
	require.NoError(t, err)
	assert.Empty(t, advs)

	// Every render format warns: svg (above), text, drawio, excalidraw.
	for _, format := range []string{"text", "drawio", "excalidraw"} {
		advs = nil
		_, err = RenderWithOptions(context.Background(), []byte(blockedFlatSpec), Options{Format: format, Advisories: &advs})
		require.NoError(t, err, format)
		assert.Equal(t, []string{"flat-edge-ranked edges[2]"}, advKeys(advs), format)
	}

	// An unknown ignore name still fails validation.
	require.NoError(t, json.Unmarshal([]byte(blockedFlatSpec), &m))
	m["ignore"] = []string{"bogus-rule"}
	bad, err := json.Marshal(m)
	require.NoError(t, err)
	_, err = RenderWithOptions(context.Background(), bad, Options{})
	require.Error(t, err)
}

// TestRenderWithOptions_FlatEdgeRankedAnchored: since P3b an anchored
// render keeps the previous layout's fallbacks (the carrier's ranked, S13),
// so the warning fires again on a render anchored on a carrier that ranked
// the edge, even though nothing here would make a fresh layout re-decide.
// The edge is kept without ever trying its side route again (keepRanked
// runs before the router), so the message says "kept", not "no clear side
// route" (finding 1, plan 6b's fix brief).
func TestRenderWithOptions_FlatEdgeRankedAnchored(t *testing.T) {
	var carrier model.LayoutHints
	_, err := RenderWithOptions(context.Background(), []byte(blockedFlatSpec), Options{LayoutHints: &carrier})
	require.NoError(t, err)
	require.Equal(t, []string{"a->b#0"}, carrier.Ranked, "the fresh layout ranked the edge")

	var advs []schema.Advisory
	_, err = RenderWithOptions(context.Background(), []byte(blockedFlatSpec), Options{Previous: &carrier, Advisories: &advs})
	require.NoError(t, err)
	require.Equal(t, []string{"flat-edge-ranked edges[2]"}, advKeys(advs))
	assert.Equal(t, "kept as an ordinary edge from the previous layout (-previous); a render without it tries a side route again", advs[0].Message)
}

// TestRenderWithOptions_FlatEdgeRankedKept: the reproduction from finding 1's
// fix brief. blockedFlatSpec falls back fresh; cleanFlatSpec is the same
// graph freed (no middle node): laid out alone it routes cleanly
// (TestRenderWithOptions_FlatEdgeRanked), but anchored on a carrier that
// ranked the edge (S13, keepRanked), the edge is kept ranked without its
// side route ever being tried this render, so the warning must say "kept",
// not "no clear side route" (which would send an agent editing v2's JSON
// forever, since nothing in v2 can clear a kept fallback).
func TestRenderWithOptions_FlatEdgeRankedKept(t *testing.T) {
	var carrier model.LayoutHints
	_, err := RenderWithOptions(context.Background(), []byte(blockedFlatSpec), Options{LayoutHints: &carrier})
	require.NoError(t, err)
	require.Equal(t, []string{"a->b#0"}, carrier.Ranked, "the blocked graph's fresh layout ranked the edge")

	var advs []schema.Advisory
	_, err = RenderWithOptions(context.Background(), []byte(cleanFlatSpec), Options{Previous: &carrier, Advisories: &advs})
	require.NoError(t, err)
	require.Equal(t, []string{"flat-edge-ranked edges[0]"}, advKeys(advs))
	assert.Equal(t, "kept as an ordinary edge from the previous layout (-previous); a render without it tries a side route again", advs[0].Message)
}

// TestRenderWithOptions_FlatEdgeRankedFreshWhileAnchored: an anchored
// render whose previous carrier does not list the flat edge tries its side
// route as usual; when that fails, the fallback is fresh this render (not
// kept), so the message stays the "no clear side route" one even though the
// render is anchored.
func TestRenderWithOptions_FlatEdgeRankedFreshWhileAnchored(t *testing.T) {
	var carrier model.LayoutHints
	_, err := RenderWithOptions(context.Background(), []byte(cleanFlatSpec), Options{LayoutHints: &carrier})
	require.NoError(t, err)
	require.Empty(t, carrier.Ranked, "the clean graph's fresh layout never falls back")

	var advs []schema.Advisory
	_, err = RenderWithOptions(context.Background(), []byte(blockedFlatSpec), Options{Previous: &carrier, Advisories: &advs})
	require.NoError(t, err)
	require.Equal(t, []string{"flat-edge-ranked edges[2]"}, advKeys(advs))
	assert.Equal(t, "no clear side route; laid out as an ordinary edge", advs[0].Message)
}

// TestRenderDiff_FlatEdgeRanked: the after spec's blocked flat edge is
// reported on the after side, matching diffAdvisories' other findings.
func TestRenderDiff_FlatEdgeRanked(t *testing.T) {
	before := []byte(`{"type":"flow","direction":"DOWN","nodes":[{"id":"a","label":"A"},{"id":"m","label":"M"},{"id":"b","label":"B"}],"edges":[{"from":"a","to":"m"},{"from":"m","to":"b"}]}`)
	var advs []schema.Advisory
	_, err := RenderDiff(context.Background(), before, []byte(blockedFlatSpec), DiffOptions{Advisories: &advs})
	require.NoError(t, err)
	assert.Contains(t, advKeys(advs), "flat-edge-ranked after.edges[2]")

	for _, a := range advs {
		if a.Rule == schema.RuleFlatEdgeRanked {
			assert.Equal(t, "no clear side route; laid out as an ordinary edge", a.Message)
		}
	}

	// The after spec's ignore list silences it, as it does for
	// text-label-dropped.
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(blockedFlatSpec), &m))
	m["ignore"] = []string{"flat-edge-ranked"}
	ignoredAfter, err := json.Marshal(m)
	require.NoError(t, err)
	advs = nil
	_, err = RenderDiff(context.Background(), before, ignoredAfter, DiffOptions{Advisories: &advs})
	require.NoError(t, err)
	assert.NotContains(t, advKeys(advs), "flat-edge-ranked after.edges[2]")
}

// TestRenderDiff_FlatEdgeRankedKept: a diff's union is anchored on the
// before spec's own fresh layout (layoutFlowDiff). When the before spec
// already falls back on its own (blockedFlatSpec, unchanged into after),
// the union's flat edge is kept ranked from that before layout without its
// side route being tried in the union layout, so the message names the
// before layout, not "no clear side route" (finding 1's diff wording).
func TestRenderDiff_FlatEdgeRankedKept(t *testing.T) {
	var advs []schema.Advisory
	_, err := RenderDiff(context.Background(), []byte(blockedFlatSpec), []byte(blockedFlatSpec), DiffOptions{Advisories: &advs})
	require.NoError(t, err)
	require.Contains(t, advKeys(advs), "flat-edge-ranked after.edges[2]")

	for _, a := range advs {
		if a.Rule == schema.RuleFlatEdgeRanked {
			assert.Equal(t, "kept as an ordinary edge from the before layout", a.Message)
		}
	}
}

// TestCheckAdvisories_DroppedLabels pins, on hand-built dropped labels,
// what checkAdvisories makes of the text renderer's report, apart from
// any layout that drops one: one text-label-dropped finding per dropped
// label or cardinality, on no field, in the sorted list, and none when
// the spec's ignore list names the rule.
func TestCheckAdvisories_DroppedLabels(t *testing.T) {
	spec := []byte(`{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":[{"from":"a","to":"b","label":"reads"}]}`)
	none, err := checkAdvisories(spec, schema.CheckOptions{}, nil, nil)
	require.NoError(t, err)
	dropped := []textrender.DroppedLabel{
		{ID: "a->b#0", From: "a", To: "b", Label: "reads"},
		{ID: "r1", From: "a", To: "b", Label: "0..*", End: "to"},
	}
	advs, err := checkAdvisories(spec, schema.CheckOptions{}, dropped, nil)
	require.NoError(t, err)
	var got []schema.Advisory
	for _, a := range advs {
		if a.Rule == schema.RuleTextLabelDropped {
			got = append(got, a)
		}
	}
	assert.Equal(t, []schema.Advisory{
		{Rule: schema.RuleTextLabelDropped, Message: `cardinality "0..*" at the to end of relation a->b (r1) could not be placed in text art`},
		{Rule: schema.RuleTextLabelDropped, Message: `label "reads" on edge a->b (a->b#0) could not be placed in text art`},
	}, got)
	assert.Len(t, advs, len(none)+2, "nothing else changes")

	var m map[string]any
	require.NoError(t, json.Unmarshal(spec, &m))
	m["ignore"] = []string{"text-label-dropped"}
	ignoring, err := json.Marshal(m)
	require.NoError(t, err)
	advs, err = checkAdvisories(ignoring, schema.CheckOptions{}, dropped, nil)
	require.NoError(t, err)
	assert.NotContains(t, advKeys(advs), "text-label-dropped ")
}

// TestFlatRankedDiffFields pins the field each ranked union edge names: an
// edge the after spec has names after.edges[i]; one only the before spec
// has, a removed edge, names before.edges[i], under its own id or under
// the __removed suffixes the union gave it to dodge a reused id
// (diff.BuildUnion), stripped to find it; an edge neither spec has names
// nothing; and an edge the layout did not rank names nothing either. Kept
// is the union edge's own id in the previous carrier's ranked list.
func TestFlatRankedDiffFields(t *testing.T) {
	edge := func(id string) model.Edge { return model.Edge{ID: id} }
	before := &model.Graph{Edges: []model.Edge{edge("gone"), edge("e"), edge("both")}}
	after := &model.Graph{Edges: []model.Edge{edge("e"), edge("both")}}
	pg := &model.PositionedGraph{Edges: []model.PositionedEdge{
		{ID: "e", FlatRanked: true},
		{ID: "both"},
		{ID: "gone", FlatRanked: true},
		{ID: "e__removed", FlatRanked: true},
		{ID: "stray", FlatRanked: true},
	}}
	prev := &model.LayoutHints{Ranked: []string{"e__removed", "gone"}}
	assert.Equal(t, []flatRankedField{
		{Field: "after.edges[0]"},
		{Field: "before.edges[0]", Kept: true},
		{Field: "before.edges[1]", Kept: true},
	}, flatRankedDiffFields(pg, before, after, prev))
	assert.Equal(t, []flatRankedField{
		{Field: "after.edges[0]"},
		{Field: "before.edges[0]"},
		{Field: "before.edges[1]"},
	}, flatRankedDiffFields(pg, before, after, nil), "no previous carrier keeps nothing")
	assert.Nil(t, flatRankedDiffFields(nil, before, after, prev))
}

func TestTextLabelDropped_Message(t *testing.T) {
	a := textLabelDropped(textrender.DroppedLabel{ID: "a->b#0", From: "a", To: "b", Label: "x"})
	assert.Equal(t, schema.RuleTextLabelDropped, a.Rule)
	assert.Equal(t, `label "x" on edge a->b (a->b#0) could not be placed in text art`, a.Message)
	a = textLabelDropped(textrender.DroppedLabel{From: "a", To: "b", Label: "x"})
	assert.Equal(t, `label "x" on edge a->b could not be placed in text art`, a.Message)
}

func TestPrefixAdvisories(t *testing.T) {
	got := prefixAdvisories([]schema.Advisory{{Rule: "r", Field: "nodes[0]"}, {Rule: "r"}}, "before")
	assert.Equal(t, "before.nodes[0]", got[0].Field)
	assert.Equal(t, "before", got[1].Field)
}
