package diff

import (
	"slices"
	"sort"
	"strings"

	"github.com/oxforge/diago/internal/model"
)

// FragmentStatus is a union fragment's status and its sections' statuses.
type FragmentStatus struct {
	Status   Status
	Sections []Status
}

// MessageChange records a changed message: its union index and its before
// content, so the legend can show the old label or style.
type MessageChange struct {
	Index  int
	Before model.Interaction
}

// SectionRef names a section for the legend. Removed sections may have
// been dropped from geometry, so the reference carries the label and the
// fragment type rather than a section index.
type SectionRef struct {
	Fragment int
	Label    string
	Type     model.FragmentType
}

// SequenceDiff holds the legend records of a sequence diff.
type SequenceDiff struct {
	AddedActors, RemovedActors, ChangedActors []string
	AddedMessages, RemovedMessages            []int
	ChangedMessages                           []MessageChange
	AddedFragments, RemovedFragments          []int
	AddedSections, RemovedSections            []SectionRef
	// ActorFields holds, per changed actor id, the fields that differ.
	ActorFields map[string][]FieldChange
}

// UnionSequence is after ∪ removed-from-before with a status per element.
// Interactions, Removed and Fragments are parallel to Diagram's slices.
type UnionSequence struct {
	Diagram      model.SequenceDiagram
	Actors       map[string]Status
	Interactions []Status
	Removed      []bool
	Fragments    []FragmentStatus
	Diff         SequenceDiff
	BeforeLabels map[string]string
	Alignment    SeqAlignment
}

func fragmentStrongKey(f model.Fragment) string {
	labels := make([]string, len(f.Sections))
	for i, s := range f.Sections {
		labels[i] = s.Label
	}
	return fragmentWeakKey(f) + "|" + strings.Join(labels, "\x00")
}

func fragmentWeakKey(f model.Fragment) string {
	over := append([]string(nil), f.Over...)
	sort.Strings(over)
	return f.Type.String() + "|" + strings.Join(over, ",")
}

// matchFragments pairs after fragments with before fragments: strong key
// first, then weak key over the leftovers, first free match in declaration
// order. Returns afterToBefore (-1 = added) and the unmatched before indices.
func matchFragments(before, after []model.Fragment) ([]int, []int) {
	afterToBefore := make([]int, len(after))
	usedBefore := make([]bool, len(before))
	for i := range afterToBefore {
		afterToBefore[i] = -1
	}
	for _, key := range []func(model.Fragment) string{fragmentStrongKey, fragmentWeakKey} {
		for ai, af := range after {
			if afterToBefore[ai] >= 0 {
				continue
			}
			for bi, bf := range before {
				if !usedBefore[bi] && key(af) == key(bf) {
					afterToBefore[ai] = bi
					usedBefore[bi] = true
					break
				}
			}
		}
	}
	var leftover []int
	for bi := range before {
		if !usedBefore[bi] {
			leftover = append(leftover, bi)
		}
	}
	return afterToBefore, leftover
}

type unionSection struct {
	sec    model.FragmentSection
	status Status
}

func remap(sec model.FragmentSection, toUnion []int) model.FragmentSection {
	out := model.FragmentSection{Label: sec.Label, Start: toUnion[sec.Start], End: toUnion[sec.End]}
	if out.End < out.Start {
		out.End = out.Start // inverted through the alignment: clamp
	}
	return out
}

func overlaps(a, b model.FragmentSection) bool { return a.Start <= b.End && b.Start <= a.End }

// BuildUnionSequence merges before and after into one timeline with a
// status per actor, message, fragment and section.
func BuildUnionSequence(before, after *model.SequenceDiagram) UnionSequence {
	u := UnionSequence{
		Actors:       map[string]Status{},
		BeforeLabels: map[string]string{},
		Alignment:    alignSequences(before, after),
		Diff:         SequenceDiff{ActorFields: map[string][]FieldChange{}},
	}
	for _, a := range before.Actors {
		u.BeforeLabels[a.ID] = a.Label
	}
	u.unionActors(before, after)
	beforeToUnion, afterToUnion := u.unionInteractions(before, after)
	u.Diagram.Activations = after.Activations
	u.unionFragments(before.Fragments, after.Fragments, beforeToUnion, afterToUnion)
	return u
}

// unionActors appends the aligned actors in column order, a removed one
// with before's content and every other with after's, and records each
// one's status and a changed one's fields.
func (u *UnionSequence) unionActors(before, after *model.SequenceDiagram) {
	beforeActor := make(map[string]model.Actor, len(before.Actors))
	for _, a := range before.Actors {
		beforeActor[a.ID] = a
	}
	afterActor := make(map[string]model.Actor, len(after.Actors))
	for _, a := range after.Actors {
		afterActor[a.ID] = a
	}
	for _, as := range u.Alignment.Actors {
		u.Actors[as.ID] = as.Status
		if as.Status == Removed {
			u.Diagram.Actors = append(u.Diagram.Actors, beforeActor[as.ID])
			u.Diff.RemovedActors = append(u.Diff.RemovedActors, as.ID)
			continue
		}
		// Added, changed and same actors all carry after's content.
		u.Diagram.Actors = append(u.Diagram.Actors, afterActor[as.ID])
		switch as.Status {
		case Added:
			u.Diff.AddedActors = append(u.Diff.AddedActors, as.ID)
		case Changed:
			u.Diff.ChangedActors = append(u.Diff.ChangedActors, as.ID)
			u.Diff.ActorFields[as.ID] = actorFields(beforeActor[as.ID], afterActor[as.ID])
		}
	}
}

// unionInteractions appends the aligned steps' messages in union order, a
// removed one from before and every other from after, records each one's
// status, and returns the maps from before's and after's message indices
// to the union's.
func (u *UnionSequence) unionInteractions(before, after *model.SequenceDiagram) (beforeToUnion, afterToUnion []int) {
	beforeToUnion = make([]int, len(before.Interactions))
	afterToUnion = make([]int, len(after.Interactions))
	for i, st := range u.Alignment.Steps {
		u.Interactions = append(u.Interactions, st.Status)
		u.Removed = append(u.Removed, st.Status == Removed)
		if st.BeforeIdx >= 0 {
			beforeToUnion[st.BeforeIdx] = i
		}
		if st.AfterIdx >= 0 {
			afterToUnion[st.AfterIdx] = i
		}
		switch st.Status {
		case Removed:
			u.Diagram.Interactions = append(u.Diagram.Interactions, before.Interactions[st.BeforeIdx])
			u.Diff.RemovedMessages = append(u.Diff.RemovedMessages, i)
		case Added:
			u.Diagram.Interactions = append(u.Diagram.Interactions, after.Interactions[st.AfterIdx])
			u.Diff.AddedMessages = append(u.Diff.AddedMessages, i)
		case Changed:
			u.Diagram.Interactions = append(u.Diagram.Interactions, after.Interactions[st.AfterIdx])
			u.Diff.ChangedMessages = append(u.Diff.ChangedMessages, MessageChange{Index: i, Before: before.Interactions[st.BeforeIdx]})
		default:
			u.Diagram.Interactions = append(u.Diagram.Interactions, after.Interactions[st.AfterIdx])
		}
	}
	return beforeToUnion, afterToUnion
}

// unionFragments appends after's fragments, matched or added, in after
// order, then the before fragments no after fragment matched, as removed;
// their sections map onto the union's messages through beforeToUnion and
// afterToUnion.
func (u *UnionSequence) unionFragments(before, after []model.Fragment, beforeToUnion, afterToUnion []int) {
	afterToBefore, leftover := matchFragments(before, after)
	for ai, af := range after {
		idx := len(u.Diagram.Fragments)
		bi := afterToBefore[ai]
		if bi < 0 {
			u.addFragment(af, sectionsAs(af.Sections, afterToUnion, Added), Added)
			u.Diff.AddedFragments = append(u.Diff.AddedFragments, idx)
			continue
		}
		secs, changed := u.unionSections(idx, before[bi], af, beforeToUnion, afterToUnion)
		status := Same
		if changed {
			status = Changed
		}
		u.addFragment(af, secs, status)
	}
	for _, bi := range leftover {
		bf := before[bi]
		idx := len(u.Diagram.Fragments)
		u.addFragment(bf, sectionsAs(bf.Sections, beforeToUnion, Removed), Removed)
		u.Diff.RemovedFragments = append(u.Diff.RemovedFragments, idx)
	}
}

// sectionsAs maps sections onto the union's messages through toUnion, each
// with status s.
func sectionsAs(sections []model.FragmentSection, toUnion []int, s Status) []unionSection {
	var out []unionSection
	for _, sec := range sections {
		out = append(out, unionSection{remap(sec, toUnion), s})
	}
	return out
}

// unionSections pairs the sections of after fragment af with those of its
// matched before fragment bf by label, the first free one in order: a
// paired section is the same, an unpaired after section added and an
// unpaired before section removed, where a removed section overlapping a
// kept one stays in the legend only. It records the added and removed
// sections under union fragment idx and reports whether there were any.
func (u *UnionSequence) unionSections(idx int, bf, af model.Fragment, beforeToUnion, afterToUnion []int) ([]unionSection, bool) {
	usedBefore := make([]bool, len(bf.Sections))
	var secs []unionSection
	changed := false
	for _, s := range af.Sections {
		status := Added
		for k, bs := range bf.Sections {
			if !usedBefore[k] && bs.Label == s.Label {
				usedBefore[k] = true
				status = Same
				break
			}
		}
		if status == Added {
			changed = true
			u.Diff.AddedSections = append(u.Diff.AddedSections, SectionRef{idx, s.Label, af.Type})
		}
		secs = append(secs, unionSection{remap(s, afterToUnion), status})
	}
	for k, bs := range bf.Sections {
		if usedBefore[k] {
			continue
		}
		changed = true
		u.Diff.RemovedSections = append(u.Diff.RemovedSections, SectionRef{idx, bs.Label, af.Type})
		r := remap(bs, beforeToUnion)
		overlapsKept := slices.ContainsFunc(secs, func(kept unionSection) bool {
			return kept.status != Removed && overlaps(kept.sec, r)
		})
		if !overlapsKept {
			secs = append(secs, unionSection{r, Removed})
		}
	}
	return secs, changed
}

// addFragment appends fragment f to the union diagram with sections secs,
// in start order, and its status with theirs.
func (u *UnionSequence) addFragment(f model.Fragment, secs []unionSection, status Status) {
	sort.SliceStable(secs, func(i, j int) bool { return secs[i].sec.Start < secs[j].sec.Start })
	out := model.Fragment{Type: f.Type, Over: append([]string(nil), f.Over...)}
	fs := FragmentStatus{Status: status}
	for _, s := range secs {
		out.Sections = append(out.Sections, s.sec)
		fs.Sections = append(fs.Sections, s.status)
	}
	u.Diagram.Fragments = append(u.Diagram.Fragments, out)
	u.Fragments = append(u.Fragments, fs)
}
