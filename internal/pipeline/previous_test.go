package pipeline

import (
	"context"
	"errors"
	"testing"

	"github.com/oxforge/diago/internal/model"
	"github.com/oxforge/diago/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePrevious_Errors(t *testing.T) {
	ctx := context.Background()
	cases := map[string][]byte{
		"garbage":          []byte("hello"),
		"svg without meta": []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"></svg>`),
		"wrong version":    []byte(`{"version":7,"scopes":{},"reversed":[]}`),
		"sequence spec":    []byte(`{"type":"sequence","actors":[{"id":"a","label":"A"}],"interactions":[]}`),
		"invalid spec":     []byte(`{"type":"flow","nodes":[{"id":"","label":""}],"edges":[]}`),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParsePrevious(ctx, data, "flow", "", "")
			var ve schema.ValidationErrors
			require.True(t, errors.As(err, &ve), "want ValidationErrors, got %v", err)
			assert.Equal(t, "previous", ve[0].Field)
		})
	}
}

// TestParsePrevious_ASequenceRender: a sequence render discards its
// previous, so ParsePrevious only checks the file's kind for it: an SVG,
// a carrier or a valid sequence spec; anything else, a spec of another
// type or an invalid sequence spec, is an error on field "previous".
func TestParsePrevious_ASequenceRender(t *testing.T) {
	ctx := context.Background()
	seq := `{"type":"sequence","actors":[{"id":"a","label":"A"}],"interactions":[]}`
	for name, data := range map[string]string{
		"sequence spec":    seq,
		"svg without meta": `<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"></svg>`,
		"carrier":          `{"version":1,"scopes":{},"reversed":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			h, err := ParsePrevious(ctx, []byte(data), "sequence", "", "")
			require.NoError(t, err)
			assert.NotNil(t, h)
		})
	}
	for name, data := range map[string]string{
		"garbage":          "hello",
		"flow spec":        `{"type":"flow","nodes":[{"id":"a","label":"A"}],"edges":[]}`,
		"invalid sequence": `{"type":"sequence","actors":[]}`,
		"wrong version":    `{"version":7,"scopes":{},"reversed":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParsePrevious(ctx, []byte(data), "sequence", "", "")
			var ve schema.ValidationErrors
			require.True(t, errors.As(err, &ve), "want ValidationErrors, got %v", err)
			assert.Equal(t, "previous", ve[0].Field)
		})
	}
}

// flatByProfileSpec has a flat edge, e6 (edges[6]), that the screen
// layout ranks and the text layout routes (S9, Flat edges).
const flatByProfileSpec = `{"type":"flow","direction":"DOWN","nodes":[{"id":"start","label":"Start","shape":"circle"},` +
	`{"id":"d1","label":"Valid?","shape":"diamond"},{"id":"d2","label":"Cached?","shape":"diamond"},{"id":"ok","label":"Serve"},` +
	`{"id":"err","label":"Reject"},{"id":"fetch","label":"Fetch"},{"id":"stop","label":"Stop","shape":"circle"}],` +
	`"edges":[{"from":"start","to":"d1","id":"e0"},{"from":"d1","to":"d2","label":"yes","id":"e1"},` +
	`{"from":"d1","to":"err","label":"no","id":"e2"},{"from":"d2","to":"ok","label":"yes","id":"e3"},` +
	`{"from":"d2","to":"fetch","label":"no","id":"e4"},{"from":"ok","to":"stop","id":"e5"},` +
	`{"from":"d1","to":"d2","label":"retry","flat":true,"id":"e6"},{"from":"err","to":"fetch","flat":true,"id":"e7"}]}`

// flatByThemeSpec has a flat edge, edges[0], that the default theme's
// screen layout ranks, n3 between its ends, and the sketch theme's routes.
const flatByThemeSpec = `{"type":"flow","direction":"DOWN","nodes":[{"id":"n1","label":"Node 1 long name","shape":"hexagon"},` +
	`{"id":"n3","label":"Node 3","shape":"hexagon"},{"id":"n8","label":"Node 8","shape":"rounded"}],` +
	`"edges":[{"from":"n1","to":"n8","flat":true}],"ignore":["isolated-node"]}`

// rankedAdvisories are advs' flat-edge-ranked findings.
func rankedAdvisories(advs []schema.Advisory) []schema.Advisory {
	var out []schema.Advisory
	for _, a := range advs {
		if a.Rule == schema.RuleFlatEdgeRanked {
			out = append(out, a)
		}
	}
	return out
}

// TestParsePrevious_ASpecLaysOutAsTheRenderDoes: a previous spec is laid
// out as the render anchored on it lays out its own spec (S13): under the
// text profile for a text render, else under the screen profile of the
// render's theme. So its carrier is the fresh render's own, and a render
// anchored on its own spec draws what the fresh render draws, with the
// fallbacks that layout has and no other: in text none, though the screen
// layout ranks e6; under sketch none, though the default theme's ranks
// edges[0]; on screen e6's, kept.
func TestParsePrevious_ASpecLaysOutAsTheRenderDoes(t *testing.T) {
	kept := []schema.Advisory{{Rule: schema.RuleFlatEdgeRanked, Field: "edges[6]",
		Message: "kept as an ordinary edge from the previous layout (-previous); a render without it tries a side route again"}}
	for _, tt := range []struct {
		name, spec, format, theme string
		otherRanks                string // the field a layout in another profile or theme ranks
		want                      []schema.Advisory
	}{
		{"text", flatByProfileSpec, "text", "", "edges[6]", nil},
		{"sketch", flatByThemeSpec, "svg", "sketch", "edges[0]", nil},
		{"screen", flatByProfileSpec, "svg", "", "", kept},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			spec := []byte(tt.spec)
			if tt.otherRanks != "" {
				var advs []schema.Advisory
				_, err := RenderWithOptions(ctx, spec, Options{Advisories: &advs})
				require.NoError(t, err)
				require.Len(t, rankedAdvisories(advs), 1, "the default screen layout ranks a flat edge")
				require.Equal(t, tt.otherRanks, rankedAdvisories(advs)[0].Field)
			}
			var own model.LayoutHints
			var freshAdvs []schema.Advisory
			fresh, err := RenderWithOptions(ctx, spec, Options{Format: tt.format, Theme: tt.theme, LayoutHints: &own, Advisories: &freshAdvs})
			require.NoError(t, err)

			previous, err := ParsePrevious(ctx, spec, "flow", tt.format, tt.theme)
			require.NoError(t, err)
			assert.Equal(t, &own, previous, "the previous spec's carrier is the render's own layout's")
			var advs []schema.Advisory
			anchored, err := RenderWithOptions(ctx, spec, Options{Format: tt.format, Theme: tt.theme, Previous: previous, Advisories: &advs})
			require.NoError(t, err)
			assert.Equal(t, string(fresh), string(anchored), "anchored on its own spec, the render is the fresh one")
			assert.Equal(t, tt.want, rankedAdvisories(advs))
			assert.Len(t, rankedAdvisories(freshAdvs), len(tt.want), "the fresh render ranks the same edges")
		})
	}
}
