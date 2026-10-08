package svg

import (
	"encoding/xml"
	"fmt"
	"strings"
	"testing"

	"github.com/oxforge/diago/internal/model"
	"github.com/oxforge/diago/internal/theme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func basicSequence() *model.PositionedSequence {
	return &model.PositionedSequence{
		Width:  600,
		Height: 400,
		Actors: []model.PositionedActor{
			{
				ID: "client", Label: "Client",
				BoxX: 40, BoxY: 20, BoxW: 100, BoxH: 40,
				LineX: 90, LineTop: 60, LineBottom: 380,
			},
			{
				ID: "server", Label: "Server",
				BoxX: 260, BoxY: 20, BoxW: 100, BoxH: 40,
				LineX: 310, LineTop: 60, LineBottom: 380,
			},
		},
		Interactions: []model.PositionedInteraction{
			{
				From: "client", To: "server",
				Label: "request",
				Style: model.InteractionSolid,
				Y:     120,
				FromX: 90, ToX: 310,
			},
			{
				From: "server", To: "client",
				Label: "response",
				Style: model.InteractionDashed,
				Y:     180,
				FromX: 310, ToX: 90,
			},
		},
	}
}

func TestRenderSequenceBasicSVGValid(t *testing.T) {
	th := theme.DefaultTheme()
	out := RenderSequence(basicSequence(), th)

	// Output must start with XML declaration.
	assert.True(t, strings.HasPrefix(out, "<?xml"), "output should be XML")

	// Must be parseable XML.
	err := xml.Unmarshal([]byte(out), new(interface{}))
	require.NoError(t, err, "output should be valid XML/SVG")

	// Must contain SVG namespace.
	assert.Contains(t, out, `xmlns="http://www.w3.org/2000/svg"`)

	// Must contain actor labels.
	assert.Contains(t, out, "Client")
	assert.Contains(t, out, "Server")

	// Must contain interaction labels.
	assert.Contains(t, out, "request")
	assert.Contains(t, out, "response")

	// Must include font face.
	assert.Contains(t, out, "@font-face")
}

func TestRenderSequenceHasLifelines(t *testing.T) {
	th := theme.DefaultTheme()
	out := RenderSequence(basicSequence(), th)

	// Lifelines are rendered as dashed lines.
	assert.Contains(t, out, "stroke-dasharray", "lifelines should be dashed")
	// Should have line elements.
	assert.Contains(t, out, "<line")
}

func TestRenderSequenceHasArrows(t *testing.T) {
	th := theme.DefaultTheme()
	out := RenderSequence(basicSequence(), th)

	// Interactions are polylines with marker-end.
	assert.Contains(t, out, "<polyline")
	assert.Contains(t, out, "marker-end")
}

func TestRenderSequenceDeterministic(t *testing.T) {
	th := theme.DefaultTheme()
	seq := basicSequence()
	out1 := RenderSequence(seq, th)
	out2 := RenderSequence(seq, th)
	assert.Equal(t, out1, out2, "same input should produce identical SVG bytes")
}

func TestRenderSequenceWithSelfMessage(t *testing.T) {
	th := theme.DefaultTheme()
	seq := &model.PositionedSequence{
		Width:  400,
		Height: 300,
		Actors: []model.PositionedActor{
			{
				ID: "server", Label: "Server",
				BoxX: 40, BoxY: 20, BoxW: 100, BoxH: 40,
				LineX: 90, LineTop: 60, LineBottom: 280,
			},
		},
		Interactions: []model.PositionedInteraction{
			{
				From: "server", To: "server",
				Label:  "process()",
				Style:  model.InteractionSolid,
				Y:      120,
				IsSelf: true,
				FromX:  90,
				ToX:    90,
				SelfPoints: []model.Point{
					{X: 90, Y: 120},
					{X: 130, Y: 120},
					{X: 130, Y: 150},
					{X: 90, Y: 150},
				},
			},
		},
	}
	out := RenderSequence(seq, th)
	assert.True(t, strings.HasPrefix(out, "<?xml"))
	assert.Contains(t, out, "process()")
	// Self-message should be a polyline with 4 points.
	assert.Contains(t, out, "<polyline")
}

func TestRenderSequenceWithFragment(t *testing.T) {
	th := theme.DefaultTheme()
	seq := basicSequence()
	seq.Fragments = []model.PositionedFragment{
		{
			Type: model.FragmentOpt,
			X:    30, Y: 100,
			Width:  380,
			Height: 110,
			Sections: []model.PositionedFragmentSection{
				{Label: "if ready", Y: 100},
			},
		},
	}
	out := RenderSequence(seq, th)
	assert.Contains(t, out, "[if ready]", "fragment section label should appear")
}

func TestRenderSequenceWithActivationBoxes(t *testing.T) {
	th := theme.DefaultTheme()
	seq := basicSequence()
	seq.Activations = []model.PositionedActivation{
		{ActorID: "server", X: 305, Y: 120, Width: 10, Height: 60},
	}
	out := RenderSequence(seq, th)
	// Activation boxes are rect elements; make sure we have at least the expected rects.
	assert.Contains(t, out, "<rect")
}

func TestOpenArrowMarkerRender(t *testing.T) {
	marker := OpenArrowMarker("test-open", 8, "#ff0000")

	var sb strings.Builder
	marker.Render(&sb)
	got := sb.String()

	assert.Contains(t, got, `id="test-open"`)
	// Open arrowhead uses a path with stroke but no fill.
	assert.Contains(t, got, `fill="none"`)
	assert.Contains(t, got, `stroke="#ff0000"`)
}

func TestRenderSequenceAsyncInteraction(t *testing.T) {
	th := theme.DefaultTheme()
	seq := &model.PositionedSequence{
		Width:  400,
		Height: 300,
		Actors: []model.PositionedActor{
			{
				ID: "a", Label: "A",
				BoxX: 40, BoxY: 20, BoxW: 80, BoxH: 40,
				LineX: 80, LineTop: 60, LineBottom: 280,
			},
			{
				ID: "b", Label: "B",
				BoxX: 240, BoxY: 20, BoxW: 80, BoxH: 40,
				LineX: 280, LineTop: 60, LineBottom: 280,
			},
		},
		Interactions: []model.PositionedInteraction{
			{
				From: "a", To: "b",
				Label: "fire",
				Style: model.InteractionAsync,
				Y:     120,
				FromX: 80, ToX: 280,
			},
		},
	}
	out := RenderSequence(seq, th)
	// Async should reference the open arrow marker.
	assert.Contains(t, out, openArrowMarkerID)
}

func TestRenderSequence_ActorWithColor(t *testing.T) {
	seq := &model.PositionedSequence{
		Actors: []model.PositionedActor{
			{ID: "a", Label: "A", BoxX: 10, BoxY: 10, BoxW: 80, BoxH: 40, LineX: 50, LineTop: 50, LineBottom: 100, Color: "green"},
		},
		Width: 200, Height: 150,
	}
	th := theme.DefaultTheme()
	svg := RenderSequence(seq, th)
	// Colored actor should not use the default theme fill.
	assert.NotContains(t, svg, fmt.Sprintf(`fill="%s"`, th.Actor.Fill), "colored actor should not use default theme fill")
}

func TestRenderSequence_InteractionWithColor(t *testing.T) {
	seq := &model.PositionedSequence{
		Width:  400,
		Height: 300,
		Actors: []model.PositionedActor{
			{ID: "a", Label: "A", BoxX: 40, BoxY: 20, BoxW: 80, BoxH: 40, LineX: 80, LineTop: 60, LineBottom: 280},
			{ID: "b", Label: "B", BoxX: 240, BoxY: 20, BoxW: 80, BoxH: 40, LineX: 280, LineTop: 60, LineBottom: 280},
		},
		Interactions: []model.PositionedInteraction{
			{
				From: "a", To: "b", Label: "call", Style: model.InteractionSolid,
				Y: 120, FromX: 80, ToX: 280, Color: "#e74c3c",
			},
		},
	}
	th := theme.DefaultTheme()
	svg := RenderSequence(seq, th)
	// Should contain a custom arrow marker for the color.
	assert.Contains(t, svg, "diago-arrow-e74c3c", "colored interaction should have a custom arrow marker")
}

// svgNode is a generic XML element, enough to walk a rendered SVG.
type svgNode struct {
	XMLName  xml.Name
	Attrs    []xml.Attr `xml:",any,attr"`
	Content  string     `xml:",chardata"`
	Children []svgNode  `xml:",any"`
}

func (n svgNode) attr(name string) string {
	for _, a := range n.Attrs {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// findByID returns the element whose id attribute is id.
func (n svgNode) findByID(id string) (svgNode, bool) {
	if n.attr("id") == id {
		return n, true
	}
	for _, c := range n.Children {
		if got, ok := c.findByID(id); ok {
			return got, true
		}
	}
	return svgNode{}, false
}

func parseSVG(t *testing.T, out string) svgNode {
	t.Helper()
	var root svgNode
	require.NoError(t, xml.Unmarshal([]byte(out), &root))
	return root
}

// The fragment group itself, never a section group, holds a solid
// pentagon tab at the frame's top-left with the operator inside it, and
// the guards sit where the layout put them.
func TestRenderSequence_FragmentTabHoldsItsOperator(t *testing.T) {
	th := theme.DefaultTheme()
	for _, typ := range []model.FragmentType{model.FragmentAlt, model.FragmentOpt, model.FragmentLoop, model.FragmentPar, model.FragmentBreak} {
		for _, diff := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/diff=%v", typ, diff), func(t *testing.T) {
				seq := basicSequence()
				seq.Fragments = []model.PositionedFragment{{
					Type: typ, X: 30, Y: 90, Width: 380, Height: 120, TabWidth: 40, TabHeight: 24, TabCut: 6,
					Sections: []model.PositionedFragmentSection{
						{Label: "ready", Y: 90, LabelX: 78, LabelY: 102, LabelWidth: 44},
						{Label: "late", Y: 150, LabelX: 38, LabelY: 162, LabelWidth: 36},
					},
				}}
				var out string
				if diff {
					out = RenderSequenceWithOptions(seq, th, seqStatusOpts(th, map[string]string{"fragment:0": "changed", "section:0/1": "added"}))
				} else {
					out = RenderSequence(seq, th)
				}
				frag, ok := parseSVG(t, out).findByID("fragment-0")
				require.True(t, ok, "a fragment-0 group")

				var tab, op *svgNode
				for i, c := range frag.Children {
					switch {
					case c.XMLName.Local == "path" && tab == nil:
						tab = &frag.Children[i]
					case c.XMLName.Local == "text" && c.Content == typ.String():
						op = &frag.Children[i]
					}
				}
				require.NotNil(t, tab, "a tab path directly in the fragment group")
				assert.True(t, strings.HasPrefix(tab.attr("d"), "M 30 90 L 70 90 L 70 "), "the tab starts at the frame's top-left and spans its width: %q", tab.attr("d"))
				assert.Contains(t, tab.attr("d"), " L 30 114 Z", "the tab closes along the frame's left side at its height")
				assert.Empty(t, tab.attr("stroke-dasharray"), "the tab is drawn solid")
				assert.Equal(t, th.Fragment.Stroke, tab.attr("stroke"))
				require.NotNil(t, op, "the operator %q as text directly in the fragment group", typ)
				assert.Equal(t, "102", op.attr("y"), "the operator sits on the tab's middle row")
				assert.Equal(t, fmt.Sprint(th.Fragment.LabelFont.Weight), op.attr("font-weight"))

				for k, want := range [][2]string{{"78", "102"}, {"38", "162"}} {
					guard := "[" + seq.Fragments[0].Sections[k].Label + "]"
					assert.Contains(t, out, fmt.Sprintf(`<text x="%s" y="%s"`, want[0], want[1]), "guard %s at the layout's position", guard)
				}
			})
		}
	}
}
