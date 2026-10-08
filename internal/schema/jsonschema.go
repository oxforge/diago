package schema

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/invopop/jsonschema"
)

// optionalProperties lists, per definition, the properties a spec may leave
// out although their struct fields have no omitempty: diago accepts a spec
// without them, and the importer still writes them, empty, so its output
// stays a complete spec.
var optionalProperties = map[string][]string{
	"FlowSpec":            {"edges"},
	"GroupSpec":           {"contains"},
	"SequenceSpec":        {"interactions"},
	"FragmentSectionSpec": {"label"},
}

// GenerateJSONSchema generates a JSON Schema document for the given diagram type.
// Returns the schema as indented JSON bytes.
func GenerateJSONSchema(diagramType string) ([]byte, error) {
	var s *jsonschema.Schema
	r := new(jsonschema.Reflector)

	switch diagramType {
	case "flow":
		s = r.Reflect(&FlowSpec{})
	case "sequence":
		s = r.Reflect(&SequenceSpec{})
	case "class":
		s = r.Reflect(&ClassSpec{})
	default:
		return nil, fmt.Errorf("unknown diagram type: %q", diagramType)
	}
	for name, def := range s.Definitions {
		def.Required = slices.DeleteFunc(def.Required, func(p string) bool {
			return slices.Contains(optionalProperties[name], p)
		})
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal schema: %w", err)
	}
	return data, nil
}
