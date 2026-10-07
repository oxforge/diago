package font

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMeasureTextNonZero(t *testing.T) {
	w, h := MeasureText("Hello, World!", 14, "Inter")
	assert.Greater(t, w, 0.0, "width should be positive for non-empty text")
	assert.Greater(t, h, 0.0, "height should be positive")
}

func TestMeasureTextEmpty(t *testing.T) {
	w, h := MeasureText("", 14, "Inter")
	assert.Equal(t, 0.0, w, "empty string should have zero width")
	assert.Greater(t, h, 0.0, "empty string should still have non-zero height")
}

func TestMeasureTextProportional(t *testing.T) {
	wWide, _ := MeasureText("WW", 14, "Inter")
	wNarrow, _ := MeasureText("ii", 14, "Inter")
	assert.Greater(t, wWide, wNarrow, "wide glyphs (W) should be wider than narrow glyphs (i)")
}

func TestMeasureTextDeterministic(t *testing.T) {
	w1, h1 := MeasureText("Hello", 14, "Inter")
	w2, h2 := MeasureText("Hello", 14, "Inter")
	assert.Equal(t, w1, w2, "same input should return same width")
	assert.Equal(t, h1, h2, "same input should return same height")
}

func TestMeasureTextScalesWithSize(t *testing.T) {
	w14, _ := MeasureText("Hello", 14, "Inter")
	w28, _ := MeasureText("Hello", 28, "Inter")
	assert.Greater(t, w28, w14, "larger font size should produce wider text")
}

func TestTextHeightPositive(t *testing.T) {
	h := TextHeight(14)
	assert.Greater(t, h, 0.0, "text height should be positive")
}

func TestMeasureTextWithFamily(t *testing.T) {
	wInter, hInter := MeasureText("Hello", 14, "Inter")
	assert.Greater(t, wInter, 0.0)
	assert.Greater(t, hInter, 0.0)

	wPango, hPango := MeasureText("Hello", 14, "Pangolin")
	assert.Greater(t, wPango, 0.0)
	assert.Greater(t, hPango, 0.0)

	assert.NotEqual(t, wInter, wPango, "Inter and Pangolin should produce different widths")
}

func TestMeasureTextUnknownFamilyFallsBackToInter(t *testing.T) {
	wInter, _ := MeasureText("Hello", 14, "Inter")
	wUnknown, _ := MeasureText("Hello", 14, "Unknown")
	assert.Equal(t, wInter, wUnknown, "unknown family should fall back to Inter")
}

func TestFontFacesCSSMultipleFamilies(t *testing.T) {
	css := FontFacesCSS([]string{"Inter", "Pangolin"})
	assert.Contains(t, css, "font-family: 'Inter'")
	assert.Contains(t, css, "font-family: 'Pangolin'")
	assert.Contains(t, css, "base64,")
}

func TestFontFacesCSSOnlyRequested(t *testing.T) {
	css := FontFacesCSS([]string{"Inter"})
	assert.Contains(t, css, "font-family: 'Inter'")
	assert.NotContains(t, css, "font-family: 'Pangolin'")
}
