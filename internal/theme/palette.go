package theme

import (
	"fmt"
	"math"
	"strconv"
)

// ElementKind determines how colors are derived from an accent.
type ElementKind int

const (
	ElementNode  ElementKind = iota // prominent: tinted fill, strong stroke, contrasting text
	ElementEdge                     // prominent: accent stroke and text
	ElementGroup                    // recessive: subtle fill, muted stroke and text
)

// DerivedColors holds the computed fill, stroke, and text colors for a colored element.
type DerivedColors struct {
	Fill   string // hex color (empty for edges — edges have no fill)
	Stroke string // hex color
	Text   string // hex color
}

// DeriveColors computes fill, stroke, and text colors from an accent hex and background hex.
// The background determines whether we're on a light or dark theme.
func DeriveColors(accent, background string, kind ElementKind) DerivedColors {
	ah, as, al := hexToHSL(accent)
	isLight := isLightBackground(background)

	switch kind {
	case ElementNode:
		return deriveNode(ah, as, al, isLight)
	case ElementEdge:
		return deriveEdge(ah, as, al, isLight)
	case ElementGroup:
		return deriveGroup(ah, as, al, isLight)
	default:
		return deriveNode(ah, as, al, isLight)
	}
}

func deriveNode(h, s, _ float64, isLight bool) DerivedColors {
	if isLight {
		fill := hslToHex(h, s, 0.92)
		stroke := hslToHex(h, s, 0.35)
		text := hslToHex(h, s, 0.25)
		text = ensureContrast(text, fill, true)
		return DerivedColors{Fill: fill, Stroke: stroke, Text: text}
	}
	fill := hslToHex(h, s, 0.18)
	stroke := hslToHex(h, s, 0.55)
	text := hslToHex(h, s, 0.82)
	text = ensureContrast(text, fill, false)
	return DerivedColors{Fill: fill, Stroke: stroke, Text: text}
}

func deriveEdge(h, s, _ float64, isLight bool) DerivedColors {
	if isLight {
		stroke := hslToHex(h, s, 0.45)
		return DerivedColors{Stroke: stroke, Text: stroke}
	}
	stroke := hslToHex(h, s, 0.55)
	return DerivedColors{Stroke: stroke, Text: stroke}
}

func deriveGroup(h, s, _ float64, isLight bool) DerivedColors {
	if isLight {
		fill := hslToHex(h, s, 0.95)
		stroke := hslToHex(h, s*0.5, 0.65)
		text := hslToHex(h, s*0.6, 0.45)
		text = ensureContrast(text, fill, true)
		return DerivedColors{Fill: fill, Stroke: stroke, Text: text}
	}
	fill := hslToHex(h, s, 0.13)
	stroke := hslToHex(h, s*0.5, 0.40)
	text := hslToHex(h, s*0.6, 0.60)
	text = ensureContrast(text, fill, false)
	return DerivedColors{Fill: fill, Stroke: stroke, Text: text}
}

// ensureContrast adjusts text lightness until it meets WCAG AA 4.5:1 against bg.
func ensureContrast(text, bg string, darken bool) string {
	ratio := contrastRatio(text, bg)
	if ratio >= 4.5 {
		return text
	}
	h, s, l := hexToHSL(text)
	for i := 0; i < 20 && ratio < 4.5; i++ {
		if darken {
			l -= 0.05
			if l < 0.05 {
				break
			}
		} else {
			l += 0.05
			if l > 0.95 {
				break
			}
		}
		text = hslToHex(h, s, l)
		ratio = contrastRatio(text, bg)
	}
	return text
}

// contrastRatio returns WCAG contrast ratio between two hex colors.
func contrastRatio(hex1, hex2 string) float64 {
	l1 := relativeLuminance(hex1)
	l2 := relativeLuminance(hex2)
	if l1 < l2 {
		l1, l2 = l2, l1
	}
	return (l1 + 0.05) / (l2 + 0.05)
}

// relativeLuminance returns WCAG relative luminance for a hex color.
func relativeLuminance(hex string) float64 {
	r, g, b := hexToRGB(hex)
	rl := linearize(float64(r) / 255.0)
	gl := linearize(float64(g) / 255.0)
	bl := linearize(float64(b) / 255.0)
	return 0.2126*rl + 0.7152*gl + 0.0722*bl
}

func linearize(v float64) float64 {
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

// isLightBackground returns true if the background is perceptually light.
// Uses 0.5 relative luminance as threshold per spec.
func isLightBackground(hex string) bool {
	return relativeLuminance(hex) > 0.5
}

// --- HSL / RGB conversions ---

func hexToRGB(hex string) (r, g, b uint8) {
	if len(hex) < 7 || hex[0] != '#' {
		return 0, 0, 0
	}
	rv, _ := strconv.ParseUint(hex[1:3], 16, 8)
	gv, _ := strconv.ParseUint(hex[3:5], 16, 8)
	bv, _ := strconv.ParseUint(hex[5:7], 16, 8)
	return uint8(rv), uint8(gv), uint8(bv)
}

func hexToHSL(hex string) (h, s, l float64) {
	r, g, b := hexToRGB(hex)
	rf := float64(r) / 255.0
	gf := float64(g) / 255.0
	bf := float64(b) / 255.0
	max := math.Max(rf, math.Max(gf, bf))
	min := math.Min(rf, math.Min(gf, bf))
	l = (max + min) / 2

	if max == min {
		return 0, 0, l
	}

	d := max - min
	if l > 0.5 {
		s = d / (2.0 - max - min)
	} else {
		s = d / (max + min)
	}

	switch max {
	case rf:
		h = (gf - bf) / d
		if gf < bf {
			h += 6
		}
	case gf:
		h = (bf-rf)/d + 2
	case bf:
		h = (rf-gf)/d + 4
	}
	h /= 6
	return h, s, l
}

func hslToHex(h, s, l float64) string {
	s = math.Max(0, math.Min(1, s))
	l = math.Max(0, math.Min(1, l))
	r, g, b := hslToRGB(h, s, l)
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

func hslToRGB(h, s, l float64) (r, g, b uint8) {
	if s == 0 {
		v := uint8(math.Round(l * 255))
		return v, v, v
	}
	var q float64
	if l < 0.5 {
		q = l * (1 + s)
	} else {
		q = l + s - l*s
	}
	p := 2*l - q
	rf := hueToRGB(p, q, h+1.0/3.0)
	gf := hueToRGB(p, q, h)
	bf := hueToRGB(p, q, h-1.0/3.0)
	return uint8(math.Round(rf * 255)), uint8(math.Round(gf * 255)), uint8(math.Round(bf * 255))
}

func hueToRGB(p, q, t float64) float64 {
	if t < 0 {
		t++
	}
	if t > 1 {
		t--
	}
	if t < 1.0/6.0 {
		return p + (q-p)*6*t
	}
	if t < 1.0/2.0 {
		return q
	}
	if t < 2.0/3.0 {
		return p + (q-p)*(2.0/3.0-t)*6
	}
	return p
}
