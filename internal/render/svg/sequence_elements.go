package svg

import (
	"fmt"

	"github.com/oxforge/diago/internal/font"
	"github.com/oxforge/diago/internal/model"
	"github.com/oxforge/diago/internal/theme"
)

// labelVerticalOffset is the vertical offset to center interaction labels on their arrow line.
// Set to 0 for vertically centered labels; positive values move labels above the line.
const labelVerticalOffset = 0.0

// renderActorBox renders an actor header box with a centered label.
func renderActorBox(a model.PositionedActor, style theme.ActorStyle, sketch bool) SVGGroup {
	var shapeEl Element
	if sketch {
		shapeEl = Path{
			D:           sketchRect(a.BoxX, a.BoxY, a.BoxW, a.BoxH, 4, "actor-"+a.ID),
			Fill:        style.Fill,
			Stroke:      style.Stroke,
			StrokeWidth: style.StrokeWidth,
		}
	} else {
		shapeEl = Rect{
			X:           a.BoxX,
			Y:           a.BoxY,
			Width:       a.BoxW,
			Height:      a.BoxH,
			Fill:        style.Fill,
			Stroke:      style.Stroke,
			StrokeWidth: style.StrokeWidth,
			Rx:          4,
		}
	}
	children := []Element{
		shapeEl,
		Text{
			X:                a.BoxX + a.BoxW/2,
			Y:                a.BoxY + a.BoxH/2,
			Content:          a.Label,
			FontFamily:       style.Font.Family,
			FontSize:         fmt.Sprintf("%g", style.Font.Size),
			FontWeight:       fmt.Sprintf("%d", style.Font.Weight),
			Fill:             style.Font.Color,
			Anchor:           "middle",
			DominantBaseline: "central",
		},
	}
	return SVGGroup{
		ID:       "actor-" + a.ID,
		Children: children,
	}
}

// renderFragment renders a fragment's frame, its operator tab, its section
// dividers and its guards, at the positions the layout gave them.
// sectionClass returns the CSS class for section k; nil keeps
// today's output unchanged (no per-section wrapper groups).
func renderFragment(idx int, f model.PositionedFragment, style theme.FragmentStyle, sectionClass func(k int) string) SVGGroup {
	children := []Element{
		// Background fill rect.
		Rect{
			X:           f.X,
			Y:           f.Y,
			Width:       f.Width,
			Height:      f.Height,
			Fill:        style.Fill,
			Stroke:      style.Stroke,
			StrokeWidth: style.StrokeWidth,
			StrokeDash:  style.StrokeDash,
			Rx:          2,
		},
	}
	if f.TabWidth > 0 && f.TabHeight > 0 {
		children = append(children, fragmentTab(f, style)...)
	}

	guard := func(sec model.PositionedFragmentSection) Element {
		return Text{
			X:                sec.LabelX,
			Y:                sec.LabelY,
			Content:          "[" + sec.Label + "]",
			FontFamily:       style.LabelFont.Family,
			FontSize:         fmt.Sprintf("%g", style.LabelFont.Size),
			FontWeight:       fmt.Sprintf("%d", style.LabelFont.Weight),
			Fill:             style.LabelFont.Color,
			Anchor:           "start",
			DominantBaseline: "central",
		}
	}

	// Every section: a dashed divider for each after the first, and the
	// guard.
	for i, sec := range f.Sections {
		var sectionChildren []Element
		if i > 0 {
			sectionChildren = append(sectionChildren, Line{
				X1:          f.X,
				Y1:          sec.Y,
				X2:          f.X + f.Width,
				Y2:          sec.Y,
				Stroke:      style.Stroke,
				StrokeWidth: style.StrokeWidth,
				StrokeDash:  "4,3",
			})
		}
		if sec.Label != "" {
			sectionChildren = append(sectionChildren, guard(sec))
		}
		if sectionClass != nil {
			children = append(children, SVGGroup{
				ID:       fmt.Sprintf("fragment-%d-section-%d", idx, i),
				Class:    sectionClass(i),
				Children: sectionChildren,
			})
		} else {
			children = append(children, sectionChildren...)
		}
	}

	return SVGGroup{
		ID:       fmt.Sprintf("fragment-%d", idx),
		Children: children,
	}
}

// fragmentTab renders the operator tab at a frame's top-left: a pentagon,
// solid in the frame's stroke and fill, its bottom-right corner cut, with
// the operator (alt, opt, loop, par, break) centered left of the cut.
func fragmentTab(f model.PositionedFragment, style theme.FragmentStyle) []Element {
	x, y, w, h := f.X, f.Y, f.TabWidth, f.TabHeight
	cut := min(f.TabCut, w/2, h/2)
	return []Element{
		Path{
			D: fmt.Sprintf("M %s %s L %s %s L %s %s L %s %s L %s %s Z",
				ff(x), ff(y), ff(x+w), ff(y), ff(x+w), ff(y+h-cut), ff(x+w-cut), ff(y+h), ff(x), ff(y+h)),
			Fill:        style.Fill,
			Stroke:      style.Stroke,
			StrokeWidth: style.StrokeWidth,
		},
		Text{
			X:                x + (w-cut)/2,
			Y:                y + h/2,
			Content:          f.Type.String(),
			FontFamily:       style.LabelFont.Family,
			FontSize:         fmt.Sprintf("%g", style.LabelFont.Size),
			FontWeight:       fmt.Sprintf("%d", style.LabelFont.Weight),
			Fill:             style.LabelFont.Color,
			Anchor:           "middle",
			DominantBaseline: "central",
		},
	}
}

// renderInteraction renders a single interaction arrow with its label.
//
// backingClass classes the label's backing rect: "backing" in a
// status-aware render, so the status stroke skips it; "" in a plain render.
func renderInteraction(idx int, it model.PositionedInteraction, style theme.EdgeAppearance, background string, sketch bool, filledMarkerOverride, openMarkerOverride, backingClass string) SVGGroup {
	strokeDash := ""
	markerID := arrowMarkerID
	switch it.Style {
	case model.InteractionDashed:
		strokeDash = "8,4"
	case model.InteractionAsync:
		markerID = openArrowMarkerID
	}
	// Apply marker overrides for colored interactions.
	if filledMarkerOverride != "" && it.Style != model.InteractionAsync {
		markerID = filledMarkerOverride
	} else if openMarkerOverride != "" && it.Style == model.InteractionAsync {
		markerID = openMarkerOverride
	}

	markerRef := fmt.Sprintf("url(#%s)", markerID)
	var children []Element

	if it.IsSelf {
		children = renderSelfInteraction(it, style, background, strokeDash, markerRef, sketch)
	} else {
		children = renderNormalInteraction(it, style, background, strokeDash, markerRef, sketch, backingClass)
	}

	return SVGGroup{
		ID:       fmt.Sprintf("interaction-%d", idx),
		Children: children,
	}
}

// renderNormalInteraction renders a straight arrow between two different actors.
func renderNormalInteraction(
	it model.PositionedInteraction,
	style theme.EdgeAppearance,
	background string,
	strokeDash string,
	markerRef string,
	sketch bool,
	backingClass string,
) []Element {
	pts := []model.Point{
		{X: it.FromX, Y: it.Y},
		{X: it.ToX, Y: it.Y},
	}
	var arrowEl Element
	if sketch {
		edgeID := fmt.Sprintf("%s-%s", it.From, it.To)
		arrowEl = Path{
			D:           sketchPolyline(pts, edgeID),
			Fill:        "none",
			Stroke:      style.Stroke,
			StrokeWidth: style.StrokeWidth,
			StrokeDash:  strokeDash,
			MarkerEnd:   markerRef,
		}
	} else {
		arrowEl = Polyline{
			Points:      pts,
			Stroke:      style.Stroke,
			StrokeWidth: style.StrokeWidth,
			StrokeDash:  strokeDash,
			MarkerEnd:   markerRef,
		}
	}
	children := []Element{arrowEl}

	if it.Label != "" {
		midX := (it.FromX + it.ToX) / 2
		labelY := it.Y - labelVerticalOffset
		lw, lh := font.MeasureText(it.Label, style.LabelFont.Size, style.LabelFont.Family)
		padX, padY := 4.0, 2.0
		children = append(children,
			Rect{
				Class:       backingClass,
				X:           midX - lw/2 - padX,
				Y:           labelY - lh/2 - padY,
				Width:       lw + padX*2,
				Height:      lh + padY*2,
				Fill:        background,
				FillOpacity: 0.75,
			},
			Text{
				X:                midX,
				Y:                labelY,
				Content:          it.Label,
				FontFamily:       style.LabelFont.Family,
				FontSize:         fmt.Sprintf("%g", style.LabelFont.Size),
				FontWeight:       fmt.Sprintf("%d", style.LabelFont.Weight),
				Fill:             style.LabelFont.Color,
				Anchor:           "middle",
				DominantBaseline: "central",
			},
		)
	}
	return children
}

// renderSelfInteraction renders a U-shaped self-message loop.
func renderSelfInteraction(
	it model.PositionedInteraction,
	style theme.EdgeAppearance,
	background string,
	strokeDash string,
	markerRef string,
	sketch bool,
) []Element {
	if len(it.SelfPoints) < 4 {
		return []Element{}
	}
	var arrowEl Element
	if sketch {
		edgeID := fmt.Sprintf("%s-self-%s", it.From, it.To)
		arrowEl = Path{
			D:           sketchPolyline(it.SelfPoints, edgeID),
			Fill:        "none",
			Stroke:      style.Stroke,
			StrokeWidth: style.StrokeWidth,
			StrokeDash:  strokeDash,
			MarkerEnd:   markerRef,
		}
	} else {
		arrowEl = Polyline{
			Points:      it.SelfPoints,
			Stroke:      style.Stroke,
			StrokeWidth: style.StrokeWidth,
			StrokeDash:  strokeDash,
			MarkerEnd:   markerRef,
		}
	}
	children := []Element{arrowEl}

	if it.Label != "" {
		// Place label to the right of the lifeline, above the loop.
		rightX := it.SelfPoints[1].X + 6
		labelY := (it.SelfPoints[0].Y + it.SelfPoints[2].Y) / 2
		children = append(children, Text{
			X:                rightX,
			Y:                labelY,
			Content:          it.Label,
			FontFamily:       style.LabelFont.Family,
			FontSize:         fmt.Sprintf("%g", style.LabelFont.Size),
			FontWeight:       fmt.Sprintf("%d", style.LabelFont.Weight),
			Fill:             style.LabelFont.Color,
			Anchor:           "start",
			DominantBaseline: "central",
		})
	}
	return children
}
