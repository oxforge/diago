package svg

import (
	"fmt"
	"slices"
	"strings"

	"github.com/oxforge/diago/internal/theme"
)

// plainArrowMarker returns a <marker> element with a filled triangular arrowhead.
// The marker is sized relative to the stroke width (markerUnits="strokeWidth").
// id is the marker's SVG id (referenced as "url(#id)").
// size controls the marker dimensions.
// color is the fill color of the arrowhead.
func plainArrowMarker(id string, size float64, color string) Marker {
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

// plainOpenArrowMarker returns a <marker> element with an open (chevron) arrowhead.
// Used for async interactions in sequence diagrams.
// The arrowhead is stroke-only — two lines forming a ">" shape — with no fill.
func plainOpenArrowMarker(id string, size float64, color string) Marker {
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

// triangleMarker returns the hollow inheritance/realization triangle. The
// viewBox is padded by 1 on every side so the stroke is not clipped;
// refX = 0 (base) and refY = half so orient="auto-start-reverse" at
// marker-start puts the tip on the node border.
func triangleMarker(id, stroke string, strokeWidth float64) Marker {
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

// diamondMarker returns the aggregation (hollow) or composition (filled)
// diamond, laid out like triangleMarker.
func diamondMarker(id, stroke string, strokeWidth float64, filled bool) Marker {
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

// arrowMarker is plainArrowMarker, drawn by hand in a sketch theme.
func arrowMarker(id string, size float64, color string, sketch bool) Marker {
	if sketch {
		return sketchArrowMarker(id, size, color)
	}
	return plainArrowMarker(id, size, color)
}

// openArrowMarker is plainOpenArrowMarker, drawn by hand in a sketch theme.
func openArrowMarker(id string, size float64, color string, sketch bool) Marker {
	if sketch {
		return sketchOpenArrowMarker(id, size, color)
	}
	return plainOpenArrowMarker(id, size, color)
}

// sketchMarker lays a hand-drawn marker out like its clean counterpart:
// the same reference point and scale, the viewBox (w by h from the origin)
// padded by pad on every side so no jittered corner is clipped.
func sketchMarker(id string, w, h, pad, refX, refY float64, shape Path) Marker {
	return Marker{
		ID:      id,
		ViewBox: fmt.Sprintf("%s %s %s %s", ff(-pad), ff(-pad), ff(w+2*pad), ff(h+2*pad)),
		RefX:    refX, RefY: refY,
		Width: w + 2*pad, Height: h + 2*pad,
		Orient: "auto-start-reverse", MarkerUnits: "userSpaceOnUse",
		Children: []Element{shape},
	}
}

// sketchArrowMarker is plainArrowMarker drawn by hand: an uneven triangle with
// a thin rounded outline in its fill color.
func sketchArrowMarker(id string, size float64, color string) Marker {
	outline := size * sketchArrowOutline
	pad := size*(sketchArrowJitter+sketchArrowBow) + outline/2
	return sketchMarker(id, size, size, pad, size, size/2, Path{
		D: sketchArrowhead(id, size, outline), Fill: color, Stroke: color, StrokeWidth: outline, LineJoin: "round",
	})
}

// sketchOpenArrowMarker is plainOpenArrowMarker drawn by hand: two strokes with
// round ends.
func sketchOpenArrowMarker(id string, size float64, color string) Marker {
	const strokeWidth = 1.5
	pad := size*(sketchArrowJitter+sketchArrowBow) + strokeWidth/2
	return sketchMarker(id, size, size, pad, size, size/2, Path{
		D: sketchOpenArrowhead(id, size, strokeWidth), Fill: "none", Stroke: color, StrokeWidth: strokeWidth,
		LineCap: "round", LineJoin: "round",
	})
}

// sketchUMLPad is how far a hand-drawn UML adornment can reach outside the
// clean one's box: a jittered corner, bowed or overshot, plus half the
// stroke.
func sketchUMLPad(strokeWidth float64) float64 {
	return sketchUMLJitter + max(sketchUMLBow, sketchUMLOvershoot) + strokeWidth/2
}

// sketchTriangleMarker is triangleMarker drawn by hand.
func sketchTriangleMarker(id, stroke string, strokeWidth float64) Marker {
	return sketchMarker(id, umlTriangleDepth, 2*umlTriangleHalf, sketchUMLPad(strokeWidth), 0, umlTriangleHalf, Path{
		D: sketchTriangle(id, strokeWidth, asDrawn), Fill: "none", Stroke: stroke, StrokeWidth: strokeWidth, LineCap: "round", LineJoin: "round",
	})
}

// sketchDiamondMarker is diamondMarker drawn by hand.
func sketchDiamondMarker(id, stroke string, strokeWidth float64, filled bool) Marker {
	fill := "none"
	if filled {
		fill = stroke
	}
	return sketchMarker(id, umlDiamondDepth, 2*umlDiamondHalf, sketchUMLPad(strokeWidth), 0, umlDiamondHalf, Path{
		D: sketchDiamond(id, strokeWidth, asDrawn), Fill: fill, Stroke: stroke, StrokeWidth: strokeWidth, LineCap: "round", LineJoin: "round",
	})
}

// openArrowMarkerID is the SVG marker ID used for open (async) arrowheads.
const openArrowMarkerID = "diago-arrow-open"

// arrowMarkerID is the SVG marker ID used for arrowheads.
const arrowMarkerID = "diago-arrow"

const arrowMarkerThickID = "diago-arrow-thick"

// thickArrowScale is the multiplier applied to arrow size for thick edges,
// matching the stroke width multiplier so the arrowhead stays proportional.
const thickArrowScale = 2.5

// markerUse is an edge's or interaction's use of an arrowhead pair beyond
// the base one: the suffix that names the pair (an accent color's hex
// digits or a status class), the fill it is drawn in, and whether the
// element is an adorned class relation, which also uses the suffix's UML
// trio.
type markerUse struct {
	suffix, fill string
	adorned      bool
}

// markerSet is the markers a render's edges or interactions use beyond the
// base ones, per accent color and per status class: a pair (a filled and a
// thick arrowhead in a flow render, a filled and an open one in a sequence
// render) and, where an adorned class relation uses it, a UML trio.
type markerSet struct {
	accents, statuses []markerUse // in element order
}

// add records an element's uses: its accent's pair, drawn in the edge
// stroke the accent derives, and its status class's pair, in the fill the
// render gives that class, when it gives one.
func (m *markerSet) add(accent, cls string, adorned bool, th theme.Theme, opts *RenderOptions) {
	if accent != "" {
		stroke := theme.DeriveColors(accent, th.Background, theme.ElementEdge).Stroke
		m.accents = append(m.accents, markerUse{accent[1:], stroke, adorned})
	}
	if fill, ok := opts.markerFill(cls); ok {
		m.statuses = append(m.statuses, markerUse{cls, fill, adorned})
	}
}

// defs returns the marker defs of the set: the accents' in the order the
// elements first use them, then the status classes' in sorted order, so
// the emission is deterministic. pair builds a suffix's pair in a fill;
// trio its UML trio, nil when no element is adorned.
func (m *markerSet) defs(pair, trio func(suffix, fill string) []Element) []Element {
	statuses := slices.SortedStableFunc(slices.Values(m.statuses), func(a, b markerUse) int {
		return strings.Compare(a.suffix, b.suffix)
	})
	return append(markerDefs(m.accents, pair, trio), markerDefs(statuses, pair, trio)...)
}

// markerDefs returns the marker defs uses need, each once: a suffix's pair
// at its first use, its UML trio at its first adorned use.
func markerDefs(uses []markerUse, pair, trio func(suffix, fill string) []Element) []Element {
	var defs []Element
	pairs, trios := map[string]bool{}, map[string]bool{}
	for _, u := range uses {
		if !pairs[u.suffix] {
			pairs[u.suffix] = true
			defs = append(defs, pair(u.suffix, u.fill)...)
		}
		if u.adorned && !trios[u.suffix] {
			trios[u.suffix] = true
			defs = append(defs, trio(u.suffix, u.fill)...)
		}
	}
	return defs
}

// markerFill is the arrowhead fill the render gives status class cls.
func (o *RenderOptions) markerFill(cls string) (string, bool) {
	if o == nil {
		return "", false
	}
	fill, ok := o.MarkerFills[cls]
	return fill, ok
}

// markerSuffix names the markers an edge or interaction of status class
// cls and accent color references: its class's when the render gives the
// class a marker fill, since status outranks the element's own color, else
// its accent's, else "" for the base markers.
func markerSuffix(opts *RenderOptions, cls, accent string) string {
	if _, ok := opts.markerFill(cls); ok && cls != "" {
		return cls
	}
	if accent != "" {
		return accent[1:]
	}
	return ""
}
