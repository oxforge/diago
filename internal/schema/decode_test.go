package schema

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiagramType(t *testing.T) {
	for _, tc := range []struct{ in, kind, field, msg string }{
		{`{"type":"flow"}`, "flow", "", ""},
		{`{"type":"class"}`, "class", "", ""},
		{`{"type":"sequence"}`, "sequence", "", ""},
		{`not json`, "", "", "invalid JSON"},
		{``, "", "", "invalid JSON"},
		{`[1]`, "", "", "must be an object, got array"},
		{`{"nodes":[]}`, "", "type", "must be given"},
		{`{"type":"bogus"}`, "", "type", `unknown diagram type "bogus"`},
		{`{"type":7}`, "", "type", "must be a string, got number"},
	} {
		kind, err := DiagramType([]byte(tc.in))
		if tc.kind != "" {
			require.NoError(t, err, tc.in)
			assert.Equal(t, tc.kind, kind)
			continue
		}
		var ve ValidationErrors
		require.True(t, errors.As(err, &ve), "%q: a validation error, got %v", tc.in, err)
		require.Len(t, ve, 1)
		assert.Equal(t, tc.field, ve[0].Field, tc.in)
		assert.Contains(t, ve[0].Message, tc.msg, tc.in)
	}
}

func TestParse_AWronglyTypedFieldIsAValidationError(t *testing.T) {
	flow := func(b []byte) error { _, err := ParseFlow(b); return err }
	class := func(b []byte) error { _, err := ParseClass(b); return err }
	seq := func(b []byte) error { _, err := ParseSequence(b); return err }
	for _, tc := range []struct {
		name, spec, field, msg string
		parse                  func([]byte) error
	}{
		{"bool", `{"type":"flow","nodes":[{"id":"a","label":"A"}],"edges":[{"from":"a","to":"a"},{"from":"a","to":"a","flat":"yes"}]}`, "edges[1].flat", "must be a boolean, got string", flow},
		{"string", `{"type":"flow","nodes":[{"id":1,"label":"A"}],"edges":[]}`, "nodes[0].id", "must be a string, got number", flow},
		{"integer", `{"type":"sequence","actors":[{"id":"a","label":"A"}],"interactions":[{"from":"a","to":"a"}],"fragments":[{"type":"loop","over":["a"],"sections":[{"start":0,"end":1e20}]}]}`, "fragments[0].sections[0].end", "must be an integer, got number 1e20", seq},
		{"map value", `{"type":"class","classes":[{"id":"a"}],"legend_labels":{"inheritance":1}}`, "legend_labels.inheritance", "must be a string, got number", class},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var ve ValidationErrors
			require.True(t, errors.As(tc.parse([]byte(tc.spec)), &ve))
			require.Len(t, ve, 1)
			assert.Equal(t, tc.field, ve[0].Field)
			assert.Equal(t, tc.msg, ve[0].Message)
		})
	}
}
