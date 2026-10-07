package pipeline

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/model"
	textrender "github.com/oxforge/diago/internal/render/text"
	"github.com/oxforge/diago/internal/schema"
)

const classSpec = `{"type":"class","legend":true,"classes":[
  {"id":"animal","label":"Animal","stereotype":"abstract","methods":[{"visibility":"+","text":"speak(): string","abstract":true}]},
  {"id":"dog","label":"Dog","attributes":[{"visibility":"-","text":"name: string"}]}],
  "relations":[{"id":"r1","from":"dog","to":"animal","kind":"inheritance","from_card":"*","to_card":"1"}]}`

func TestRenderClass_SVG(t *testing.T) {
	out, err := Render(context.Background(), []byte(classSpec))
	require.NoError(t, err)
	assert.Contains(t, out, `<g id="node-animal">`)
	assert.Contains(t, out, `class="members"`)
	assert.Contains(t, out, `marker-start="url(#diago-uml-triangle)"`)
	assert.Contains(t, out, `<g id="legend">`)
	assert.Contains(t, out, `<metadata id="diago-layout">`, "class renders carry the anchoring carrier")
	themed, err := RenderWithTheme(context.Background(), []byte(classSpec), "midnight")
	require.NoError(t, err)
	assert.Contains(t, themed, `#0f0f14`)
}

func TestRenderClass_TextWithLegendAndAdvisories(t *testing.T) {
	var advs []schema.Advisory
	out, err := RenderWithOptions(context.Background(), []byte(classSpec), Options{Format: "text", Advisories: &advs})
	require.NoError(t, err)
	s := string(out)
	assert.Contains(t, s, "«abstract»")
	assert.Contains(t, s, "+ speak(): string *")
	assert.Contains(t, s, "△")
	art, legend, found := strings.Cut(s, "\n\n")
	require.True(t, found)
	assert.NotContains(t, art, "◁──")
	assert.Equal(t, "◁── inheritance\n", legend)
	assert.Empty(t, advs)
}

func TestRenderClass_PNGAndFormats(t *testing.T) {
	requireResvg(t)
	out, err := RenderWithOptions(context.Background(), []byte(classSpec), Options{Format: "png"})
	require.NoError(t, err)
	assert.Equal(t, "\x89PNG", string(out[:4]))
}

func TestRenderClass_ValidationError(t *testing.T) {
	_, err := Render(context.Background(), []byte(`{"type":"class","classes":[{"id":""}]}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "classes[0].id")
}

func TestRenderClass_PreviousAnchorsAndTypeMustMatch(t *testing.T) {
	var hints model.LayoutHints
	_, err := RenderWithOptions(context.Background(), []byte(classSpec), Options{LayoutHints: &hints})
	require.NoError(t, err)
	assert.Contains(t, hints.Scopes[""].Layers, "animal")
	again, err := RenderWithOptions(context.Background(), []byte(classSpec), Options{Previous: &hints})
	require.NoError(t, err)
	plain, err := Render(context.Background(), []byte(classSpec))
	require.NoError(t, err)
	assert.Equal(t, plain, string(again), "anchoring on its own carrier reproduces the render")

	h, err := ParsePrevious(context.Background(), []byte(classSpec), "class", "", "")
	require.NoError(t, err)
	assert.Contains(t, h.Scopes[""].Layers, "dog")
	_, err = ParsePrevious(context.Background(), []byte(classSpec), "flow", "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `a previous spec must be a flow diagram, got type "class"`)
	_, err = ParsePrevious(context.Background(), []byte(`{"type":"flow","nodes":[{"id":"a","label":"A"}],"edges":[]}`), "class", "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `a previous spec must be a class diagram, got type "flow"`)
}

func TestRenderClass_CardDroppedMessage(t *testing.T) {
	d := textrender.DroppedLabel{ID: "r1", From: "order", To: "item", Label: "1", End: "from"}
	a := textLabelDropped(d)
	assert.Equal(t, schema.RuleTextLabelDropped, a.Rule)
	assert.Equal(t, `cardinality "1" at the from end of relation order->item (r1) could not be placed in text art`, a.Message)
	plain := textLabelDropped(textrender.DroppedLabel{ID: "e", From: "a", To: "b", Label: "x"})
	assert.Equal(t, `label "x" on edge a->b (e) could not be placed in text art`, plain.Message)
}

// requireResvg skips the test unless the resvg binary is on $PATH: PNG
// rasterization shells out to it, and a machine without it must skip
// rather than fail.
func requireResvg(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("resvg"); err != nil {
		t.Skip("resvg not installed, skipping")
	}
}
