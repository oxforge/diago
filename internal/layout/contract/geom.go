package contract

import (
	"math"

	"github.com/oxforge/diago/internal/model"
)

// box is an axis-aligned rectangle.
type box struct{ left, top, right, bottom float64 }

func nodeBox(n model.PositionedNode) box {
	return box{n.X - n.Width/2, n.Y - n.Height/2, n.X + n.Width/2, n.Y + n.Height/2}
}

func groupBox(g model.PositionedGroup) box {
	return box{g.X, g.Y, g.X + g.Width, g.Y + g.Height}
}

func (b box) inflate(d float64) box {
	return box{b.left - d, b.top - d, b.right + d, b.bottom + d}
}

// penetration returns the length of segment p-q strictly inside b, by
// Liang-Barsky clipping. A segment on the border or outside returns 0.
func penetration(p, q model.Point, b box) float64 {
	if b.left >= b.right || b.top >= b.bottom {
		return 0
	}
	dx, dy := q.X-p.X, q.Y-p.Y
	t0, t1 := 0.0, 1.0
	for _, cl := range [4]struct{ p, q float64 }{
		{-dx, p.X - b.left}, {dx, b.right - p.X}, {-dy, p.Y - b.top}, {dy, b.bottom - p.Y},
	} {
		if cl.p == 0 {
			if cl.q <= 0 {
				return 0
			}
			continue
		}
		r := cl.q / cl.p
		if cl.p < 0 {
			t0 = math.Max(t0, r)
		} else {
			t1 = math.Min(t1, r)
		}
		if t1 <= t0 {
			return 0
		}
	}
	return (t1 - t0) * math.Hypot(dx, dy)
}

// pointSegDist is the distance from p to segment a-b.
func pointSegDist(p, a, b model.Point) float64 {
	abx, aby := b.X-a.X, b.Y-a.Y
	den := abx*abx + aby*aby
	t := 0.0
	if den > 0 {
		t = math.Max(0, math.Min(1, ((p.X-a.X)*abx+(p.Y-a.Y)*aby)/den))
	}
	return math.Hypot(p.X-(a.X+t*abx), p.Y-(a.Y+t*aby))
}

// pointBoxDist is the distance from p to b; 0 inside.
func pointBoxDist(p model.Point, b box) float64 {
	dx := math.Max(math.Max(b.left-p.X, 0), p.X-b.right)
	dy := math.Max(math.Max(b.top-p.Y, 0), p.Y-b.bottom)
	return math.Hypot(dx, dy)
}

func dist(a, b model.Point) float64 { return math.Hypot(a.X-b.X, a.Y-b.Y) }

func horizontal(p, q model.Point) bool { return math.Abs(p.Y-q.Y) <= orthoTol }
func vertical(p, q model.Point) bool   { return math.Abs(p.X-q.X) <= orthoTol }

// side is a side of a node's bounding box.
type side int

const (
	top side = iota
	bottom
	left
	right
)

func (s side) String() string {
	return [...]string{"top", "bottom", "left", "right"}[s]
}

// outward reports whether the direction (dx, dy) leaves side s
// perpendicularly, away from the node.
func (s side) outward(dx, dy float64) bool {
	switch s {
	case top:
		return math.Abs(dx) <= orthoTol && dy < 0
	case bottom:
		return math.Abs(dx) <= orthoTol && dy > 0
	case left:
		return math.Abs(dy) <= orthoTol && dx < 0
	default:
		return math.Abs(dy) <= orthoTol && dx > 0
	}
}

// vertices returns a diamond's or circle's four connection points, indexed
// by side.
func vertices(n model.PositionedNode) [4]model.Point {
	rx, ry := n.Width/2, n.Height/2
	if n.Shape == model.ShapeCircle {
		r := math.Min(rx, ry)
		rx, ry = r, r
	}
	return [4]model.Point{
		top:    {X: n.X, Y: n.Y - ry},
		bottom: {X: n.X, Y: n.Y + ry},
		left:   {X: n.X - rx, Y: n.Y},
		right:  {X: n.X + rx, Y: n.Y},
	}
}

// nearestVertex returns the vertex side nearest p and its distance.
func nearestVertex(n model.PositionedNode, p model.Point) (side, float64) {
	vs := vertices(n)
	best, bestD := top, math.Inf(1)
	for s, v := range vs {
		if d := dist(p, v); d < bestD {
			best, bestD = side(s), d
		}
	}
	return best, bestD
}

// polygon returns the outline of a diamond, hexagon or parallelogram.
func polygon(n model.PositionedNode) []model.Point {
	hw, hh := n.Width/2, n.Height/2
	switch n.Shape {
	case model.ShapeDiamond:
		return []model.Point{{X: n.X, Y: n.Y - hh}, {X: n.X + hw, Y: n.Y}, {X: n.X, Y: n.Y + hh}, {X: n.X - hw, Y: n.Y}}
	case model.ShapeHexagon:
		in := hw / 2
		return []model.Point{
			{X: n.X - hw + in, Y: n.Y - hh}, {X: n.X + hw - in, Y: n.Y - hh}, {X: n.X + hw, Y: n.Y},
			{X: n.X + hw - in, Y: n.Y + hh}, {X: n.X - hw + in, Y: n.Y + hh}, {X: n.X - hw, Y: n.Y},
		}
	default: // parallelogram, slant 0.3 × height
		s := 0.3 * n.Height
		return []model.Point{
			{X: n.X - hw + s, Y: n.Y - hh}, {X: n.X + hw, Y: n.Y - hh},
			{X: n.X + hw - s, Y: n.Y + hh}, {X: n.X - hw, Y: n.Y + hh},
		}
	}
}

// cylinderCap is the cap ellipse's vertical radius: 0.18 × height, as the
// renderer draws it.
const cylinderCap = 0.18

// outlineDist is the distance from p to n's drawn outline (C2.2).
func outlineDist(n model.PositionedNode, p model.Point) float64 {
	hw, hh := n.Width/2, n.Height/2
	switch n.Shape {
	case model.ShapeCircle:
		return math.Abs(dist(p, model.Point{X: n.X, Y: n.Y}) - math.Min(hw, hh))
	case model.ShapeDiamond, model.ShapeHexagon, model.ShapeParallelogram:
		vs := polygon(n)
		d := math.Inf(1)
		for i := range vs {
			d = math.Min(d, pointSegDist(p, vs[i], vs[(i+1)%len(vs)]))
		}
		return d
	case model.ShapeCylinder:
		ry := cylinderCap * n.Height
		topC, botC := n.Y-hh+ry, n.Y+hh-ry
		d := math.Min(
			pointSegDist(p, model.Point{X: n.X - hw, Y: topC}, model.Point{X: n.X - hw, Y: botC}),
			pointSegDist(p, model.Point{X: n.X + hw, Y: topC}, model.Point{X: n.X + hw, Y: botC}))
		// The upper arc of the top cap and the lower arc of the bottom cap,
		// measured vertically: ports sit on them at their own x.
		if dx := p.X - n.X; math.Abs(dx) <= hw {
			rise := ry * math.Sqrt(1-(dx/hw)*(dx/hw))
			d = math.Min(d, math.Abs(p.Y-(topC-rise)))
			d = math.Min(d, math.Abs(p.Y-(botC+rise)))
		}
		return d
	default: // rect, rounded, record box: the bounding box
		b := nodeBox(n)
		if pointBoxDist(p, b) > 0 {
			return pointBoxDist(p, b)
		}
		return math.Min(math.Min(p.X-b.left, b.right-p.X), math.Min(p.Y-b.top, b.bottom-p.Y))
	}
}

// sides returns the box side(s) an endpoint on n's outline connects on
// (C7). Corners belong to two sides. An endpoint off the outline returns
// none (C2.2 reports it).
func sides(n model.PositionedNode, p model.Point) []side {
	hw, hh := n.Width/2, n.Height/2
	rx, ry := p.X-n.X, p.Y-n.Y
	near := func(a, b float64) bool { return math.Abs(a-b) <= attachTol }
	vert := func() side {
		if ry < 0 {
			return top
		}
		return bottom
	}
	horiz := func() side {
		if rx < 0 {
			return left
		}
		return right
	}
	switch n.Shape {
	case model.ShapeDiamond, model.ShapeCircle:
		s, _ := nearestVertex(n, p)
		return []side{s}
	case model.ShapeHexagon:
		// The flat top and bottom faces span |rx| <= hw/2; the angled sides
		// and the side vertices face left and right.
		var out []side
		if near(math.Abs(ry), hh) && math.Abs(rx) <= hw/2+attachTol {
			out = append(out, vert())
		}
		if math.Abs(rx) >= hw/2-attachTol {
			out = append(out, horiz())
		}
		return out
	case model.ShapeParallelogram:
		vs := polygon(n)
		var out []side
		if pointSegDist(p, vs[0], vs[1]) <= attachTol {
			out = append(out, top)
		}
		if pointSegDist(p, vs[2], vs[3]) <= attachTol {
			out = append(out, bottom)
		}
		if pointSegDist(p, vs[3], vs[0]) <= attachTol {
			out = append(out, left)
		}
		if pointSegDist(p, vs[1], vs[2]) <= attachTol {
			out = append(out, right)
		}
		return out
	case model.ShapeCylinder:
		capH := cylinderCap * n.Height
		var out []side
		if math.Abs(rx) >= hw-attachTol && math.Abs(ry) <= hh-capH+attachTol {
			out = append(out, horiz())
		}
		if math.Abs(ry) >= hh-capH-attachTol {
			out = append(out, vert())
		}
		return out
	default:
		var out []side
		if near(ry, -hh) && math.Abs(rx) <= hw+attachTol {
			out = append(out, top)
		}
		if near(ry, hh) && math.Abs(rx) <= hw+attachTol {
			out = append(out, bottom)
		}
		if near(rx, -hw) && math.Abs(ry) <= hh+attachTol {
			out = append(out, left)
		}
		if near(rx, hw) && math.Abs(ry) <= hh+attachTol {
			out = append(out, right)
		}
		return out
	}
}
