package svg

import (
	"fmt"
	"strconv"

	"github.com/oxforge/diago/internal/font"
	"github.com/oxforge/diago/internal/model"
	"github.com/oxforge/diago/internal/theme"
)

// lifelineLine renders an actor's dashed vertical lifeline, with its accent
// color (if any, and if ownColor) applied to the stroke. Shared by both the
// plain and the status-aware render paths so they cannot silently diverge.
func lifelineLine(a model.PositionedActor, th theme.Theme, ownColor bool) Line {
	lifelineStroke := th.Actor.Stroke
	if accent := resolveAccent(a.Color, th); accent != "" && ownColor {
		dc := theme.DeriveColors(accent, th.Background, theme.ElementNode)
		lifelineStroke = dc.Stroke
	}
	return Line{
		X1:          a.LineX,
		Y1:          a.LineTop,
		X2:          a.LineX,
		Y2:          a.LineBottom,
		Stroke:      lifelineStroke,
		StrokeWidth: th.Actor.StrokeWidth,
		StrokeDash:  "6,4",
	}
}

// activationRect renders an activation bar on a lifeline. Shared by both the
// plain and the status-aware render paths so they cannot silently diverge.
func activationRect(act model.PositionedActivation, th theme.Theme) Rect {
	return Rect{
		X:           act.X,
		Y:           act.Y,
		Width:       act.Width,
		Height:      act.Height,
		Fill:        th.Activation.Fill,
		Stroke:      th.Activation.Stroke,
		StrokeWidth: th.Activation.StrokeWidth,
	}
}

// RenderSequence converts a PositionedSequence and a Theme into a complete SVG document string.
// The output is fully deterministic: same inputs always produce the same bytes.
// RenderSequence(seq, th) is exactly RenderSequenceWithOptions(seq, th, nil).
func RenderSequence(seq *model.PositionedSequence, th theme.Theme) string {
	return RenderSequenceWithOptions(seq, th, nil)
}

// RenderSequenceWithOptions is RenderSequence, parameterized by an optional
// status-aware RenderOptions (Phase 2b diff rendering). A nil opts produces
// byte-identical output to RenderSequence.
func RenderSequenceWithOptions(seq *model.PositionedSequence, th theme.Theme, opts *RenderOptions) string {
	band, titleW := titleBand(seq.Title, th.Actor.Font)
	changesW, changesH := changeListSize(opts.caption(), opts.changes(), th)
	w := max(seq.Width, titleW, changesW)
	h := seq.Height + changesH + band

	doc := SVGDoc{
		Width:    w,
		Height:   h,
		ViewBox:  fmt.Sprintf("0 0 %s %s", ff(w), ff(h)),
		FontFace: font.FontFacesCSS(themeFontFamilies(th)),
	}
	if opts != nil {
		doc.Styles = opts.Styles
	}

	sketch := th.Style == "sketch"

	// Defs: filled arrow marker + open arrow marker.
	doc.Defs = append(doc.Defs,
		arrowMarker(arrowMarkerID, th.Edge.ArrowSize, th.Edge.Stroke, sketch),
		openArrowMarker(openArrowMarkerID, th.Edge.ArrowSize, th.Edge.Stroke, sketch),
	)

	// Background rectangle.
	doc.Children = append(doc.Children, Rect{
		X: 0, Y: 0, Width: w, Height: h,
		Fill: th.Background,
	})

	// Render fragments first (behind everything else).
	for i, f := range seq.Fragments {
		var sectionClass func(k int) string
		if opts != nil {
			idx := i
			sectionClass = func(k int) string { return opts.class("section", fmt.Sprintf("%d/%d", idx, k)) }
		}
		grp := renderFragment(i, f, th.Fragment, sectionClass)
		if opts != nil {
			grp.Class = opts.class("fragment", strconv.Itoa(i))
		}
		doc.Children = append(doc.Children, grp)
	}

	if opts == nil {
		// Render lifelines (dashed vertical lines, behind interactions).
		for _, a := range seq.Actors {
			doc.Children = append(doc.Children, lifelineLine(a, th, true))
		}

		// Render activation boxes.
		for _, act := range seq.Activations {
			doc.Children = append(doc.Children, activationRect(act, th))
		}
	} else {
		// Per-actor lifeline group: the lifeline followed by that actor's
		// activation rects, so a lifeline's status also carries its bars.
		for _, a := range seq.Actors {
			children := []Element{lifelineLine(a, th, opts.ownStrokes())}
			for _, act := range seq.Activations {
				if act.ActorID != a.ID {
					continue
				}
				children = append(children, activationRect(act, th))
			}
			doc.Children = append(doc.Children, SVGGroup{
				ID:       "lifeline-" + a.ID,
				Class:    opts.class("lifeline", a.ID),
				Children: children,
			})
		}
	}

	// itAccent is an interaction's own color, which a status-paint render
	// drops.
	itAccent := func(it model.PositionedInteraction) string {
		if !opts.ownStrokes() {
			return ""
		}
		return resolveAccent(it.Color, th)
	}

	// Collect unique interaction colors and create per-color arrow markers.
	itMarkers := map[string]string{}
	itOpenMarkers := map[string]string{}
	for _, it := range seq.Interactions {
		accent := itAccent(it)
		if accent == "" {
			continue
		}
		if _, ok := itMarkers[accent]; ok {
			continue
		}
		dc := theme.DeriveColors(accent, th.Background, theme.ElementEdge)
		id := fmt.Sprintf("diago-arrow-%s", accent[1:])
		openID := fmt.Sprintf("diago-arrow-open-%s", accent[1:])
		doc.Defs = append(doc.Defs,
			arrowMarker(id, th.Edge.ArrowSize, dc.Stroke, sketch),
			openArrowMarker(openID, th.Edge.ArrowSize, dc.Stroke, sketch),
		)
		itMarkers[accent] = id
		itOpenMarkers[accent] = openID
	}

	// Status markers: one filled + one open marker per class an interaction
	// actually uses, emitted after the base and accent markers, in sorted
	// order for determinism.
	if opts != nil && len(opts.MarkerFills) > 0 {
		needed := map[string]string{} // class → fill
		for i := range seq.Interactions {
			cls := opts.class("interaction", strconv.Itoa(i))
			if fill, ok := opts.MarkerFills[cls]; ok {
				needed[cls] = fill
			}
		}
		for _, cls := range sortedKeys(needed) {
			fill := needed[cls]
			doc.Defs = append(doc.Defs,
				arrowMarker(statusMarkerID(cls, false), th.Edge.ArrowSize, fill, sketch),
				openArrowMarker(openStatusMarkerID(cls), th.Edge.ArrowSize, fill, sketch),
			)
		}
	}

	// Render interactions.
	for i, it := range seq.Interactions {
		edgeStyle := th.Edge
		accent := itAccent(it)
		var filledOverride, openOverride string
		if accent != "" {
			dc := theme.DeriveColors(accent, th.Background, theme.ElementEdge)
			edgeStyle.Stroke = dc.Stroke
			edgeStyle.LabelFont.Color = dc.Text
			filledOverride = itMarkers[accent]
			openOverride = itOpenMarkers[accent]
		}
		var cls, backingClass string
		if opts != nil {
			cls = opts.class("interaction", strconv.Itoa(i))
			if cls != "" {
				if _, ok := opts.MarkerFills[cls]; ok {
					// Status outranks the user's own interaction color for the marker.
					filledOverride, openOverride = statusMarkerID(cls, false), openStatusMarkerID(cls)
				}
			}
			backingClass = "backing" // the status stroke skips the label's backing
		}
		grp := renderInteraction(i, it, edgeStyle, th.Background, sketch, filledOverride, openOverride, backingClass)
		if opts != nil {
			grp.Class = cls
		}
		doc.Children = append(doc.Children, grp)
	}

	// Render actor header boxes (on top, covering lifeline tops).
	for _, a := range seq.Actors {
		actorStyle := th.Actor
		if accent := resolveAccent(a.Color, th); accent != "" {
			dc := theme.DeriveColors(accent, th.Background, theme.ElementNode)
			actorStyle.Fill = dc.Fill
			if opts.ownStrokes() {
				actorStyle.Stroke = dc.Stroke
				actorStyle.Font.Color = dc.Text
			}
		}
		grp := renderActorBox(a, actorStyle, sketch)
		if opts != nil {
			grp.Class = opts.class("actor", a.ID)
		}
		doc.Children = append(doc.Children, grp)
	}

	if caption, lines := opts.caption(), opts.changes(); caption != "" || len(lines) > 0 {
		doc.Children = append(doc.Children, renderChangeList(caption, lines, seq.Height, th))
	}

	doc.Children = withTitleBand(doc.Children, seq.Title, w, band, th.Actor.Font)

	return doc.Render()
}
