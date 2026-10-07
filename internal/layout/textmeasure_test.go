package layout

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestRuneMeasure: a rune is one 8 px cell and a line one 16 px row,
// whatever the font; the widest line sets the width.
func TestRuneMeasure(t *testing.T) {
	for _, tc := range []struct {
		text string
		w, h float64
	}{
		{"", 0, 16},
		{"abc", 24, 16},
		{"ab\ncdef\nx", 32, 48},
		{"é→ö", 24, 16},
	} {
		w, h := RuneMeasure(tc.text, 14, "Inter")
		assert.Equal(t, tc.w, w, "%q width", tc.text)
		assert.Equal(t, tc.h, h, "%q height", tc.text)
	}
}
