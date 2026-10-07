package svg

import (
	"strings"
	"testing"

	"github.com/oxforge/diago/internal/model"
	"github.com/oxforge/diago/internal/theme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRender_EmbedsLayoutMetadata(t *testing.T) {
	g := twoNodeGraph()
	g.LayoutHints = model.NewLayoutHints()
	s := g.LayoutHints.Scope("")
	s.Layers["A"], s.Layers["B"] = 0, 1
	s.Order["A"], s.Order["B"] = 0, 0
	g.LayoutHints.Reversed = []string{"x->y#0"}
	g.LayoutHints.Ranked = []string{"A->B#0"}
	out := Render(g, theme.DefaultTheme())
	assert.Contains(t, out, `<metadata id="diago-layout">`)
	assert.Less(t, strings.Index(out, "</defs>"), strings.Index(out, "<metadata"), "metadata follows defs")
	assert.Less(t, strings.Index(out, "<metadata"), strings.Index(out, "<rect"), "metadata precedes the background")
	got, err := ExtractLayoutHints(out)
	require.NoError(t, err)
	assert.Equal(t, g.LayoutHints, got)
}

func TestRender_NoMetadataWithoutHints(t *testing.T) {
	out := Render(twoNodeGraph(), theme.DefaultTheme())
	assert.NotContains(t, out, "<metadata")
	_, err := ExtractLayoutHints(out)
	assert.Error(t, err)
}

func TestExtractLayoutHints_Errors(t *testing.T) {
	_, err := ExtractLayoutHints(`<svg><metadata id="diago-layout">{"version":9,"scopes":{}}</metadata></svg>`)
	assert.ErrorContains(t, err, "unsupported version")
	_, err = ExtractLayoutHints(`<svg><metadata id="diago-layout">not json</metadata></svg>`)
	assert.Error(t, err)
}
