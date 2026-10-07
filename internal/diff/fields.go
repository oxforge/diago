package diff

import (
	"strconv"
	"strings"

	"github.com/oxforge/diago/internal/model"
)

// FieldChange is one field of a changed element: its spec key and its
// value on each side, formatted as the change list prints them.
type FieldChange struct {
	Field, Before, After string
}

// fieldDiffer collects the fields that differ, in the order they are
// compared.
type fieldDiffer []FieldChange

// add records a field whose printed values differ.
func (f *fieldDiffer) add(field, before, after string) {
	if before != after {
		*f = append(*f, FieldChange{field, before, after})
	}
}

// quotedValue prints a string field: in double quotes, or none when empty.
func quotedValue(s string) string {
	if s == "" {
		return "none"
	}
	return strconv.Quote(s)
}

// specCards returns an edge's cardinalities in the orientation the spec wrote
// them: a swapped relation kind stores them with its endpoints exchanged.
func specCards(e model.Edge) (from, to string) {
	from, to = e.FromCard, e.ToCard
	if e.Relation.Swapped() {
		from, to = to, from
	}
	return from, to
}

// oneLine flattens the line breaks of a label to spaces, so an element's name
// keeps its change on one line.
func oneLine(s string) string {
	return strings.Join(strings.Fields(strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(s)), " ")
}

// wordValue prints an enum word or a color as written, or none when empty.
func wordValue(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

// parentRef is a node's parent group: its id, which decides a move, and its
// label, which the change list prints; both empty at the top level.
type parentRef struct{ id, label string }

// nodeFields lists a changed node's differing fields: label, shape, color,
// parent group, then a class's stereotype and type parameters. Members
// are left to the member lines.
func nodeFields(o, n model.Node, op, np parentRef) []FieldChange {
	var f fieldDiffer
	f.add("label", quotedValue(o.Label), quotedValue(n.Label))
	f.add("shape", o.Shape.String(), n.Shape.String())
	f.add("color", wordValue(o.Color), wordValue(n.Color))
	if op.id != np.id {
		f = append(f, FieldChange{"group", quotedValue(op.label), quotedValue(np.label)})
	}
	if o.Members != nil && n.Members != nil {
		f.add("stereotype", quotedValue(o.Members.Stereotype), quotedValue(n.Members.Stereotype))
		f.add("type_params", wordValue(strings.Join(o.Members.TypeParams, ", ")), wordValue(strings.Join(n.Members.TypeParams, ", ")))
	}
	return f
}

// edgeFields lists a changed edge's or relation's differing fields.
func edgeFields(o, n model.Edge) []FieldChange {
	var f fieldDiffer
	f.add("label", quotedValue(o.Label), quotedValue(n.Label))
	// Only report style for edges, not relations (style is implicit in kind for relations)
	if o.Relation.String() == "" && n.Relation.String() == "" {
		f.add("style", o.Style.String(), n.Style.String())
	}
	f.add("direction", o.Direction.String(), n.Direction.String())
	f.add("color", wordValue(o.Color), wordValue(n.Color))
	f.add("kind", wordValue(o.Relation.String()), wordValue(n.Relation.String()))
	of, ot := specCards(o)
	nf, nt := specCards(n)
	f.add("from_card", quotedValue(of), quotedValue(nf))
	f.add("to_card", quotedValue(ot), quotedValue(nt))
	f.add("directed", strconv.FormatBool(o.Directed), strconv.FormatBool(n.Directed))
	f.add("flat", strconv.FormatBool(o.Flat), strconv.FormatBool(n.Flat))
	return f
}

// groupFields lists a changed group's or package's differing fields.
func groupFields(o, n model.Group) []FieldChange {
	var f fieldDiffer
	f.add("label", quotedValue(o.Label), quotedValue(n.Label))
	f.add("color", wordValue(o.Color), wordValue(n.Color))
	return f
}

// actorFields lists a changed actor's differing fields.
func actorFields(o, n model.Actor) []FieldChange {
	var f fieldDiffer
	f.add("label", quotedValue(o.Label), quotedValue(n.Label))
	f.add("color", wordValue(o.Color), wordValue(n.Color))
	return f
}

// messageFields lists a changed message's differing fields.
func messageFields(o, n model.Interaction) []FieldChange {
	var f fieldDiffer
	f.add("label", quotedValue(o.Label), quotedValue(n.Label))
	f.add("style", o.Style.String(), n.Style.String())
	f.add("color", wordValue(o.Color), wordValue(n.Color))
	return f
}
