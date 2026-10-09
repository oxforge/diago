package schema

import (
	"fmt"
	"strings"
)

// singleLine returns s with every control character written as its escape:
// \n, \t, \r, or \uXXXX for the rest. A field drawn on one line never breaks
// its row, and no character XML forbids reaches an SVG.
func singleLine(s string) string { return escapeControls(s, false) }

// hasControl reports whether s holds a control character.
func hasControl(s string) bool {
	return strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 || r == 0x7f })
}

// multiLine is singleLine for a node's label, whose "\n" (or "\r\n") breaks
// it into lines.
func multiLine(s string) string {
	return escapeControls(strings.ReplaceAll(s, "\r\n", "\n"), true)
}

func escapeControls(s string, keepNewline bool) string {
	needs := func(r rune) bool {
		if r == '\n' {
			return !keepNewline
		}
		return r < 0x20 || r == 0x7f || r == 0xfffe || r == 0xffff
	}
	if !strings.ContainsFunc(s, needs) {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case !needs(r):
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\r':
			b.WriteString(`\r`)
		default:
			fmt.Fprintf(&b, `\u%04x`, r)
		}
	}
	return b.String()
}

// singleLines is singleLine over each of ss; nil stays nil.
func singleLines(ss []string) []string {
	if ss == nil {
		return nil
	}
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = singleLine(s)
	}
	return out
}
