package pipeline

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/diff"
	"github.com/oxforge/diago/internal/schema"
)

// example reads the fixture name under testdata/.
func example(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	require.NoError(t, err)
	return b
}

func TestRenderDiff_SVGStatuses(t *testing.T) {
	out, err := RenderDiff(context.Background(), example(t, "flow-shop-v1.json"), example(t, "flow-shop-v2.json"), DiffOptions{})
	require.NoError(t, err)
	s := string(out)
	assert.Contains(t, s, `<g id="node-search" class="added">`)
	assert.Contains(t, s, `<g id="node-notify" class="removed">`)
	assert.Contains(t, s, `<g id="node-app" class="changed">`)
	assert.Contains(t, s, `<g id="node-browser" class="dimmed">`)
	assert.Contains(t, s, `<g id="edge-app-notify" class="removed">`)
	assert.Contains(t, s, `<g id="edge-app-search" class="added">`)
	assert.Contains(t, s, `url(#diago-arrow-added)`)
	assert.Contains(t, s, ".dimmed { opacity: 0.5; }")
	assert.NotContains(t, s, `<metadata`, "diff SVGs carry no anchoring carrier (they are not a render to anchor on)")
	assert.Contains(t, s, `<g id="changes">`)
	assert.Contains(t, s, "changed: node App Service: label &quot;App Server&quot; → &quot;App Service&quot;")
}

func TestRenderDiff_TextPrefixesAndLegend(t *testing.T) {
	out, err := RenderDiff(context.Background(), example(t, "flow-shop-v1.json"), example(t, "flow-shop-v2.json"), DiffOptions{Format: "text"})
	require.NoError(t, err)
	s := string(out)
	// The text profile wraps a node label across the lines of its box, so a
	// stamped label shows as its prefix plus the first wrapped word
	// ("+ Search" / "Index"); the prefix is what the stamping guarantees.
	assert.Contains(t, s, "+ Search")
	assert.Contains(t, s, "- Notifier")
	assert.Contains(t, s, "~ App")
	art, legend, found := strings.Cut(s, "\n\n")
	require.True(t, found, "a blank line separates art and legend")
	assert.NotContains(t, art, "added:")
	assert.Equal(t, []string{
		"added: node Search Index",
		"added: node Event Bus",
		"added: node Billing Worker",
		"added: node Ledger",
		"added: edge App Service -> Search Index",
		"added: edge App Service -> Event Bus",
		"added: edge Event Bus -> Billing Worker",
		"added: edge Billing Worker -> Ledger",
		"removed: node Notifier",
		"removed: edge App Server -> Notifier",
		`changed: node App Service: label "App Server" → "App Service"`,
	}, strings.Split(strings.TrimRight(legend, "\n"), "\n"))
}

func TestRenderDiff_IdenticalHasNoLegend(t *testing.T) {
	v1 := example(t, "flow-shop-v1.json")
	out, err := RenderDiff(context.Background(), v1, v1, DiffOptions{Format: "text"})
	require.NoError(t, err)
	assert.NotContains(t, string(out), "\n\nadded")
	assert.NotContains(t, string(out), "+ ")
	svgOut, err := RenderDiff(context.Background(), v1, v1, DiffOptions{})
	require.NoError(t, err)
	assert.Contains(t, string(svgOut), `class="dimmed"`)
	assert.NotContains(t, string(svgOut), `class="added"`)
}

func TestRenderDiff_Errors(t *testing.T) {
	v1 := example(t, "flow-shop-v1.json")
	seq := example(t, "sequence-relay.json")
	cases := []struct {
		name            string
		before, after   []byte
		opts            DiffOptions
		field, contains string
	}{
		{"mixed types", v1, seq, DiffOptions{}, "after.type", "must match"},
		{"invalid after", v1, []byte(`{"type":"flow","nodes":[{"id":"","label":""}],"edges":[]}`), DiffOptions{}, "after.nodes[0].id", ""},
		{"invalid before", []byte(`{"type":"flow","nodes":[{"id":"a","label":"A","shape":"octagon"}],"edges":[]}`), v1, DiffOptions{}, "before.nodes[0].shape", ""},
		{"invalid after sequence", seq, []byte(`{"type":"sequence","actors":[{"id":"a","label":"A"}],"interactions":[{"from":"a","to":"zz"}]}`), DiffOptions{}, "after.interactions[0].to", "unknown actor"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := RenderDiff(context.Background(), tc.before, tc.after, tc.opts)
			var ve schema.ValidationErrors
			require.True(t, errors.As(err, &ve), "want ValidationErrors, got %v", err)
			found := false
			for _, e := range ve {
				if e.Field == tc.field && strings.Contains(e.Message, tc.contains) {
					found = true
				}
			}
			assert.True(t, found, "want field %q containing %q in %v", tc.field, tc.contains, ve)
		})
	}

	_, err := RenderDiff(context.Background(), v1, v1, DiffOptions{Format: "drawio"})
	require.ErrorContains(t, err, "not supported for diff")
	var ve schema.ValidationErrors
	assert.False(t, errors.As(err, &ve), "an unsupported format is a usage error, not the spec's")
}

// TestRenderDiff_SpecTheme: a diff uses the Theme option when set, else the
// after spec's theme field, else default; the before spec's field is not
// read. A theme the after spec names that does not load is a validation
// error on after.theme, in text art too.
func TestRenderDiff_SpecTheme(t *testing.T) {
	ctx := context.Background()
	v1, v2 := example(t, "flow-shop-v1.json"), example(t, "flow-shop-v2.json")
	withTheme := func(spec []byte, theme string) []byte {
		return []byte(strings.Replace(string(spec), "{", `{"theme":"`+theme+`",`, 1))
	}
	render := func(before, after []byte, theme string) string {
		t.Helper()
		out, err := RenderDiff(ctx, before, after, DiffOptions{Theme: theme})
		require.NoError(t, err)
		return string(out)
	}
	midnight := render(v1, v2, "midnight")
	assert.NotEqual(t, render(v1, v2, ""), midnight, "the two themes draw differently")
	assert.Equal(t, midnight, render(v1, withTheme(v2, "midnight"), ""), "the after spec's field")
	assert.Equal(t, render(v1, v2, ""), render(withTheme(v1, "midnight"), v2, ""), "not the before spec's")
	assert.Equal(t, render(v1, v2, "dark"), render(v1, withTheme(v2, "midnight"), "dark"), "the option over the field")

	_, err := RenderDiff(ctx, v1, withTheme(v2, "nope"), DiffOptions{})
	var ve schema.ValidationErrors
	require.ErrorAs(t, err, &ve)
	require.Len(t, ve, 1)
	assert.Equal(t, "after.theme", ve[0].Field)
	_, err = RenderDiff(ctx, v1, withTheme(v2, "nope"), DiffOptions{Format: "text"})
	require.ErrorAs(t, err, &ve, "text art takes no theme, but the name must load")
	assert.Equal(t, "after.theme", ve[0].Field)
}

// The union layout's property: removed elements take real space, so no
// removed node overlaps any other node.
func TestRenderDiff_NoRemovedNodeOverlaps(t *testing.T) {
	pairs := [][2]string{{"flow-shop-v1.json", "flow-shop-v2.json"}, {"flow-shop-v2.json", "flow-shop-v3.json"}, {"flow-shop-v3.json", "flow-shop-v1.json"}}
	for _, p := range pairs {
		for _, text := range []bool{false, true} {
			in, err := parseDiffInputs(example(t, p[0]), example(t, p[1]))
			require.NoError(t, err)
			require.Equal(t, "flow", in.kind)
			u := diff.BuildUnion(in.flowBefore, in.flowAfter)
			th, _ := resolveTheme("default")
			pg, _, err := layoutFlowDiff(context.Background(), u, in.flowBefore, in.flowAfter, th, text)
			require.NoError(t, err)
			for _, a := range pg.Nodes {
				if u.Nodes[a.ID] != diff.Removed {
					continue
				}
				for _, b := range pg.Nodes {
					if a.ID == b.ID {
						continue
					}
					overlap := a.X-a.Width/2 < b.X+b.Width/2 && b.X-b.Width/2 < a.X+a.Width/2 &&
						a.Y-a.Height/2 < b.Y+b.Height/2 && b.Y-b.Height/2 < a.Y+a.Height/2
					assert.False(t, overlap, "%s: removed %s overlaps %s (text=%v)", p, a.ID, b.ID, text)
				}
			}
		}
	}
}

func TestRenderDiff_SequenceSVGStatuses(t *testing.T) {
	out, err := RenderDiff(context.Background(), example(t, "sequence-checkout-v1.json"), example(t, "sequence-checkout-v2.json"), DiffOptions{})
	require.NoError(t, err)
	s := string(out)
	assert.Contains(t, s, `<g id="actor-ledger" class="added">`)
	assert.Contains(t, s, `<g id="lifeline-ledger" class="added">`)
	assert.Contains(t, s, `<g id="actor-shopper" class="dimmed">`)
	assert.Contains(t, s, `<g id="interaction-0" class="changed">`)
	assert.Contains(t, s, `<g id="interaction-5" class="added">`)
	assert.Contains(t, s, `<g id="interaction-1" class="dimmed">`)
	assert.Contains(t, s, `<g id="fragment-0" class="changed">`)
	assert.Contains(t, s, `<g id="fragment-0-section-0">`)
	assert.Contains(t, s, `<g id="fragment-0-section-2" class="added">`)
	assert.Contains(t, s, `url(#diago-arrow-open-added)`)
	assert.Contains(t, s, `url(#diago-arrow-changed)`)
	assert.Contains(t, s, ".added line {")
	assert.NotContains(t, s, "<metadata")
	assert.Contains(t, s, "changed: message Shopper -&gt; Storefront: label &quot;checkout&quot; → &quot;confirm checkout&quot;")

	// Reversed: v2 -> v1 pins the removed actor, message and section.
	out, err = RenderDiff(context.Background(), example(t, "sequence-checkout-v2.json"), example(t, "sequence-checkout-v1.json"), DiffOptions{})
	require.NoError(t, err)
	s = string(out)
	assert.Contains(t, s, `<g id="actor-ledger" class="removed">`)
	assert.Contains(t, s, `<g id="lifeline-ledger" class="removed">`)
	assert.Contains(t, s, `<g id="interaction-5" class="removed">`)
	assert.Contains(t, s, `<g id="fragment-0-section-2" class="removed">`)
	assert.Contains(t, s, "url(#diago-arrow-open-removed)", "a removed async message takes the removed open marker")
}

func TestRenderDiff_SequenceTextPrefixesAndLegend(t *testing.T) {
	out, err := RenderDiff(context.Background(), example(t, "sequence-checkout-v1.json"), example(t, "sequence-checkout-v2.json"), DiffOptions{Format: "text"})
	require.NoError(t, err)
	s := string(out)
	for _, want := range []string{"+ Ledger", "~ confirm checkout", "+ record sale", "+ card on hold", "[~ approved]", "[+ flagged]", "[refused]"} {
		assert.Contains(t, s, want)
	}
	art, legend, found := strings.Cut(s, "\n\n")
	require.True(t, found)
	assert.NotContains(t, art, "added:")
	assert.Equal(t, []string{
		"added: actor Ledger",
		`added: message Storefront -> Ledger "record sale"`,
		`added: message Payment API -> Storefront "flagged"`,
		`added: message Storefront -> Shopper "card on hold"`,
		"added: section [flagged] of alt",
		`changed: message Shopper -> Storefront: label "checkout" → "confirm checkout"`,
	}, strings.Split(strings.TrimRight(legend, "\n"), "\n"))
}

func TestRenderDiff_SequenceIdenticalHasNoLegend(t *testing.T) {
	v1 := example(t, "sequence-checkout-v1.json")
	out, err := RenderDiff(context.Background(), v1, v1, DiffOptions{Format: "text"})
	require.NoError(t, err)
	assert.NotContains(t, string(out), "\n\nadded")
	assert.NotContains(t, string(out), "+ ")
	svgOut, err := RenderDiff(context.Background(), v1, v1, DiffOptions{})
	require.NoError(t, err)
	assert.Contains(t, string(svgOut), `class="dimmed"`)
	assert.NotContains(t, string(svgOut), `class="added"`)
}

func TestRenderDiff_ClassSVGStatuses(t *testing.T) {
	out, err := RenderDiff(context.Background(), example(t, "class-library.json"), example(t, "class-library-v2.json"), DiffOptions{})
	require.NoError(t, err)
	s := string(out)
	assert.Contains(t, s, `<g id="node-hold"><g class="added">`)
	assert.Contains(t, s, `url(#diago-uml-diamond-added)`, "the added aggregation carries a status-coloured adornment")
	assert.Contains(t, s, "text.member.removed { fill: #e03131; text-decoration: line-through; }")
	assert.NotContains(t, s, `<metadata`, "diff SVGs carry no anchoring carrier")
	assert.Contains(t, s, `<g id="changes">`)

	assert.Contains(t, nodeGroup(t, s, "fine"), `class="member removed"`)
	assert.Contains(t, nodeGroup(t, s, "copy"), `class="member changed"`)
	hold := nodeGroup(t, s, "hold")
	assert.Contains(t, hold, `class="members"`)
	assert.Contains(t, hold, `class="member added"`)
	assert.NotContains(t, hold, `class="member removed"`)
}

// nodeGroup returns the <g id="node-<id>"> subtree of an SVG.
func nodeGroup(t *testing.T, svg, id string) string {
	t.Helper()
	_, rest, found := strings.Cut(svg, `<g id="node-`+id+`">`)
	require.True(t, found, "node-%s not found", id)
	end := strings.Index(rest, `<g id="node-`)
	if end < 0 {
		end = len(rest)
	}
	return rest[:end]
}

func TestRenderDiff_ClassTextMarksAndFooter(t *testing.T) {
	out, err := RenderDiff(context.Background(), example(t, "class-library.json"), example(t, "class-library-v2.json"), DiffOptions{Format: "text"})
	require.NoError(t, err)
	s := string(out)
	art, legend, found := strings.Cut(s, "\n\n")
	require.True(t, found, "a blank line separates art and legend")
	assert.Contains(t, art, "- - amount: Money", "a removed member keeps its line with a removed column")
	assert.Contains(t, art, "~ + fee: Money", "a changed member shows the after side with a changed column")
	assert.Contains(t, art, "  - id: string", "an unchanged member gets a blank column")
	assert.Contains(t, art, "+ Hold")
	assert.Equal(t, []string{
		"added: class Hold",
		"added: relation Loan -> Hold",
		`changed: relation Member -> LibraryCard: to_card "1..*" → "0..*"`,
		"changed: class BookCopy, changed attribute + fee: Money",
		"changed: class Fine, removed attribute - amount: Money",
	}, strings.Split(strings.TrimRight(legend, "\n"), "\n"))
}
