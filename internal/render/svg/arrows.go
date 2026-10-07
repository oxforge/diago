package svg

import "fmt"

// ArrowMarker returns a <marker> element with a filled triangular arrowhead.
// The marker is sized relative to the stroke width (markerUnits="strokeWidth").
// id is the marker's SVG id (referenced as "url(#id)").
// size controls the marker dimensions.
// color is the fill color of the arrowhead.
func ArrowMarker(id string, size float64, color string) Marker {
	// The triangle path occupies a viewBox of "0 0 10 10".
	// The tip of the arrow is at x=10, y=5 (the refX/refY point).
	// markerUnits=strokeWidth means marker dimensions scale with the edge stroke.
	half := size / 2.0
	return Marker{
		ID:          id,
		ViewBox:     fmt.Sprintf("0 0 %s %s", ff(size), ff(size)),
		RefX:        size,
		RefY:        half,
		Width:       size,
		Height:      size,
		Orient:      "auto-start-reverse",
		MarkerUnits: "userSpaceOnUse",
		Children: []Element{
			Path{
				D:    fmt.Sprintf("M 0 0 L %s %s L 0 %s Z", ff(size), ff(half), ff(size)),
				Fill: color,
			},
		},
	}
}

// OpenArrowMarker returns a <marker> element with an open (chevron) arrowhead.
// Used for async interactions in sequence diagrams.
// The arrowhead is stroke-only — two lines forming a ">" shape — with no fill.
func OpenArrowMarker(id string, size float64, color string) Marker {
	half := size / 2.0
	return Marker{
		ID:          id,
		ViewBox:     fmt.Sprintf("0 0 %s %s", ff(size), ff(size)),
		RefX:        size,
		RefY:        half,
		Width:       size,
		Height:      size,
		Orient:      "auto-start-reverse",
		MarkerUnits: "userSpaceOnUse",
		Children: []Element{
			// Two lines forming a ">" shape: top arm and bottom arm.
			Path{
				D:           fmt.Sprintf("M 0 0 L %s %s L 0 %s", ff(size), ff(half), ff(size)),
				Fill:        "none",
				Stroke:      color,
				StrokeWidth: 1.5,
			},
		},
	}
}

// UML adornment geometry, inside the envelope the layout reserves for it
// (C14: 14 deep, 7 half-wide). Depth runs along the wire from the marker's
// base (x = 0, at the wire's shortened start) to its tip (x = depth, at the
// box border); half-width is across it.
const (
	umlTriangleDepth = 12.0
	umlTriangleHalf  = 7.0
	umlDiamondDepth  = 14.0
	umlDiamondHalf   = 6.0
)

// TriangleMarker returns the hollow inheritance/realization triangle. The
// viewBox is padded by 1 on every side so the stroke is not clipped;
// refX = 0 (base) and refY = half so orient="auto-start-reverse" at
// marker-start puts the tip on the node border.
func TriangleMarker(id, stroke string, strokeWidth float64) Marker {
	return Marker{
		ID:      id,
		ViewBox: fmt.Sprintf("-1 -1 %s %s", ff(umlTriangleDepth+2), ff(2*umlTriangleHalf+2)),
		RefX:    0, RefY: umlTriangleHalf,
		Width: umlTriangleDepth + 2, Height: 2*umlTriangleHalf + 2,
		Orient: "auto-start-reverse", MarkerUnits: "userSpaceOnUse",
		Children: []Element{Path{
			D:    fmt.Sprintf("M 0 0 L %s %s L 0 %s Z", ff(umlTriangleDepth), ff(umlTriangleHalf), ff(2*umlTriangleHalf)),
			Fill: "none", Stroke: stroke, StrokeWidth: strokeWidth,
		}},
	}
}

// DiamondMarker returns the aggregation (hollow) or composition (filled)
// diamond, laid out like TriangleMarker.
func DiamondMarker(id, stroke string, strokeWidth float64, filled bool) Marker {
	fill := "none"
	if filled {
		fill = stroke
	}
	return Marker{
		ID:      id,
		ViewBox: fmt.Sprintf("-1 -1 %s %s", ff(umlDiamondDepth+2), ff(2*umlDiamondHalf+2)),
		RefX:    0, RefY: umlDiamondHalf,
		Width: umlDiamondDepth + 2, Height: 2*umlDiamondHalf + 2,
		Orient: "auto-start-reverse", MarkerUnits: "userSpaceOnUse",
		Children: []Element{Path{
			D: fmt.Sprintf("M 0 %s L %s 0 L %s %s L %s %s Z", ff(umlDiamondHalf), ff(umlDiamondDepth/2),
				ff(umlDiamondDepth), ff(umlDiamondHalf), ff(umlDiamondDepth/2), ff(2*umlDiamondHalf)),
			Fill: fill, Stroke: stroke, StrokeWidth: strokeWidth,
		}},
	}
}
