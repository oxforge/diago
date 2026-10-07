package schema

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fieldsOf(advs []Advisory) []string {
	out := make([]string, len(advs))
	for i, a := range advs {
		out[i] = a.Field
	}
	return out
}

func TestUnknownFields_NestedAndArrays(t *testing.T) {
	spec := `{"type":"flow","colour":"red","nodes":[{"id":"a","label":"A","colour":"red"},{"id":"b","label":"B","shpae":"rect"}],
	  "edges":[{"from":"a","to":"b","lable":"x"}],"groups":[{"id":"g","label":"G","contains":["a"],"extra":1}]}`
	advs, err := unknownFields([]byte(spec), reflect.TypeOf(FlowSpec{}), nil)
	require.NoError(t, err)
	SortAdvisories(advs)
	assert.Equal(t, []string{"colour", "edges[0].lable", "groups[0].extra", "nodes[0].colour", "nodes[1].shpae"}, fieldsOf(advs))
	for _, a := range advs {
		assert.Equal(t, RuleUnknownField, a.Rule)
	}
	byField := map[string]string{}
	for _, a := range advs {
		byField[a.Field] = a.Message
	}
	assert.Equal(t, `unknown key "colour" is ignored; did you mean "color"?`, byField["nodes[0].colour"])
	assert.Equal(t, `unknown key "shpae" is ignored; did you mean "shape"?`, byField["nodes[1].shpae"])
	assert.Equal(t, `unknown key "lable" is ignored; did you mean "label"?`, byField["edges[0].lable"])
	assert.Equal(t, `unknown key "extra" is ignored`, byField["groups[0].extra"])
	assert.Equal(t, `unknown key "colour" is ignored`, byField["colour"], "no top-level key is within distance 2 of colour")
}

func TestUnknownFields_EnvelopeAndCase(t *testing.T) {
	spec := `{"type":"flow","format":"png","theme":"dark","store":false,"Nodes":[{"ID":"a","Label":"A"}],"edges":[],"ignore":["whatever-not-a-key"]}`
	advs, err := unknownFields([]byte(spec), reflect.TypeOf(FlowSpec{}), nil)
	require.NoError(t, err)
	assert.Empty(t, advs, "envelope keys are allowed at the top level, keys match case-insensitively like encoding/json, ignore entries are values")

	seq := `{"type":"sequence","format":"svg","actors":[{"id":"a","label":"A"}],"interactions":[{"from":"a","to":"a","lbl":"x"}],
	  "fragments":[{"type":"alt","over":["a"],"sections":[{"label":"l","start":0,"end":0,"finish":0}]}]}`
	advs, err = unknownFields([]byte(seq), reflect.TypeOf(SequenceSpec{}), nil)
	require.NoError(t, err)
	SortAdvisories(advs)
	assert.Equal(t, []string{"fragments[0].sections[0].finish", "interactions[0].lbl"}, fieldsOf(advs))
}

func TestUnknownFields_NonObjectShapesDoNotPanic(t *testing.T) {
	for _, spec := range []string{`[]`, `"x"`, `{"nodes":"notalist"}`, `{"nodes":[1,2]}`, `{"nodes":[{"id":["x"]}]}`} {
		_, err := unknownFields([]byte(spec), reflect.TypeOf(FlowSpec{}), nil)
		assert.NoError(t, err, spec)
	}
	_, err := unknownFields([]byte(`{`), reflect.TypeOf(FlowSpec{}), nil)
	assert.Error(t, err)
}

func TestSuggest_RelativeDistanceGate(t *testing.T) {
	// A short unknown key can be within the flat edit-distance cap of a known
	// key while still being a bad rename (the edit rewrites most or all of
	// the string). Those must report no suggestion at all.
	nodeNames := jsonFields(reflect.TypeOf(NodeSpec{})).names
	assert.Equal(t, "", suggest("wt", nodeNames), `"wt" is distance 2 from "id" (len 2); too close to the string's own length to suggest`)
	assert.Equal(t, "", suggest("kind", nodeNames), `"kind" is distance 2 from "id" (len 2); same relative-distance gate`)

	// Longer keys at the same flat distance keep their suggestion.
	assert.Equal(t, "color", suggest("colour", nodeNames))
	assert.Equal(t, "shape", suggest("shpae", nodeNames))
	edgeNames := jsonFields(reflect.TypeOf(EdgeSpec{})).names
	assert.Equal(t, "label", suggest("lable", edgeNames))
	assert.Equal(t, "to", suggest("to2", edgeNames))
}

func TestUnknownFields_ShortKeyReportsNoSuggestion(t *testing.T) {
	spec := `{"type":"flow","nodes":[{"id":"a","label":"A","wt":1},{"id":"b","label":"B"}],"edges":[]}`
	advs, err := unknownFields([]byte(spec), reflect.TypeOf(FlowSpec{}), nil)
	require.NoError(t, err)
	require.Len(t, advs, 1)
	assert.Equal(t, `unknown key "wt" is ignored`, advs[0].Message, "no suggestion: distance 2 from \"id\" is not less than id's own length")
}

func TestCheck_RemovedFields(t *testing.T) {
	const nodes = `"nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":[{"from":"a","to":"b","style":"dashed"}]`
	tests := []struct {
		name string
		spec string
		want []Advisory
	}{
		{
			name: "flow style, hints, alignment",
			spec: `{"type":"flow","style":"neutral","alignment":"balanced","hints":[],` + nodes + `}`,
			want: []Advisory{
				{Rule: RuleRemovedField, Field: "alignment", Message: "alignment was removed; the field is ignored"},
				{Rule: RuleRemovedField, Field: "hints", Message: "layout hints were removed; the field is ignored"},
				{Rule: RuleRemovedField, Field: "style", Message: "diago is orthogonal-only; the field is ignored"},
			},
		},
		{
			name: "class style",
			spec: `{"type":"class","style":"lawful","classes":[{"id":"a","label":"A"}]}`,
			want: []Advisory{
				{Rule: RuleRemovedField, Field: "style", Message: "diago is orthogonal-only; the field is ignored"},
			},
		},
		{
			name: "silenced through ignore",
			spec: `{"type":"flow","style":"lawful","ignore":["removed-field"],` + nodes + `}`,
			want: nil,
		},
		{
			name: "sequence never had a style: still unknown-field",
			spec: `{"type":"sequence","style":"lawful","actors":[{"id":"a","label":"A"}],"interactions":[]}`,
			want: []Advisory{
				// "style" is edit-distance 2 from SequenceSpec's own "type" field
				// (pre-existing suggest() behavior, unrelated to removed-field),
				// so the generic unknown-field path attaches its usual
				// "did you mean" suggestion.
				{Rule: RuleUnknownField, Field: "style", Message: `unknown key "style" is ignored; did you mean "type"?`},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Check([]byte(tt.spec), CheckOptions{})
			require.NoError(t, err)
			var filtered []Advisory
			for _, a := range got {
				if a.Rule == RuleRemovedField || a.Rule == RuleUnknownField {
					filtered = append(filtered, a)
				}
			}
			assert.Equal(t, tt.want, filtered)
		})
	}
}

func TestLevenshtein(t *testing.T) {
	assert.Equal(t, 0, levenshtein("color", "color"))
	assert.Equal(t, 1, levenshtein("colour", "color"))
	assert.Equal(t, 2, levenshtein("shpae", "shape"))
	assert.Equal(t, 3, levenshtein("abc", ""))
	assert.Equal(t, 1, levenshtein("é", "e"))
}
