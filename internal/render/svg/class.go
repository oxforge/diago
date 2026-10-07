package svg

import (
	"fmt"
	"io"
	"math"

	"github.com/oxforge/diago/internal/font"
	"github.com/oxforge/diago/internal/model"
	"github.com/oxforge/diago/internal/theme"
)

// Legend geometry: one row per entry below the layout, a wire sample and
// its label.
const (
	legendPad      = 10.0
	legendRow      = 22.0
	legendSampleX1 = 20.0
	legendSampleX2 = 46.0
	legendLabelX   = 54.0
	legendRightPad = 20.0
)

// umlMarkerID names the adornment marker a relation references: the base
// id, or with "-<suffix>" for an accent or status variant.
func umlMarkerID(r model.Relation, suffix string) string {
	var base string
	switch r {
	case model.RelationInheritance, model.RelationRealization:
		base = "diago-uml-triangle"
	case model.RelationAggregation:
		base = "diago-uml-diamond"
	case model.RelationComposition:
		base = "diago-uml-diamond-filled"
	default:
		return ""
	}
	if suffix != "" {
		return base + "-" + suffix
	}
	return base
}

// umlMarkers returns the three adornment markers for one stroke, in the
// fixed order triangle, diamond, filled diamond.
func umlMarkers(suffix, stroke string, strokeWidth float64) []Element {
	return []Element{
		TriangleMarker(umlMarkerID(model.RelationInheritance, suffix), stroke, strokeWidth),
		DiamondMarker(umlMarkerID(model.RelationAggregation, suffix), stroke, strokeWidth, false),
		DiamondMarker(umlMarkerID(model.RelationComposition, suffix), stroke, strokeWidth, true),
	}
}

// adornmentDepth is how far the drawn wire starts from its first point so
// the marker's base meets it (0 for an unadorned relation).
func adornmentDepth(r model.Relation) float64 {
	switch r {
	case model.RelationInheritance, model.RelationRealization:
		return umlTriangleDepth
	case model.RelationAggregation, model.RelationComposition:
		return umlDiamondDepth
	}
	return 0
}

// shortenStart returns a copy of pts whose first point moved depth along
// the first segment (capped at that segment's length). Render only: the
// positioned graph, and the contract checker that reads it, keep the full
// polyline.
func shortenStart(pts []model.Point, depth float64) []model.Point {
	if depth <= 0 || len(pts) < 2 {
		return pts
	}
	out := make([]model.Point, len(pts))
	copy(out, pts)
	dx, dy := pts[1].X-pts[0].X, pts[1].Y-pts[0].Y
	l := math.Hypot(dx, dy)
	if l < 1e-9 {
		return out
	}
	d := math.Min(depth, l)
	out[0] = model.Point{X: pts[0].X + dx/l*d, Y: pts[0].Y + dy/l*d}
	return out
}

// shortenStartWithCrossings shortens the first segment by depth (see
// shortenStart) and remaps any crossing recorded on segment 0 so the hop arc
// keeps its absolute position on the wire. Crossings are parametric against
// the unshortened polyline (C15.1, model.Crossing), so re-using the raw
// T on the shortened segment would slide the arc by depth×(1−T). A crossing
// the shortening swallowed is dropped. depth 0 (every flow edge) returns the
// inputs untouched, so flow output is byte-identical.
func shortenStartWithCrossings(pts []model.Point, depth float64, crossings []model.Crossing) ([]model.Point, []model.Crossing) {
	out := shortenStart(pts, depth)
	if len(crossings) == 0 || len(pts) < 2 || out[0] == pts[0] {
		return out, crossings
	}
	origLen := math.Hypot(pts[1].X-pts[0].X, pts[1].Y-pts[0].Y)
	newLen := math.Hypot(pts[1].X-out[0].X, pts[1].Y-out[0].Y)
	shift := origLen - newLen
	remapped := make([]model.Crossing, 0, len(crossings))
	for _, c := range crossings {
		if c.SegmentIndex != 0 {
			remapped = append(remapped, c)
			continue
		}
		if newLen < 1e-9 {
			continue // the whole first segment is under the adornment
		}
		t := (c.T*origLen - shift) / newLen
		if t < 0 {
			continue
		}
		remapped = append(remapped, model.Crossing{SegmentIndex: 0, T: t})
	}
	return out, remapped
}

// RenderClassNode draws a UML record box (S1 sizes it): a square-cornered
// rect, the header lines, a separator above each non-empty compartment and
// the left-aligned member lines. status is the node-level diff class (on
// the inner group; "" for none); memberClass returns the per-line class
// suffix ("" for none). Exported, so both optional inputs are guarded: a node
// with no Members falls back to the plain shape render, and a nil memberClass
// is read as "no class on any line".
func RenderClassNode(node model.PositionedNode, style theme.NodeStyle, class theme.ClassStyle, sketch bool, status string, memberClass func(compartment string, i int) string) SVGGroup {
	m := node.Members
	if m == nil {
		return RenderNode(node, style, sketch)
	}
	if memberClass == nil {
		memberClass = func(string, int) string { return "" }
	}
	x0, y0 := node.X-node.Width/2, node.Y-node.Height/2
	var shapeEl Element
	if sketch {
		shapeEl = Path{D: sketchRect(x0, y0, node.Width, node.Height, 0, node.ID), Fill: style.Fill, Stroke: style.Stroke, StrokeWidth: style.StrokeWidth, Filter: sketchDistortionFilterRef}
	} else {
		shapeEl = Rect{X: x0, Y: y0, Width: node.Width, Height: node.Height, Fill: style.Fill, Stroke: style.Stroke, StrokeWidth: style.StrokeWidth}
	}
	box := []Element{shapeEl}
	for i, line := range m.Header {
		t := Text{
			X: node.X, Y: y0 + model.ClassPadY + m.HeaderLineHeight/2 + float64(i)*m.HeaderLineHeight,
			Content: line.Text, FontFamily: style.Font.Family,
			FontSize: fmt.Sprintf("%g", style.Font.Size), FontWeight: "600",
			Fill: style.Font.Color, Anchor: "middle", DominantBaseline: "central",
		}
		if line.Stereotype {
			t.FontSize = fmt.Sprintf("%g", style.Font.Size-2)
			t.FontWeight = fmt.Sprintf("%d", style.Font.Weight)
		}
		if line.Italic {
			t.FontStyle = "italic"
		}
		box = append(box, t)
	}
	headerH := float64(len(m.Header))*m.HeaderLineHeight + 2*model.ClassPadY
	compartmentH := func(n int) float64 {
		if n == 0 {
			return 0
		}
		return float64(n)*m.MemberLineHeight + 2*model.ClassPadY
	}
	var members []Element
	top := y0 + headerH
	for _, comp := range []struct {
		name  string
		lines []model.PositionedLine
	}{{"attributes", m.Attributes}, {"methods", m.Methods}} {
		if len(comp.lines) == 0 {
			continue
		}
		sep := Line{X1: x0, Y1: top, X2: x0 + node.Width, Y2: top, Stroke: style.Stroke, StrokeWidth: class.SeparatorWidth}
		box = append(box, classedLine{Line: sep, Class: "compartment-sep"})
		for j, line := range comp.lines {
			t := Text{
				X: x0 + model.ClassPadX, Y: top + model.ClassPadY + m.MemberLineHeight/2 + float64(j)*m.MemberLineHeight,
				Class: "member", Content: line.Text,
				FontFamily: class.MemberFont.Family, FontSize: fmt.Sprintf("%g", class.MemberFont.Size),
				FontWeight: fmt.Sprintf("%d", class.MemberFont.Weight), Fill: style.Font.Color,
				Anchor: "start", DominantBaseline: "central",
			}
			if mc := memberClass(comp.name, j); mc != "" {
				t.Class += " " + mc
			}
			if line.Italic {
				t.FontStyle = "italic"
			}
			if line.Underline {
				t.TextDecoration = "underline"
			}
			members = append(members, t)
		}
		top += compartmentH(len(comp.lines))
	}
	return SVGGroup{ID: "node-" + node.ID, Children: []Element{
		SVGGroup{Class: status, Children: box},
		SVGGroup{Class: "members", Children: members},
	}}
}

// classedLine is a <line> with a class attribute (the compartment separator).
type classedLine struct {
	Line
	Class string
}

// Render writes the classed <line> element to w.
func (l classedLine) Render(w io.Writer) {
	fmt.Fprintf(w, `<line class="%s" x1="%s" y1="%s" x2="%s" y2="%s"`, escapeXML(l.Class), ff(l.X1), ff(l.Y1), ff(l.X2), ff(l.Y2))
	if l.Stroke != "" {
		fmt.Fprintf(w, ` stroke="%s"`, escapeXML(l.Stroke))
	}
	if l.StrokeWidth > 0 {
		fmt.Fprintf(w, ` stroke-width="%s"`, ff(l.StrokeWidth))
	}
	io.WriteString(w, `/>`)
}

// legendSize returns the legend block's required width and height.
func legendSize(entries []model.LegendEntry, th theme.Theme) (w, h float64) {
	if len(entries) == 0 {
		return 0, 0
	}
	widest := 0.0
	for _, e := range entries {
		lw, _ := font.MeasureText(e.Label, th.Edge.LabelFont.Size, th.Edge.LabelFont.Family)
		widest = math.Max(widest, lw)
	}
	return legendLabelX + widest + legendRightPad, legendPad + float64(len(entries))*legendRow + legendPad
}

// renderLegend draws one row per entry below top.
func renderLegend(entries []model.LegendEntry, top float64, th theme.Theme) SVGGroup {
	g := SVGGroup{ID: "legend"}
	for i, e := range entries {
		cy := top + legendPad + float64(i)*legendRow + legendRow/2
		r, _ := model.ParseRelation(e.Kind)
		dash := ""
		if r.Dashed() {
			dash = "8,4"
		}
		x1 := legendSampleX1 + adornmentDepth(r)
		line := Line{X1: x1, Y1: cy, X2: legendSampleX2, Y2: cy, Stroke: th.Edge.Stroke, StrokeWidth: th.Edge.StrokeWidth, StrokeDash: dash}
		if r == model.RelationDependency {
			line.MarkerEnd = "url(#" + arrowMarkerID + ")"
		}
		g.Children = append(g.Children, line)
		switch r {
		case model.RelationInheritance, model.RelationRealization:
			g.Children = append(g.Children, Polygon{Points: []model.Point{{X: legendSampleX1, Y: cy}, {X: legendSampleX1 + umlTriangleDepth, Y: cy - umlTriangleHalf}, {X: legendSampleX1 + umlTriangleDepth, Y: cy + umlTriangleHalf}}, Fill: "none", Stroke: th.Edge.Stroke, StrokeWidth: th.Edge.StrokeWidth})
		case model.RelationAggregation, model.RelationComposition:
			fill := "none"
			if r == model.RelationComposition {
				fill = th.Edge.Stroke
			}
			mid := legendSampleX1 + umlDiamondDepth/2
			g.Children = append(g.Children, Polygon{Points: []model.Point{{X: legendSampleX1, Y: cy}, {X: mid, Y: cy - umlDiamondHalf}, {X: legendSampleX1 + umlDiamondDepth, Y: cy}, {X: mid, Y: cy + umlDiamondHalf}}, Fill: fill, Stroke: th.Edge.Stroke, StrokeWidth: th.Edge.StrokeWidth})
		}
		g.Children = append(g.Children, Text{X: legendLabelX, Y: cy, Content: e.Label, FontFamily: th.Edge.LabelFont.Family,
			FontSize: fmt.Sprintf("%g", th.Edge.LabelFont.Size), FontWeight: fmt.Sprintf("%d", th.Edge.LabelFont.Weight),
			Fill: th.Edge.LabelFont.Color, DominantBaseline: "central"})
	}
	return g
}
