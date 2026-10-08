package mermaid

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/oxforge/diago/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func parseFlow(t *testing.T, src string) (schema.FlowSpec, []Report) {
	t.Helper()
	res, err := Parse([]byte(src))
	require.NoError(t, err)
	require.Equal(t, KindFlowchart, res.Kind)
	requireValid(t, res.Kind, res.JSON)
	var spec schema.FlowSpec
	require.NoError(t, json.Unmarshal(res.JSON, &spec))
	return spec, res.Reports
}

func errOf(t *testing.T, src string) *ParseError {
	t.Helper()
	_, err := Parse([]byte(src))
	var pe *ParseError
	require.True(t, errors.As(err, &pe), "want ParseError, got %v", err)
	return pe
}

func node(id, label, shape string) schema.NodeSpec {
	return schema.NodeSpec{ID: id, Label: label, Shape: shape}
}
func edge(from, to string) schema.EdgeSpec { return schema.EdgeSpec{From: from, To: to} }

func TestFlowchart_Basics(t *testing.T) {
	// parses a minimal graph with direction
	spec, reports := parseFlow(t, "graph TD\n  a --> b\n")
	assert.Equal(t, "flow", spec.Type)
	assert.Equal(t, "DOWN", spec.Direction)
	assert.Equal(t, []schema.NodeSpec{node("a", "a", ""), node("b", "b", "")}, spec.Nodes)
	assert.Equal(t, []schema.EdgeSpec{edge("a", "b")}, spec.Edges)
	assert.Empty(t, reports)
	assert.Empty(t, spec.Groups)

	// accepts flowchart header and LR direction; plus the BT/RL extension
	spec, _ = parseFlow(t, "flowchart LR\n  x --> y\n")
	assert.Equal(t, "RIGHT", spec.Direction)
	spec, _ = parseFlow(t, "flowchart BT\n  x --> y\n")
	assert.Equal(t, "UP", spec.Direction)
	spec, _ = parseFlow(t, "graph RL\n  x --> y\n")
	assert.Equal(t, "LEFT", spec.Direction)

	// parses node shapes and labels
	spec, _ = parseFlow(t, "graph TD\n  a[Plain box]\n  b(Rounded box)\n  c{Decision?}\n  d((Circle))\n  a --> b\n")
	assert.Equal(t, []schema.NodeSpec{node("a", "Plain box", ""), node("b", "Rounded box", "rounded"),
		node("c", "Decision?", "diamond"), node("d", "Circle", "circle")}, spec.Nodes)

	// parses inline node defs inside edge lines
	spec, _ = parseFlow(t, "graph TD\n  start[Begin] --> stop[End]\n")
	assert.Equal(t, []schema.NodeSpec{node("start", "Begin", ""), node("stop", "End", "")}, spec.Nodes)

	// strips quotes from labels
	spec, _ = parseFlow(t, "graph TD\n  a[\"Quoted [label]\"] --> b\n")
	assert.Equal(t, "Quoted [label]", spec.Nodes[0].Label)

	// ids with inner dashes and no spaces around the arrow (plan ruling 1)
	spec, _ = parseFlow(t, "graph TD\na-->b\nmy-node-->x.y\n")
	assert.Equal(t, []schema.EdgeSpec{edge("a", "b"), edge("my-node", "x.y")}, spec.Edges)

	// a trailing ";" terminator on the header or a statement is ignored
	spec, reports = parseFlow(t, "graph TD;\n  a-->b;\n  b --> c;\n")
	assert.Equal(t, "DOWN", spec.Direction)
	assert.Equal(t, []schema.EdgeSpec{edge("a", "b"), edge("b", "c")}, spec.Edges)
	assert.Empty(t, reports)
}

func TestFlowchart_Edges(t *testing.T) {
	// edge labels via pipes and via inline dashes
	spec, _ := parseFlow(t, "graph TD\n  gate -->|yes| go\n  gate -- no --> stop")
	assert.Equal(t, []schema.EdgeSpec{
		{From: "gate", To: "go", Label: "yes"},
		{From: "gate", To: "stop", Label: "no"},
	}, spec.Edges)

	// inline dash labels may contain hyphens themselves; the label ends at
	// the first " -->", not at the first "-"
	spec, _ = parseFlow(t, "graph TD\n  gate -- non-2xx --> err\n  a -- re-try --> b")
	assert.Equal(t, []schema.EdgeSpec{
		{From: "gate", To: "err", Label: "non-2xx"},
		{From: "a", To: "b", Label: "re-try"},
	}, spec.Edges)

	// undirected, dotted and thick arrows; plus the extensions
	spec, _ = parseFlow(t, "graph TD\n  a --- b\n  b -.-> c\n  c ==> d\n  d <--> e\n  e -.- f\n  f === g")
	assert.Equal(t, []schema.EdgeSpec{
		{From: "a", To: "b", Direction: "none"},
		{From: "b", To: "c", Style: "dotted"},
		{From: "c", To: "d", Style: "thick"},
		{From: "d", To: "e", Direction: "both"},
		{From: "e", To: "f", Style: "dotted", Direction: "none"},
		{From: "f", To: "g", Style: "thick", Direction: "none"},
	}, spec.Edges)

	// chained edges create one edge per hop
	spec, _ = parseFlow(t, "graph TD\n  a --> b --> c --> d\n")
	assert.Equal(t, []schema.EdgeSpec{edge("a", "b"), edge("b", "c"), edge("c", "d")}, spec.Edges)

	// ampersand fans out to multiple endpoints
	spec, _ = parseFlow(t, "graph TD\n  a & b --> c & d\n")
	assert.Equal(t, []schema.EdgeSpec{edge("a", "c"), edge("a", "d"), edge("b", "c"), edge("b", "d")}, spec.Edges)

	// a quoted pipe label is unquoted, like a node label
	spec, _ = parseFlow(t, "graph TD\n  a -->|\"yes, go\"| b")
	assert.Equal(t, []schema.EdgeSpec{{From: "a", To: "b", Label: "yes, go"}}, spec.Edges)

	// maps ==> to a thick directed edge, label preserved
	spec, _ = parseFlow(t, "graph TD\n  a ==>|hot path| b")
	assert.Equal(t, []schema.EdgeSpec{{From: "a", To: "b", Style: "thick", Label: "hot path"}}, spec.Edges)

	// plain --> and --- edges carry no style key
	res, err := Parse([]byte("graph TD\n  a --> b\n  a --- c"))
	require.NoError(t, err)
	assert.NotContains(t, string(res.JSON), `"style"`)
	assert.NotContains(t, string(res.JSON), `"shape"`)
}

func TestFlowchart_EdgeIDs(t *testing.T) {
	id := func(s string) *string { return &s }

	// an id before the arrow, in each arrow form, becomes the edge's id
	spec, reports := parseFlow(t, "flowchart LR\n  a e1@--> b\n  b e-2@-->|yes| c\n  c e3@-- no --> d\n  d e4@==> e\n  e e5@-.- f\n  f e6@<--> g\n  g --> a")
	assert.Equal(t, []schema.EdgeSpec{
		{ID: id("e1"), From: "a", To: "b"},
		{ID: id("e-2"), From: "b", To: "c", Label: "yes"},
		{ID: id("e3"), From: "c", To: "d", Label: "no"},
		{ID: id("e4"), From: "d", To: "e", Style: "thick"},
		{ID: id("e5"), From: "e", To: "f", Style: "dotted", Direction: "none"},
		{ID: id("e6"), From: "f", To: "g", Direction: "both"},
		edge("g", "a"),
	}, spec.Edges)
	assert.Empty(t, reports)

	// each hop of a chain takes its own id; without spaces around the arrow too
	spec, _ = parseFlow(t, "graph TD\n  a e1@--> b --> c e3@-->d\n")
	assert.Equal(t, []schema.EdgeSpec{{ID: id("e1"), From: "a", To: "b"}, edge("b", "c"), {ID: id("e3"), From: "c", To: "d"}}, spec.Edges)

	// on a fan-out, the id goes to the edge from the last source to the
	// first target, as Mermaid gives it; the others have none
	spec, _ = parseFlow(t, "graph TD\n  a & b e1@--> c & d\n")
	assert.Equal(t, []schema.EdgeSpec{edge("a", "c"), edge("a", "d"), {ID: id("e1"), From: "b", To: "c"}, edge("b", "d")}, spec.Edges)

	// a repeated id stays with its first edge; the later one has none, as
	// in Mermaid, and the importer says so
	spec, reports = parseFlow(t, "graph TD\n  a e1@--> b\n  b e1@--> c\n")
	assert.Equal(t, []schema.EdgeSpec{{ID: id("e1"), From: "a", To: "b"}, edge("b", "c")}, spec.Edges)
	assert.Equal(t, []Report{{Line: 3, Message: `edge id "e1" already used on line 2, b->c imported without one`}}, reports)

	// an edge's properties (animation, curve) have no diago meaning: dropped
	// with a report, the edge kept
	spec, reports = parseFlow(t, "graph TD\n  a e1@--> b\n  e1@{ animate: true }\n  e1@{ curve: linear };\n")
	assert.Equal(t, []schema.EdgeSpec{{ID: id("e1"), From: "a", To: "b"}}, spec.Edges)
	assert.Equal(t, []Report{
		{Line: 3, Message: `properties of edge "e1" ignored, diago draws no edge animation or curve`},
		{Line: 4, Message: `properties of edge "e1" ignored, diago draws no edge animation or curve`},
	}, reports)
	// "@{" after an id no edge has stays unsupported
	assert.Equal(t, `expected an arrow near "@{ animate: true }"`, errOf(t, "graph TD\n  a e1@--> b\n  e2@{ animate: true }\n").Message)
}

func TestFlowchart_Subgraphs(t *testing.T) {
	// subgraph nodes become children of a group
	spec, _ := parseFlow(t, strings.Join([]string{"graph TD", "  gw --> api", "  subgraph backend[Backend]", "    api --> db", "  end"}, "\n"))
	assert.Equal(t, []schema.GroupSpec{{ID: "backend", Label: "Backend", Contains: []string{"api", "db"}}}, spec.Groups)
	assert.Equal(t, []schema.NodeSpec{node("gw", "gw", ""), node("api", "api", ""), node("db", "db", "")}, spec.Nodes)

	// a subgraph without a bracket title is labelled by its id; nesting
	spec, _ = parseFlow(t, "graph TD\nsubgraph outer\n  subgraph inner\n    a --> b\n  end\n  c\nend\n")
	assert.Equal(t, []schema.GroupSpec{
		{ID: "outer", Label: "outer", Contains: []string{"inner", "c"}},
		{ID: "inner", Label: "inner", Contains: []string{"a", "b"}},
	}, spec.Groups)

	// the fourth level is flattened into the depth-3 ancestor, reported
	spec, reports := parseFlow(t, "graph TD\nsubgraph l1\nsubgraph l2\nsubgraph l3\nsubgraph l4\n  a --> b\nend\n  c\nend\nend\nend\n")
	assert.Equal(t, []Report{{5, `subgraph "l4" flattened into "l3", depth limit 3`}}, reports)
	assert.Equal(t, []string{"a", "b", "c"}, spec.Groups[2].Contains)
	assert.Len(t, spec.Groups, 3)

	// comments and blank lines are ignored
	spec, _ = parseFlow(t, "graph TD\n\n  %% a comment\n  a --> b %% trailing\n")
	assert.Len(t, spec.Edges, 1)

	// an edge to a subgraph id is rejected
	pe := errOf(t, "graph TD\n  subgraph s\n    a\n  end\n  x --> s\n")
	assert.Equal(t, 5, pe.Line)
	assert.Equal(t, `edges to a subgraph are not supported ("s")`, pe.Message)
}

func TestFlowchart_LossyShapes(t *testing.T) {
	// cylinder, stadium and subroutine brackets parse with clean labels;
	// maps every shape bracket, the four lossy ones reported
	src := "graph TD\n  a[Rect]\n  b(Round)\n  c([Stadium])\n  d{Diamond}\n  e((Circle))\n  f(((Double)))\n  g[(Cylinder)]\n  h[[Subroutine]]\n  i{{Hexagon}}\n  j[/Slant/]\n  k[/Trap\\]\n  a --> b"
	spec, reports := parseFlow(t, src)
	want := []schema.NodeSpec{node("a", "Rect", ""), node("b", "Round", "rounded"), node("c", "Stadium", "rounded"),
		node("d", "Diamond", "diamond"), node("e", "Circle", "circle"), node("f", "Double", "circle"),
		node("g", "Cylinder", "cylinder"), node("h", "Subroutine", ""), node("i", "Hexagon", "hexagon"),
		node("j", "Slant", "parallelogram"), node("k", "Trap", "parallelogram")}
	assert.Equal(t, want, spec.Nodes)
	assert.Equal(t, []Report{
		{4, `stadium "c" rendered as rounded`},
		{7, `doublecircle "f" rendered as circle`},
		{9, `subroutine "h" rendered as rect`},
		{12, `trapezoid "k" rendered as parallelogram`},
	}, reports)
}

func TestFlowchart_Frontmatter(t *testing.T) {
	spec, _ := parseFlow(t, "---\ntitle: Deploy pipeline\n---\ngraph TD\n  a --> b\n")
	assert.Equal(t, "Deploy pipeline", spec.Title)
	assert.Equal(t, []schema.EdgeSpec{edge("a", "b")}, spec.Edges)
	spec, _ = parseFlow(t, "---\ntitle: \"Order: states\"\n---\ngraph TD\n  a --> b\n")
	assert.Equal(t, "Order: states", spec.Title)
	spec, _ = parseFlow(t, "graph TD\n  a --> b\n")
	assert.Equal(t, "", spec.Title)
	// undirected --- edges still parse when frontmatter is present
	spec, _ = parseFlow(t, "---\ntitle: T\n---\ngraph TD\n  a --- b\n")
	assert.Equal(t, []schema.EdgeSpec{{From: "a", To: "b", Direction: "none"}}, spec.Edges)
}

func TestFlowchart_Errors(t *testing.T) {
	// missing header (sniffed as not Mermaid at all)
	assert.Equal(t, "not a Mermaid source", errOf(t, "a --> b").Message)
	// unsupported direction reports the line (plan ruling 7: BT is accepted here)
	pe := errOf(t, "graph XX\n a --> b")
	assert.Equal(t, 1, pe.Line)
	assert.Equal(t, `direction "XX" is not supported (supported: TD, TB, LR, BT, RL)`, pe.Message)
	// unsupported directives fail with a clear message
	for _, d := range []string{"classDef foo fill:#f9f", "style a fill:#bbf", "click a callback", "linkStyle 0 stroke:red", "direction LR"} {
		pe := errOf(t, "graph TD\n  a --> b\n  "+d)
		assert.Equal(t, 3, pe.Line, d)
		assert.Contains(t, pe.Message, "is not supported by diago's flowchart subset", d)
		assert.Contains(t, pe.Message, `"`+strings.Fields(d)[0]+`"`, d)
	}
	// unparseable line reports its content and number
	pe = errOf(t, "graph TD\n  a --> b\n  ???")
	assert.Equal(t, 3, pe.Line)
	assert.Equal(t, `expected a node reference near "???"`, pe.Message)
	pe = errOf(t, "graph TD\n  a => b")
	assert.Equal(t, `expected an arrow near "=> b"`, pe.Message)
	// unbalanced end, unclosed subgraph, duplicate header
	assert.Equal(t, `"end" without a matching "subgraph"`, errOf(t, "graph TD\n  end").Message)
	pe = errOf(t, "graph TD\n  subgraph s\n  a --> b\n")
	assert.Equal(t, `unclosed subgraph "s"`, pe.Message)
	assert.Equal(t, 4, pe.Line)
	assert.Equal(t, "duplicate graph header", errOf(t, "graph TD\ngraph LR\n").Message)
	// a header with nothing after it is a valid, empty translation: the
	// importer never invents nodes; schema.ParseFlow owns "must have at
	// least one node" when such a spec is rendered
	// parseFlow is not used here: its requireValid property does not hold
	// for a node-less spec, which is exactly what this case pins.
	res, err := Parse([]byte("%% c\ngraph TD\n"))
	require.NoError(t, err)
	var spec schema.FlowSpec
	require.NoError(t, json.Unmarshal(res.JSON, &spec))
	assert.Empty(t, spec.Nodes)
}
