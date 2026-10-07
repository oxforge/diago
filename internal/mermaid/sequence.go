package mermaid

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/oxforge/diago/internal/schema"
)

func init() { dialects[KindSequence] = parseSequence }

const seqID = `[A-Za-z0-9_.-]+`

// The "from" endpoint is lazy: the id class includes "-", so a greedy match
// would swallow the leading dashes of the arrow ("W-->>X" would split as
// from "W-", arrow "->>"). Lazy matching grows only until an arrow fits.
const seqIDLazy = `[A-Za-z0-9_.-]+?`

var (
	seqMsgRE         = regexp.MustCompile(`^(` + seqIDLazy + `)\s*(-->>|->>|--\)|-\))\s*([+-])?\s*(` + seqID + `)\s*:\s*(.*)$`)
	seqParticipantRE = regexp.MustCompile(`^(participant|actor)\s+(` + seqID + `)(?:\s+as\s+(.+))?$`)
	seqNoteRE        = regexp.MustCompile(`^[Nn]ote\s+(over|left of|right of)\s+(` + seqID + `)(?:\s*,\s*(` + seqID + `))?\s*:\s*(.*)$`)
	seqBlockRE       = regexp.MustCompile(`^(loop|opt|alt|par|break)(?:\s+(.*))?$`)
	seqElseRE        = regexp.MustCompile(`^else(?:\s+(.*))?$`)
	seqAndRE         = regexp.MustCompile(`^and(?:\s+(.*))?$`)
	seqActivateRE    = regexp.MustCompile(`^(activate|deactivate)\s+(` + seqID + `)$`)
	seqLostRE        = regexp.MustCompile(`^` + seqID + `\s*--?x`)
	seqUnsupportedRE = regexp.MustCompile(`^(critical|autonumber|create|destroy|rect|box|links?|properties|details)\b`)
)

var seqStyles = map[string]string{"->>": "", "-->>": "dashed", "-)": "async", "--)": "async"}

// seqSection is one branch of an open block: its interaction range, with
// start -1 until the first message lands in it.
type seqSection struct {
	keyword, label string
	line           int // the branch keyword's line, where an empty branch is reported
	start, end     int
}

type seqFrame struct {
	kind     string
	line     int
	sections []*seqSection
}

type seqFragment struct {
	line int // the block's opening line, for outer-first ordering
	spec schema.FragmentSpec
}

type seqParser struct {
	actors   []schema.ActorSpec
	actorIdx map[string]int
	explicit map[string]bool
	msgs     []schema.InteractionSpec
	stack    []*seqFrame
	frags    []seqFragment
	reports  []Report
}

func (p *seqParser) report(n int, format string, args ...any) {
	p.reports = append(p.reports, Report{Line: n, Message: fmt.Sprintf(format, args...)})
}

func (p *seqParser) ensure(id string) {
	if _, ok := p.actorIdx[id]; ok {
		return
	}
	p.actorIdx[id] = len(p.actors)
	p.actors = append(p.actors, schema.ActorSpec{ID: id, Label: id})
}

// current is the innermost open section, or nil at top level.
func (p *seqParser) current() *seqSection {
	if len(p.stack) == 0 {
		return nil
	}
	f := p.stack[len(p.stack)-1]
	return f.sections[len(f.sections)-1]
}

// lastInBlock is the index of the last interaction emitted in the current
// block, or -1 when the block has none yet.
func (p *seqParser) lastInBlock() int {
	if s := p.current(); s != nil {
		if s.start < 0 {
			return -1
		}
		return s.end
	}
	return len(p.msgs) - 1
}

// blockStart is the first interaction index of the current block (0 at
// top level, -1 when the block has none yet).
func (p *seqParser) blockStart() int {
	if s := p.current(); s != nil {
		return s.start
	}
	return 0
}

func (p *seqParser) addMessage(m schema.InteractionSpec) {
	idx := len(p.msgs)
	p.msgs = append(p.msgs, m)
	for _, f := range p.stack {
		s := f.sections[len(f.sections)-1]
		if s.start < 0 {
			s.start = idx
		}
		s.end = idx
	}
}

func parseSequence(title string, body []line) (any, []Report, error) {
	p := &seqParser{actorIdx: map[string]int{}, explicit: map[string]bool{}}
	headerSeen := false
	lastLine := 1
	for _, ln := range body {
		lastLine = ln.n
		if ln.text == "" {
			continue
		}
		if !headerSeen {
			if !sequenceHeaderRE.MatchString(ln.text) {
				return nil, nil, perr(ln.n, `expected a "sequenceDiagram" header`)
			}
			headerSeen = true
			continue
		}
		if err := p.statement(ln); err != nil {
			return nil, nil, err
		}
	}
	if !headerSeen {
		return nil, nil, perr(lastLine, `expected a "sequenceDiagram" header`)
	}
	if len(p.stack) > 0 {
		f := p.stack[len(p.stack)-1]
		return nil, nil, perr(f.line, "unclosed %q block", f.kind)
	}
	sort.SliceStable(p.frags, func(i, j int) bool { return p.frags[i].line < p.frags[j].line })
	spec := schema.SequenceSpec{Type: "sequence", Title: title, Actors: p.actors, Interactions: p.msgs}
	if spec.Actors == nil {
		spec.Actors = []schema.ActorSpec{}
	}
	if spec.Interactions == nil {
		spec.Interactions = []schema.InteractionSpec{}
	}
	for _, f := range p.frags {
		spec.Fragments = append(spec.Fragments, f.spec)
	}
	return spec, p.reports, nil
}

func (p *seqParser) statement(ln line) error {
	text := ln.text
	if m := seqUnsupportedRE.FindStringSubmatch(text); m != nil {
		return perr(ln.n, "%q is not supported by diago's sequence subset", m[1])
	}
	if m := seqParticipantRE.FindStringSubmatch(text); m != nil {
		kind, id, label := m[1], m[2], strings.TrimSpace(m[3])
		if p.explicit[id] {
			return perr(ln.n, "duplicate participant %q", id)
		}
		p.explicit[id] = true
		p.ensure(id)
		if label != "" {
			p.actors[p.actorIdx[id]].Label = label
		}
		if kind == "actor" {
			p.report(ln.n, "actor %q rendered as participant", id)
		}
		return nil
	}
	if m := seqBlockRE.FindStringSubmatch(text); m != nil {
		kind, label := m[1], strings.TrimSpace(m[2])
		p.stack = append(p.stack, &seqFrame{kind: kind, line: ln.n,
			sections: []*seqSection{{keyword: kind, label: label, line: ln.n, start: -1, end: -1}}})
		return nil
	}
	if m := seqElseRE.FindStringSubmatch(text); m != nil {
		return p.branch(ln, "else", "alt", strings.TrimSpace(m[1]))
	}
	if m := seqAndRE.FindStringSubmatch(text); m != nil {
		return p.branch(ln, "and", "par", strings.TrimSpace(m[1]))
	}
	if text == "end" {
		return p.closeBlock(ln)
	}
	if m := seqNoteRE.FindStringSubmatch(text); m != nil {
		for _, id := range []string{m[2], m[3]} {
			if id != "" {
				p.ensure(id)
			}
		}
		note := strings.TrimSpace(m[4])
		idx := p.lastInBlock()
		if idx < 0 {
			p.report(ln.n, "note dropped, no preceding message in its block")
			return nil
		}
		target := &p.msgs[idx]
		p.report(ln.n, "note folded into %q", target.Label)
		target.Label = strings.TrimSpace(target.Label + " (note: " + note + ")")
		return nil
	}
	if m := seqActivateRE.FindStringSubmatch(text); m != nil {
		verb, id := m[1], m[2]
		found := false
		for i := len(p.msgs) - 1; i >= p.blockStart() && i >= 0; i-- {
			if (verb == "activate" && p.msgs[i].To == id) || (verb == "deactivate" && p.msgs[i].From == id) {
				found = true
				break
			}
		}
		if !found {
			if verb == "activate" {
				return perr(ln.n, "activate %q must follow a message to %q", id, id)
			}
			return perr(ln.n, "deactivate %q must follow a message from %q", id, id)
		}
		p.report(ln.n, "%q ignored, activations are engine-computed", verb+" "+id)
		return nil
	}
	if m := seqMsgRE.FindStringSubmatch(text); m != nil {
		from, arrow, sign, to, label := m[1], m[2], m[3], m[4], strings.TrimSpace(m[5])
		p.ensure(from)
		p.ensure(to)
		if sign != "" {
			p.report(ln.n, "activation mark ignored, activations are engine-computed")
		}
		p.addMessage(schema.InteractionSpec{From: from, To: to, Label: label, Style: seqStyles[arrow]})
		return nil
	}
	if seqLostRE.MatchString(text) {
		return perr(ln.n, "lost-message arrows (-x) are not supported")
	}
	return perr(ln.n, "expected a message, note, participant, block, or end near %q", text)
}

// branch opens the next section of the innermost block, which must be of
// the matching kind.
func (p *seqParser) branch(ln line, keyword, kind, label string) error {
	if len(p.stack) == 0 || p.stack[len(p.stack)-1].kind != kind {
		article := "an"
		if kind == "par" {
			article = "a"
		}
		return perr(ln.n, "%q outside %s %q block", keyword, article, kind)
	}
	f := p.stack[len(p.stack)-1]
	f.sections = append(f.sections, &seqSection{keyword: keyword, label: label, line: ln.n, start: -1, end: -1})
	return nil
}

// closeBlock pops the innermost block into a fragment: empty branches are
// dropped with a report on the branch keyword's line, an entirely empty
// block is dropped with a report on its opening line, and "over" is every
// actor a message in the block's range touches, in declaration order.
// Parse sorts reports by line afterwards, so these late reports land in
// source order.
func (p *seqParser) closeBlock(ln line) error {
	if len(p.stack) == 0 {
		return perr(ln.n, `"end" without an open block`)
	}
	f := p.stack[len(p.stack)-1]
	p.stack = p.stack[:len(p.stack)-1]
	var sections []schema.FragmentSectionSpec
	first, last := -1, -1
	for _, s := range f.sections {
		if s.start < 0 {
			p.report(s.line, "empty %q branch dropped", s.keyword)
			continue
		}
		sections = append(sections, schema.FragmentSectionSpec{Label: s.label, Start: s.start, End: s.end})
		if first < 0 {
			first = s.start
		}
		last = s.end
	}
	if len(sections) == 0 {
		// Drop the branch reports: the whole block goes.
		p.reports = p.reports[:len(p.reports)-len(f.sections)]
		p.report(f.line, "empty %q block dropped", f.kind)
		return nil
	}
	touched := map[string]bool{}
	for _, m := range p.msgs[first : last+1] {
		touched[m.From], touched[m.To] = true, true
	}
	var over []string
	for _, a := range p.actors {
		if touched[a.ID] {
			over = append(over, a.ID)
		}
	}
	p.frags = append(p.frags, seqFragment{line: f.line, spec: schema.FragmentSpec{Type: f.kind, Over: over, Sections: sections}})
	return nil
}
