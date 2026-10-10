package svg

import (
	"fmt"
	"slices"

	"github.com/oxforge/diago/internal/font"
	"github.com/oxforge/diago/internal/model"
	"github.com/oxforge/diago/internal/theme"
)

// renderGroups renders the groups, parents behind children, and returns
// their titles apart, for the caller to draw in front of the edges and
// nodes (C13).
func renderGroups(groups []model.PositionedGroup, th theme.Theme, opts *RenderOptions, sketch bool, ids idSet) (boxes, titles []Element) {
	sorted := make([]model.PositionedGroup, len(groups))
	copy(sorted, groups)
	slices.SortFunc(sorted, func(a, b model.PositionedGroup) int {
		return a.Depth - b.Depth
	})
	for _, g := range sorted {
		style := groupStyle(g, th, opts)
		box := renderGroup(g, style, sketch)
		box.ID = ids.claim(box.ID)
		box.Class = opts.class("group", g.ID)
		boxes = append(boxes, box)
		if title, ok := renderGroupTitle(g, style); ok {
			title.ID = ids.claim(title.ID)
			title.Class = box.Class
			titles = append(titles, title)
		}
	}
	return boxes, titles
}

// groupStyle is group g's style: its accent's fill and, unless the render
// paints status, the accent's stroke and label color; without an accent,
// the theme's fill shaded for g's depth.
func groupStyle(g model.PositionedGroup, th theme.Theme, opts *RenderOptions) theme.GroupStyle {
	style := th.Group
	accent := th.Accent(g.Color)
	if accent == "" {
		style.Fill = adjustGroupFill(th.Group.Fill, g.Depth)
		return style
	}
	dc := theme.DeriveColors(accent, th.Background, theme.ElementGroup)
	style.Fill = dc.Fill
	if opts.ownStrokes() {
		style.Stroke = dc.Stroke
		style.LabelFont.Color = dc.Text
	}
	return style
}

// renderGroup renders a group bounding box; its title is renderGroupTitle's.
func renderGroup(g model.PositionedGroup, style theme.GroupStyle, sketch bool) SVGGroup {
	var shapeEl Element
	if sketch {
		shapeEl = Path{
			D:           sketchRect(g.X, g.Y, g.Width, g.Height, 4, "group-"+g.ID),
			Fill:        style.Fill,
			Stroke:      style.Stroke,
			StrokeWidth: style.StrokeWidth,
			StrokeDash:  style.StrokeDash,
		}
	} else {
		shapeEl = Rect{
			X: g.X, Y: g.Y, Width: g.Width, Height: g.Height,
			Fill:        style.Fill,
			Stroke:      style.Stroke,
			StrokeWidth: style.StrokeWidth,
			StrokeDash:  style.StrokeDash,
			Rx:          4,
		}
	}
	return SVGGroup{
		ID:       "group-" + g.ID,
		Children: []Element{shapeEl},
	}
}

// renderGroupTitle renders group g's title, drawn in front of the edges and
// nodes (C13): 16 px below the top border, LabelOffset from the left one,
// or 8 px in when the layout left the title to the renderer, on a backing
// in the group's fill, 4 px wider than the drawn text on each side, that
// gaps any wire under it. A title without its measured extents gets no
// backing; an untitled group, no title.
func renderGroupTitle(g model.PositionedGroup, style theme.GroupStyle) (SVGGroup, bool) {
	if g.Label == "" {
		return SVGGroup{}, false
	}
	const (
		labelOffsetY = 16.0 // label sits inside the top border
		backingPad   = 4.0
	)
	labelX := 8.0
	if g.LabelOffset > 0 {
		labelX = g.LabelOffset
	}
	var children []Element
	if g.LabelWidth > 0 {
		// The text is drawn at font-size px, while MeasureText, like the
		// layout's LabelWidth, reads the size as points: measure the glyphs
		// as drawn, so the backing cuts no wire beside them.
		drawn, _ := font.MeasureText(g.Label, style.LabelFont.Size*72/96, style.LabelFont.Family)
		children = append(children, Rect{
			Class: "backing",
			X:     g.X + labelX - backingPad, Y: g.Y + labelOffsetY - g.LabelHeight/2,
			Width: drawn + 2*backingPad, Height: g.LabelHeight,
			Fill: style.Fill,
		})
	}
	children = append(children, Text{
		X:                g.X + labelX,
		Y:                g.Y + labelOffsetY,
		Content:          g.Label,
		FontFamily:       style.LabelFont.Family,
		FontSize:         fmt.Sprintf("%g", style.LabelFont.Size),
		FontWeight:       fmt.Sprintf("%d", style.LabelFont.Weight),
		Fill:             style.LabelFont.Color,
		Anchor:           "start",
		DominantBaseline: "middle",
	})
	return SVGGroup{ID: "group-title-" + g.ID, Children: children}, true
}
