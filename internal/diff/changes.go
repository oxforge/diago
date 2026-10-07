package diff

import "strings"

// Change is one line of a diff's change list: the footer of a text diff and
// the list under an SVG or PNG diff.
type Change struct {
	Status Status // Added, Removed or Changed: the line's color
	Wire   bool   // an edge, relation or message (a wire sample), else a box
	Text   string // the line as printed
}

// Texts returns the lines' texts.
func Texts(changes []Change) []string {
	out := make([]string, len(changes))
	for i, c := range changes {
		out[i] = c.Text
	}
	return out
}

// changedSuffix is ": field old → new, …" for a changed line's fields, or
// "" when it has none.
func changedSuffix(fields []FieldChange) string {
	if len(fields) == 0 {
		return ""
	}
	parts := make([]string, len(fields))
	for i, f := range fields {
		parts[i] = f.Field + " " + f.Before + " → " + f.After
	}
	return ": " + strings.Join(parts, ", ")
}
