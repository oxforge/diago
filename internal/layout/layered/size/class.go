package size

import (
	"slices"
	"strings"

	"github.com/oxforge/diago/internal/model"
)

// Member-line composition exists in three places that must agree:
// classLines below, schema.memberLine (internal/schema/check_class.go)
// and diff.memberText (internal/diff/members.go). Change one, check the
// others.
//
// classLines composes a record box's lines (S1): the header («stereotype»,
// then the name with its type parameters), then the attribute and method
// lines (diff mark, visibility, text, and in cells the " *" or " $"
// suffix of an abstract or static member).
func classLines(n model.Node, cells bool) (header, attrs, methods []model.PositionedLine) {
	m := n.Members
	if m.Stereotype != "" {
		header = append(header, model.PositionedLine{Text: "«" + m.Stereotype + "»", Stereotype: true})
	}
	name := n.Label
	if len(m.TypeParams) > 0 {
		name += "<" + strings.Join(m.TypeParams, ",") + ">"
	}
	header = append(header, model.PositionedLine{Text: name, Italic: m.Stereotype == "abstract"})
	compose := func(mem model.Member) model.PositionedLine {
		text := mem.Text
		if mem.Visibility != "" {
			text = mem.Visibility + " " + text
		}
		if cells {
			switch {
			case mem.Abstract:
				text += " *"
			case mem.Static:
				text += " $"
			}
		}
		return model.PositionedLine{Text: mem.Mark + text, Italic: mem.Abstract, Underline: mem.Static}
	}
	for _, a := range m.Attributes {
		attrs = append(attrs, compose(a))
	}
	for _, me := range m.Methods {
		methods = append(methods, compose(me))
	}
	return header, attrs, methods
}

// record sizes a class node's record box (S1). It never wraps, and the
// node minimums and shape rules do not apply.
func record(n model.Node, o Options) Size {
	header, attrs, methods := classLines(n, o.Cells)
	members := &model.PositionedMembers{Header: header, Attributes: attrs, Methods: methods}
	all := slices.Concat(header, attrs, methods)
	if o.Cells {
		widest := 0.0
		for _, l := range all {
			widest = max(widest, runes(l.Text))
		}
		rows := 2 + len(header)
		for _, compartment := range [][]model.PositionedLine{attrs, methods} {
			if len(compartment) > 0 {
				rows += 1 + len(compartment)
			}
		}
		members.HeaderLineHeight, members.MemberLineHeight = 1, 1
		return Size{W: even(max(o.MinW, widest+2*o.PadX)), H: float64(rows), Members: members}
	}
	_, headerH := o.Measure("X", o.NodeFont.Size, o.NodeFont.Family)
	_, memberH := o.Measure("X", o.MemberFont.Size, o.MemberFont.Family)
	widest := 0.0
	for i, l := range all {
		f := o.MemberFont
		if i < len(header) {
			f = o.NodeFont
		}
		w, _ := o.Measure(l.Text, f.Size, f.Family)
		widest = max(widest, w)
	}
	compartment := func(lines int, lineH float64) float64 {
		if lines == 0 {
			return 0
		}
		return float64(lines)*lineH + 2*o.ClassPadY
	}
	members.HeaderLineHeight, members.MemberLineHeight = headerH, memberH
	return Size{
		W:       max(o.ClassMinW, widest) + 2*o.ClassPadX,
		H:       compartment(len(header), headerH) + compartment(len(attrs), memberH) + compartment(len(methods), memberH),
		Members: members,
	}
}
