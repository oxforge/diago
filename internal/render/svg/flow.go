package svg

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/oxforge/diago/internal/font"
	"github.com/oxforge/diago/internal/model"
	"github.com/oxforge/diago/internal/theme"
)

// openArrowMarkerID is the SVG marker ID used for open (async) arrowheads.
const openArrowMarkerID = "diago-arrow-open"

// arrowMarkerID is the SVG marker ID used for arrowheads.
const arrowMarkerID = "diago-arrow"
const arrowMarkerThickID = "diago-arrow-thick"

// thickArrowScale is the multiplier applied to arrow size for thick edges,
// matching the stroke width multiplier so the arrowhead stays proportional.
const thickArrowScale = 2.5

// resolveAccent resolves a named color or hex to an accent hex string.
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

// Render converts a PositionedGraph and a Theme into a complete SVG document string.
// The output is fully deterministic: same inputs always produce the same bytes.
// Render(graph, th) is exactly RenderWithOptions(graph, th, nil).
func Render(graph *model.PositionedGraph, th theme.Theme) string {
	return RenderWithOptions(graph, th, nil)
}

// RenderWithOptions is Render, parameterized by an optional status-aware
// RenderOptions (Phase 2b diff rendering). A nil opts produces byte-identical
// output to Render.
func RenderWithOptions(graph *model.PositionedGraph, th theme.Theme, opts *RenderOptions) string {
	// The class legend and then the change list grow the canvas below the
	// graph and, for a long label, to the right. Without either both are
	// zero and the canvas is exactly the graph's.
	legendW, legendH := legendSize(graph.Legend, th)
	band, titleW := titleBand(graph.Title, th.Node.Font)
	changesW, changesH := changeListSize(opts.changes(), th)
	w := max(graph.Width, legendW, changesW, titleW)
	h := graph.Height + legendH + changesH + band

	doc := SVGDoc{
		Width:    w,
		Height:   h,
		ViewBox:  fmt.Sprintf("0 0 %s %s", ff(w), ff(h)),
		FontFace: font.FontFacesCSS(themeFontFamilies(th)),
		Metadata: layoutMetadataJSON(graph.LayoutHints),
	}
	if opts != nil {
		doc.Styles = opts.Styles
	}

	sketch := th.Style == "sketch"

	// Arrow marker definitions.
	marker := arrowMarker(arrowMarkerID, th.Edge.ArrowSize, th.Edge.Stroke, sketch)
	markerThick := arrowMarker(arrowMarkerThickID, th.Edge.ArrowSize*thickArrowScale, th.Edge.Stroke, sketch)
	doc.Defs = append(doc.Defs, marker, markerThick)

	// UML adornment markers, emitted only when a class relation needs one.
	if slices.ContainsFunc(graph.Edges, func(e model.PositionedEdge) bool { return e.Relation.Adorned() }) {
		doc.Defs = append(doc.Defs, umlMarkers("", th.Edge.Stroke, th.Edge.StrokeWidth, sketch)...)
	}

	// Background rectangle.
	doc.Children = append(doc.Children, Rect{
		X: 0, Y: 0, Width: w, Height: h,
		Fill: th.Background,
	})

	// Add sketch distortion filter to defs if using sketch style.
	if sketch {
		doc.Defs = append(doc.Defs, RawXML{Content: sketchDistortionFilter()})
	}

	// Groups (rendered in depth order: parents behind children, all behind
	// edges/nodes). Their titles are collected and drawn in front of the
	// edges and nodes (C13).
	var groupTitles []Element
	sortedGroups := make([]model.PositionedGroup, len(graph.Groups))
	copy(sortedGroups, graph.Groups)
	slices.SortFunc(sortedGroups, func(a, b model.PositionedGroup) int {
		return a.Depth - b.Depth
	})
	for _, g := range sortedGroups {
		adjustedStyle := th.Group
		if accent := resolveAccent(g.Color, th); accent != "" {
			dc := theme.DeriveColors(accent, th.Background, theme.ElementGroup)
			adjustedStyle.Fill = dc.Fill
			if opts.ownStrokes() {
				adjustedStyle.Stroke = dc.Stroke
				adjustedStyle.LabelFont.Color = dc.Text
			}
		} else {
			adjustedStyle.Fill = adjustGroupFill(th.Group.Fill, g.Depth)
		}
		grp := renderGroup(g, adjustedStyle, sketch)
		grp.Class = opts.class("group", g.ID)
		doc.Children = append(doc.Children, grp)
		if title, ok := renderGroupTitle(g, adjustedStyle); ok {
			title.Class = grp.Class
			groupTitles = append(groupTitles, title)
		}
	}

	// edgeAccent is an edge's own color, which a status-paint render drops.
	edgeAccent := func(e model.PositionedEdge) string {
		if !opts.ownStrokes() {
			return ""
		}
		return resolveAccent(e.Color, th)
	}

	// Collect unique edge colors and create markers for each.
	edgeMarkers := map[string]string{}      // accent hex → marker ID
	edgeMarkersThick := map[string]string{} // accent hex → thick marker ID
	umlAccents := map[string]bool{}         // accent hex → UML trio already emitted
	for _, e := range graph.Edges {
		accent := edgeAccent(e)
		if accent == "" {
			continue
		}
		_, haveArrow := edgeMarkers[accent]
		needUML := e.Relation.Adorned() && !umlAccents[accent]
		if haveArrow && !needUML {
			continue
		}
		dc := theme.DeriveColors(accent, th.Background, theme.ElementEdge)
		if !haveArrow {
			id := fmt.Sprintf("diago-arrow-%s", accent[1:]) // strip #
			idThick := fmt.Sprintf("diago-arrow-thick-%s", accent[1:])
			doc.Defs = append(doc.Defs,
				arrowMarker(id, th.Edge.ArrowSize, dc.Stroke, sketch),
				arrowMarker(idThick, th.Edge.ArrowSize*thickArrowScale, dc.Stroke, sketch),
			)
			edgeMarkers[accent] = id
			edgeMarkersThick[accent] = idThick
		}
		if needUML {
			umlAccents[accent] = true
			doc.Defs = append(doc.Defs, umlMarkers(accent[1:], dc.Stroke, th.Edge.StrokeWidth, sketch)...)
		}
	}

	// Status markers: one pair per class an edge actually uses, emitted
	// after the base and accent markers, in sorted order for determinism.
	if opts != nil && len(opts.MarkerFills) > 0 {
		needed := map[string]string{}    // class → fill
		neededUML := map[string]string{} // class → fill, for adorned relations only
		for _, e := range graph.Edges {
			cls := opts.class("edge", e.ID)
			if fill, ok := opts.MarkerFills[cls]; ok {
				needed[cls] = fill
				if e.Relation.Adorned() {
					neededUML[cls] = fill
				}
			}
		}
		for _, cls := range sortedKeys(needed) {
			fill := needed[cls]
			doc.Defs = append(doc.Defs,
				arrowMarker(statusMarkerID(cls, false), th.Edge.ArrowSize, fill, sketch),
				arrowMarker(statusMarkerID(cls, true), th.Edge.ArrowSize*thickArrowScale, fill, sketch),
			)
			if umlFill, ok := neededUML[cls]; ok {
				doc.Defs = append(doc.Defs, umlMarkers(cls, umlFill, th.Edge.StrokeWidth, sketch)...)
			}
		}
	}

	// Edges (rendered before nodes so arrowheads are not occluded).
	// Labels are collected separately and rendered after nodes so they
	// appear above all edge lines in the SVG z-order.
	var edgeLabels []Element
	for _, e := range graph.Edges {
		edgeStyle := th.Edge
		accent := edgeAccent(e)
		markerIDOverride := ""
		markerThickIDOverride := ""
		if accent != "" {
			dc := theme.DeriveColors(accent, th.Background, theme.ElementEdge)
			edgeStyle.Stroke = dc.Stroke
			edgeStyle.LabelFont.Color = dc.Text
			markerIDOverride = edgeMarkers[accent]
			markerThickIDOverride = edgeMarkersThick[accent]
		}
		cls := opts.class("edge", e.ID)
		if cls != "" {
			if _, ok := opts.MarkerFills[cls]; ok {
				// Status outranks the user's own edge color for the marker.
				markerIDOverride, markerThickIDOverride = statusMarkerID(cls, false), statusMarkerID(cls, true)
			}
		}
		adornRef := ""
		if e.Relation.Adorned() {
			suffix := ""
			if cls != "" {
				if _, ok := opts.MarkerFills[cls]; ok {
					suffix = cls
				}
			}
			if suffix == "" && accent != "" {
				suffix = accent[1:]
			}
			adornRef = fmt.Sprintf("url(#%s)", umlMarkerID(e.Relation, suffix))
		}
		edgeGroup, label := renderEdge(e, edgeStyle, th.Background, sketch, markerIDOverride, markerThickIDOverride, adornRef)
		edgeGroup.Class = cls
		doc.Children = append(doc.Children, edgeGroup)
		if label != nil {
			if cls != "" {
				label = SVGGroup{ID: "edge-label-" + strings.TrimPrefix(edgeGroup.ID, "edge-"), Class: cls, Children: []Element{label}}
			}
			edgeLabels = append(edgeLabels, label)
		}
		// Cardinalities (C14) ride in the same z-layer as edge labels.
		for _, card := range []struct {
			side string
			lbl  *model.EndLabel
		}{{"from", e.FromCard}, {"to", e.ToCard}} {
			if card.lbl == nil || card.lbl.Pos == nil || card.lbl.Text == "" {
				continue
			}
			var el Element = Text{
				X:                card.lbl.Pos.X,
				Y:                card.lbl.Pos.Y,
				Content:          card.lbl.Text,
				FontFamily:       edgeStyle.LabelFont.Family,
				FontSize:         fmt.Sprintf("%g", edgeStyle.LabelFont.Size),
				FontWeight:       fmt.Sprintf("%d", edgeStyle.LabelFont.Weight),
				Fill:             edgeStyle.LabelFont.Color,
				Anchor:           "middle",
				DominantBaseline: "central",
			}
			if cls != "" {
				el = SVGGroup{ID: "edge-card-" + card.side + "-" + strings.TrimPrefix(edgeGroup.ID, "edge-"), Class: cls, Children: []Element{el}}
			}
			edgeLabels = append(edgeLabels, el)
		}
	}

	// Nodes.
	for _, n := range graph.Nodes {
		nodeStyle := th.Node
		if accent := resolveAccent(n.Color, th); accent != "" {
			dc := theme.DeriveColors(accent, th.Background, theme.ElementNode)
			nodeStyle.Fill = dc.Fill
			if opts.ownStrokes() {
				nodeStyle.Stroke = dc.Stroke
				nodeStyle.Font.Color = dc.Text
			}
		}
		var grp SVGGroup
		if n.Members != nil {
			grp = RenderClassNode(n, nodeStyle, th.Class, sketch, opts.class("node", n.ID), func(compartment string, i int) string {
				return opts.class("member", n.ID+"/"+compartment+"/"+strconv.Itoa(i))
			})
		} else {
			grp = RenderNode(n, nodeStyle, sketch)
			grp.Class = opts.class("node", n.ID)
		}
		doc.Children = append(doc.Children, grp)
	}

	// Group titles, then edge labels, above all edges and nodes.
	doc.Children = append(doc.Children, groupTitles...)
	doc.Children = append(doc.Children, edgeLabels...)

	if len(graph.Legend) > 0 {
		doc.Children = append(doc.Children, renderLegend(graph.Legend, graph.Height, th))
	}
	if lines := opts.changes(); len(lines) > 0 {
		doc.Children = append(doc.Children, renderChangeList(lines, graph.Height+legendH, th))
	}

	doc.Children = withTitleBand(doc.Children, graph.Title, w, band, th.Node.Font)

	return doc.Render()
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
			Filter:      sketchDistortionFilterRef,
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

// edgeElementID maps an edge's logical id to its SVG element id. Derived ids
// keep today's edge-<from>-<to> form (with -<n> for the second and later
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
func renderEdge(e model.PositionedEdge, style theme.EdgeAppearance, background string, sketch bool, markerIDOverride, markerThickIDOverride, adornmentRef string) (SVGGroup, Element) {
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
			D:           sketchPolyline(pts, edgeID),
			Fill:        "none",
			Stroke:      style.Stroke,
			StrokeWidth: strokeWidth,
			StrokeDash:  strokeDash,
			MarkerStart: markerStart,
			MarkerEnd:   markerEnd,
			Filter:      sketchDistortionFilterRef,
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
			lx, ly, _ = edgeLabelPosition(e.Points)
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

// themeFontFamilies collects unique font families referenced by the theme.
func themeFontFamilies(th theme.Theme) []string {
	seen := map[string]bool{}
	families := []string{}
	for _, f := range []string{
		th.Node.Font.Family,
		th.Edge.LabelFont.Family,
		th.Group.LabelFont.Family,
		th.Actor.Font.Family,
		th.Fragment.LabelFont.Family,
		th.Class.MemberFont.Family,
	} {
		if !seen[f] {
			seen[f] = true
			families = append(families, f)
		}
	}
	return families
}

// edgeLabelPosition returns the midpoint of the polyline (by total path length)
// and the angle of the segment at that point.
func edgeLabelPosition(pts []model.Point) (x, y, angle float64) {
	if len(pts) < 2 {
		if len(pts) == 1 {
			return pts[0].X, pts[0].Y, 0
		}
		return 0, 0, 0
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
			dx := pts[i+1].X - pts[i].X
			dy := pts[i+1].Y - pts[i].Y
			angle = math.Atan2(dy, dx)
			return x, y, angle
		}
		cum += seg
	}

	// Fallback: last point.
	last := len(pts) - 1
	x, y = pts[last].X, pts[last].Y
	dx := pts[last].X - pts[last-1].X
	dy := pts[last].Y - pts[last-1].Y
	angle = math.Atan2(dy, dx)
	return x, y, angle
}
