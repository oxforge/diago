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
// RenderOptions. A nil opts produces byte-identical output to Render.
func RenderWithOptions(graph *model.PositionedGraph, th theme.Theme, opts *RenderOptions) string {
	// The class legend and then the change list grow the canvas below the
	// graph and, for a long label, to the right. Without either both are
	// zero and the canvas is exactly the graph's.
	legendW, legendH := legendSize(graph.Legend, th)
	band, titleW := titleBand(graph.Title, th.Node.Font)
	changesW, changesH := changeListSize(opts.caption(), opts.changes(), th)
	w := max(graph.Width, legendW, changesW, titleW)
	h := graph.Height + legendH + changesH + band

	doc := newDocument(w, h, th, opts)
	doc.Metadata = layoutMetadataJSON(graph.LayoutHints)
	sketch := th.Style == "sketch"
	doc.Defs = flowMarkers(graph.Edges, th, opts, sketch)

	// Groups, parents behind children, then edges, then nodes, so no node
	// hides an arrowhead; group titles (C13), then edge labels and
	// cardinalities, above all edges and nodes.
	ids := idSet{}
	groups, titles := renderGroups(graph.Groups, th, opts, sketch, ids)
	edges, labels := renderEdges(graph.Edges, th, opts, sketch, ids)
	doc.Children = append(doc.Children, groups...)
	doc.Children = append(doc.Children, edges...)
	doc.Children = append(doc.Children, renderNodes(graph.Nodes, th, opts, sketch)...)
	doc.Children = append(doc.Children, titles...)
	doc.Children = append(doc.Children, labels...)

	if len(graph.Legend) > 0 {
		doc.Children = append(doc.Children, renderLegend(graph.Legend, graph.Height, th))
	}
	if caption, lines := opts.caption(), opts.changes(); caption != "" || len(lines) > 0 {
		doc.Children = append(doc.Children, renderChangeList(caption, lines, graph.Height+legendH, th))
	}

	doc.Children = withTitleBand(doc.Children, graph.Title, w, band, th.Node.Font)

	return doc.Render()
}

// newDocument is a w × h document with the theme's font faces, the
// render's extra style block, and the background rectangle as its first
// child.
func newDocument(w, h float64, th theme.Theme, opts *RenderOptions) SVGDoc {
	doc := SVGDoc{
		Width:    w,
		Height:   h,
		ViewBox:  fmt.Sprintf("0 0 %s %s", ff(w), ff(h)),
		FontFace: font.FontFacesCSS(themeFontFamilies(th)),
		Children: []Element{Rect{X: 0, Y: 0, Width: w, Height: h, Fill: th.Background}},
	}
	if opts != nil {
		doc.Styles = opts.Styles
	}
	return doc
}

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
	slices.SortStableFunc(m.statuses, func(a, b markerUse) int { return strings.Compare(a.suffix, b.suffix) })
	return append(markerDefs(m.accents, pair, trio), markerDefs(m.statuses, pair, trio)...)
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

// ownAccent is an edge's or interaction's own accent color, which a
// status-paint render drops.
func ownAccent(color string, th theme.Theme, opts *RenderOptions) string {
	if !opts.ownStrokes() {
		return ""
	}
	return resolveAccent(color, th)
}

// accentEdge is the theme's edge appearance, with the stroke and label
// color accent derives when there is one.
func accentEdge(th theme.Theme, accent string) theme.EdgeAppearance {
	style := th.Edge
	if accent != "" {
		dc := theme.DeriveColors(accent, th.Background, theme.ElementEdge)
		style.Stroke = dc.Stroke
		style.LabelFont.Color = dc.Text
	}
	return style
}

// flowMarkers returns a flow render's marker defs: the base filled and
// thick arrowheads, the base UML trio when some class relation is adorned,
// then the markerSet the edges use.
func flowMarkers(edges []model.PositionedEdge, th theme.Theme, opts *RenderOptions, sketch bool) []Element {
	size, stroke := th.Edge.ArrowSize, th.Edge.Stroke
	pair := func(suffix, fill string) []Element {
		return []Element{
			arrowMarker(statusMarkerID(suffix, false), size, fill, sketch),
			arrowMarker(statusMarkerID(suffix, true), size*thickArrowScale, fill, sketch),
		}
	}
	trio := func(suffix, fill string) []Element {
		return umlMarkers(suffix, fill, th.Edge.StrokeWidth, sketch)
	}
	defs := []Element{
		arrowMarker(arrowMarkerID, size, stroke, sketch),
		arrowMarker(arrowMarkerThickID, size*thickArrowScale, stroke, sketch),
	}
	if slices.ContainsFunc(edges, func(e model.PositionedEdge) bool { return e.Relation.Adorned() }) {
		defs = append(defs, trio("", stroke)...)
	}
	var set markerSet
	for _, e := range edges {
		set.add(ownAccent(e.Color, th, opts), opts.class("edge", e.ID), e.Relation.Adorned(), th, opts)
	}
	return append(defs, set.defs(pair, trio)...)
}

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
	accent := resolveAccent(g.Color, th)
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

// renderNodes renders the nodes, a class node as its record box with a
// class per member line. A node's accent sets its fill and, unless the
// render paints status, its stroke and text color.
func renderNodes(nodes []model.PositionedNode, th theme.Theme, opts *RenderOptions, sketch bool) []Element {
	out := make([]Element, 0, len(nodes))
	for _, n := range nodes {
		style := th.Node
		if accent := resolveAccent(n.Color, th); accent != "" {
			dc := theme.DeriveColors(accent, th.Background, theme.ElementNode)
			style.Fill = dc.Fill
			if opts.ownStrokes() {
				style.Stroke = dc.Stroke
				style.Font.Color = dc.Text
			}
		}
		if n.Members == nil {
			grp := renderNode(n, style, sketch)
			grp.Class = opts.class("node", n.ID)
			out = append(out, grp)
			continue
		}
		out = append(out, renderClassNode(n, style, th.Class, sketch, opts.class("node", n.ID), func(compartment string, i int) string {
			return opts.class("member", n.ID+"/"+compartment+"/"+strconv.Itoa(i))
		}))
	}
	return out
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
