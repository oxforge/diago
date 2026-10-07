package excalidraw

import (
	"math"

	"github.com/oxforge/diago/internal/model"
)

// mapShape returns the Excalidraw element type and optional roundness for a Diago shape.
func mapShape(s model.Shape) (string, *roundness) {
	switch s {
	case model.ShapeCircle:
		return "ellipse", nil
	case model.ShapeDiamond:
		return "diamond", nil
	case model.ShapeRounded:
		return "rectangle", &roundness{Type: 3}
	default:
		// rect, cylinder, hexagon, parallelogram all map to rectangle (lossy).
		return "rectangle", nil
	}
}

// mapEdgeStyle returns the Excalidraw strokeStyle and strokeWidth for a Diago edge style.
func mapEdgeStyle(s model.EdgeStyle) (string, float64) {
	switch s {
	case model.EdgeDashed:
		return "dashed", 2
	case model.EdgeDotted:
		return "dotted", 2
	case model.EdgeThick:
		return "solid", 4
	default:
		return "solid", 2
	}
}

// arrowBindings creates start/end binding structs for an edge.
func arrowBindings(e model.PositionedEdge) (*binding, *binding) {
	return &binding{ElementID: e.From, Focus: 0, Gap: 5},
		&binding{ElementID: e.To, Focus: 0, Gap: 5}
}

// arrowheadStart returns the start arrowhead type based on edge direction.
func arrowheadStart(d model.EdgeDirection) *string {
	switch d {
	case model.EdgeBackward, model.EdgeBoth:
		s := "arrow"
		return &s
	default:
		return nil
	}
}

// arrowheadEnd returns the end arrowhead type based on edge direction.
func arrowheadEnd(d model.EdgeDirection) *string {
	switch d {
	case model.EdgeForward, model.EdgeBoth:
		s := "arrow"
		return &s
	default:
		return nil
	}
}

// edgeLabelPos returns the position for an edge label (top-left).
func edgeLabelPos(e model.PositionedEdge) (float64, float64) {
	if e.LabelPos != nil {
		return e.LabelPos.X - e.LabelWidth/2, e.LabelPos.Y - e.LabelHeight/2
	}
	// Fallback: midpoint of the edge polyline.
	if len(e.Points) >= 2 {
		mid := len(e.Points) / 2
		return e.Points[mid].X - e.LabelWidth/2, e.Points[mid].Y - e.LabelHeight/2
	}
	return 0, 0
}

// round rounds to 1 decimal place.
func round(v float64) float64 {
	return math.Round(v*10) / 10
}
