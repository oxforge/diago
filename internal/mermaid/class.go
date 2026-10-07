package mermaid

import (
	"regexp"
	"strings"

	"github.com/oxforge/diago/internal/schema"
)

func init() { dialects[KindClass] = parseClass }

var (
	classDeclRE        = regexp.MustCompile(`^class\s+([A-Za-z_]\w*)(?:~([^~]+)~)?\s*(\{)?$`)
	classDirectionRE   = regexp.MustCompile(`^direction\s+(TB|LR|BT|RL)$`)
	classStereoLineRE  = regexp.MustCompile(`^<<(\w+)>>$`)
	classStereoAloneRE = regexp.MustCompile(`^<<(\w+)>>\s+(\w+)$`)
	classInlineRE      = regexp.MustCompile(`^(\w+)\s*:\s*(.+)$`)
	classMemberRE      = regexp.MustCompile(`^([+\-#~])?\s*(.+?)\s*([*$])?$`)
	classRelationRE    = regexp.MustCompile(`^(\w+)\s*(?:"([^"]*)"\s*)?(<\|--|<\|\.\.|--\|>|\.\.\|>|\*--|--\*|o--|--o|-->|<--|\.\.>|<\.\.|--)\s*(?:"([^"]*)"\s*)?(\w+)\s*(?::\s*(.+))?$`)
	classUnsupportedRE = regexp.MustCompile(`^(note|namespace|cssClass|click)\b`)
	classGenericRE     = regexp.MustCompile(`~([^~]+)~`)
	classKeywordRE     = regexp.MustCompile(`^class\s`)
)

var classDirections = map[string]string{"TB": "DOWN", "LR": "RIGHT", "BT": "UP", "RL": "LEFT"}

// classOp maps an operator to its kind, whether the written operands swap
// to reach the JSON's canonical orientation (from: subtype, implementer,
// owner or dependent), and whether it writes directed: true.
type classOp struct {
	kind     string
	swap     bool
	directed bool
}

var classOps = map[string]classOp{
	"<|--": {"inheritance", true, false}, "--|>": {"inheritance", false, false},
	"<|..": {"realization", true, false}, "..|>": {"realization", false, false},
	"*--": {"composition", false, false}, "--*": {"composition", true, false},
	"o--": {"aggregation", false, false}, "--o": {"aggregation", true, false},
	"-->": {"association", false, true}, "<--": {"association", true, true},
	"..>": {"dependency", false, false}, "<..": {"dependency", true, false},
	"--": {"association", false, false},
}

type classParser struct {
	classes map[string]*schema.ClassNodeSpec
	order   []string
	rels    []schema.RelationSpec
}

func (p *classParser) ensure(id string) *schema.ClassNodeSpec {
	c, ok := p.classes[id]
	if !ok {
		c = &schema.ClassNodeSpec{ID: id}
		p.classes[id] = c
		p.order = append(p.order, id)
	}
	return c
}

// addMember parses "[+-#~] text [*|$]"; a "(" in the text makes it a
// method; "~T~" inside the text becomes "<T>".
func (p *classParser) addMember(c *schema.ClassNodeSpec, text string) {
	m := classMemberRE.FindStringSubmatch(text)
	member := schema.MemberSpec{Visibility: m[1], Text: classGenericRE.ReplaceAllString(m[2], "<$1>")}
	switch m[3] {
	case "*":
		member.Abstract = true
	case "$":
		member.Static = true
	}
	if strings.Contains(member.Text, "(") {
		c.Methods = append(c.Methods, member)
	} else {
		c.Attributes = append(c.Attributes, member)
	}
}

func parseClass(title string, body []line) (any, []Report, error) {
	p := &classParser{classes: map[string]*schema.ClassNodeSpec{}}
	headerSeen := false
	direction := ""
	current := "" // the class whose "{" block is open
	lastLine := 1
	for _, ln := range body {
		lastLine = ln.n
		text := ln.text
		if text == "" {
			continue
		}
		if !headerSeen {
			if !classHeaderLineRE.MatchString(text) {
				return nil, nil, perr(ln.n, `expected a "classDiagram" header`)
			}
			headerSeen = true
			continue
		}
		if current != "" {
			if text == "}" {
				current = ""
				continue
			}
			if m := classStereoLineRE.FindStringSubmatch(text); m != nil {
				p.ensure(current).Stereotype = m[1]
				continue
			}
			if classKeywordRE.MatchString(text) {
				return nil, nil, perr(ln.n, "unclosed class block %q", current)
			}
			p.addMember(p.ensure(current), text)
			continue
		}
		if m := classDirectionRE.FindStringSubmatch(text); m != nil {
			direction = classDirections[m[1]]
			continue
		}
		if m := classRelationRE.FindStringSubmatch(text); m != nil {
			if op, ok := classOps[m[3]]; ok {
				// Classes are declared in written order, before the swap,
				// so first-mention order follows the source.
				p.ensure(m[1])
				p.ensure(m[5])
				from, to, fromCard, toCard := m[1], m[5], m[2], m[4]
				if op.swap {
					from, to = to, from
					fromCard, toCard = toCard, fromCard
				}
				r := schema.RelationSpec{From: from, To: to, Kind: op.kind, FromCard: fromCard, ToCard: toCard, Label: strings.TrimSpace(m[6])}
				if op.directed {
					yes := true
					r.Directed = &yes
				}
				p.rels = append(p.rels, r)
				continue
			}
		}
		if m := classDeclRE.FindStringSubmatch(text); m != nil {
			c := p.ensure(m[1])
			if m[2] != "" {
				for _, tp := range strings.Split(m[2], ",") {
					if tp = strings.TrimSpace(tp); tp != "" {
						c.TypeParams = append(c.TypeParams, tp)
					}
				}
			}
			if m[3] == "{" {
				current = m[1]
			}
			continue
		}
		if m := classStereoAloneRE.FindStringSubmatch(text); m != nil {
			p.ensure(m[2]).Stereotype = m[1]
			continue
		}
		if m := classInlineRE.FindStringSubmatch(text); m != nil {
			p.addMember(p.ensure(m[1]), strings.TrimSpace(m[2]))
			continue
		}
		if strings.Contains(text, "<-->") {
			return nil, nil, perr(ln.n, `bidirectional arrows ("<-->") are not supported by diago's class subset`)
		}
		if strings.Contains(text, "()--") {
			return nil, nil, perr(ln.n, `lollipop interfaces ("()--") are not supported by diago's class subset`)
		}
		if m := classUnsupportedRE.FindStringSubmatch(text); m != nil {
			return nil, nil, perr(ln.n, "%q is not supported by diago's class subset", m[1])
		}
		return nil, nil, perr(ln.n, "expected a class, relation, or member near %q", text)
	}
	if !headerSeen {
		return nil, nil, perr(lastLine, `expected a "classDiagram" header`)
	}
	if current != "" {
		return nil, nil, perr(lastLine, "unclosed class block %q", current)
	}
	spec := schema.ClassSpec{Type: "class", Title: title, Direction: direction, Classes: make([]schema.ClassNodeSpec, 0, len(p.order))}
	for _, id := range p.order {
		spec.Classes = append(spec.Classes, *p.classes[id])
	}
	spec.Relations = p.rels
	return spec, nil, nil
}
