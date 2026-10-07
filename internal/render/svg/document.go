package svg

import (
	"fmt"
	"io"
	"strings"
)

// SVGGroup is an SVG <g> element that groups child elements.
type SVGGroup struct {
	ID        string
	Class     string // CSS class; "" = no class attribute
	Children  []Element
	Transform string
	Opacity   string // e.g., "0.25"
}

// Render writes the <g> element and all children to w.
func (g SVGGroup) Render(w io.Writer) {
	io.WriteString(w, "<g")
	if g.ID != "" {
		fmt.Fprintf(w, ` id="%s"`, escapeXML(g.ID))
	}
	if g.Class != "" {
		fmt.Fprintf(w, ` class="%s"`, escapeXML(g.Class))
	}
	if g.Transform != "" {
		fmt.Fprintf(w, ` transform="%s"`, escapeXML(g.Transform))
	}
	if g.Opacity != "" {
		fmt.Fprintf(w, ` opacity="%s"`, escapeXML(g.Opacity))
	}
	io.WriteString(w, ">")
	for _, child := range g.Children {
		child.Render(w)
	}
	io.WriteString(w, "</g>")
}

// Defs is an SVG <defs> element holding reusable elements like markers.
type Defs struct {
	Children []Element
}

// Render writes the <defs> element and its children to w.
func (d Defs) Render(w io.Writer) {
	io.WriteString(w, "<defs>")
	for _, child := range d.Children {
		child.Render(w)
	}
	io.WriteString(w, "</defs>")
}

// Marker is an SVG <marker> element used to define arrowheads.
type Marker struct {
	ID            string
	ViewBox       string
	RefX, RefY    float64
	Width, Height float64
	Orient        string
	MarkerUnits   string
	Children      []Element
}

// Render writes the <marker> element and its children to w.
func (m Marker) Render(w io.Writer) {
	fmt.Fprintf(w, `<marker id="%s"`, escapeXML(m.ID))
	if m.ViewBox != "" {
		fmt.Fprintf(w, ` viewBox="%s"`, escapeXML(m.ViewBox))
	}
	fmt.Fprintf(w, ` refX="%s" refY="%s"`, ff(m.RefX), ff(m.RefY))
	fmt.Fprintf(w, ` markerWidth="%s" markerHeight="%s"`, ff(m.Width), ff(m.Height))
	if m.Orient != "" {
		fmt.Fprintf(w, ` orient="%s"`, escapeXML(m.Orient))
	}
	if m.MarkerUnits != "" {
		fmt.Fprintf(w, ` markerUnits="%s"`, escapeXML(m.MarkerUnits))
	}
	io.WriteString(w, ">")
	for _, child := range m.Children {
		child.Render(w)
	}
	io.WriteString(w, "</marker>")
}

// RawXML is an element that writes pre-formed XML content verbatim.
type RawXML struct {
	Content string
}

// Render writes the raw XML content to w.
func (r RawXML) Render(w io.Writer) {
	io.WriteString(w, r.Content)
}

// SVGDoc is a complete SVG document.
type SVGDoc struct {
	Width, Height float64
	ViewBox       string
	Defs          []Element
	Children      []Element
	FontFace      string // @font-face CSS block
	Styles        string // extra <style> block (diff renders); "" = none
	Metadata      string // JSON for <metadata id="diago-layout">; empty = none
}

// Render writes the complete SVG document as a string.
func (d SVGDoc) Render() string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	fmt.Fprintf(&sb, `<svg xmlns="http://www.w3.org/2000/svg" width="%s" height="%s"`,
		ff(d.Width), ff(d.Height))
	if d.ViewBox != "" {
		fmt.Fprintf(&sb, ` viewBox="%s"`, d.ViewBox)
	}
	sb.WriteString(`>`)

	// Emit <style> with embedded @font-face.
	if d.FontFace != "" {
		sb.WriteString("<style>")
		sb.WriteString(d.FontFace)
		sb.WriteString("</style>")
	}

	// Emit the extra diff style block, right after the font faces.
	if d.Styles != "" {
		sb.WriteString("<style>")
		sb.WriteString(d.Styles)
		sb.WriteString("</style>")
	}

	// Emit <defs>.
	if len(d.Defs) > 0 {
		defs := Defs{Children: d.Defs}
		defs.Render(&sb)
	}

	// Emit <metadata id="diago-layout"> carrying the anchoring carrier (C18).
	if d.Metadata != "" {
		fmt.Fprintf(&sb, `<metadata id="%s">%s</metadata>`, LayoutMetadataID, escapeXML(d.Metadata))
	}

	// Emit children.
	for _, child := range d.Children {
		child.Render(&sb)
	}

	sb.WriteString(`</svg>`)
	return sb.String()
}
