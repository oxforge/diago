package pipeline

import (
	"context"
	"errors"
	"slices"
	"strings"
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

// In text art a frame's header runs (its tab, its guards) never cover a
// lifeline where it crosses the frame, and a frame starts right of every
// lifeline left of its span.
func TestRenderWithOptions_TextFrameHeadersKeepTheLifelines(t *testing.T) {
	for _, tc := range []struct {
		name, spec string
		actors     int
		outside    int // how many leftmost lifelines lie left of the frame
	}{
		{"guard over the first actor", `{"type":"sequence","actors":[{"id":"c","label":"Client"},{"id":"s","label":"Server"}],"interactions":[{"from":"c","to":"s","label":"try"},{"from":"s","to":"c","label":"fail"}],"fragments":[{"type":"loop","over":["c","s"],"sections":[{"label":"retry < 3","start":0,"end":1}]}]}`, 2, 0},
		{"guard right of the first actor", `{"type":"sequence","actors":[{"id":"a","label":"A"},{"id":"b","label":"B"},{"id":"c","label":"C"}],"interactions":[{"from":"b","to":"c","label":"get"},{"from":"c","to":"b","label":"miss"}],"fragments":[{"type":"opt","over":["b","c"],"sections":[{"label":"cache miss","start":0,"end":1}]}]}`, 3, 1},
		{"two guards", `{"type":"sequence","actors":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"interactions":[{"from":"a","to":"b","label":"x"},{"from":"b","to":"a","label":"y"}],"fragments":[{"type":"alt","over":["a","b"],"sections":[{"label":"valid token","start":0,"end":0},{"label":"expired token","start":1,"end":1}]}]}`, 2, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := RenderWithOptions(context.Background(), []byte(tc.spec), Options{Format: "text"})
			require.NoError(t, err)
			rows := strings.Split(string(out), "\n")
			var lifelines []int // the lifeline columns, on the row under the headers
			for i, r := range []rune(rows[3]) {
				if r == '│' {
					lifelines = append(lifelines, i)
				}
			}
			require.Len(t, lifelines, tc.actors, "lifelines on the row under the headers:\n%s", out)
			for _, row := range rows {
				rs := []rune(row)
				left := slices.IndexFunc(rs, func(r rune) bool { return r == '╔' || r == '╟' })
				if left < 0 {
					continue
				}
				right := slices.IndexFunc(rs[left+1:], func(r rune) bool { return r == '╗' || r == '╢' }) + left + 1
				for k, c := range lifelines {
					if k < tc.outside {
						assert.Less(t, c, left, "lifeline %d lies left of the frame: %q", k, row)
						continue
					}
					if c > left && c < right {
						assert.Equal(t, '╪', rs[c], "lifeline %d crosses the frame on %q", k, row)
					}
				}
			}
		})
	}
}
