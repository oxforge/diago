package pipeline

import "github.com/oxforge/diago/internal/schema"

// Validate checks spec as a render does, without laying it out: its type,
// every field, and the theme it names.
func Validate(spec []byte) error {
	kind, err := schema.DiagramType(spec)
	if err != nil {
		return err
	}
	if err := validateFields(spec, kind); err != nil {
		return err
	}
	_, err = specTheme(spec, "")
	return err
}

// validateFields parses spec as a kind diagram for its validation errors.
func validateFields(spec []byte, kind string) error {
	var err error
	switch kind {
	case "flow":
		_, err = schema.ParseFlow(spec)
	case "class":
		_, err = schema.ParseClass(spec)
	default:
		_, err = schema.ParseSequence(spec)
	}
	return err
}
