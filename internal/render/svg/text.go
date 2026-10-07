package svg

import (
	"fmt"
	"io"
)

// Text is an SVG <text> element.
type Text struct {
	X, Y             float64
	Class            string   // CSS class; "" = no class attribute
	Content          string   // single-line text (used when Lines is nil)
	Lines            []string // multi-line text (takes precedence over Content)
	LineHeight       float64  // line spacing in px (used with Lines)
	FontFamily       string
	FontSize         string // e.g., "14"
	FontWeight       string // e.g., "500"
	FontStyle        string // e.g., "italic"
	Fill             string
	TextDecoration   string // e.g., "underline"
	Anchor           string // text-anchor: "start", "middle", "end"
	DominantBaseline string // e.g., "middle", "auto"
	LetterSpacing    string // e.g., "0.3"
}

// Render writes the <text> element to w.
func (t Text) Render(w io.Writer) {
	if len(t.Lines) > 1 {
		t.renderMultiLine(w)
		return
	}
	fmt.Fprintf(w, `<text x="%s" y="%s"`, ff(t.X), ff(t.Y))
	t.writeAttrs(w)
	fmt.Fprintf(w, `>%s</text>`, escapeXML(t.Content))
}

// renderMultiLine writes a <text> with <tspan> children, vertically centered.
func (t Text) renderMultiLine(w io.Writer) {
	nLines := len(t.Lines)
	lh := t.LineHeight
	if lh == 0 {
		lh = 18
	}
	// Start Y so the block is vertically centered around t.Y.
	startY := t.Y - float64(nLines-1)*lh/2

	fmt.Fprintf(w, `<text x="%s" y="%s"`, ff(t.X), ff(startY))
	t.writeAttrs(w)
	io.WriteString(w, ">")
	for i, line := range t.Lines {
		if i == 0 {
			fmt.Fprintf(w, `<tspan x="%s">%s</tspan>`, ff(t.X), escapeXML(line))
		} else {
			fmt.Fprintf(w, `<tspan x="%s" dy="%s">%s</tspan>`, ff(t.X), ff(lh), escapeXML(line))
		}
	}
	io.WriteString(w, "</text>")
}

// writeAttrs writes the common font/style attributes.
func (t Text) writeAttrs(w io.Writer) {
	if t.Class != "" {
		fmt.Fprintf(w, ` class="%s"`, escapeXML(t.Class))
	}
	if t.FontFamily != "" {
		fmt.Fprintf(w, ` font-family="%s"`, escapeXML(t.FontFamily))
	}
	if t.FontSize != "" {
		fmt.Fprintf(w, ` font-size="%s"`, escapeXML(t.FontSize))
	}
	if t.FontWeight != "" {
		fmt.Fprintf(w, ` font-weight="%s"`, escapeXML(t.FontWeight))
	}
	if t.FontStyle != "" {
		fmt.Fprintf(w, ` font-style="%s"`, escapeXML(t.FontStyle))
	}
	if t.Fill != "" {
		fmt.Fprintf(w, ` fill="%s"`, escapeXML(t.Fill))
	}
	if t.TextDecoration != "" {
		fmt.Fprintf(w, ` text-decoration="%s"`, escapeXML(t.TextDecoration))
	}
	if t.Anchor != "" {
		fmt.Fprintf(w, ` text-anchor="%s"`, escapeXML(t.Anchor))
	}
	if t.DominantBaseline != "" {
		fmt.Fprintf(w, ` dominant-baseline="%s"`, escapeXML(t.DominantBaseline))
	}
	if t.LetterSpacing != "" {
		fmt.Fprintf(w, ` letter-spacing="%s"`, escapeXML(t.LetterSpacing))
	}
}
