package svg

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/oxforge/diago/internal/model"
)

const hopRadius = model.HopRadius // a lone hop's radius, and how high every hop rises

// hop is where a wire's arc over its crossings starts and ends, and the
// arc's radius along the wire: hopRadius over one crossing, wider over
// crossings close enough that their hops would overlap.
type hop struct {
	pre, post model.Point
	r         float64
}

// hopsBySegment returns, per segment of points, the hops over its crossings
// in order along it. A crossing too near its segment's ends for a whole arc
// is left out, and crossings whose hops would overlap share one hop.
func hopsBySegment(points []model.Point, crossings []model.Crossing) map[int][]hop {
	type at struct {
		lo, hi float64 // the hop's ends, as distances along the segment
		h      hop
	}
	seg := map[int][]at{}
	for _, c := range crossings {
		si := c.SegmentIndex
		if si < 0 || si >= len(points)-1 {
			continue
		}
		p1, p2 := points[si], points[si+1]
		dx, dy := p2.X-p1.X, p2.Y-p1.Y
		segLen := math.Hypot(dx, dy)
		if segLen < 0.01 || c.T*segLen < hopRadius || (1-c.T)*segLen < hopRadius {
			continue
		}
		ux, uy := dx/segLen, dy/segLen
		cx, cy := p1.X+dx*c.T, p1.Y+dy*c.T
		seg[si] = append(seg[si], at{c.T*segLen - hopRadius, c.T*segLen + hopRadius, hop{
			pre:  model.Point{X: cx - ux*hopRadius, Y: cy - uy*hopRadius},
			post: model.Point{X: cx + ux*hopRadius, Y: cy + uy*hopRadius},
			r:    hopRadius,
		}})
	}
	out := make(map[int][]hop, len(seg))
	for si, hs := range seg {
		sort.SliceStable(hs, func(a, b int) bool { return hs[a].lo < hs[b].lo })
		var merged []at
		for _, x := range hs {
			if n := len(merged); n > 0 && x.lo < merged[n-1].hi {
				m := &merged[n-1]
				if x.hi > m.hi {
					m.hi, m.h.post = x.hi, x.h.post
				}
				m.h.r = (m.hi - m.lo) / 2
				continue
			}
			merged = append(merged, x)
		}
		for _, x := range merged {
			out[si] = append(out[si], x.h)
		}
	}
	return out
}

// writeHop draws h's arc, from its start, where the path stands, to its
// end: a half circle over one crossing, a half ellipse along the wire over
// several, rising no higher than hopRadius so it clears a parallel wire.
func writeHop(b *strings.Builder, h hop) {
	rot := 0.0
	if h.r != hopRadius {
		rot = math.Atan2(h.post.Y-h.pre.Y, h.post.X-h.pre.X) * 180 / math.Pi
	}
	fmt.Fprintf(b, " A%s,%s %s 0 1 %s,%s", ff(h.r), ff(hopRadius), ff(rot), ff(h.post.X), ff(h.post.Y))
}

// polylineWithHops converts a polyline to an SVG path string that hops
// over its crossings (hopsBySegment): an arc over each crossing, or one
// over crossings whose hops would overlap (writeHop).
func polylineWithHops(points []model.Point, crossings []model.Crossing) string {
	if len(points) < 2 {
		return ""
	}
	hops := hopsBySegment(points, crossings)
	var b strings.Builder
	fmt.Fprintf(&b, "M%s,%s", ff(points[0].X), ff(points[0].Y))
	for si := 0; si < len(points)-1; si++ {
		for _, h := range hops[si] {
			fmt.Fprintf(&b, " L%s,%s", ff(h.pre.X), ff(h.pre.Y))
			writeHop(&b, h)
		}
		fmt.Fprintf(&b, " L%s,%s", ff(points[si+1].X), ff(points[si+1].Y))
	}
	return b.String()
}
