package schema

import (
	"regexp"
	"strings"
)

// slotSuffix is the end of a layout carrier's slot key, <edge id>@<layer>:
// a node or group id that ended this way would read as a slot.
var slotSuffix = regexp.MustCompile(`@[0-9]+$`)

// idProblem says why id cannot name an element, or returns "" when it can.
// No id may hold a control character, U+FFFE or U+FFFF (escaped). An id a layout keys on (keyed: a
// node, class, group or package) must also not hold "->", which derived
// edge ids join their ends with, or end in "@" and digits, the form of a
// layout slot.
func idProblem(id string, keyed bool) string {
	switch {
	case hasControl(id):
		return "must not contain control characters, U+FFFE or U+FFFF"
	case keyed && strings.Contains(id, "->"):
		return `must not contain "->", which derived edge ids use`
	case keyed && slotSuffix.MatchString(id):
		return `must not end in "@" and digits, which layout anchoring uses`
	}
	return ""
}
