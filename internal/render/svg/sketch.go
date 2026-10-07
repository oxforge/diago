package svg

import (
	"fmt"
	"hash/fnv"
	"math"
	"math/rand"
	"strings"

	"github.com/oxforge/diago/internal/model"
)

// seedRNG creates a deterministic PRNG seeded from the given string ID using FNV hash.
func seedRNG(id string) *rand.Rand {
	h := fnv.New64a()
	h.Write([]byte(id))
	return rand.New(rand.NewSource(int64(h.Sum64())))
}

// sketchSegSize is the target length (px) of each sub-segment when subdividing lines.
const sketchSegSize = 20.0

// sketchWobble is the maximum perpendicular displacement (px) for each subdivision point.
const sketchWobble = 0.75

// sketchDistortionFilterID is the SVG filter ID for the sketch distortion effect.

// sketchDistortionFilterRef is the filter attribute value referencing the sketch distortion filter.
const sketchDistortionFilterRef = "url(#sketch-distortion)"

// sketchDistortionFilter returns the SVG filter definition XML for a subtle
// hand-drawn distortion effect using feTurbulence + feDisplacementMap.
// The seed is fixed for deterministic SVG output.
func sketchDistortionFilter() string {
	return `<filter id="sketch-distortion" filterUnits="userSpaceOnUse" x="-5%" y="-5%" width="110%" height="110%">` +
		`<feTurbulence type="turbulence" baseFrequency="0.03" numOctaves="2" seed="42" result="noise"/>` +
		`<feDisplacementMap in="SourceGraphic" in2="noise" scale="1" xChannelSelector="R" yChannelSelector="G"/>` +
		`</filter>`
}

// sketchLine writes subdivided line commands from (x0,y0) to (x1,y1) with subtle
// perpendicular perturbation at each subdivision point, giving a hand-drawn tremor.
// The start point is assumed to already be the current path position (not emitted).
func sketchLine(sb *strings.Builder, rng *rand.Rand, x0, y0, x1, y1 float64) {
	dx := x1 - x0
	dy := y1 - y0
	segLen := math.Sqrt(dx*dx + dy*dy)

	if segLen < 1.0 {
		fmt.Fprintf(sb, " L %s %s", ff(x1), ff(y1))
		return
	}

	// Perpendicular unit vector.
	px := -dy / segLen
	py := dx / segLen

	// Number of subdivisions.
	n := int(math.Ceil(segLen / sketchSegSize))
	if n < 1 {
		n = 1
	}

	for j := 1; j <= n; j++ {
		t := float64(j) / float64(n)
		ex := x0 + dx*t
		ey := y0 + dy*t

		// Don't perturb the final endpoint — keep it exact.
		if j < n {
			offset := (rng.Float64()*2.0 - 1.0) * sketchWobble
			ex += px * offset
			ey += py * offset
		}

		fmt.Fprintf(sb, " L %s %s", ff(ex), ff(ey))
	}
}

// sketchPolyline converts a polyline into a path with subdivided, slightly wobbly segments.
// The edgeID seeds a deterministic PRNG so the result is reproducible.
// Returns an SVG path d attribute string.
func sketchPolyline(pts []model.Point, edgeID string) string {
	if len(pts) < 2 {
		if len(pts) == 1 {
			return fmt.Sprintf("M %s %s", ff(pts[0].X), ff(pts[0].Y))
		}
		return ""
	}

	rng := seedRNG(edgeID)
	var sb strings.Builder
	fmt.Fprintf(&sb, "M %s %s", ff(pts[0].X), ff(pts[0].Y))

	for i := 0; i < len(pts)-1; i++ {
		sketchLine(&sb, rng, pts[i].X, pts[i].Y, pts[i+1].X, pts[i+1].Y)
	}

	return sb.String()
}

// sketchRect creates a hand-drawn rectangle path with slightly varied corner radii
// and wobbly straight edges.
// Returns an SVG path d attribute string using arcs for corners.
func sketchRect(x, y, w, h, baseRadius float64, elementID string) string {
	rng := seedRNG(elementID)

	// Generate a slightly varied radius for each corner (base +/- 1-2px).
	vary := func() float64 {
		r := baseRadius + (rng.Float64()*4.0 - 2.0) // base +/- 2px
		if r < 0 {
			r = 0
		}
		// Don't let radius exceed half the smallest dimension.
		maxR := math.Min(w/2, h/2)
		if r > maxR {
			r = maxR
		}
		return r
	}

	rTL := vary() // top-left
	rTR := vary() // top-right
	rBR := vary() // bottom-right
	rBL := vary() // bottom-left

	// Build the path: start at top-left corner (after the arc start).
	// Go clockwise: top edge -> top-right arc -> right edge -> bottom-right arc ->
	// bottom edge -> bottom-left arc -> left edge -> top-left arc.
	var sb strings.Builder
	fmt.Fprintf(&sb, "M %s %s", ff(x+rTL), ff(y))

	// Top edge.
	sketchLine(&sb, rng, x+rTL, y, x+w-rTR, y)
	// Top-right corner arc.
	if rTR > 0 {
		fmt.Fprintf(&sb, " A %s %s 0 0 1 %s %s", ff(rTR), ff(rTR), ff(x+w), ff(y+rTR))
	}

	// Right edge.
	sketchLine(&sb, rng, x+w, y+rTR, x+w, y+h-rBR)
	// Bottom-right corner arc.
	if rBR > 0 {
		fmt.Fprintf(&sb, " A %s %s 0 0 1 %s %s", ff(rBR), ff(rBR), ff(x+w-rBR), ff(y+h))
	}

	// Bottom edge.
	sketchLine(&sb, rng, x+w-rBR, y+h, x+rBL, y+h)
	// Bottom-left corner arc.
	if rBL > 0 {
		fmt.Fprintf(&sb, " A %s %s 0 0 1 %s %s", ff(rBL), ff(rBL), ff(x), ff(y+h-rBL))
	}

	// Left edge.
	sketchLine(&sb, rng, x, y+h-rBL, x, y+rTL)
	// Top-left corner arc.
	if rTL > 0 {
		fmt.Fprintf(&sb, " A %s %s 0 0 1 %s %s", ff(rTL), ff(rTL), ff(x+rTL), ff(y))
	}

	sb.WriteString(" Z")
	return sb.String()
}

// sketchPolygon creates a hand-drawn closed polygon path with wobbly edges.
// Returns an SVG path d attribute string.
func sketchPolygon(pts []model.Point, elementID string) string {
	if len(pts) < 2 {
		return ""
	}

	rng := seedRNG(elementID)
	var sb strings.Builder
	fmt.Fprintf(&sb, "M %s %s", ff(pts[0].X), ff(pts[0].Y))

	for i := 0; i < len(pts)-1; i++ {
		sketchLine(&sb, rng, pts[i].X, pts[i].Y, pts[i+1].X, pts[i+1].Y)
	}
	// Close: last point back to first.
	sketchLine(&sb, rng, pts[len(pts)-1].X, pts[len(pts)-1].Y, pts[0].X, pts[0].Y)
	sb.WriteString(" Z")

	return sb.String()
}

// sketchCircle creates a hand-drawn circle path with subtle wobble.
// The circle is approximated as a polygon with many points, each slightly perturbed radially.
// Returns an SVG path d attribute string.
func sketchCircle(cx, cy, r float64, elementID string) string {
	rng := seedRNG(elementID)

	// Use enough segments for a smooth circle (~1 segment per 15px of circumference).
	circumference := 2.0 * math.Pi * r
	n := int(math.Max(24, math.Ceil(circumference/15.0)))

	var sb strings.Builder
	for i := 0; i <= n; i++ {
		angle := 2.0 * math.Pi * float64(i) / float64(n)
		pr := r
		if i < n { // don't perturb last point (same as first)
			pr += (rng.Float64()*2.0 - 1.0) * sketchWobble
		}
		px := cx + pr*math.Cos(angle)
		py := cy + pr*math.Sin(angle)

		if i == 0 {
			fmt.Fprintf(&sb, "M %s %s", ff(px), ff(py))
		} else {
			fmt.Fprintf(&sb, " L %s %s", ff(px), ff(py))
		}
	}
	sb.WriteString(" Z")

	return sb.String()
}

// sketchEllipse creates a hand-drawn ellipse path with subtle radial wobble.
// Returns an SVG path d attribute string.
func sketchEllipse(cx, cy, rx, ry float64, elementID string) string {
	rng := seedRNG(elementID)

	circumference := math.Pi * (3*(rx+ry) - math.Sqrt((3*rx+ry)*(rx+3*ry)))
	n := int(math.Max(24, math.Ceil(circumference/15.0)))

	var sb strings.Builder
	for i := 0; i <= n; i++ {
		angle := 2.0 * math.Pi * float64(i) / float64(n)
		prx := rx
		pry := ry
		if i < n {
			offset := (rng.Float64()*2.0 - 1.0) * sketchWobble
			prx += offset
			pry += offset
		}
		px := cx + prx*math.Cos(angle)
		py := cy + pry*math.Sin(angle)

		if i == 0 {
			fmt.Fprintf(&sb, "M %s %s", ff(px), ff(py))
		} else {
			fmt.Fprintf(&sb, " L %s %s", ff(px), ff(py))
		}
	}
	sb.WriteString(" Z")

	return sb.String()
}

// sketchCylinder creates a hand-drawn cylinder body path with wobbly sides and arcs.
// The body consists of: left side down, bottom elliptical arc, right side up, top elliptical arc.
// Returns an SVG path d attribute string.
func sketchCylinder(cx, hw, topY, botY, ry float64, elementID string) string {
	rng := seedRNG(elementID)

	var sb strings.Builder

	// Start at top-left.
	fmt.Fprintf(&sb, "M %s %s", ff(cx-hw), ff(topY))

	// Left side down.
	sketchLine(&sb, rng, cx-hw, topY, cx-hw, botY)

	// Bottom arc (half ellipse, left to right, curving downward).
	n := 16
	for i := 1; i <= n; i++ {
		// Go from angle π to 0 (left to right, bottom arc).
		angle := math.Pi - math.Pi*float64(i)/float64(n)
		prx := hw
		pry := ry
		if i < n {
			offset := (rng.Float64()*2.0 - 1.0) * sketchWobble
			prx += offset
			pry += offset
		}
		px := cx + prx*math.Cos(angle)
		py := botY + pry*math.Sin(angle)
		fmt.Fprintf(&sb, " L %s %s", ff(px), ff(py))
	}

	// Right side up.
	sketchLine(&sb, rng, cx+hw, botY, cx+hw, topY)

	// Top arc (half ellipse, right to left, curving upward).
	for i := 1; i <= n; i++ {
		// Go from angle 0 to π (right to left, top arc curving up).
		angle := math.Pi * float64(i) / float64(n)
		prx := hw
		pry := ry
		if i < n {
			offset := (rng.Float64()*2.0 - 1.0) * sketchWobble
			prx += offset
			pry += offset
		}
		px := cx + prx*math.Cos(angle)
		py := topY - pry*math.Sin(angle)
		fmt.Fprintf(&sb, " L %s %s", ff(px), ff(py))
	}

	sb.WriteString(" Z")
	return sb.String()
}
