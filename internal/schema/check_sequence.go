package schema

import (
	"fmt"
	"strings"
)

// checkSequence returns the content findings for a sequence spec, unsorted
// and before the ignore list; Check sorts and filters.
func checkSequence(spec *SequenceSpec) []Advisory {
	var out []Advisory
	if len(spec.Actors) > MaxParticipants {
		out = append(out, Advisory{Rule: RuleSeqTooManyParticipants,
			Message: fmt.Sprintf("%d participants is a lot for one sequence diagram (advice threshold: %d); consider splitting into focused diagrams or collapsing minor participants",
				len(spec.Actors), MaxParticipants)})
	}
	for i, a := range spec.Actors {
		out = append(out, labelAdvisories(fmt.Sprintf("actors[%d]", i), fmt.Sprintf("actor %q label", a.ID), "the diagram", a.Label)...)
	}
	for i, it := range spec.Interactions {
		field := fmt.Sprintf("interactions[%d]", i)
		subject := fmt.Sprintf("message %s->%s at %s label", it.From, it.To, field)
		out = append(out, labelAdvisories(field, subject, "the diagram", it.Label)...)
		if isVague(it.Label) {
			out = append(out, Advisory{Rule: RuleVagueMessageLabel, Field: field,
				Message: fmt.Sprintf("%s %q says little; prefer a specific verb phrase like \"validates order\" or \"publishes event\"",
					subject, strings.TrimSpace(it.Label))})
		}
	}
	ranges := fragmentRanges(spec.Fragments)
	memo := make(map[int]int)
	for i, f := range spec.Fragments {
		field := fmt.Sprintf("fragments[%d]", i)
		for k, s := range f.Sections {
			sf := fmt.Sprintf("%s.sections[%d]", field, k)
			out = append(out, labelAdvisories(sf, fmt.Sprintf("section %q of %s label", s.Label, field), "the diagram", s.Label)...)
			if f.Type == "alt" && strings.TrimSpace(s.Label) == "" {
				out = append(out, Advisory{Rule: RuleUnlabeledAltSection, Field: sf,
					Message: fmt.Sprintf("alt section %s has no guard label; label each section with its condition so readers know which path runs", sf)})
			}
		}
		if d := fragmentDepth(ranges, i, memo); d > MaxNesting {
			name := f.Type
			if len(f.Sections) > 0 && strings.TrimSpace(f.Sections[0].Label) != "" {
				name = f.Sections[0].Label
			}
			out = append(out, Advisory{Rule: RuleDeepNesting, Field: field,
				Message: fmt.Sprintf("fragment %q (%s) is nested %d levels deep; over %d gets hard to read, flatten it or split into a referenced diagram",
					name, field, d, MaxNesting)})
		}
	}
	return out
}

// span is a fragment's outer range: its lowest section start to its highest
// section end. ok is false for a fragment without sections.
type span struct {
	lo, hi int
	ok     bool
}

func fragmentRanges(fragments []FragmentSpec) []span {
	out := make([]span, len(fragments))
	for i, f := range fragments {
		for k, s := range f.Sections {
			if k == 0 {
				out[i] = span{s.Start, s.End, true}
				continue
			}
			out[i].lo = min(out[i].lo, s.Start)
			out[i].hi = max(out[i].hi, s.End)
		}
	}
	return out
}

// contains reports whether a's range covers b's and the two ranges differ.
func contains(a, b span) bool {
	return a.ok && b.ok && a.lo <= b.lo && b.hi <= a.hi && (a.lo != b.lo || a.hi != b.hi)
}

// fragmentDepth is 1 plus the depth of the deepest fragment containing i.
// Containment is strict, so the recursion terminates.
func fragmentDepth(ranges []span, i int, memo map[int]int) int {
	if d, ok := memo[i]; ok {
		return d
	}
	d := 1
	for j := range ranges {
		if j != i && contains(ranges[j], ranges[i]) {
			if dj := fragmentDepth(ranges, j, memo) + 1; dj > d {
				d = dj
			}
		}
	}
	memo[i] = d
	return d
}
