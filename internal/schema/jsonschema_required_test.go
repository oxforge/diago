package schema

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// requiredSamples are specs that hold every object kind of their type,
// most with their optional properties set, so the walk below meets every
// property the schema describes.
var requiredSamples = map[string]string{
	"flow": `{"type":"flow","title":"T","theme":"default","direction":"DOWN",
		"nodes":[{"id":"a","label":"A","shape":"rect","color":"red"},{"id":"b","label":"B"},{"id":"c","label":"C"}],
		"edges":[{"id":"e1","from":"a","to":"b","label":"x","style":"dashed","direction":"both","color":"blue","flat":false}],
		"groups":[{"id":"g","label":"G","contains":["b","h"],"color":"gray"},{"id":"h","label":"H","contains":["c"]}],
		"ignore":["isolated-node"]}`,
	"sequence": `{"type":"sequence","title":"T","theme":"default","activations":true,
		"actors":[{"id":"a","label":"A","color":"red"},{"id":"b","label":"B"}],
		"interactions":[{"from":"a","to":"b","label":"hi","style":"solid","color":"blue"},{"from":"b","to":"a","label":"ok"}],
		"fragments":[{"type":"alt","over":["a","b"],"sections":[{"label":"yes","start":0,"end":0},{"label":"no","start":1,"end":1}]}],
		"ignore":["vague-edge-label"]}`,
	"class": `{"type":"class","title":"T","theme":"default","direction":"DOWN",
		"classes":[{"id":"o","label":"Order","stereotype":"entity","type_params":["T"],"color":"blue",
			"attributes":[{"visibility":"-","text":"id: string"}],
			"methods":[{"visibility":"+","text":"total(): Money","static":true,"abstract":false}]},
			{"id":"i","label":"Item"}],
		"relations":[{"id":"r1","from":"o","to":"i","kind":"composition","label":"has","from_card":"1","to_card":"*","color":"red"}],
		"packages":[{"id":"p","label":"P","contains":["o","i"],"color":"gray"}],
		"legend":true,"legend_labels":{"composition":"has"},"ignore":["god-class"]}`,
}

// instance is an object of the sample spec, the JSON path to it and the
// schema definition that describes it.
type instance struct {
	path []any // string keys and int indexes
	def  string
}

// fieldPath renders path the way validation errors name fields:
// nodes[0].label.
func fieldPath(path []any) string {
	var b strings.Builder
	for _, p := range path {
		switch p := p.(type) {
		case int:
			fmt.Fprintf(&b, "[%d]", p)
		case string:
			if b.Len() > 0 {
				b.WriteByte('.')
			}
			b.WriteString(p)
		}
	}
	return b.String()
}

// refName returns the definition a property schema points at, directly
// or through its items, or "".
func refName(prop map[string]any) string {
	if ref, ok := prop["$ref"].(string); ok {
		return strings.TrimPrefix(ref, "#/$defs/")
	}
	if items, ok := prop["items"].(map[string]any); ok {
		return refName(items)
	}
	return ""
}

// walk collects every object of node that the schema describes, from
// definition def down.
func walk(defs map[string]any, def string, node any, path []any, out *[]instance) {
	obj, ok := node.(map[string]any)
	if !ok {
		return
	}
	*out = append(*out, instance{path: path, def: def})
	props, _ := defs[def].(map[string]any)["properties"].(map[string]any)
	for key, val := range obj {
		prop, ok := props[key].(map[string]any)
		if !ok {
			continue
		}
		ref := refName(prop)
		if ref == "" {
			continue
		}
		child := append(append([]any{}, path...), key)
		if arr, ok := val.([]any); ok {
			for i, el := range arr {
				walk(defs, ref, el, append(append([]any{}, child...), i), out)
			}
			continue
		}
		walk(defs, ref, val, child, out)
	}
}

// without returns a deep copy of doc with key deleted from the object at
// path.
func without(t *testing.T, doc string, path []any, key string) []byte {
	t.Helper()
	var v any
	require.NoError(t, json.Unmarshal([]byte(doc), &v))
	node := v
	for _, p := range path {
		switch p := p.(type) {
		case int:
			node = node.([]any)[p]
		case string:
			node = node.(map[string]any)[p]
		}
	}
	delete(node.(map[string]any), key)
	out, err := json.Marshal(v)
	require.NoError(t, err)
	return out
}

func parseFor(specType string, data []byte) error {
	var err error
	switch specType {
	case "flow":
		_, err = ParseFlow(data)
	case "sequence":
		_, err = ParseSequence(data)
	case "class":
		_, err = ParseClass(data)
	}
	return err
}

// errorsOn returns the validation errors of err whose field is field or
// lies under it.
func errorsOn(err error, field string) []string {
	var ve ValidationErrors
	if !errors.As(err, &ve) {
		if err != nil {
			return []string{err.Error()}
		}
		return nil
	}
	var out []string
	for _, e := range ve {
		if e.Field == field || strings.HasPrefix(e.Field, field+".") || strings.HasPrefix(e.Field, field+"[") {
			out = append(out, e.Field+": "+e.Message)
		}
	}
	return out
}

// TestJSONSchema_RequiredMatchesValidation: the schema marks a property
// required exactly when diago rejects a spec without it. Every object of a
// sample spec loses each of its properties in turn: a required one must
// draw a validation error on its field, an optional one none.
func TestJSONSchema_RequiredMatchesValidation(t *testing.T) {
	for _, specType := range []string{"flow", "sequence", "class"} {
		t.Run(specType, func(t *testing.T) {
			sample := requiredSamples[specType]
			require.NoError(t, parseFor(specType, []byte(sample)), "the sample is valid")
			data, err := GenerateJSONSchema(specType)
			require.NoError(t, err)
			var raw map[string]any
			require.NoError(t, json.Unmarshal(data, &raw))
			defs := raw["$defs"].(map[string]any)
			root := strings.TrimPrefix(raw["$ref"].(string), "#/$defs/")
			var doc any
			require.NoError(t, json.Unmarshal([]byte(sample), &doc))
			var objs []instance
			walk(defs, root, doc, nil, &objs)

			seen := map[string]bool{}
			for _, o := range objs {
				def := defs[o.def].(map[string]any)
				required := map[string]bool{}
				reqList, _ := def["required"].([]any)
				for _, r := range reqList {
					required[r.(string)] = true
				}
				var keys []string
				for k := range def["properties"].(map[string]any) {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, key := range keys {
					obj := doc
					for _, p := range o.path {
						switch p := p.(type) {
						case int:
							obj = obj.([]any)[p]
						case string:
							obj = obj.(map[string]any)[p]
						}
					}
					if _, present := obj.(map[string]any)[key]; !present {
						continue
					}
					seen[o.def+"."+key] = true
					field := fieldPath(append(append([]any{}, o.path...), key))
					got := errorsOn(parseFor(specType, without(t, sample, o.path, key)), field)
					if required[key] {
						assert.NotEmpty(t, got, "%s.%s is required in the schema, but a spec without %s parses", o.def, key, field)
					} else {
						assert.Empty(t, got, "%s.%s is optional in the schema, but a spec without %s fails", o.def, key, field)
					}
				}
			}
			for name, d := range defs {
				reqList, _ := d.(map[string]any)["required"].([]any)
				for _, r := range reqList {
					assert.True(t, seen[name+"."+r.(string)], "the %s sample never sets required %s.%s", specType, name, r)
				}
			}
		})
	}
}

// TestJSONSchema_FilesAreCurrent: schemas/*.json is what `just schemas`
// writes from the spec structs today.
func TestJSONSchema_FilesAreCurrent(t *testing.T) {
	for _, specType := range []string{"flow", "sequence", "class"} {
		want, err := GenerateJSONSchema(specType)
		require.NoError(t, err)
		got, err := os.ReadFile(filepath.Join("..", "..", "schemas", specType+".json"))
		require.NoError(t, err)
		assert.Equal(t, string(want)+"\n", string(got), "schemas/%s.json is stale: run just schemas", specType)
	}
}
