package pipeline

import "github.com/oxforge/diago/internal/schema"

// Validate checks spec as a render does, without laying it out: its type,
// every field, and the theme it names.
func Validate(spec []byte) error {
	kind, err := schema.DiagramType(spec)
	if err != nil {
		return err
	}
	switch kind {
	case "flow":
		_, err = schema.ParseFlow(spec)
	case "class":
		_, err = schema.ParseClass(spec)
	default:
		_, err = schema.ParseSequence(spec)
	}
	if err != nil {
		return err
	}
	_, err = specTheme(spec, "")
	return err
}
