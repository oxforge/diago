package svg

import (
	"fmt"
	"math"
	"strings"

	"github.com/oxforge/diago/internal/model"
	"github.com/oxforge/diago/internal/theme"
)

// renderEdges renders the edges, in edge order, and returns their labels
// and cardinalities apart, for the caller to draw above the nodes. An
// edge's arrowheads and adornment are its markerSuffix's; its status
// class, when it has one, also wraps its label and each cardinality.
func renderEdges(edges []model.PositionedEdge, th theme.Theme, opts *RenderOptions, sketch bool, ids idSet) (wires, labels []Element) {
	for _, e := range edges {
		accent := ownAccent(e.Color, th, opts)
		style := accentEdge(th, accent)
		cls := opts.class("edge", e.ID)
		suffix := markerSuffix(opts, cls, accent)
		var marker, thick, adornRef string
		if suffix != "" {
			marker, thick = statusMarkerID(suffix, false), statusMarkerID(suffix, true)
		}
		if e.Relation.Adorned() {
			adornRef = fmt.Sprintf("url(#%s)", umlMarkerID(e.Relation, suffix))
		}
		wire, label := renderEdge(e, style, sketch, marker, thick, adornRef)
		wire.ID = ids.claim(wire.ID)
		wire.Class = cls
		wires = append(wires, wire)
		key := strings.TrimPrefix(wire.ID, "edge-")
		if label != nil {
			labels = append(labels, inClass(label, "edge-label-"+key, cls, ids))
		}
		labels = append(labels, renderCardinalities(e, style.LabelFont, cls, key, ids)...)
	}
	return wires, labels
}

// renderCardinalities renders e's cardinalities (C14), the from end's
// first, in the edge label font lf, each in a group of class cls when
// there is one; key is the edge's element id without its "edge-" prefix.
func renderCardinalities(e model.PositionedEdge, lf theme.FontStyle, cls, key string, ids idSet) []Element {
	var out []Element
	for _, card := range []struct {
		side string
		lbl  *model.EndLabel
	}{{"from", e.FromCard}, {"to", e.ToCard}} {
		if card.lbl == nil || card.lbl.Pos == nil || card.lbl.Text == "" {
			continue
		}
		text := Text{
			X:                card.lbl.Pos.X,
			Y:                card.lbl.Pos.Y,
			Content:          card.lbl.Text,
			FontFamily:       lf.Family,
			FontSize:         fmt.Sprintf("%g", lf.Size),
			FontWeight:       fmt.Sprintf("%d", lf.Weight),
			Fill:             lf.Color,
			Anchor:           "middle",
			DominantBaseline: "central",
		}
		out = append(out, inClass(text, "edge-card-"+card.side+"-"+key, cls, ids))
	}
	return out
}

// inClass wraps el in a group of class cls, its id claimed from ids, so a
// status style reaches it; el itself when cls is "".
func inClass(el Element, id, cls string, ids idSet) Element {
	if cls == "" {
		return el
	}
	return SVGGroup{ID: ids.claim(id), Class: cls, Children: []Element{el}}
}

// edgeElementID maps an edge's logical id to its SVG element id. Derived ids
// take the edge-<from>-<to> form (with -<n> for the second and later
// parallel edges); explicit ids render as edge-<id>. An empty id (a
// hand-built graph) falls back to the endpoint form.
func edgeElementID(e model.PositionedEdge) string {
	base := "edge-" + e.From + "-" + e.To
	if e.ID == "" {
		return base
	}
	if n, ok := strings.CutPrefix(e.ID, e.From+"->"+e.To+"#"); ok {
		if n == "0" {
			return base
		}
		return base + "-" + n
	}
	return "edge-" + e.ID
}

// renderEdge renders an edge as a polyline with an arrowhead. The label is
// returned separately so the caller can render all labels in a higher z-layer.
func renderEdge(e model.PositionedEdge, style theme.EdgeAppearance, sketch bool, markerIDOverride, markerThickIDOverride, adornmentRef string) (SVGGroup, Element) {
	if len(e.Points) < 2 {
		return SVGGroup{}, nil
	}

	strokeWidth := style.StrokeWidth
	strokeDash := ""
	switch e.Style {
	case model.EdgeDashed:
		strokeDash = "8,4"
	case model.EdgeDotted:
		strokeDash = "2,4"
	case model.EdgeThick:
		strokeWidth = style.StrokeWidth * 2.5
	}

	markerID := arrowMarkerID
	if e.Style == model.EdgeThick {
		markerID = arrowMarkerThickID
	}
	if markerIDOverride != "" {
		if e.Style == model.EdgeThick {
			markerID = markerThickIDOverride
		} else {
			markerID = markerIDOverride
		}
	}
	markerRef := fmt.Sprintf("url(#%s)", markerID)
	// A class relation fixes its own end policy: the UML adornment at the
	// source end, an arrowhead at the target only when the relation is
	// directed. A flow edge keeps the EdgeDirection switch.
	var markerStart, markerEnd string
	if e.Relation != model.RelationNone {
		markerStart = adornmentRef
		if e.Directed {
			markerEnd = markerRef
		}
	} else {
		switch e.Direction {
		case model.EdgeBackward:
			markerStart = markerRef
		case model.EdgeBoth:
			markerStart = markerRef
			markerEnd = markerRef
		case model.EdgeNone: // undirected: no arrowhead at either end
			// Don't set any markers
		default: // EdgeForward
			markerEnd = markerRef
		}
	}
	// The wire starts where the adornment's base sits; the positioned
	// polyline itself is untouched. Crossings ride along so hop arcs stay on
	// their real crossing points (see shortenStartWithCrossings).
	pts, crossings := shortenStartWithCrossings(e.Points, adornmentDepth(e.Relation), e.Crossings)

	var edgeEl Element
	if sketch {
		edgeID := fmt.Sprintf("%s-%s", e.From, e.To)
		edgeEl = Path{
			D:           sketchPolyline(pts, edgeID, crossings),
			Fill:        "none",
			Stroke:      style.Stroke,
			StrokeWidth: strokeWidth,
			StrokeDash:  strokeDash,
			MarkerStart: markerStart,
			MarkerEnd:   markerEnd,
		}
	} else if len(crossings) > 0 {
		// Edge has crossings — render as path with hop arcs.
		edgeEl = Path{
			D:           polylineWithHops(pts, crossings),
			Fill:        "none",
			Stroke:      style.Stroke,
			StrokeWidth: strokeWidth,
			StrokeDash:  strokeDash,
			MarkerStart: markerStart,
			MarkerEnd:   markerEnd,
		}
	} else {
		edgeEl = Polyline{
			Points:      pts,
			Stroke:      style.Stroke,
			StrokeWidth: strokeWidth,
			StrokeDash:  strokeDash,
			MarkerStart: markerStart,
			MarkerEnd:   markerEnd,
		}
	}
	children := []Element{edgeEl}

	// Edge label returned separately for z-order layering.
	var label Element
	if e.Label != "" {
		var lx, ly float64
		if e.LabelPos != nil {
			lx, ly = e.LabelPos.X, e.LabelPos.Y
		} else {
			lx, ly = edgeLabelPosition(e.Points)
		}
		label = Text{
			X:                lx,
			Y:                ly,
			Content:          e.Label,
			FontFamily:       style.LabelFont.Family,
			FontSize:         fmt.Sprintf("%g", style.LabelFont.Size),
			FontWeight:       fmt.Sprintf("%d", style.LabelFont.Weight),
			Fill:             style.LabelFont.Color,
			Anchor:           "middle",
			DominantBaseline: "central",
		}
	}

	return SVGGroup{
		ID:       edgeElementID(e),
		Children: children,
	}, label
}

// edgeLabelPosition returns the midpoint of the polyline, by total path length.
func edgeLabelPosition(pts []model.Point) (x, y float64) {
	if len(pts) < 2 {
		if len(pts) == 1 {
			return pts[0].X, pts[0].Y
		}
		return 0, 0
	}

	// Compute total length.
	totalLen := 0.0
	segLengths := make([]float64, len(pts)-1)
	for i := 0; i < len(pts)-1; i++ {
		dx := pts[i+1].X - pts[i].X
		dy := pts[i+1].Y - pts[i].Y
		segLengths[i] = math.Sqrt(dx*dx + dy*dy)
		totalLen += segLengths[i]
	}

	// Find the midpoint.
	target := totalLen / 2.0
	cum := 0.0
	for i, seg := range segLengths {
		if cum+seg >= target {
			t := 0.5
			if seg > 0 {
				t = (target - cum) / seg
			}
			x = pts[i].X + t*(pts[i+1].X-pts[i].X)
			y = pts[i].Y + t*(pts[i+1].Y-pts[i].Y)
			return x, y
		}
		cum += seg
	}

	// Fallback: last point.
	last := len(pts) - 1
	return pts[last].X, pts[last].Y
}
