package diff

import (
	"strconv"
	"strings"

	"github.com/oxforge/diago/internal/model"
)

// StampSequenceLabels returns a copy of the union diagram whose actor
// labels, message labels and section guards carry their status prefix. It
// runs before layout so boxes and rows are sized for the stamped text. An
// unlabeled non-same message gets the bare marker as its label.
func StampSequenceLabels(u UnionSequence) model.SequenceDiagram {
	d := u.Diagram
	d.Actors = append([]model.Actor(nil), u.Diagram.Actors...)
	for i := range d.Actors {
		d.Actors[i].Label = TextPrefix(u.Actors[d.Actors[i].ID]) + d.Actors[i].Label
	}
	d.Interactions = append([]model.Interaction(nil), u.Diagram.Interactions...)
	for i := range d.Interactions {
		p := TextPrefix(u.Interactions[i])
		switch {
		case p == "":
		case d.Interactions[i].Label == "":
			d.Interactions[i].Label = strings.TrimSpace(p)
		default:
			d.Interactions[i].Label = p + d.Interactions[i].Label
		}
	}
	d.Fragments = make([]model.Fragment, len(u.Diagram.Fragments))
	for i, f := range u.Diagram.Fragments {
		f.Over = append([]string(nil), f.Over...)
		f.Sections = append([]model.FragmentSection(nil), f.Sections...)
		fs := u.Fragments[i]
		for k := range f.Sections {
			if p := TextPrefix(fs.Sections[k]); p != "" {
				f.Sections[k].Label = p + f.Sections[k].Label
			}
		}
		// The frame's status stamps the first guard only when that guard is
		// not already marked on its own account.
		if len(f.Sections) > 0 && fs.Sections[0] == Same {
			if p := TextPrefix(fs.Status); p != "" {
				f.Sections[0].Label = p + f.Sections[0].Label
			}
		}
		d.Fragments[i] = f
	}
	return d
}

// SequenceChanges lists every change, one line each, in the fixed order
// added / removed / changed × actor / message / fragment / section.
// Removed actors and the endpoints of removed messages are named by their
// before labels; a message is named by its endpoints and, unless the label
// is one of its changed fields, its label. A changed line names the fields
// that differ; changed fragments have no line of their own.
func SequenceChanges(u UnionSequence) []Change {
	unionLabel := func(id string) string {
		for _, a := range u.Diagram.Actors {
			if a.ID == id {
				return oneLine(a.Label)
			}
		}
		return id
	}
	beforeLabel := func(id string) string {
		if l, ok := u.BeforeLabels[id]; ok {
			return oneLine(l)
		}
		return unionLabel(id)
	}
	endpoints := func(it model.Interaction, removed bool) string {
		name := unionLabel
		if removed {
			name = beforeLabel
		}
		return name(it.From) + " -> " + name(it.To)
	}
	quoted := func(label string) string {
		if label == "" {
			return ""
		}
		return " " + strconv.Quote(label)
	}
	var out []Change
	add := func(s Status, wire bool, rest string) {
		out = append(out, Change{Status: s, Wire: wire, Text: string(s) + ": " + rest})
	}

	for _, id := range u.Diff.AddedActors {
		add(Added, false, "actor "+unionLabel(id))
	}
	for _, i := range u.Diff.AddedMessages {
		it := u.Diagram.Interactions[i]
		add(Added, true, "message "+endpoints(it, false)+quoted(it.Label))
	}
	for _, i := range u.Diff.AddedFragments {
		add(Added, false, "fragment "+u.Diagram.Fragments[i].Type.String())
	}
	for _, s := range u.Diff.AddedSections {
		add(Added, false, "section ["+oneLine(s.Label)+"] of "+s.Type.String())
	}
	for _, id := range u.Diff.RemovedActors {
		add(Removed, false, "actor "+beforeLabel(id))
	}
	for _, i := range u.Diff.RemovedMessages {
		it := u.Diagram.Interactions[i]
		add(Removed, true, "message "+endpoints(it, true)+quoted(it.Label))
	}
	for _, i := range u.Diff.RemovedFragments {
		add(Removed, false, "fragment "+u.Diagram.Fragments[i].Type.String())
	}
	for _, s := range u.Diff.RemovedSections {
		add(Removed, false, "section ["+oneLine(s.Label)+"] of "+s.Type.String())
	}
	for _, id := range u.Diff.ChangedActors {
		add(Changed, false, "actor "+unionLabel(id)+changedSuffix(u.Diff.ActorFields[id]))
	}
	for _, c := range u.Diff.ChangedMessages {
		now := u.Diagram.Interactions[c.Index]
		name := endpoints(now, false)
		if c.Before.Label == now.Label {
			name += quoted(now.Label)
		}
		add(Changed, true, "message "+name+changedSuffix(messageFields(c.Before, now)))
	}
	return out
}

// SequenceLegend is the text diff's footer: SequenceChanges' lines.
func SequenceLegend(u UnionSequence) []string { return Texts(SequenceChanges(u)) }
