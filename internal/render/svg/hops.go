package svg

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/oxforge/diago/internal/model"
)

const hopRadius = 5.0 // radius of the semicircular hop arc

// polylineWithHops converts a polyline to an SVG path string that includes
// semicircular hop arcs at crossing points. The hop arc goes over the
// crossing edge, perpendicular to the segment direction.
func polylineWithHops(points []model.Point, crossings []model.Crossing) string {
	if len(points) < 2 {
		return ""
	}

	// Group crossings by segment index and sort by distance along segment.
	type segCrossing struct {
		x, y float64
		t    float64 // parameter along segment [0, 1]
	}
	segMap := map[int][]segCrossing{}
	for _, c := range crossings {
		si := c.SegmentIndex
		if si < 0 || si >= len(points)-1 {
			continue
		}
		p1 := points[si]
		p2 := points[si+1]
		dx := p2.X - p1.X
		dy := p2.Y - p1.Y
		segLen := math.Hypot(dx, dy)
		if segLen < 0.01 {
			continue
		}
		// Skip hops too close to segment endpoints: the arc would overflow the segment.
		if c.T*segLen < hopRadius || (1-c.T)*segLen < hopRadius {
			continue
		}
		cx := p1.X + (p2.X-p1.X)*c.T
		cy := p1.Y + (p2.Y-p1.Y)*c.T
		segMap[si] = append(segMap[si], segCrossing{cx, cy, c.T})
	}

	// Sort crossings within each segment by t.
	for si := range segMap {
		sort.Slice(segMap[si], func(a, b int) bool {
			return segMap[si][a].t < segMap[si][b].t
		})
	}

	var b strings.Builder
	fmt.Fprintf(&b, "M%s,%s", ff(points[0].X), ff(points[0].Y))

	for si := 0; si < len(points)-1; si++ {
		p1 := points[si]
		p2 := points[si+1]

		crossingsOnSeg := segMap[si]
		if len(crossingsOnSeg) == 0 {
			// No crossings on this segment — straight line.
			fmt.Fprintf(&b, " L%s,%s", ff(p2.X), ff(p2.Y))
			continue
		}

		// Segment direction unit vector.
		dx := p2.X - p1.X
		dy := p2.Y - p1.Y
		segLen := math.Hypot(dx, dy)
		ux := dx / segLen
		uy := dy / segLen

		// Normal vector (perpendicular, pointing "left" of direction).
		nx := -uy
		ny := ux

		for _, c := range crossingsOnSeg {
			// Draw line to just before the crossing.
			preX := c.x - ux*hopRadius
			preY := c.y - uy*hopRadius
			fmt.Fprintf(&b, " L%s,%s", ff(preX), ff(preY))

			// Draw semicircular arc over the crossing.
			// Arc goes from pre-crossing to post-crossing via a control point
			// offset perpendicular to the segment.
			postX := c.x + ux*hopRadius
			postY := c.y + uy*hopRadius

			// SVG arc: A rx ry x-rotation large-arc-flag sweep-flag x y
			// Use sweep-flag=1 for clockwise arc (hops "above" the segment).
			_ = nx
			_ = ny
			fmt.Fprintf(&b, " A%s,%s 0 0 1 %s,%s",
				ff(hopRadius), ff(hopRadius),
				ff(postX), ff(postY))
		}

		// Line to segment end.
		fmt.Fprintf(&b, " L%s,%s", ff(p2.X), ff(p2.Y))
	}

	return b.String()
}
