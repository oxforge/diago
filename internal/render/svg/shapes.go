package svg

import (
	"fmt"
	"math"

	"github.com/oxforge/diago/internal/model"
	"github.com/oxforge/diago/internal/theme"
)

// nodeLabel builds a centered text element for a node's label.
// If the node has wrapped lines, it produces a multi-line text element
// using SVG tspan elements.
func nodeLabel(node model.PositionedNode, style theme.NodeStyle) Text {
	return nodeLabelAt(node, style, node.Y)
}

// nodeLabelAt is like nodeLabel but places the label at a custom Y center.
// Used by shapes like cylinder where the label is centered in the body,
// not at the overall node center.
func nodeLabelAt(node model.PositionedNode, style theme.NodeStyle, y float64) Text {
	t := Text{
		X:                node.X,
		Y:                y,
		FontFamily:       style.Font.Family,
		FontSize:         fmt.Sprintf("%g", style.Font.Size),
		FontWeight:       fmt.Sprintf("%d", style.Font.Weight),
		Fill:             style.Font.Color,
		Anchor:           "middle",
		DominantBaseline: "middle",
	}
	switch len(node.Lines) {
	case 0:
		t.Content = node.Label
	case 1:
		t.Content = node.Lines[0]
	default:
		t.Lines = node.Lines
		t.LineHeight = style.Font.Size * 1.3
	}
	return t
}

// RenderRect renders a rectangular node.
func RenderRect(node model.PositionedNode, style theme.NodeStyle, sketch bool) SVGGroup {
	hw := node.Width / 2
	hh := node.Height / 2
	var shapeEl Element
	if sketch {
		shapeEl = Path{
			D:           sketchRect(node.X-hw, node.Y-hh, node.Width, node.Height, 2, node.ID),
			Fill:        style.Fill,
			Stroke:      style.Stroke,
			StrokeWidth: style.StrokeWidth,
		}
	} else {
		shapeEl = Rect{
			X: node.X - hw, Y: node.Y - hh,
			Width: node.Width, Height: node.Height,
			Fill: style.Fill, Stroke: style.Stroke, StrokeWidth: style.StrokeWidth,
		}
	}
	return SVGGroup{
		ID: "node-" + node.ID,
		Children: []Element{
			shapeEl,
			nodeLabel(node, style),
		},
	}
}

// RenderRounded renders a rectangle with rounded corners.
func RenderRounded(node model.PositionedNode, style theme.NodeStyle, sketch bool) SVGGroup {
	hw := node.Width / 2
	hh := node.Height / 2
	var shapeEl Element
	if sketch {
		shapeEl = Path{
			D:           sketchRect(node.X-hw, node.Y-hh, node.Width, node.Height, style.CornerRadius, node.ID),
			Fill:        style.Fill,
			Stroke:      style.Stroke,
			StrokeWidth: style.StrokeWidth,
		}
	} else {
		shapeEl = Rect{
			X: node.X - hw, Y: node.Y - hh,
			Width: node.Width, Height: node.Height,
			Rx:   style.CornerRadius,
			Fill: style.Fill, Stroke: style.Stroke, StrokeWidth: style.StrokeWidth,
		}
	}
	return SVGGroup{
		ID: "node-" + node.ID,
		Children: []Element{
			shapeEl,
			nodeLabel(node, style),
		},
	}
}

// RenderCircle renders a circular node.
// The radius is half the maximum of width and height to ensure the label fits.
func RenderCircle(node model.PositionedNode, style theme.NodeStyle, sketch bool) SVGGroup {
	r := math.Max(node.Width, node.Height) / 2
	var shapeEl Element
	if sketch {
		shapeEl = Path{
			D:           sketchCircle(node.X, node.Y, r, node.ID),
			Fill:        style.Fill,
			Stroke:      style.Stroke,
			StrokeWidth: style.StrokeWidth,
		}
	} else {
		shapeEl = Circle{
			CX: node.X, CY: node.Y, R: r,
			Fill: style.Fill, Stroke: style.Stroke, StrokeWidth: style.StrokeWidth,
		}
	}
	return SVGGroup{
		ID: "node-" + node.ID,
		Children: []Element{
			shapeEl,
			nodeLabel(node, style),
		},
	}
}

// RenderDiamond renders a diamond (rotated square) node using a <polygon>.
// The diamond points are at the four cardinal directions of the bounding box.
func RenderDiamond(node model.PositionedNode, style theme.NodeStyle, sketch bool) SVGGroup {
	hw := node.Width / 2
	hh := node.Height / 2
	pts := []model.Point{
		{X: node.X, Y: node.Y - hh}, // top
		{X: node.X + hw, Y: node.Y}, // right
		{X: node.X, Y: node.Y + hh}, // bottom
		{X: node.X - hw, Y: node.Y}, // left
	}
	var shapeEl Element
	if sketch {
		shapeEl = Path{
			D:           sketchPolygon(pts, node.ID),
			Fill:        style.Fill,
			Stroke:      style.Stroke,
			StrokeWidth: style.StrokeWidth,
		}
	} else {
		shapeEl = Polygon{
			Points: pts,
			Fill:   style.Fill, Stroke: style.Stroke, StrokeWidth: style.StrokeWidth,
		}
	}
	return SVGGroup{
		ID: "node-" + node.ID,
		Children: []Element{
			shapeEl,
			nodeLabel(node, style),
		},
	}
}

// RenderCylinder renders a cylinder (database) node.
// The body is a rectangle; the top and bottom are elliptical arcs.
func RenderCylinder(node model.PositionedNode, style theme.NodeStyle, sketch bool) SVGGroup {
	hw := node.Width / 2
	hh := node.Height / 2
	// Ellipse vertical radius — about 20% of height.
	ry := node.Height * 0.18
	// Body top and bottom y positions.
	topY := node.Y - hh + ry
	botY := node.Y + hh - ry
	bodyH := botY - topY

	var bodyEl, capEl Element
	if sketch {
		bodyEl = Path{
			D:           sketchCylinder(node.X, hw, topY, botY, ry, node.ID+"-body"),
			Fill:        style.Fill,
			Stroke:      style.Stroke,
			StrokeWidth: style.StrokeWidth,
		}
		capEl = Path{
			D:           sketchEllipse(node.X, topY, hw, ry, node.ID+"-cap"),
			Fill:        style.Fill,
			Stroke:      style.Stroke,
			StrokeWidth: style.StrokeWidth,
		}
	} else {
		// The cylinder path: left side down, bottom arc, right side up, top arc.
		d := fmt.Sprintf(
			"M %s %s "+ // move to top-left
				"L %s %s "+ // left side down to bottom-left
				"A %s %s 0 0 0 %s %s "+ // bottom arc (sweep=0 for "going right" at bottom)
				"L %s %s "+ // right side up to top-right
				"A %s %s 0 0 0 %s %s Z", // top arc (sweep=0 for "going left" at top)
			ff(node.X-hw), ff(topY),
			ff(node.X-hw), ff(botY),
			ff(hw), ff(ry), ff(node.X+hw), ff(botY),
			ff(node.X+hw), ff(topY),
			ff(hw), ff(ry), ff(node.X-hw), ff(topY),
		)
		bodyEl = Path{
			D:           d,
			Fill:        style.Fill,
			Stroke:      style.Stroke,
			StrokeWidth: style.StrokeWidth,
		}
		capEl = Ellipse{
			CX: node.X, CY: topY,
			RX: hw, RY: ry,
			Fill:        style.Fill,
			Stroke:      style.Stroke,
			StrokeWidth: style.StrokeWidth,
		}
	}

	return SVGGroup{
		ID: "node-" + node.ID,
		Children: []Element{
			bodyEl,
			capEl,
			// Label centered below the cap ellipse so text never overlaps it.
			nodeLabelAt(node, style, topY+ry+bodyH/2),
		},
	}
}

// RenderHexagon renders an equilateral hexagonal node (flat top and bottom).
// The layout engine ensures width and height maintain equilateral proportions
// (height = width * sqrt(3)/2), so we render a regular hexagon.
func RenderHexagon(node model.PositionedNode, style theme.NodeStyle, sketch bool) SVGGroup {
	hw := node.Width / 2
	hh := node.Height / 2
	// Equilateral flat-top hexagon: inset = width/4 on each side.
	inset := hw / 2
	pts := []model.Point{
		{X: node.X - hw + inset, Y: node.Y - hh}, // top-left
		{X: node.X + hw - inset, Y: node.Y - hh}, // top-right
		{X: node.X + hw, Y: node.Y},              // right
		{X: node.X + hw - inset, Y: node.Y + hh}, // bottom-right
		{X: node.X - hw + inset, Y: node.Y + hh}, // bottom-left
		{X: node.X - hw, Y: node.Y},              // left
	}
	var shapeEl Element
	if sketch {
		shapeEl = Path{
			D:           sketchPolygon(pts, node.ID),
			Fill:        style.Fill,
			Stroke:      style.Stroke,
			StrokeWidth: style.StrokeWidth,
		}
	} else {
		shapeEl = Polygon{
			Points:      pts,
			Fill:        style.Fill,
			Stroke:      style.Stroke,
			StrokeWidth: style.StrokeWidth,
		}
	}
	return SVGGroup{
		ID: "node-" + node.ID,
		Children: []Element{
			shapeEl,
			nodeLabel(node, style),
		},
	}
}

// parallelogramSlantRatio is the horizontal lean (offset between top and
// bottom edges) as a fraction of node height. Must match C2.2's
// parallelogram outline, by which the layered engine sizes the node (S1)
// and attaches its ports (ports.Slant, S8).
const parallelogramSlantRatio = 0.3

// RenderParallelogram renders a parallelogram node (skewed rectangle).
// The polygon is C2.2's outline, fully inside the node box (C0):
// the top edge is inset on the left, the bottom edge on the right, so the
// shape leans right without poking past the bbox. The layout engine adds
// extra width so the label fits within the flat interior.
func RenderParallelogram(node model.PositionedNode, style theme.NodeStyle, sketch bool) SVGGroup {
	hw := node.Width / 2
	hh := node.Height / 2
	slant := node.Height * parallelogramSlantRatio
	pts := []model.Point{
		{X: node.X - hw + slant, Y: node.Y - hh}, // top-left
		{X: node.X + hw, Y: node.Y - hh},         // top-right
		{X: node.X + hw - slant, Y: node.Y + hh}, // bottom-right
		{X: node.X - hw, Y: node.Y + hh},         // bottom-left
	}
	var shapeEl Element
	if sketch {
		shapeEl = Path{
			D:           sketchPolygon(pts, node.ID),
			Fill:        style.Fill,
			Stroke:      style.Stroke,
			StrokeWidth: style.StrokeWidth,
		}
	} else {
		shapeEl = Polygon{
			Points:      pts,
			Fill:        style.Fill,
			Stroke:      style.Stroke,
			StrokeWidth: style.StrokeWidth,
		}
	}
	return SVGGroup{
		ID: "node-" + node.ID,
		Children: []Element{
			shapeEl,
			nodeLabel(node, style),
		},
	}
}

// RenderNode dispatches to the correct shape renderer based on node.Shape.
func RenderNode(node model.PositionedNode, style theme.NodeStyle, sketch bool) SVGGroup {
	switch node.Shape {
	case model.ShapeRounded:
		return RenderRounded(node, style, sketch)
	case model.ShapeCircle:
		return RenderCircle(node, style, sketch)
	case model.ShapeDiamond:
		return RenderDiamond(node, style, sketch)
	case model.ShapeCylinder:
		return RenderCylinder(node, style, sketch)
	case model.ShapeHexagon:
		return RenderHexagon(node, style, sketch)
	case model.ShapeParallelogram:
		return RenderParallelogram(node, style, sketch)
	default: // ShapeRect and any unknown
		return RenderRect(node, style, sketch)
	}
}
