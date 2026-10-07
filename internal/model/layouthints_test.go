package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLayoutHints_JSONShapeIsStable(t *testing.T) {
	h := NewLayoutHints()
	s := h.Scope("")
	s.Layers["b"], s.Layers["a"] = 1, 0
	s.Order["b"], s.Order["a"] = 0, 0
	h.Scope("g1").Layers["c"] = 0
	h.Reversed = append(h.Reversed, "b->a#0")
	b, err := json.Marshal(h)
	require.NoError(t, err)
	// json.Marshal HTML-escapes '<', '>', '&' unconditionally (encOpts.escapeHTML
	// is hardwired true at the top-level Marshal call, and encoding/json's
	// marshalerEncoder re-runs appendCompact with that same setting over any
	// nested Marshaler's output, so a per-type MarshalJSON cannot opt out of
	// it here). "\u003e" is the standard-library's escaped ">" and decodes
	// back to the same edge id; only an Encoder with SetEscapeHTML(false)
	// (the choice available to a caller embedding this carrier, e.g. Task 9's
	// SVG metadata writer) produces the literal "->" byte-for-byte.
	assert.Equal(t, `{"version":1,"scopes":{"":{"layers":{"a":0,"b":1},"order":{"a":0,"b":0}},"g1":{"layers":{"c":0},"order":{}}},"reversed":["b-\u003ea#0"]}`, string(b))
}

func TestLayoutHints_RankedIsOmittedWhenEmpty(t *testing.T) {
	// A carrier without fallbacks is byte-identical to one written before
	// ranked existed (C18); one with fallbacks lists them last.
	h := NewLayoutHints()
	h.Ranked = []string{}
	b, err := json.Marshal(h)
	require.NoError(t, err)
	assert.Equal(t, `{"version":1,"scopes":{},"reversed":[]}`, string(b))
	h.Ranked = []string{"a_b", "c_d"}
	b, err = json.Marshal(h)
	require.NoError(t, err)
	assert.Equal(t, `{"version":1,"scopes":{},"reversed":[],"ranked":["a_b","c_d"]}`, string(b))
	var back LayoutHints
	require.NoError(t, json.Unmarshal(b, &back))
	assert.Equal(t, h.Ranked, back.Ranked)
}

func TestLayoutHints_EmptyReversedIsArray(t *testing.T) {
	b, _ := json.Marshal(NewLayoutHints())
	assert.Equal(t, `{"version":1,"scopes":{},"reversed":[]}`, string(b))
}

func TestLayoutHints_Validate(t *testing.T) {
	assert.NoError(t, NewLayoutHints().Validate())
	assert.Error(t, (&LayoutHints{Version: 2, Scopes: map[string]ScopeHints{}}).Validate())
	assert.Error(t, (&LayoutHints{Version: 1}).Validate(), "nil scopes")
	var h LayoutHints
	require.NoError(t, json.Unmarshal([]byte(`{"version":1,"scopes":{"":{"layers":{"a":0}}},"reversed":null}`), &h))
	assert.NoError(t, h.Validate(), "absent order map and null reversed are tolerated")
	assert.NotNil(t, h.Scope("").Order, "Scope backfills nil maps")
}
