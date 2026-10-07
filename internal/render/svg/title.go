package svg

import (
	"fmt"

	"github.com/oxforge/diago/internal/font"
	"github.com/oxforge/diago/internal/theme"
)

// Title band. A non-empty title is drawn centered in a band
// above the content, in the node (or actor) font three points larger at
// weight 600. The band grows the canvas; the content is shifted down by
// it. An empty title emits nothing, so plain renders are byte-identical.
const (
	titleSizeDelta  = 3.0   // added to the base font size
	titleBandPad    = 16.0  // vertical padding, 8 above and 8 below the text
	titleSidePad    = 20.0  // width reserved around the measured title
	titleFontWeight = "600" // semibold, one step above the node label weight
)

// titleBand returns the band height and the canvas width the title needs;
// both are 0 for an empty title.
func titleBand(title string, f theme.FontStyle) (height, width float64) {
	if title == "" {
		return 0, 0
	}
	size := f.Size + titleSizeDelta
	w, _ := font.MeasureText(title, size, f.Family)
	return size + titleBandPad, w + titleSidePad
}

// withTitleBand inserts the title element after the background rect
// (children[0], already sized to the grown canvas) and wraps every other
// child in a group translated down by band. A zero band returns children
// unchanged.
func withTitleBand(children []Element, title string, w, band float64, f theme.FontStyle) []Element {
	if band == 0 {
		return children
	}
	label := Text{
		X: w / 2, Y: band / 2,
		Content:          title,
		FontFamily:       f.Family,
		FontSize:         ff(f.Size + titleSizeDelta),
		FontWeight:       titleFontWeight,
		Fill:             f.Color,
		Anchor:           "middle",
		DominantBaseline: "middle",
	}
	content := SVGGroup{Transform: fmt.Sprintf("translate(0,%s)", ff(band)), Children: children[1:]}
	return []Element{children[0], label, content}
}
