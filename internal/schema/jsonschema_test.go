package schema

import (
	"encoding/json"
	"io/fs"
	"strings"
	"testing"

	"github.com/oxforge/diago/internal/model"
	"github.com/oxforge/diago/themes"
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

// TestJSONSchema_EnumsAreTheModels: the values the schema lists for a
// field are the ones diago accepts there, the model enum's and the
// built-in theme names, so a value added to the enum generator or a theme
// added to themes/ cannot be missing from the schema.
func TestJSONSchema_EnumsAreTheModels(t *testing.T) {
	files, err := fs.Glob(themes.FS, "*.json")
	require.NoError(t, err)
	var themeNames []string
	for _, f := range files {
		themeNames = append(themeNames, strings.TrimSuffix(f, ".json"))
	}
	for _, tc := range []struct {
		spec, def, prop string
		want            []string
	}{
		{"flow", "FlowSpec", "theme", themeNames},
		{"flow", "FlowSpec", "direction", model.DirectionValues()},
		{"flow", "NodeSpec", "shape", model.ShapeValues()},
		{"flow", "EdgeSpec", "style", model.EdgeStyleValues()},
		{"flow", "EdgeSpec", "direction", model.EdgeDirectionValues()},
		{"sequence", "SequenceSpec", "theme", themeNames},
		{"sequence", "InteractionSpec", "style", model.InteractionStyleValues()},
		{"sequence", "FragmentSpec", "type", model.FragmentTypeValues()},
		{"class", "ClassSpec", "theme", themeNames},
		{"class", "ClassSpec", "direction", model.DirectionValues()},
		{"class", "RelationSpec", "kind", model.RelationValues()},
	} {
		data, err := GenerateJSONSchema(tc.spec)
		require.NoError(t, err)
		var raw map[string]any
		require.NoError(t, json.Unmarshal(data, &raw))
		prop, ok := getDefProperties(t, raw, tc.def)[tc.prop].(map[string]any)
		require.True(t, ok, "%s.%s", tc.def, tc.prop)
		enum, _ := prop["enum"].([]any)
		var got []string
		for _, v := range enum {
			got = append(got, v.(string))
		}
		assert.ElementsMatch(t, tc.want, got, "%s %s.%s", tc.spec, tc.def, tc.prop)
	}
}
