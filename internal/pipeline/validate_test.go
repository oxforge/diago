package pipeline

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/schema"
)

const okFlow = `{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":[{"from":"a","to":"b"}]}`

// field returns the field of err's only validation error, failing when err
// is not one.
func field(t *testing.T, err error) string {
	t.Helper()
	var ve schema.ValidationErrors
	require.True(t, errors.As(err, &ve), "a validation error, got %v", err)
	require.Len(t, ve, 1)
	return ve[0].Field
}

func TestValidate(t *testing.T) {
	assert.NoError(t, Validate([]byte(okFlow)))
	assert.Equal(t, "theme", field(t, Validate([]byte(`{"type":"flow","theme":"nope","nodes":[{"id":"a","label":"A"}],"edges":[]}`))))
	assert.Equal(t, "nodes[0].shape", field(t, Validate([]byte(`{"type":"flow","nodes":[{"id":"a","label":"A","shape":"oval"}],"edges":[]}`))))
	assert.Equal(t, "", field(t, Validate([]byte(`nope`))))
	assert.Equal(t, "type", field(t, Validate([]byte(`{"nodes":[]}`))))
}

func TestRenderWithOptions_ASpecErrorIsAValidationError(t *testing.T) {
	for _, in := range []string{`nope`, `{"nodes":[]}`, `{"type":"flow","nodes":[{"id":"a","label":"A"}],"edges":[{"from":"a","to":"a","flat":"yes"}]}`} {
		for _, format := range []string{"svg", "text", "drawio"} {
			_, err := RenderWithOptions(context.Background(), []byte(in), Options{Format: format})
			var ve schema.ValidationErrors
			assert.True(t, errors.As(err, &ve), "%s as %s: %v", in, format, err)
		}
	}
}

func TestRenderWithOptions_TextArtChecksTheTheme(t *testing.T) {
	_, err := RenderWithOptions(context.Background(), []byte(`{"type":"flow","theme":"nope","nodes":[{"id":"a","label":"A"}],"edges":[]}`), Options{Format: "text"})
	assert.Equal(t, "theme", field(t, err))
	_, err = RenderWithOptions(context.Background(), []byte(okFlow), Options{Format: "text", Theme: "nope"})
	require.Error(t, err)
	var ve schema.ValidationErrors
	assert.False(t, errors.As(err, &ve), "an unknown -theme is an invocation error, not the spec's")
}

func TestRenderDiff_AnUnsupportedFormatIsAUsageError(t *testing.T) {
	for _, f := range []string{"drawio", "bogus"} {
		_, err := RenderDiff(context.Background(), []byte(okFlow), []byte(okFlow), DiffOptions{Format: f})
		require.Error(t, err, f)
		var ve schema.ValidationErrors
		assert.False(t, errors.As(err, &ve), "%s: %v", f, err)
	}
}

func TestRenderDiff_InvalidJSONNamesItsSide(t *testing.T) {
	_, err := RenderDiff(context.Background(), []byte(`nope`), []byte(okFlow), DiffOptions{})
	assert.Equal(t, "before", field(t, err))
	_, err = RenderDiff(context.Background(), []byte(okFlow), []byte(`nope`), DiffOptions{})
	assert.Equal(t, "after", field(t, err))
	_, err = RenderDiff(context.Background(), []byte(`nope`), []byte(`nope`), DiffOptions{})
	assert.Equal(t, "before", field(t, err), "both invalid: the first side checked")
	_, err = RenderDiff(context.Background(), []byte(`{"nodes":[]}`), []byte(okFlow), DiffOptions{})
	assert.Equal(t, "before.type", field(t, err))
}

func TestRender_StopsWhenTheContextIsDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	seq := `{"type":"sequence","actors":[{"id":"a","label":"A"}],"interactions":[{"from":"a","to":"a"}]}`
	for _, spec := range []string{okFlow, seq} {
		_, err := RenderWithOptions(ctx, []byte(spec), Options{})
		assert.ErrorIs(t, err, context.Canceled, spec)
	}
	_, err := RenderDiff(ctx, []byte(okFlow), []byte(okFlow), DiffOptions{})
	assert.ErrorIs(t, err, context.Canceled)
}
