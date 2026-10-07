package schema

import (
	"fmt"
	"regexp"
)

var (
	hexColorRE  = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	namedColors = map[string]bool{
		"red": true, "green": true, "blue": true,
		"yellow": true, "orange": true, "purple": true, "gray": true,
	}
)

// validateColor checks that a color value is a valid named color or #rrggbb hex.
// Empty string is valid (means "use theme default").
func validateColor(field, color string) *ValidationError {
	if color == "" {
		return nil
	}
	if namedColors[color] {
		return nil
	}
	if hexColorRE.MatchString(color) {
		return nil
	}
	return &ValidationError{
		Field:   field,
		Message: fmt.Sprintf("unknown color %q; valid values: red, green, blue, yellow, orange, purple, gray, or #rrggbb hex", color),
	}
}
