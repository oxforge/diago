package svg

import (
	"fmt"
	"math"
	"strconv"
)

// adjustGroupFill returns a fill color adjusted for the given nesting depth.
// Depth 1 returns the base fill unchanged. Deeper levels lighten dark fills
// or darken light fills to create visual differentiation between nesting levels.
func adjustGroupFill(baseFill string, depth int) string {
	if depth <= 1 || len(baseFill) != 7 || baseFill[0] != '#' {
		return baseFill
	}

	r, _ := strconv.ParseUint(baseFill[1:3], 16, 8)
	g, _ := strconv.ParseUint(baseFill[3:5], 16, 8)
	b, _ := strconv.ParseUint(baseFill[5:7], 16, 8)

	luminance := (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)) / 255.0

	steps := float64(depth - 1)
	var nr, ng, nb float64

	if luminance > 0.5 {
		factor := 1.0 - steps*0.08
		nr = float64(r) * factor
		ng = float64(g) * factor
		nb = float64(b) * factor
	} else {
		factor := steps * 0.05
		nr = float64(r) + (255-float64(r))*factor
		ng = float64(g) + (255-float64(g))*factor
		nb = float64(b) + (255-float64(b))*factor
	}

	clamp := func(v float64) uint8 {
		return uint8(math.Max(0, math.Min(255, math.Round(v))))
	}

	return fmt.Sprintf("#%02x%02x%02x", clamp(nr), clamp(ng), clamp(nb))
}
