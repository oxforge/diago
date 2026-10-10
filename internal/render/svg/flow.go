package svg

import (
	"fmt"
	"slices"
	"strconv"

	"github.com/oxforge/diago/internal/font"
	"github.com/oxforge/diago/internal/model"
	"github.com/oxforge/diago/internal/theme"
)

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

// ownAccent is an edge's or interaction's own accent color, which a
// status-paint render drops.
func ownAccent(color string, th theme.Theme, opts *RenderOptions) string {
	if !opts.ownStrokes() {
		return ""
	}
	return th.Accent(color)
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

// renderNodes renders the nodes, a class node as its record box with a
// class per member line. A node's accent sets its fill and, unless the
// render paints status, its stroke and text color.
func renderNodes(nodes []model.PositionedNode, th theme.Theme, opts *RenderOptions, sketch bool) []Element {
	out := make([]Element, 0, len(nodes))
	for _, n := range nodes {
		style := th.Node
		if accent := th.Accent(n.Color); accent != "" {
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
