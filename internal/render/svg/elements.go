// Package svg provides structured SVG element types and a document assembler.
// Each element implements the Element interface and writes its XML representation
// to an io.Writer. No string concatenation is used for SVG generation.
package svg

import (
	"fmt"
	"io"
	"strings"

	"github.com/oxforge/diago/internal/model"
)

// Element is the interface implemented by all SVG elements.
type Element interface {
	Render(w io.Writer)
}

// escapeXML replaces XML special characters so they are safe in attribute values
// and text content.
func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

// ff formats a float64 for SVG output, trimming unnecessary trailing zeros.
func ff(v float64) string {
	s := fmt.Sprintf("%.4f", v)
	// Trim trailing zeros after the decimal point.
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	return s
}

// Rect is an SVG <rect> element.
type Rect struct {
	Class               string // CSS class; "" = no class attribute
	X, Y, Width, Height float64
	Rx                  float64 // corner radius
	Fill, Stroke        string
	FillOpacity         float64 // 0 means default (fully opaque); 0 < v <= 1 sets fill-opacity
	StrokeWidth         float64
	StrokeDash          string
	Filter              string // e.g., "url(#sketch-wobble)"
}

// Render writes the <rect> element to w.
func (r Rect) Render(w io.Writer) {
	io.WriteString(w, "<rect")
	if r.Class != "" {
		fmt.Fprintf(w, ` class="%s"`, escapeXML(r.Class))
	}
	fmt.Fprintf(w, ` x="%s" y="%s" width="%s" height="%s"`,
		ff(r.X), ff(r.Y), ff(r.Width), ff(r.Height))
	if r.Rx > 0 {
		fmt.Fprintf(w, ` rx="%s" ry="%s"`, ff(r.Rx), ff(r.Rx))
	}
	if r.Fill != "" {
		fmt.Fprintf(w, ` fill="%s"`, escapeXML(r.Fill))
	}
	if r.FillOpacity > 0 && r.FillOpacity <= 1 {
		fmt.Fprintf(w, ` fill-opacity="%s"`, ff(r.FillOpacity))
	}
	if r.Stroke != "" {
		fmt.Fprintf(w, ` stroke="%s"`, escapeXML(r.Stroke))
	}
	if r.StrokeWidth > 0 {
		fmt.Fprintf(w, ` stroke-width="%s"`, ff(r.StrokeWidth))
	}
	if r.StrokeDash != "" {
		fmt.Fprintf(w, ` stroke-dasharray="%s"`, escapeXML(r.StrokeDash))
	}
	if r.Filter != "" {
		fmt.Fprintf(w, ` filter="%s"`, escapeXML(r.Filter))
	}
	io.WriteString(w, `/>`)
}

// Circle is an SVG <circle> element.
type Circle struct {
	CX, CY, R    float64
	Fill, Stroke string
	StrokeWidth  float64
	Filter       string
}

// Render writes the <circle> element to w.
func (c Circle) Render(w io.Writer) {
	fmt.Fprintf(w, `<circle cx="%s" cy="%s" r="%s"`, ff(c.CX), ff(c.CY), ff(c.R))
	if c.Fill != "" {
		fmt.Fprintf(w, ` fill="%s"`, escapeXML(c.Fill))
	}
	if c.Stroke != "" {
		fmt.Fprintf(w, ` stroke="%s"`, escapeXML(c.Stroke))
	}
	if c.StrokeWidth > 0 {
		fmt.Fprintf(w, ` stroke-width="%s"`, ff(c.StrokeWidth))
	}
	if c.Filter != "" {
		fmt.Fprintf(w, ` filter="%s"`, escapeXML(c.Filter))
	}
	io.WriteString(w, `/>`)
}

// Ellipse is an SVG <ellipse> element.
type Ellipse struct {
	CX, CY, RX, RY float64
	Fill, Stroke   string
	StrokeWidth    float64
	Filter         string
}

// Render writes the <ellipse> element to w.
func (e Ellipse) Render(w io.Writer) {
	fmt.Fprintf(w, `<ellipse cx="%s" cy="%s" rx="%s" ry="%s"`,
		ff(e.CX), ff(e.CY), ff(e.RX), ff(e.RY))
	if e.Fill != "" {
		fmt.Fprintf(w, ` fill="%s"`, escapeXML(e.Fill))
	}
	if e.Stroke != "" {
		fmt.Fprintf(w, ` stroke="%s"`, escapeXML(e.Stroke))
	}
	if e.StrokeWidth > 0 {
		fmt.Fprintf(w, ` stroke-width="%s"`, ff(e.StrokeWidth))
	}
	if e.Filter != "" {
		fmt.Fprintf(w, ` filter="%s"`, escapeXML(e.Filter))
	}
	io.WriteString(w, `/>`)
}

// Line is an SVG <line> element.
type Line struct {
	X1, Y1, X2, Y2 float64
	Stroke         string
	StrokeWidth    float64
	StrokeDash     string
	MarkerEnd      string // e.g., "url(#arrowhead)"
	Filter         string
}

// Render writes the <line> element to w.
func (l Line) Render(w io.Writer) {
	fmt.Fprintf(w, `<line x1="%s" y1="%s" x2="%s" y2="%s"`,
		ff(l.X1), ff(l.Y1), ff(l.X2), ff(l.Y2))
	if l.Stroke != "" {
		fmt.Fprintf(w, ` stroke="%s"`, escapeXML(l.Stroke))
	}
	if l.StrokeWidth > 0 {
		fmt.Fprintf(w, ` stroke-width="%s"`, ff(l.StrokeWidth))
	}
	if l.StrokeDash != "" {
		fmt.Fprintf(w, ` stroke-dasharray="%s"`, escapeXML(l.StrokeDash))
	}
	if l.MarkerEnd != "" {
		fmt.Fprintf(w, ` marker-end="%s"`, escapeXML(l.MarkerEnd))
	}
	if l.Filter != "" {
		fmt.Fprintf(w, ` filter="%s"`, escapeXML(l.Filter))
	}
	io.WriteString(w, `/>`)
}

// Polyline is an SVG <polyline> element used for edge paths.
type Polyline struct {
	Points      []model.Point
	Stroke      string
	StrokeWidth float64
	StrokeDash  string
	MarkerStart string // e.g., "url(#arrowhead)"
	MarkerEnd   string // e.g., "url(#arrowhead)"
	Filter      string
}

// Render writes the <polyline> element to w.
func (p Polyline) Render(w io.Writer) {
	if len(p.Points) == 0 {
		return
	}
	io.WriteString(w, `<polyline points="`)
	for i, pt := range p.Points {
		if i > 0 {
			io.WriteString(w, " ")
		}
		fmt.Fprintf(w, "%s,%s", ff(pt.X), ff(pt.Y))
	}
	io.WriteString(w, `" fill="none"`)
	if p.Stroke != "" {
		fmt.Fprintf(w, ` stroke="%s"`, escapeXML(p.Stroke))
	}
	if p.StrokeWidth > 0 {
		fmt.Fprintf(w, ` stroke-width="%s"`, ff(p.StrokeWidth))
	}
	if p.StrokeDash != "" {
		fmt.Fprintf(w, ` stroke-dasharray="%s"`, escapeXML(p.StrokeDash))
	}
	if p.MarkerStart != "" {
		fmt.Fprintf(w, ` marker-start="%s"`, escapeXML(p.MarkerStart))
	}
	if p.MarkerEnd != "" {
		fmt.Fprintf(w, ` marker-end="%s"`, escapeXML(p.MarkerEnd))
	}
	if p.Filter != "" {
		fmt.Fprintf(w, ` filter="%s"`, escapeXML(p.Filter))
	}
	io.WriteString(w, `/>`)
}

// Path is an SVG <path> element.
type Path struct {
	D            string
	Fill, Stroke string
	StrokeWidth  float64
	StrokeDash   string
	MarkerStart  string // e.g., "url(#arrowhead)"
	MarkerEnd    string // e.g., "url(#arrowhead)"
	Filter       string
}

// Render writes the <path> element to w.
func (p Path) Render(w io.Writer) {
	fmt.Fprintf(w, `<path d="%s"`, escapeXML(p.D))
	if p.Fill != "" {
		fmt.Fprintf(w, ` fill="%s"`, escapeXML(p.Fill))
	}
	if p.Stroke != "" {
		fmt.Fprintf(w, ` stroke="%s"`, escapeXML(p.Stroke))
	}
	if p.StrokeWidth > 0 {
		fmt.Fprintf(w, ` stroke-width="%s"`, ff(p.StrokeWidth))
	}
	if p.StrokeDash != "" {
		fmt.Fprintf(w, ` stroke-dasharray="%s"`, escapeXML(p.StrokeDash))
	}
	if p.MarkerStart != "" {
		fmt.Fprintf(w, ` marker-start="%s"`, escapeXML(p.MarkerStart))
	}
	if p.MarkerEnd != "" {
		fmt.Fprintf(w, ` marker-end="%s"`, escapeXML(p.MarkerEnd))
	}
	if p.Filter != "" {
		fmt.Fprintf(w, ` filter="%s"`, escapeXML(p.Filter))
	}
	io.WriteString(w, `/>`)
}

// Polygon is an SVG <polygon> element.
type Polygon struct {
	Points       []model.Point
	Fill, Stroke string
	StrokeWidth  float64
	Filter       string
}

// Render writes the <polygon> element to w.
func (p Polygon) Render(w io.Writer) {
	io.WriteString(w, `<polygon points="`)
	for i, pt := range p.Points {
		if i > 0 {
			io.WriteString(w, " ")
		}
		fmt.Fprintf(w, "%s,%s", ff(pt.X), ff(pt.Y))
	}
	io.WriteString(w, `"`)
	if p.Fill != "" {
		fmt.Fprintf(w, ` fill="%s"`, escapeXML(p.Fill))
	}
	if p.Stroke != "" {
		fmt.Fprintf(w, ` stroke="%s"`, escapeXML(p.Stroke))
	}
	if p.StrokeWidth > 0 {
		fmt.Fprintf(w, ` stroke-width="%s"`, ff(p.StrokeWidth))
	}
	if p.Filter != "" {
		fmt.Fprintf(w, ` filter="%s"`, escapeXML(p.Filter))
	}
	io.WriteString(w, `/>`)
}
