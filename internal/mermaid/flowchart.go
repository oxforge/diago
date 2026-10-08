package mermaid

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/oxforge/diago/internal/schema"
)

func init() { dialects[KindFlowchart] = parseFlowchart }

// flowID is a node or subgraph id: letters, digits, "_" and ".", with "-"
// allowed only between two such runs, so "a-->b" splits as id "a" plus
// the arrow "-->" (plan ruling 1).
const flowID = `[A-Za-z0-9_.]+(?:-[A-Za-z0-9_.]+)*`

const quotedLabel = `"[^"]*"`

var (
	flowHeaderRE      = regexp.MustCompile(`^(graph|flowchart)\s+(\S+)\s*$`)
	flowUnsupportedRE = regexp.MustCompile(`^(classDef|class|style|linkStyle|click|accTitle|accDescr|direction)\b`)
	flowSubgraphRE    = regexp.MustCompile(`^subgraph\s+(` + flowID + `)\s*(?:\[(.*?)\])?\s*$`)
	// Arrow alternatives are ordered longest first: <--> before -->, -.->
	// before -.-, ==> before ===, --> before ---.
	flowArrowRE     = regexp.MustCompile(`^(<-->|-->|---|-\.->|-\.-|==>|===)\s*(?:\|([^|]*)\|\s*)?`)
	flowDashLabelRE = regexp.MustCompile(`^--\s+(.+?)\s+-->\s*`)
	// An edge id, Mermaid 11's "e1@" before an arrow: any run without
	// whitespace, quotes, "@" or "|" (so a "|label|" is never taken for one).
	flowEdgeIDRE = regexp.MustCompile(`^([^\s"@|]+)@`)
	// An edge's properties, "e1@{ animate: true }", set after the edge.
	flowEdgePropsRE = regexp.MustCompile(`^([^\s"@|]+)@\{.*\}$`)
	// One node reference: id, optional shape bracket (submatch 2 to 12, in
	// the order of flowShapes), trailing whitespace.
	flowNodeRefRE = regexp.MustCompile(`^(` + flowID + `)\s*(?:` +
		`\(\(\((` + quotedLabel + `|[^)]*)\)\)\)|` + // 2 doublecircle
		`\[\[(` + quotedLabel + `|[^\]]*)\]\]|` + // 3 subroutine
		`\[\((` + quotedLabel + `|[^)]*)\)\]|` + // 4 cylinder
		`\(\[(` + quotedLabel + `|[^\]]*)\]\)|` + // 5 stadium
		`\(\((` + quotedLabel + `|[^)]*)\)\)|` + // 6 circle
		`\{\{(` + quotedLabel + `|[^}]*)\}\}|` + // 7 hexagon
		`\[/(` + quotedLabel + `|[^/\\\]]*)/\]|` + // 8 parallelogram
		`\[/(` + quotedLabel + `|[^/\\\]]*)\\\]|` + // 9 trapezoid
		`\[(` + quotedLabel + `|[^\]]*)\]|` + // 10 rect
		`\((` + quotedLabel + `|[^)]*)\)|` + // 11 rounded
		`\{(` + quotedLabel + `|[^}]*)\}` + // 12 diamond
		`)?\s*`)
)

// flowShapes maps a flowNodeRefRE submatch index to the diago shape and,
// for a lossy bracket, its Mermaid name.
var flowShapes = map[int]struct{ shape, mermaid string }{
	2: {"circle", "doublecircle"}, 3: {"rect", "subroutine"}, 4: {"cylinder", ""},
	5: {"rounded", "stadium"}, 6: {"circle", ""}, 7: {"hexagon", ""}, 8: {"parallelogram", ""},
	9: {"parallelogram", "trapezoid"}, 10: {"rect", ""}, 11: {"rounded", ""}, 12: {"diamond", ""},
}

var flowDirections = map[string]string{"TD": "DOWN", "TB": "DOWN", "LR": "RIGHT", "BT": "UP", "RL": "LEFT"}

// flowArrows maps an arrow token to the edge style and direction it emits
// ("" is the default in both cases).
var flowArrows = map[string]struct{ style, dir string }{
	"-->": {}, "---": {"", "none"}, "-.->": {"dotted", ""}, "-.-": {"dotted", "none"},
	"==>": {"thick", ""}, "===": {"thick", "none"}, "<-->": {"", "both"},
}

// maxSubgraphDepth mirrors schema's group nesting limit; deeper subgraphs
// are flattened into the depth-3 ancestor.
const maxSubgraphDepth = 3

type flowNode struct {
	id, label, shape, parent string
	order                    int // first-mention order, shared with groups
}

type flowGroup struct {
	id, label, parent string
	order             int
}

type flowEdge struct {
	id, from, to, label, style, dir string
	line                            int
}

type flowParser struct {
	nodes   map[string]*flowNode
	groups  map[string]*flowGroup
	order   int
	stack   []string // open subgraph ids; a flattened subgraph pushes its ancestor again
	edges   []flowEdge
	edgeIDs map[string]int // edge id → the line that gave it
	reports []Report
}

func (p *flowParser) top() string {
	if len(p.stack) == 0 {
		return ""
	}
	return p.stack[len(p.stack)-1]
}

func parseFlowchart(title string, body []line) (any, []Report, error) {
	p := &flowParser{nodes: map[string]*flowNode{}, groups: map[string]*flowGroup{}, edgeIDs: map[string]int{}}
	direction := ""
	lastLine := 1
	for _, ln := range body {
		lastLine = ln.n
		if ln.text == "" {
			continue
		}
		// A trailing ";" is Mermaid's optional statement terminator and
		// carries no meaning; drop one before matching anything.
		text := strings.TrimSpace(strings.TrimSuffix(ln.text, ";"))
		if m := flowHeaderRE.FindStringSubmatch(text); m != nil {
			if direction != "" {
				return nil, nil, perr(ln.n, "duplicate graph header")
			}
			d, ok := flowDirections[m[2]]
			if !ok {
				return nil, nil, perr(ln.n, "direction %q is not supported (supported: TD, TB, LR, BT, RL)", m[2])
			}
			direction = d
			continue
		}
		if direction == "" {
			return nil, nil, perr(ln.n, `expected a "graph <dir>" or "flowchart <dir>" header`)
		}
		if m := flowUnsupportedRE.FindStringSubmatch(text); m != nil {
			return nil, nil, perr(ln.n, "%q is not supported by diago's flowchart subset", m[1])
		}
		if m := flowSubgraphRE.FindStringSubmatch(text); m != nil {
			p.openSubgraph(m[1], m[2], ln.n)
			continue
		}
		if text == "end" {
			if len(p.stack) == 0 {
				return nil, nil, perr(ln.n, `"end" without a matching "subgraph"`)
			}
			p.stack = p.stack[:len(p.stack)-1]
			continue
		}
		if err := p.statement(line{n: ln.n, text: text}); err != nil {
			return nil, nil, err
		}
	}
	if direction == "" {
		return nil, nil, perr(1, `empty source: expected a "graph <dir>" header`)
	}
	if len(p.stack) > 0 {
		return nil, nil, perr(lastLine, "unclosed subgraph %q", p.top())
	}
	return p.assemble(title, direction)
}

// openSubgraph declares a group under the current one, or flattens it
// into the depth-3 ancestor with a report.
func (p *flowParser) openSubgraph(id, bracketTitle string, n int) {
	parent := p.top()
	if len(p.stack) >= maxSubgraphDepth {
		p.reports = append(p.reports, Report{Line: n, Message: fmt.Sprintf("subgraph %q flattened into %q, depth limit %d", id, parent, maxSubgraphDepth)})
		p.stack = append(p.stack, parent)
		return
	}
	label := id
	if t := unquote(bracketTitle); t != "" {
		label = t
	}
	p.order++
	p.groups[id] = &flowGroup{id: id, label: label, parent: parent, order: p.order}
	delete(p.nodes, id) // a bare earlier mention becomes the group itself (Mermaid's rule)
	p.stack = append(p.stack, id)
}

// ensureNode records a mention. The first bracketed mention sets label and
// shape; a node re-referenced inside a subgraph while ungrouped is adopted
// by it (Mermaid's rule). A lossy bracket is reported once, when it takes
// effect.
func (p *flowParser) ensureNode(id, label, shape, lossy string, n int) {
	if _, isGroup := p.groups[id]; isGroup {
		return
	}
	node, ok := p.nodes[id]
	if !ok {
		p.order++
		node = &flowNode{id: id, parent: p.top(), order: p.order}
		p.nodes[id] = node
	} else if node.parent == "" && p.top() != "" {
		node.parent = p.top()
	}
	if label != "" && node.label == "" {
		node.label = label
	}
	if shape != "" && node.shape == "" {
		if lossy != "" {
			p.reports = append(p.reports, Report{Line: n, Message: fmt.Sprintf("%s %q rendered as %s", lossy, id, shape)})
		}
		node.shape = shape
	}
}

// endpoint parses one node reference, with "&" fan-out, at the head of
// rest; it returns the ids and the unparsed remainder.
func (p *flowParser) endpoint(rest string, n int) ([]string, string, error) {
	var ids []string
	for {
		loc := flowNodeRefRE.FindStringSubmatchIndex(rest)
		if loc == nil {
			return nil, "", perr(n, "expected a node reference near %q", strings.TrimSpace(rest))
		}
		id := rest[loc[2]:loc[3]]
		label, shape, lossy := "", "", ""
		for g := 2; g <= 12; g++ {
			if loc[2*g] >= 0 {
				label = unquote(rest[loc[2*g]:loc[2*g+1]])
				shape, lossy = flowShapes[g].shape, flowShapes[g].mermaid
				break
			}
		}
		p.ensureNode(id, label, shape, lossy, n)
		ids = append(ids, id)
		rest = rest[loc[1]:]
		if strings.HasPrefix(rest, "&") {
			rest = strings.TrimLeft(rest[1:], " \t")
			continue
		}
		return ids, rest, nil
	}
}

// statement parses "endpoint (arrow endpoint)*", emitting one edge per
// source-target pair and advancing the current set along a chain. An arrow
// may carry an edge id ("e1@-->"); the properties statement of an edge
// that has one ("e1@{ animate: true }") is dropped with a report.
func (p *flowParser) statement(ln line) error {
	if m := flowEdgePropsRE.FindStringSubmatch(ln.text); m != nil {
		if _, ok := p.edgeIDs[m[1]]; ok {
			p.reports = append(p.reports, Report{Line: ln.n, Message: fmt.Sprintf("properties of edge %q ignored, diago draws no edge animation or curve", m[1])})
			return nil
		}
	}
	current, rest, err := p.endpoint(ln.text, ln.n)
	if err != nil {
		return err
	}
	for rest != "" {
		var id, label, style, dir string
		arrow := rest
		if m := flowEdgeIDRE.FindStringSubmatch(arrow); m != nil {
			id, arrow = m[1], arrow[len(m[0]):]
		}
		if m := flowDashLabelRE.FindStringSubmatch(arrow); m != nil {
			label = strings.TrimSpace(m[1])
			arrow = arrow[len(m[0]):]
		} else if m := flowArrowRE.FindStringSubmatch(arrow); m != nil {
			style, dir = flowArrows[m[1]].style, flowArrows[m[1]].dir
			label = unquote(m[2])
			arrow = arrow[len(m[0]):]
		} else {
			return perr(ln.n, "expected an arrow near %q", strings.TrimSpace(rest))
		}
		targets, remaining, err := p.endpoint(arrow, ln.n)
		if err != nil {
			return err
		}
		rest = remaining
		for _, s := range current {
			for _, t := range targets {
				// Mermaid gives a fan-out's id to the edge from its last
				// source to its first target only.
				e := flowEdge{from: s, to: t, label: label, style: style, dir: dir, line: ln.n}
				if id != "" && s == current[len(current)-1] && t == targets[0] {
					e.id = p.edgeID(id, s, t, ln.n)
				}
				p.edges = append(p.edges, e)
			}
		}
		current = targets
	}
	return nil
}

// edgeID claims id for the edge from s to t, or reports it and returns ""
// when an earlier edge has it: Mermaid keeps an id with its first edge.
func (p *flowParser) edgeID(id, s, t string, n int) string {
	if first, taken := p.edgeIDs[id]; taken {
		p.reports = append(p.reports, Report{Line: n, Message: fmt.Sprintf("edge id %q already used on line %d, %s->%s imported without one", id, first, s, t)})
		return ""
	}
	p.edgeIDs[id] = n
	return id
}

// assemble emits the spec: nodes in first-mention order, edges in source
// order, groups in declaration order with members in first-mention order.
func (p *flowParser) assemble(title, direction string) (any, []Report, error) {
	nodes := make([]*flowNode, 0, len(p.nodes))
	for _, n := range p.nodes {
		nodes = append(nodes, n)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].order < nodes[j].order })
	groups := make([]*flowGroup, 0, len(p.groups))
	for _, g := range p.groups {
		groups = append(groups, g)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].order < groups[j].order })

	spec := schema.FlowSpec{Type: "flow", Title: title, Direction: direction, Nodes: []schema.NodeSpec{}, Edges: []schema.EdgeSpec{}}
	for _, n := range nodes {
		label, shape := n.label, n.shape
		if label == "" {
			label = n.id
		}
		if shape == "rect" {
			shape = "" // the default (plan ruling 6)
		}
		spec.Nodes = append(spec.Nodes, schema.NodeSpec{ID: n.id, Label: label, Shape: shape})
	}
	for _, e := range p.edges {
		for _, end := range []string{e.from, e.to} {
			if _, isGroup := p.groups[end]; isGroup {
				return nil, nil, perr(e.line, "edges to a subgraph are not supported (%q)", end)
			}
		}
		es := schema.EdgeSpec{From: e.from, To: e.to, Label: e.label, Style: e.style, Direction: e.dir}
		if e.id != "" {
			es.ID = &e.id
		}
		spec.Edges = append(spec.Edges, es)
	}
	type member struct {
		id    string
		order int
	}
	members := map[string][]member{}
	for _, n := range nodes {
		if n.parent != "" {
			members[n.parent] = append(members[n.parent], member{n.id, n.order})
		}
	}
	for _, g := range groups {
		if g.parent != "" {
			members[g.parent] = append(members[g.parent], member{g.id, g.order})
		}
	}
	for _, g := range groups {
		ms := members[g.id]
		sort.Slice(ms, func(i, j int) bool { return ms[i].order < ms[j].order })
		contains := make([]string, 0, len(ms))
		for _, m := range ms {
			contains = append(contains, m.id)
		}
		spec.Groups = append(spec.Groups, schema.GroupSpec{ID: g.id, Label: g.label, Contains: contains})
	}
	return spec, p.reports, nil
}
