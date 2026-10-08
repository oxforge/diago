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

// Hand-drawn markers. A sketch theme's arrowheads and UML adornments keep
// the clean markers' reference point and scale, so a tip still lands on the
// node border and a wire still meets its adornment's base; their corners
// are jittered, their sides bowed and their joins rounded, by an rng seeded
// from the marker id.

// An arrowhead's corner jitter, side bow and filled outline width, per px
// of arrow size (1, 0.8 and 1 px at the usual 8).
const (
	sketchArrowJitter  = 0.125
	sketchArrowBow     = 0.1
	sketchArrowOutline = 0.125
)

// A UML adornment's corner jitter and side bow, and how far its outline
// runs on past the corner it started from, as a pen does, in px.
const (
	sketchUMLJitter    = 1.0
	sketchUMLBow       = 1.0
	sketchUMLOvershoot = 2.0
)

// sketchJitter returns a value in [-amp, amp].
func sketchJitter(rng *rand.Rand, amp float64) float64 {
	return (rng.Float64()*2.0 - 1.0) * amp
}

// sketchSide writes a quadratic curve from a to b whose control point sits
// off the side's middle by bow, along its normal. place maps marker
// coordinates to the drawing's.
func sketchSide(sb *strings.Builder, a, b model.Point, bow float64, place func(model.Point) model.Point) {
	c := model.Point{X: (a.X + b.X) / 2, Y: (a.Y + b.Y) / 2}
	if l := math.Hypot(b.X-a.X, b.Y-a.Y); l > 0 {
		c.X -= (b.Y - a.Y) / l * bow
		c.Y += (b.X - a.X) / l * bow
	}
	c, b = place(c), place(b)
	fmt.Fprintf(sb, " Q %s %s %s %s", ff(c.X), ff(c.Y), ff(b.X), ff(b.Y))
}

// sketchClosed returns a closed outline through pts, side i (from pts[i]
// to the next corner) bowed by bows[i]. With an overshoot the last side
// runs that far on past pts[0] instead of closing on it, so its fill closes
// the outline implicitly.
func sketchClosed(pts []model.Point, bows []float64, overshoot float64, place func(model.Point) model.Point) string {
	var sb strings.Builder
	p0 := place(pts[0])
	fmt.Fprintf(&sb, "M %s %s", ff(p0.X), ff(p0.Y))
	n := len(pts)
	for i := 0; i < n-1; i++ {
		sketchSide(&sb, pts[i], pts[i+1], bows[i], place)
	}
	last, end := pts[n-1], pts[0]
	if l := math.Hypot(end.X-last.X, end.Y-last.Y); overshoot > 0 && l > 0 {
		end.X += (end.X - last.X) / l * overshoot
		end.Y += (end.Y - last.Y) / l * overshoot
		sketchSide(&sb, last, end, bows[n-1], place)
		return sb.String()
	}
	sketchSide(&sb, last, end, bows[n-1], place)
	sb.WriteString(" Z")
	return sb.String()
}

// asDrawn places marker coordinates as they are.
func asDrawn(p model.Point) model.Point { return p }

// sketchArrowhead is the filled arrowhead of ArrowMarker drawn by hand: its
// tip, rounded by an outline of width outline, reaches (size, size/2).
func sketchArrowhead(id string, size, outline float64) string {
	rng := seedRNG(id)
	j, half := size*sketchArrowJitter, outline/2
	tip := model.Point{X: size - half, Y: size/2 + sketchJitter(rng, j/2)}
	top := model.Point{X: half + sketchJitter(rng, j), Y: half + sketchJitter(rng, j)}
	bottom := model.Point{X: half + sketchJitter(rng, j), Y: size - half + sketchJitter(rng, j)}
	bow := size * sketchArrowBow
	bows := []float64{sketchJitter(rng, bow), sketchJitter(rng, bow), sketchJitter(rng, bow)}
	return sketchClosed([]model.Point{top, tip, bottom}, bows, 0, asDrawn)
}

// sketchOpenArrowhead is the open arrowhead of OpenArrowMarker drawn by
// hand: two strokes of slightly uneven length meeting at a tip that, with
// its round cap, reaches (size, size/2).
func sketchOpenArrowhead(id string, size, strokeWidth float64) string {
	rng := seedRNG(id)
	j, half := size*sketchArrowJitter, strokeWidth/2
	tip := model.Point{X: size - half, Y: size/2 + sketchJitter(rng, j/2)}
	top := model.Point{X: half + sketchJitter(rng, j), Y: half + sketchJitter(rng, j)}
	bottom := model.Point{X: half + sketchJitter(rng, j), Y: size - half + sketchJitter(rng, j)}
	bow := size * sketchArrowBow
	var sb strings.Builder
	for _, arm := range []model.Point{top, bottom} {
		if sb.Len() > 0 {
			sb.WriteString(" ")
		}
		fmt.Fprintf(&sb, "M %s %s", ff(arm.X), ff(arm.Y))
		sketchSide(&sb, arm, tip, sketchJitter(rng, bow), asDrawn)
	}
	return sb.String()
}

// sketchTriangle is TriangleMarker's hollow triangle drawn by hand, base at
// x = 0 on the wire's start and tip, rounded by the stroke, reaching
// x = umlTriangleDepth on the node border; the base runs on past its top
// corner.
func sketchTriangle(id string, strokeWidth float64, place func(model.Point) model.Point) string {
	rng := seedRNG(id)
	j := sketchUMLJitter
	top := model.Point{X: 0, Y: sketchJitter(rng, j)}
	tip := model.Point{X: umlTriangleDepth - strokeWidth/2, Y: umlTriangleHalf + sketchJitter(rng, j/2)}
	bottom := model.Point{X: 0, Y: 2*umlTriangleHalf + sketchJitter(rng, j)}
	// The base bows half as far, so it still covers the wire's end.
	bows := []float64{sketchJitter(rng, sketchUMLBow), sketchJitter(rng, sketchUMLBow), sketchJitter(rng, sketchUMLBow/2)}
	return sketchClosed([]model.Point{top, tip, bottom}, bows, sketchUMLOvershoot, place)
}

// sketchDiamond is DiamondMarker's diamond drawn by hand, back corner at
// x = 0 on the wire's start and tip, rounded by the stroke, reaching
// x = umlDiamondDepth on the node border; the last side runs on past the
// back corner.
func sketchDiamond(id string, strokeWidth float64, place func(model.Point) model.Point) string {
	rng := seedRNG(id)
	j := sketchUMLJitter
	back := model.Point{X: 0, Y: umlDiamondHalf + sketchJitter(rng, j/2)}
	top := model.Point{X: umlDiamondDepth/2 + sketchJitter(rng, j), Y: sketchJitter(rng, j)}
	tip := model.Point{X: umlDiamondDepth - strokeWidth/2, Y: umlDiamondHalf + sketchJitter(rng, j/2)}
	bottom := model.Point{X: umlDiamondDepth/2 + sketchJitter(rng, j), Y: 2*umlDiamondHalf + sketchJitter(rng, j)}
	bows := []float64{sketchJitter(rng, sketchUMLBow), sketchJitter(rng, sketchUMLBow), sketchJitter(rng, sketchUMLBow), sketchJitter(rng, sketchUMLBow)}
	return sketchClosed([]model.Point{back, top, tip, bottom}, bows, sketchUMLOvershoot, place)
}
