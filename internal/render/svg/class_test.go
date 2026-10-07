package svg

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/model"
	"github.com/oxforge/diago/internal/theme"
)

func classGraph() *model.PositionedGraph {
	members := &model.PositionedMembers{
		Header:           []model.PositionedLine{{Text: "«abstract»", Stereotype: true}, {Text: "Shape<T>", Italic: true}},
		Attributes:       []model.PositionedLine{{Text: "- id: string"}},
		Methods:          []model.PositionedLine{{Text: "+ area(): float", Italic: true}, {Text: "+ count(): int", Underline: true}},
		HeaderLineHeight: 17, MemberLineHeight: 15,
	}
	return &model.PositionedGraph{
		Width: 300, Height: 300,
		Nodes: []model.PositionedNode{
			{ID: "shape", Label: "Shape", Shape: model.ShapeRect, X: 100, Y: 80, Width: 120, Height: 34 + 27 + 42, Members: members},
			{ID: "sq", Label: "Square", Shape: model.ShapeRect, X: 100, Y: 240, Width: 80, Height: 29, Members: &model.PositionedMembers{Header: []model.PositionedLine{{Text: "Square"}}, HeaderLineHeight: 17, MemberLineHeight: 15}},
		},
		Edges: []model.PositionedEdge{
			{ID: "sq->shape#0", From: "shape", To: "sq", Relation: model.RelationInheritance, Points: []model.Point{{X: 100, Y: 131.5}, {X: 100, Y: 225.5}}},
			{ID: "r2", From: "shape", To: "sq", Relation: model.RelationComposition, Points: []model.Point{{X: 120, Y: 131.5}, {X: 120, Y: 225.5}},
				FromCard: &model.EndLabel{Text: "1", Pos: &model.Point{X: 134, Y: 141}, Width: 8, Height: 15},
				ToCard:   &model.EndLabel{Text: "*", Pos: &model.Point{X: 134, Y: 215}, Width: 8, Height: 15}},
			{ID: "r3", From: "sq", To: "shape", Relation: model.RelationDependency, Directed: true, Style: model.EdgeDashed, Points: []model.Point{{X: 80, Y: 225.5}, {X: 80, Y: 131.5}}},
			{ID: "r4", From: "sq", To: "shape", Relation: model.RelationAssociation, Points: []model.Point{{X: 60, Y: 225.5}, {X: 60, Y: 131.5}}},
		},
	}
}

func TestRenderClassNode_Structure(t *testing.T) {
	th := theme.DefaultTheme()
	out := Render(classGraph(), th)
	// Outer group has no class; the inner group is bare too in a plain render.
	assert.Contains(t, out, `<g id="node-shape"><g>`)
	assert.Contains(t, out, `>«abstract»</text>`)
	assert.Contains(t, out, `font-style="italic"`)
	assert.Contains(t, out, `text-decoration="underline"`)
	assert.Contains(t, out, `<g class="members">`)
	assert.Contains(t, out, `class="member"`)
	assert.NotContains(t, out, `class="member added"`)
	assert.Equal(t, 2, strings.Count(out, `class="compartment-sep"`), "one separator above each non-empty compartment")
	// Square corners: the box rect carries no rx.
	idx := strings.Index(out, `<g id="node-sq">`)
	require.Positive(t, idx)
	assert.NotContains(t, out[idx:idx+200], `rx=`)
	// Header geometry: first header line centered at y0 + 6 + 17/2 with y0 = 80 - 103/2.
	assert.Contains(t, out, `y="43`)
}

func TestRenderClassNode_StatusSplit(t *testing.T) {
	th := theme.DefaultTheme()
	opts := &RenderOptions{
		Class: func(kind, id string) string {
			switch {
			case kind == "node" && id == "shape":
				return "changed"
			case kind == "member" && id == "shape/methods/1":
				return "added"
			case kind == "member" && id == "shape/attributes/0":
				return "removed"
			}
			return ""
		},
	}
	out := RenderWithOptions(classGraph(), th, opts)
	assert.Contains(t, out, `<g id="node-shape"><g class="changed">`)
	assert.Contains(t, out, `class="member removed"`)
	assert.Contains(t, out, `class="member added"`)
	assert.Regexp(t, `class="member"[^>]*>\+ area\(\): float<`, out, "a same member keeps the bare class")
}

func TestRenderClass_MarkersAndShortening(t *testing.T) {
	th := theme.DefaultTheme()
	out := Render(classGraph(), th)
	assert.Contains(t, out, `<marker id="diago-uml-triangle" viewBox="-1 -1 14 16" refX="0" refY="7" markerWidth="14" markerHeight="16" orient="auto-start-reverse" markerUnits="userSpaceOnUse"><path d="M 0 0 L 12 7 L 0 14 Z" fill="none" stroke="#6c757d" stroke-width="1.5"/></marker>`)
	assert.Contains(t, out, `<marker id="diago-uml-diamond-filled" viewBox="-1 -1 16 14" refX="0" refY="6" markerWidth="16" markerHeight="14" orient="auto-start-reverse" markerUnits="userSpaceOnUse"><path d="M 0 6 L 7 0 L 14 6 L 7 12 Z" fill="#6c757d" stroke="#6c757d" stroke-width="1.5"/></marker>`)
	assert.Contains(t, out, `<marker id="diago-uml-diamond" `)
	// Inheritance: wire starts 12 px down the segment, triangle at the start, no arrowhead.
	assert.Contains(t, out, `<polyline points="100,143.5 100,225.5" fill="none" stroke="#6c757d" stroke-width="1.5" marker-start="url(#diago-uml-triangle)"/>`)
	// Composition: 14 px, filled diamond.
	assert.Contains(t, out, `<polyline points="120,145.5 120,225.5" fill="none" stroke="#6c757d" stroke-width="1.5" marker-start="url(#diago-uml-diamond-filled)"/>`)
	// Dependency: dashed, arrowhead at the target, full length.
	assert.Contains(t, out, `<polyline points="80,225.5 80,131.5" fill="none" stroke="#6c757d" stroke-width="1.5" stroke-dasharray="8,4" marker-end="url(#diago-arrow)"/>`)
	// Undirected association: no marker at all.
	assert.Contains(t, out, `<polyline points="60,225.5 60,131.5" fill="none" stroke="#6c757d" stroke-width="1.5"/>`)
	// Cards in the edge-label font at their centers.
	assert.Contains(t, out, `<text x="134" y="141" font-family="Inter" font-size="12" font-weight="400" fill="#495057" text-anchor="middle" dominant-baseline="central">1</text>`)
}

func TestRenderClass_ShorteningOnEveryPathKind(t *testing.T) {
	pts := []model.Point{{X: 0, Y: 0}, {X: 0, Y: 100}, {X: 50, Y: 100}}
	assert.Equal(t, []model.Point{{X: 0, Y: 12}, {X: 0, Y: 100}, {X: 50, Y: 100}}, shortenStart(pts, 12))
	assert.Equal(t, []model.Point{{X: 0, Y: 5}, {X: 0, Y: 5}}, shortenStart([]model.Point{{X: 0, Y: 0}, {X: 0, Y: 5}}, 12), "capped at the first segment's length")
	assert.Equal(t, pts, shortenStart(pts, 0))
	sk, err := theme.Load("sketch")
	require.NoError(t, err)
	out := Render(classGraph(), sk)
	assert.Contains(t, out, `marker-start="url(#diago-uml-triangle)"`)
	hop := classGraph()
	hop.Edges[0].Crossings = []model.Crossing{{SegmentIndex: 0, T: 0.5}}
	out = Render(hop, theme.DefaultTheme())
	assert.Contains(t, out, `d="M100,143.5 `)
	// The crossing is parametric against the UNSHORTENED polyline, so its
	// absolute position is (100, 131.5 + 0.5*94) = (100, 178.5). Shortening
	// the first segment must not drag the hop arc with it: the arc still
	// straddles y=178.5 by hopRadius on either side.
	p0, p1 := hop.Edges[0].Points[0], hop.Edges[0].Points[1]
	absY := p0.Y + (p1.Y-p0.Y)*0.5
	assert.Contains(t, out, ` L100,`+ff(absY-hopRadius)+` A5,5 0 0 1 100,`+ff(absY+hopRadius)+` `,
		"hop arc stays on the crossing's absolute position after start shortening")
	// A crossing that the shortening swallowed is dropped, not re-projected.
	swallowed := classGraph()
	swallowed.Edges[0].Crossings = []model.Crossing{{SegmentIndex: 0, T: 0.05}}
	out = Render(swallowed, theme.DefaultTheme())
	assert.NotContains(t, out, ` A5,5 `, "a crossing before the shortened start is dropped")
}

// elementString renders one element to a string for assertions.
func elementString(e Element) string {
	var b strings.Builder
	e.Render(&b)
	return b.String()
}

func TestRenderClassNode_NilGuards(t *testing.T) {
	th := theme.DefaultTheme()
	style := th.Node
	// No Members: fall back to the plain shape render, not a nil dereference.
	plain := model.PositionedNode{ID: "n", Label: "N", Shape: model.ShapeRect, X: 50, Y: 50, Width: 80, Height: 40}
	assert.Equal(t, elementString(RenderNode(plain, style, false)), elementString(RenderClassNode(plain, style, th.Class, false, "", nil)))
	// Members but no memberClass: every line keeps the bare class.
	withMembers := classGraph().Nodes[0]
	out := elementString(RenderClassNode(withMembers, style, th.Class, false, "", nil))
	assert.Contains(t, out, `class="member"`)
	assert.NotContains(t, out, `class="member `)
}

func TestRenderClass_AccentAndStatusMarkers(t *testing.T) {
	th := theme.DefaultTheme()
	g := classGraph()
	g.Edges[0].Color = "red"
	out := Render(g, th)
	dc := theme.DeriveColors(th.Colors["red"], th.Background, theme.ElementEdge)
	assert.Contains(t, out, `<marker id="diago-uml-triangle-`+th.Colors["red"][1:]+`"`)
	assert.Contains(t, out, `stroke="`+dc.Stroke+`"`)
	assert.Contains(t, out, `marker-start="url(#diago-uml-triangle-`+th.Colors["red"][1:]+`)"`)
	opts := &RenderOptions{
		Class: func(kind, id string) string {
			if kind == "edge" && id == "r2" {
				return "added"
			}
			return ""
		},
		MarkerFills: map[string]string{"added": th.Diff.Added},
	}
	out = RenderWithOptions(classGraph(), th, opts)
	assert.Contains(t, out, `<marker id="diago-uml-diamond-filled-added"`)
	assert.Contains(t, out, `fill="`+th.Diff.Added+`" stroke="`+th.Diff.Added+`"`)
	assert.Contains(t, out, `marker-start="url(#diago-uml-diamond-filled-added)"`)
}

func TestRenderClass_Legend(t *testing.T) {
	th := theme.DefaultTheme()
	g := classGraph()
	g.Legend = []model.LegendEntry{{Kind: "composition", Label: "composition"}, {Kind: "dependency", Label: "depends on"}, {Kind: "inheritance", Label: "is-a"}, {Kind: "association", Label: "association"}, {Kind: "realization", Label: "implements"}}
	out := Render(g, th)
	_, lh := legendSize(g.Legend, th)
	assert.Equal(t, 5*22+20.0, lh)
	assert.Contains(t, out, `height="`+ff(300+lh)+`"`)
	assert.Contains(t, out, `<g id="legend">`)
	// Row 0 center y = 300 + 10 + 11 = 321: composition line from 34 to 46 with a filled diamond tip at 20.
	assert.Contains(t, out, `<line x1="34" y1="321" x2="46" y2="321" stroke="#6c757d" stroke-width="1.5"/>`)
	assert.Contains(t, out, `<polygon points="20,321 27,315 34,321 27,327" fill="#6c757d" stroke="#6c757d" stroke-width="1.5"/>`)
	// Row 1: dependency, dashed with an arrowhead at the right end.
	assert.Contains(t, out, `<line x1="20" y1="343" x2="46" y2="343" stroke="#6c757d" stroke-width="1.5" stroke-dasharray="8,4" marker-end="url(#diago-arrow)"/>`)
	// Row 2: inheritance, hollow triangle, line from 32.
	assert.Contains(t, out, `<polygon points="20,365 32,358 32,372" fill="none" stroke="#6c757d" stroke-width="1.5"/>`)
	assert.Contains(t, out, `<text x="54" y="365" font-family="Inter" font-size="12" font-weight="400" fill="#495057" dominant-baseline="central">is-a</text>`)
	// Row 3: association, plain line only.
	assert.Contains(t, out, `<line x1="20" y1="387" x2="46" y2="387" stroke="#6c757d" stroke-width="1.5"/>`)
	// Row 4: realization, dashed with a hollow triangle.
	assert.Contains(t, out, `<line x1="32" y1="409" x2="46" y2="409" stroke="#6c757d" stroke-width="1.5" stroke-dasharray="8,4"/>`)
	// Width grows to fit a long label.
	g.Legend = []model.LegendEntry{{Kind: "association", Label: strings.Repeat("association ", 8)}}
	out = Render(g, th)
	lw, _ := legendSize(g.Legend, th)
	assert.Greater(t, lw, 300.0)
	assert.Contains(t, out, `width="`+ff(lw)+`"`)
	assert.Contains(t, out, `<g id="legend">`)
}

func TestRenderClass_FlowRenderUnchanged(t *testing.T) {
	// A graph without class data emits no UML markers, no legend and no members group.
	g := &model.PositionedGraph{Width: 100, Height: 100, Nodes: []model.PositionedNode{{ID: "a", Label: "A", X: 50, Y: 50, Width: 40, Height: 20}}}
	out := Render(g, theme.DefaultTheme())
	assert.NotContains(t, out, "diago-uml")
	assert.NotContains(t, out, `id="legend"`)
	assert.NotContains(t, out, `class="members"`)
}
