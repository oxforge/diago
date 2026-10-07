package svg

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAdjustGroupFill(t *testing.T) {
	tests := []struct {
		name     string
		baseFill string
		depth    int
	}{
		{"depth 1 unchanged", "#f0f0f0", 1},
		{"depth 2 light fill darkens", "#f0f0f0", 2},
		{"depth 3 light fill darkens more", "#f0f0f0", 3},
		{"depth 1 dark unchanged", "#1a1a2e", 1},
		{"depth 2 dark fill lightens", "#1a1a2e", 2},
		{"depth 3 dark fill lightens more", "#1a1a2e", 3},
		{"non-hex passthrough", "white", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adjustGroupFill(tt.baseFill, tt.depth)
			if tt.depth == 1 || tt.baseFill == "white" {
				assert.Equal(t, tt.baseFill, got)
			} else {
				assert.NotEqual(t, tt.baseFill, got)
				assert.Regexp(t, `^#[0-9a-f]{6}$`, got)
			}
		})
	}

	// Verify monotonic darkening for light fills.
	d1 := adjustGroupFill("#f0f0f0", 1)
	d2 := adjustGroupFill("#f0f0f0", 2)
	d3 := adjustGroupFill("#f0f0f0", 3)
	assert.Equal(t, "#f0f0f0", d1)
	assert.NotEqual(t, d1, d2)
	assert.NotEqual(t, d2, d3)

	// Verify monotonic lightening for dark fills.
	dk1 := adjustGroupFill("#1a1a2e", 1)
	dk2 := adjustGroupFill("#1a1a2e", 2)
	dk3 := adjustGroupFill("#1a1a2e", 3)
	assert.Equal(t, "#1a1a2e", dk1)
	assert.NotEqual(t, dk1, dk2)
	assert.NotEqual(t, dk2, dk3)
}
