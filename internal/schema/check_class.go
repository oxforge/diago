package schema

import (
	"fmt"
	"unicode/utf8"
)

// memberLine composes a member as the box shows it: visibility, a space,
// then the text; just the text when the visibility is absent.
// Member-line composition exists in three places that must agree: this
// one, diff.memberText (internal/diff/members.go) and size.classLines
// (internal/layout/layered/size/class.go). Change one, check the others.
func memberLine(m MemberSpec) string {
	if m.Visibility == "" {
		return m.Text
	}
	return m.Visibility + " " + m.Text
}

// checkClass returns the content findings for a class spec, unsorted and
// before the ignore list; Check sorts and filters. Only the class rules,
// edge-without-id over relations and empty-group over packages run here:
// no other flow or sequence rule applies to a class document.
func checkClass(spec *ClassSpec, opts CheckOptions) []Advisory {
	var out []Advisory
	touched := make(map[string]bool, len(spec.Relations))
	for _, r := range spec.Relations {
		touched[r.From], touched[r.To] = true, true
	}
	for i, c := range spec.Classes {
		field := fmt.Sprintf("classes[%d]", i)
		if n := len(c.Attributes) + len(c.Methods); n > MaxClassMembers {
			out = append(out, Advisory{Rule: RuleGodClass, Field: field,
				Message: fmt.Sprintf("class %q has %d members (advice threshold: %d); it is likely doing too much, split its responsibilities into smaller classes", c.ID, n, MaxClassMembers)})
		}
		if len(spec.Classes) >= 2 && !touched[c.ID] {
			out = append(out, Advisory{Rule: RuleIsolatedClass, Field: field,
				Message: fmt.Sprintf("class %q has no relations; connect it to the diagram or remove it", c.ID)})
		}
		for _, comp := range []struct {
			name    string
			members []MemberSpec
		}{{"attributes", c.Attributes}, {"methods", c.Methods}} {
			for j, m := range comp.members {
				line := memberLine(m)
				if n := utf8.RuneCountInString(line); n > MaxMemberRunes {
					out = append(out, Advisory{Rule: RuleOverlongMember, Field: fmt.Sprintf("%s.%s[%d].text", field, comp.name, j),
						Message: fmt.Sprintf("member %q is %d runes long (advice threshold: %d); it widens the box, shorten the signature or its types", line, n, MaxMemberRunes)})
				}
			}
		}
	}
	if len(spec.Classes) > MaxNodes {
		out = append(out, Advisory{Rule: RuleOversizedClassDiagram,
			Message: fmt.Sprintf("%d classes is a lot for one diagram (advice threshold: %d); consider decomposing into linked sub-diagrams", len(spec.Classes), MaxNodes)})
	}
	if opts.Anchored {
		for i, r := range spec.Relations {
			if r.ID == nil || *r.ID == "" {
				out = append(out, Advisory{Rule: RuleEdgeWithoutID, Field: fmt.Sprintf("relations[%d]", i),
					Message: fmt.Sprintf("relation %s->%s has no id; anchoring and diffs key on ids, so give it one that survives edits", r.From, r.To)})
			}
		}
	}
	for i, p := range spec.Packages {
		if len(p.Contains) == 0 {
			out = append(out, Advisory{Rule: RuleEmptyGroup, Field: fmt.Sprintf("packages[%d]", i),
				Message: fmt.Sprintf("group %q contains nothing and renders as an empty box", p.ID)})
		}
	}
	return out
}
