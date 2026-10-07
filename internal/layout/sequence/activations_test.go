package sequence

import (
	"encoding/json"
	"testing"

	"github.com/oxforge/diago/internal/font"
	"github.com/oxforge/diago/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const maskSpec = `{"type":"sequence",
 "actors":[{"id":"u","label":"User"},{"id":"w","label":"Web"}],
 "interactions":[
  {"from":"u","to":"w","label":"login"},
  {"from":"w","to":"u","label":"ok","style":"dashed"},
  {"from":"u","to":"w","label":"logout"}]}`

func TestLayoutWithOptions_MaskSkipsActivations(t *testing.T) {
	d, err := schema.ParseSequence([]byte(maskSpec))
	require.NoError(t, err)
	p := ScreenProfile(testFonts)
	p.MeasureText = font.MeasureText
	plain := LayoutWithOptions(*d, p, LayoutOptions{})
	masked := LayoutWithOptions(*d, p, LayoutOptions{InactiveInteractions: []bool{false, true, false}})
	// Unmasked: login opens w, ok closes w and opens u, logout closes u and
	// opens w (closed with the default height). Masked "ok": w stays open
	// until logout closes it; u is never activated.
	require.Len(t, plain.Activations, 3)
	require.Len(t, masked.Activations, 1)
	assert.Equal(t, "w", masked.Activations[0].ActorID)
	assert.Equal(t, plain.Interactions[1].Y, masked.Interactions[1].Y, "the masked row keeps its place")
	assert.Equal(t, plain.Width, masked.Width)
	assert.Equal(t, plain.Height, masked.Height)
}

func TestLayoutWithOptions_NoMaskIsLayoutWithProfile(t *testing.T) {
	d, err := schema.ParseSequence([]byte(maskSpec))
	require.NoError(t, err)
	p := ScreenProfile(testFonts)
	p.MeasureText = font.MeasureText
	a, _ := json.Marshal(LayoutWithProfile(*d, p))
	b, _ := json.Marshal(LayoutWithOptions(*d, p, LayoutOptions{}))
	c, _ := json.Marshal(LayoutWithOptions(*d, p, LayoutOptions{InactiveInteractions: []bool{false, false, false}}))
	assert.Equal(t, string(a), string(b))
	assert.Equal(t, string(a), string(c))
}
