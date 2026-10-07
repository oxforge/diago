package route

import (
	"math"

	"github.com/oxforge/diago/internal/model"
)

// Crossings finds where the routes cross (S8, C15): every point where a
// segment of one edge crosses a segment of another strictly inside both,
// and more than radius from all four of their ends, is recorded once, on
// the later edge, parametrically along its segment. A crossing closer to
// a segment end is a corner, which the renderers leave alone. It runs on
// the final geometry, since the frame (S11) keeps the parameters but not
// the distances.
func Crossings(routes [][]model.Point, radius float64) [][]model.Crossing {
	out := make([][]model.Crossing, len(routes))
	for i, a := range routes {
		for j := i + 1; j < len(routes); j++ {
			b := routes[j]
			for si := 0; si+1 < len(a); si++ {
				for sj := 0; sj+1 < len(b); sj++ {
					if t, ok := cross(a[si], a[si+1], b[sj], b[sj+1], radius); ok {
						out[j] = append(out[j], model.Crossing{SegmentIndex: sj, T: t})
					}
				}
			}
		}
	}
	return out
}

// cross returns the parameter along (b1, b2) where it crosses (a1, a2)
// strictly inside both, more than radius from every end.
func cross(a1, a2, b1, b2 model.Point, radius float64) (float64, bool) {
	dx1, dy1 := a2.X-a1.X, a2.Y-a1.Y
	dx2, dy2 := b2.X-b1.X, b2.Y-b1.Y
	denom := dx1*dy2 - dy1*dx2
	if math.Abs(denom) < 1e-10 {
		return 0, false
	}
	t := ((b1.X-a1.X)*dy2 - (b1.Y-a1.Y)*dx2) / denom
	u := ((b1.X-a1.X)*dy1 - (b1.Y-a1.Y)*dx1) / denom
	if t <= 0 || t >= 1 || u <= 0 || u >= 1 {
		return 0, false
	}
	p := model.Point{X: a1.X + t*dx1, Y: a1.Y + t*dy1}
	for _, q := range [4]model.Point{a1, a2, b1, b2} {
		if math.Hypot(p.X-q.X, p.Y-q.Y) <= radius {
			return 0, false
		}
	}
	return u, true
}
