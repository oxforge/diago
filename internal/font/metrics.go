// Package font provides embedded font files and text measurement utilities.
// Font metrics are computed from the embedded TTF using golang.org/x/image/font/sfnt,
// so measurement is identical on all platforms with no system font dependency.
package font

import (
	_ "embed"
	"encoding/base64"
	"fmt"
	"math"
	"strings"

	xfont "golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

//go:embed fonts/Inter-Regular.ttf
var interRegularTTF []byte

//go:embed fonts/Pangolin-Regular.ttf
var pangolinRegularTTF []byte

type fontEntry struct {
	font *sfnt.Font
	ttf  []byte
}

var fonts map[string]fontEntry

func init() {
	fonts = make(map[string]fontEntry, 2)
	for name, ttf := range map[string][]byte{
		"Inter":    interRegularTTF,
		"Pangolin": pangolinRegularTTF,
	} {
		f, err := sfnt.Parse(ttf)
		if err != nil {
			// Panic is intentional: embedded font assets are baked in at compile time.
			// A parse failure here means the binary itself is corrupt.
			panic(fmt.Sprintf("font: failed to parse embedded %s: %v", name, err))
		}
		fonts[name] = fontEntry{font: f, ttf: ttf}
	}
}

func resolveFont(family string) *sfnt.Font {
	if entry, ok := fonts[family]; ok {
		return entry.font
	}
	return fonts["Inter"].font
}

// pointsToPixels converts typographic points to SVG user units (pixels at 96 dpi).
// 1pt = 1.333...px at 96 dpi  (96/72 = 1.3333).
func pointsToPixels(pt float64) float64 {
	return pt * 96.0 / 72.0
}

// MeasureText returns the rendered width and line height of text at a given font size
// using the specified font family. sizePt is in typographic points.
// Returns dimensions in SVG user units (pixels at 96 dpi).
// An empty string returns zero width and non-zero height.
// Unknown font families fall back to Inter.
func MeasureText(text string, sizePt float64, family string) (width, height float64) {
	f := resolveFont(family)
	height = textHeightForFont(f, sizePt)
	if text == "" {
		return 0, height
	}

	ppem := fixed.Int26_6(math.Round(pointsToPixels(sizePt) * 64))
	var buf sfnt.Buffer
	totalAdv := fixed.Int26_6(0)

	for _, r := range text {
		gid, err := f.GlyphIndex(&buf, r)
		if err != nil {
			// Unmapped glyph: use advance of space as fallback.
			spaceGID, _ := f.GlyphIndex(&buf, ' ')
			if adv, e2 := f.GlyphAdvance(&buf, spaceGID, ppem, xfont.HintingNone); e2 == nil {
				totalAdv += adv
			}
			continue
		}
		adv, err := f.GlyphAdvance(&buf, gid, ppem, xfont.HintingNone)
		if err == nil {
			totalAdv += adv
		}
	}

	width = float64(totalAdv) / 64.0
	return width, height
}

// TextHeight returns the line height (ascent + descent) for the given font size using Inter.
// sizePt is in typographic points. Returns pixels at 96 dpi.
func TextHeight(sizePt float64) float64 {
	return textHeightForFont(fonts["Inter"].font, sizePt)
}

func textHeightForFont(f *sfnt.Font, sizePt float64) float64 {
	ppem := fixed.Int26_6(math.Round(pointsToPixels(sizePt) * 64))

	var buf sfnt.Buffer
	metrics, err := f.Metrics(&buf, ppem, xfont.HintingNone)
	if err != nil {
		// Fallback: approximate line height as 1.2× the pixel size.
		return pointsToPixels(sizePt) * 1.2
	}

	ascent := float64(metrics.Ascent) / 64.0
	descent := float64(metrics.Descent) / 64.0
	return ascent + descent
}

// FontFacesCSS returns @font-face CSS blocks for the requested font families.
// The returned string is ready to embed inside an SVG <style> element.
func FontFacesCSS(families []string) string {
	var blocks []string
	for _, name := range families {
		entry, ok := fonts[name]
		if !ok {
			continue
		}
		b64 := base64.StdEncoding.EncodeToString(entry.ttf)
		block := fmt.Sprintf(`@font-face {
  font-family: '%s';
  font-style: normal;
  font-weight: 400;
  src: url('data:font/truetype;base64,%s') format('truetype');
}`, name, b64)
		blocks = append(blocks, block)
	}
	return strings.Join(blocks, "\n")
}

// EmbeddedTTFs returns a map of font family name to raw TTF bytes for all
// embedded fonts. Used by the PNG renderer to supply font files to resvg.
func EmbeddedTTFs() map[string][]byte {
	result := make(map[string][]byte, len(fonts))
	for name, entry := range fonts {
		result[name] = entry.ttf
	}
	return result
}
