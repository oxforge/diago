package schema

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getDefProperties extracts properties from a JSON Schema that uses $defs/$ref pattern.
func getDefProperties(t *testing.T, raw map[string]any, defName string) map[string]any {
	t.Helper()
	defs, ok := raw["$defs"].(map[string]any)
	require.True(t, ok, "$defs should exist")
	def, ok := defs[defName].(map[string]any)
	require.True(t, ok, "%s definition should exist", defName)
	props, ok := def["properties"].(map[string]any)
	require.True(t, ok, "%s should have properties", defName)
	return props
}

func TestGenerateFlowSchema(t *testing.T) {
	data, err := GenerateJSONSchema("flow")
	require.NoError(t, err)

	// Must be valid JSON.
	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))

	// Must have $defs with FlowSpec properties.
	props := getDefProperties(t, raw, "FlowSpec")
	assert.Contains(t, props, "type")
	assert.Contains(t, props, "nodes")
	assert.Contains(t, props, "edges")
	assert.Contains(t, props, "direction")
	assert.Contains(t, props, "theme")

	edge := getDefProperties(t, raw, "EdgeSpec")
	require.Contains(t, edge, "flat")
	flat := edge["flat"].(map[string]any)
	assert.Equal(t, "boolean", flat["type"])
	desc, _ := flat["description"].(string)
	assert.Contains(t, desc, "does not imply order")
	assert.Contains(t, desc, "self-loop")
	assert.NotContains(t, desc, `\`, "the escaped commas must not leak into the generated text")
}

func TestGenerateSequenceSchema(t *testing.T) {
	data, err := GenerateJSONSchema("sequence")
	require.NoError(t, err)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))

	props := getDefProperties(t, raw, "SequenceSpec")
	assert.Contains(t, props, "type")
	assert.Contains(t, props, "actors")
	assert.Contains(t, props, "interactions")
}

func TestGenerateUnknownSchema(t *testing.T) {
	_, err := GenerateJSONSchema("unknown")
	assert.Error(t, err)
}

func TestGenerateClassSchema(t *testing.T) {
	data, err := GenerateJSONSchema("class")
	require.NoError(t, err)
	var s map[string]any
	require.NoError(t, json.Unmarshal(data, &s))
	defs := s["$defs"].(map[string]any)
	props := defs["ClassSpec"].(map[string]any)["properties"].(map[string]any)
	for _, k := range []string{"type", "theme", "classes", "relations", "packages", "legend", "legend_labels", "ignore"} {
		assert.Contains(t, props, k)
	}
	rel := defs["RelationSpec"].(map[string]any)["properties"].(map[string]any)
	assert.Contains(t, rel["kind"].(map[string]any)["enum"], "composition")
	// The jsonschema tag parser splits on commas, so a description containing
	// one is truncated in the generated file. These two carry the load-bearing
	// tail of their sentence; keep every description comma-free.
	assert.Contains(t, rel["kind"].(map[string]any)["description"], "subtype")
	assert.Contains(t, rel["directed"].(map[string]any)["description"], "adorned")
}
