package excalidraw

import (
	"github.com/oxforge/diago/internal/model"
	"github.com/oxforge/diago/internal/theme"
)

type builder struct {
	th              theme.Theme
	elements        []element
	nodeGroupIDs    map[string][]string
	nodeMembership  map[string]string
	groupMembership map[string]string
	groupByID       map[string]*model.PositionedGroup
}

// resolveAccent resolves a color name or hex value to a hex accent string.
func resolveAccent(color string, th theme.Theme) string {
	if color == "" {
		return ""
	}
	if color[0] == '#' {
		return color
	}
	if hex, ok := th.Colors[color]; ok {
		return hex
	}
	return ""
}

// canvasBackground is Excalidraw's default canvas color; the export sets
// no other.
const canvasBackground = "#ffffff"

// addGroupLabel emits a text element for a group label: where the layout
// placed the title in its band, LabelOffset from the group's left side and
// centered 16 px below its top, or above the group area when the layout
// left the title to the renderer. A backing rectangle in the canvas color
// comes first, 4 px wider than the title on each side, and gaps an arrow
// under it (C13): Excalidraw draws no group frame. LabelWidth only
// approximates the title as Excalidraw draws it, in its own font at 14.
// A title without its measured extents gets no backing.
func (b *builder) addGroupLabel(g model.PositionedGroup, gids []string) {
	strokeColor := "#1e1e1e"
	if accent := resolveAccent(g.Color, b.th); accent != "" {
		dc := theme.DeriveColors(accent, b.th.Background, theme.ElementGroup)
		strokeColor = dc.Text
	}
	x, y, w := g.X, g.Y-20, g.Width
	if g.LabelOffset > 0 {
		// 16 is TitleMid, the title's middle below the group's top side
		// (the screen value, as in the SVG renderer); 10 is half this
		// element's 20 px height. LabelWidth is measured in the theme's
		// group font, while Excalidraw draws the label at its own
		// FontSize 14.
		x, y, w = g.X+g.LabelOffset, g.Y+16-10, g.LabelWidth
	}

	if g.LabelWidth > 0 {
		const pad = 4.0
		b.elements = append(b.elements, element{
			Type:            "rectangle",
			ID:              "grpbacking_" + g.ID,
			X:               round(x - pad),
			Y:               round(y),
			Width:           round(g.LabelWidth + 2*pad),
			Height:          20,
			StrokeColor:     "transparent",
			BackgroundColor: canvasBackground,
			FillStyle:       "solid",
			StrokeWidth:     0,
			Roughness:       0,
			Opacity:         100,
			Seed:            1,
			Version:         1,
			GroupIDs:        gids,
		})
	}
	b.elements = append(b.elements, element{
		Type:            "text",
		ID:              "grplabel_" + g.ID,
		X:               round(x),
		Y:               round(y),
		Width:           round(w),
		Height:          20,
		Text:            g.Label,
		FontSize:        14,
		FontFamily:      1,
		TextAlign:       "left",
		VerticalAlign:   "middle",
		StrokeColor:     strokeColor,
		BackgroundColor: "transparent",
		FillStyle:       "solid",
		StrokeWidth:     0,
		Roughness:       1,
		Opacity:         100,
		Angle:           0,
		Seed:            1,
		Version:         1,
		IsDeleted:       false,
		GroupIDs:        gids,
		BoundElements:   nil,
	})
}

// addNode emits a shape element and a bound text element for a node.
func (b *builder) addNode(n model.PositionedNode, gids []string) {
	shapeID := n.ID
	textID := "text_" + n.ID

	// Center-based to top-left conversion.
	ex := round(n.X - n.Width/2)
	ey := round(n.Y - n.Height/2)

	shapeType, rnd := mapShape(n.Shape)

	shapeStroke := "#1e1e1e"
	shapeBg := "transparent"
	textColor := "#1e1e1e"
	if accent := resolveAccent(n.Color, b.th); accent != "" {
		dc := theme.DeriveColors(accent, b.th.Background, theme.ElementNode)
		shapeStroke = dc.Stroke
		shapeBg = dc.Fill
		textColor = dc.Text
	}

	shape := element{
		Type:            shapeType,
		ID:              shapeID,
		X:               ex,
		Y:               ey,
		Width:           round(n.Width),
		Height:          round(n.Height),
		StrokeColor:     shapeStroke,
		BackgroundColor: shapeBg,
		FillStyle:       "solid",
		StrokeWidth:     2,
		Roughness:       1,
		Opacity:         100,
		Angle:           0,
		Seed:            1,
		Version:         1,
		IsDeleted:       false,
		GroupIDs:        gids,
		BoundElements: []boundRef{
			{ID: textID, Type: "text"},
		},
		Roundness: rnd,
	}
	b.elements = append(b.elements, shape)

	text := element{
		Type:            "text",
		ID:              textID,
		X:               ex,
		Y:               ey,
		Width:           round(n.Width),
		Height:          round(n.Height),
		Text:            n.Label,
		FontSize:        16,
		FontFamily:      1,
		TextAlign:       "center",
		VerticalAlign:   "middle",
		StrokeColor:     textColor,
		BackgroundColor: "transparent",
		FillStyle:       "solid",
		StrokeWidth:     0,
		Roughness:       1,
		Opacity:         100,
		Angle:           0,
		Seed:            1,
		Version:         1,
		IsDeleted:       false,
		ContainerID:     &shapeID,
		GroupIDs:        gids,
	}
	b.elements = append(b.elements, text)
}

// addEdge emits an arrow element and an optional bound label text element for an edge.
func (b *builder) addEdge(e model.PositionedEdge, arrowID, labelID string) {
	startBinding, endBinding := arrowBindings(e)

	var arrowX, arrowY float64
	var points [][]float64
	if len(e.Points) > 0 {
		arrowX = round(e.Points[0].X)
		arrowY = round(e.Points[0].Y)
		points = make([][]float64, len(e.Points))
		for j, p := range e.Points {
			points[j] = []float64{round(p.X - e.Points[0].X), round(p.Y - e.Points[0].Y)}
		}
	}

	strokeStyle, strokeWidth := mapEdgeStyle(e.Style)

	edgeStroke := "#1e1e1e"
	edgeTextColor := "#1e1e1e"
	if accent := resolveAccent(e.Color, b.th); accent != "" {
		dc := theme.DeriveColors(accent, b.th.Background, theme.ElementEdge)
		edgeStroke = dc.Stroke
		edgeTextColor = dc.Text
	}

	arrow := element{
		Type:            "arrow",
		ID:              arrowID,
		X:               arrowX,
		Y:               arrowY,
		Width:           0,
		Height:          0,
		StrokeColor:     edgeStroke,
		BackgroundColor: "transparent",
		FillStyle:       "solid",
		StrokeWidth:     strokeWidth,
		Roughness:       1,
		Opacity:         100,
		Angle:           0,
		Seed:            1,
		Version:         1,
		IsDeleted:       false,
		Points:          points,
		StartBinding:    startBinding,
		EndBinding:      endBinding,
		StartArrowhead:  arrowheadStart(e.Direction),
		EndArrowhead:    arrowheadEnd(e.Direction),
		StrokeStyle:     strokeStyle,
		GroupIDs:        nil,
	}

	// If edge has a label, create a bound text element.
	if e.Label != "" {
		arrow.BoundElements = []boundRef{{ID: labelID, Type: "text"}}

		lx, ly := edgeLabelPos(e)

		labelText := element{
			Type:            "text",
			ID:              labelID,
			X:               round(lx),
			Y:               round(ly),
			Width:           round(e.LabelWidth),
			Height:          round(e.LabelHeight),
			Text:            e.Label,
			FontSize:        14,
			FontFamily:      1,
			TextAlign:       "center",
			VerticalAlign:   "middle",
			StrokeColor:     edgeTextColor,
			BackgroundColor: "transparent",
			FillStyle:       "solid",
			StrokeWidth:     0,
			Roughness:       1,
			Opacity:         100,
			Angle:           0,
			Seed:            1,
			Version:         1,
			IsDeleted:       false,
			ContainerID:     &arrowID,
		}
		b.elements = append(b.elements, labelText)
	}

	b.elements = append(b.elements, arrow)
}
