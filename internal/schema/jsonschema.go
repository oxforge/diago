package schema

import (
	"encoding/json"
	"fmt"

	"github.com/invopop/jsonschema"
)

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

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal schema: %w", err)
	}
	return data, nil
}
