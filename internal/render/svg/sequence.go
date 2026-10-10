package svg

import (
	"fmt"
	"strconv"

	"github.com/oxforge/diago/internal/model"
	"github.com/oxforge/diago/internal/theme"
)

// lifelineLine renders an actor's dashed vertical lifeline, with its accent
// color (if any, and if ownColor) applied to the stroke. Shared by both the
// plain and the status-aware render paths so they cannot silently diverge.
func lifelineLine(a model.PositionedActor, th theme.Theme, ownColor bool) Line {
	lifelineStroke := th.Actor.Stroke
	if accent := th.Accent(a.Color); accent != "" && ownColor {
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
// status-aware RenderOptions. A nil opts produces byte-identical output to
// RenderSequence.
func RenderSequenceWithOptions(seq *model.PositionedSequence, th theme.Theme, opts *RenderOptions) string {
	band, titleW := titleBand(seq.Title, th.Actor.Font)
	changesW, changesH := changeListSize(opts.caption(), opts.changes(), th)
	w := max(seq.Width, titleW, changesW)
	h := seq.Height + changesH + band

	doc := newDocument(w, h, th, opts)
	sketch := th.Style == "sketch"
	doc.Defs = sequenceMarkers(seq.Interactions, th, opts, sketch)

	// Fragments behind everything, then lifelines and activation bars, then
	// interactions, then the actor header boxes, covering the lifelines'
	// tops.
	doc.Children = append(doc.Children, renderFragments(seq.Fragments, th, opts)...)
	doc.Children = append(doc.Children, renderLifelines(seq, th, opts)...)
	doc.Children = append(doc.Children, renderInteractions(seq.Interactions, th, opts, sketch)...)
	doc.Children = append(doc.Children, renderActors(seq.Actors, th, opts, sketch)...)

	if caption, lines := opts.caption(), opts.changes(); caption != "" || len(lines) > 0 {
		doc.Children = append(doc.Children, renderChangeList(caption, lines, seq.Height, th))
	}

	doc.Children = withTitleBand(doc.Children, seq.Title, w, band, th.Actor.Font)

	return doc.Render()
}

// sequenceMarkers returns a sequence render's marker defs: the base filled
// and open arrowheads, then the markerSet the interactions use.
func sequenceMarkers(its []model.PositionedInteraction, th theme.Theme, opts *RenderOptions, sketch bool) []Element {
	size, stroke := th.Edge.ArrowSize, th.Edge.Stroke
	pair := func(suffix, fill string) []Element {
		return []Element{
			arrowMarker(statusMarkerID(suffix, false), size, fill, sketch),
			openArrowMarker(openStatusMarkerID(suffix), size, fill, sketch),
		}
	}
	defs := []Element{
		arrowMarker(arrowMarkerID, size, stroke, sketch),
		openArrowMarker(openArrowMarkerID, size, stroke, sketch),
	}
	var set markerSet
	for i, it := range its {
		set.add(ownAccent(it.Color, th, opts), opts.class("interaction", strconv.Itoa(i)), false, th, opts)
	}
	return append(defs, set.defs(pair, nil)...)
}

// renderFragments renders the fragments, in order; a status-aware render
// gives each fragment and each of its sections its class.
func renderFragments(fragments []model.PositionedFragment, th theme.Theme, opts *RenderOptions) []Element {
	out := make([]Element, 0, len(fragments))
	for i, f := range fragments {
		var sectionClass func(k int) string
		if opts != nil {
			sectionClass = func(k int) string { return opts.class("section", fmt.Sprintf("%d/%d", i, k)) }
		}
		grp := renderFragment(i, f, th.Fragment, sectionClass)
		if opts != nil {
			grp.Class = opts.class("fragment", strconv.Itoa(i))
		}
		out = append(out, grp)
	}
	return out
}

// renderLifelines renders the dashed lifelines, then the activation bars.
// A status-aware render instead groups each lifeline with its actor's bars,
// so the lifeline's status also carries them.
func renderLifelines(seq *model.PositionedSequence, th theme.Theme, opts *RenderOptions) []Element {
	var out []Element
	if opts == nil {
		for _, a := range seq.Actors {
			out = append(out, lifelineLine(a, th, true))
		}
		for _, act := range seq.Activations {
			out = append(out, activationRect(act, th))
		}
		return out
	}
	for _, a := range seq.Actors {
		children := []Element{lifelineLine(a, th, opts.ownStrokes())}
		for _, act := range seq.Activations {
			if act.ActorID == a.ID {
				children = append(children, activationRect(act, th))
			}
		}
		out = append(out, SVGGroup{
			ID:       "lifeline-" + a.ID,
			Class:    opts.class("lifeline", a.ID),
			Children: children,
		})
	}
	return out
}

// renderInteractions renders the interactions, in order, each with the
// arrowheads of its markerSuffix. A status-aware render gives each its
// class and its label's backing the class "backing", which the status
// stroke skips.
func renderInteractions(its []model.PositionedInteraction, th theme.Theme, opts *RenderOptions, sketch bool) []Element {
	out := make([]Element, 0, len(its))
	for i, it := range its {
		accent := ownAccent(it.Color, th, opts)
		var cls, backingClass string
		if opts != nil {
			cls = opts.class("interaction", strconv.Itoa(i))
			backingClass = "backing"
		}
		var filled, open string
		if suffix := markerSuffix(opts, cls, accent); suffix != "" {
			filled, open = statusMarkerID(suffix, false), openStatusMarkerID(suffix)
		}
		grp := renderInteraction(i, it, accentEdge(th, accent), th.Background, sketch, filled, open, backingClass)
		if opts != nil {
			grp.Class = cls
		}
		out = append(out, grp)
	}
	return out
}

// renderActors renders the actor header boxes. An actor's accent sets its
// fill and, unless the render paints status, its stroke and text color.
func renderActors(actors []model.PositionedActor, th theme.Theme, opts *RenderOptions, sketch bool) []Element {
	out := make([]Element, 0, len(actors))
	for _, a := range actors {
		style := th.Actor
		if accent := th.Accent(a.Color); accent != "" {
			dc := theme.DeriveColors(accent, th.Background, theme.ElementNode)
			style.Fill = dc.Fill
			if opts.ownStrokes() {
				style.Stroke = dc.Stroke
				style.Font.Color = dc.Text
			}
		}
		grp := renderActorBox(a, style, sketch)
		if opts != nil {
			grp.Class = opts.class("actor", a.ID)
		}
		out = append(out, grp)
	}
	return out
}
