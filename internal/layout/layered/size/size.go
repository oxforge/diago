// Package size measures every node (S1).
package size

import (
	"context"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/oxforge/diago/internal/layout/layered/ports"
	"github.com/oxforge/diago/internal/layoutdbg"
	"github.com/oxforge/diago/internal/model"
)

// MeasureFunc measures one line of text in a font: its width and its line
// height.
type MeasureFunc func(text string, sizePt float64, family string) (w, h float64)

// Font is a font family and a point size.
type Font struct {
	Family string
	Size   float64
}

// Options carries S1's values from the Config (S14). With Cells set, every
// length is in character cells and text is measured in runes; Measure and
// the fonts are then unused.
type Options struct {
	Cells            bool
	Measure          MeasureFunc
	NodeFont         Font
	MemberFont       Font
	LabelFont        Font    // edge labels and cardinalities (S10)
	GroupFont        Font    // group titles (S9)
	WrapWidth        float64 // a label line wraps where it would grow wider
	PadX, PadY       float64 // label padding inside a node, on each side
	MinW, MinH       float64 // the smallest node box
	CylinderCap      float64 // screen: the height a cylinder adds for its caps
	ParallelogramPad float64 // screen: the width a parallelogram adds beside its slant
	DiamondPad       float64 // cells: the width a diamond adds for its decoration
	ClassPadX        float64 // screen: record-box padding beside the text
	ClassPadY        float64 // screen: record-box padding above and below a compartment
	ClassMinW        float64 // screen: the narrowest record-box text
}

// Size is a node's measured box in the output frame: W is horizontal.
type Size struct {
	W, H    float64
	Lines   []string                 // wrapped label lines; nil for one line without "\n"
	Members *model.PositionedMembers // a class node's record box; nil otherwise
}

// Unit-free shape ratios and counts (S1).
const (
	diamondScale   = 1.5 // a diamond's allowance for its text
	explicitAspect = 2.5 // the width-to-height floor of a box with explicit lines
	moreLines      = 3   // the line counts a compact label tries past its wrap's
)

// noise is S1's float tolerance, a millionth of a unit: a compact label's
// splits and counts within it of each other tie.
const noise = 1e-6

// Measure sizes every node, in order (S1).
func Measure(ctx context.Context, nodes []model.Node, o Options) []Size {
	out := make([]Size, len(nodes))
	for i, n := range nodes {
		switch {
		case n.Members != nil:
			out[i] = record(n, o)
		case o.Cells:
			out[i] = cellBox(n, o)
		default:
			out[i] = screenBox(ctx, n, o)
		}
		layoutdbg.Decision(ctx, "node_sized",
			"phase", "size", "module", "diago", "spec_ref", "S1",
			"node", n.ID, "width", out[i].W, "height", out[i].H,
			"lines", max(1, len(out[i].Lines)), "record", n.Members != nil)
	}
	return out
}

func screenBox(ctx context.Context, n model.Node, o Options) Size {
	// a compact label measures the same runs of words for every line count
	// it tries, so each text is measured once
	measured := map[string]float64{}
	width := func(s string) float64 {
		w, ok := measured[s]
		if !ok {
			w, _ = o.Measure(s, o.NodeFont.Size, o.NodeFont.Family)
			measured[s] = w
		}
		return w
	}
	_, lineH := o.Measure("X", o.NodeFont.Size, o.NodeFont.Family)
	explicit := strings.Contains(n.Label, "\n")
	box := func(lines []string) (w, h float64) {
		widest := 0.0
		for _, l := range lines {
			widest = max(widest, width(l))
		}
		w = widest + 2*o.PadX
		h = lineH*float64(len(lines)) + 2*o.PadY
		w, h = max(w, o.MinW), max(h, o.MinH)
		w = max(w, h)
		if explicit && !fixedRatio(n.Shape) {
			w = max(w, explicitAspect*h)
		}
		switch n.Shape {
		case model.ShapeDiamond:
			w, h = w*diamondScale, h*diamondScale
			w = max(w, h*math.Sqrt(3))
			h = max(h, w/math.Sqrt(3))
		case model.ShapeCircle:
			d := max(w, h)
			w, h = d, d
		case model.ShapeCylinder:
			h += o.CylinderCap
		case model.ShapeHexagon:
			h = max(h, w*math.Sqrt(3)/2)
			w = max(w, h*2/math.Sqrt(3))
		case model.ShapeParallelogram:
			w += o.ParallelogramPad + ports.Slant*h
		}
		return w, h
	}
	lines := wrap(n.Label, o.WrapWidth, width)
	if fixedRatio(n.Shape) && !explicit {
		lines = compact(ctx, n.ID, lines, strings.Fields(n.Label), box, width)
	}
	w, h := box(lines)
	return Size{W: w, H: h, Lines: lineList(lines, n.Label)}
}

// compact is S1's Compact labels: a fixed-ratio shape's label of two or
// more words tries the balanced wrap of every line count from its wrap's
// own up to moreLines more, never more lines than words, and keeps the
// count whose box is narrowest, which the shape rule makes its larger
// extent; a tie keeps fewer lines. A label of one word keeps its wrap.
func compact(ctx context.Context, id string, wrapped, words []string, box func([]string) (w, h float64), width func(string) float64) []string {
	if len(words) < 2 {
		return wrapped
	}
	var best []string
	bestW := math.Inf(1)
	first, last := len(wrapped), min(len(wrapped)+moreLines, len(words))
	// the wrap's lines, split further, give every count from its own on
	// with no line wider than its widest, so no balanced line is wider
	bound := 0.0
	for _, l := range wrapped {
		bound = max(bound, width(l))
	}
	widths := make([]float64, 0, last-first+1)
	for count := first; count <= last; count++ {
		lines := balanced(words, count, bound, width)
		w, _ := box(lines)
		widths = append(widths, w)
		if w < bestW-noise {
			best, bestW = lines, w
		}
	}
	layoutdbg.Decision(ctx, "label_compacted",
		"phase", "size", "module", "diago", "spec_ref", "S1",
		"node", id, "wrap_lines", first, "lines", len(best), "widths", widths)
	return best
}

func cellBox(n model.Node, o Options) Size {
	lines := wrap(n.Label, o.WrapWidth, runes)
	widest := 0.0
	for _, l := range lines {
		widest = max(widest, runes(l))
	}
	w := widest + 2*o.PadX
	h := float64(len(lines)) + 2*o.PadY
	if n.Shape == model.ShapeDiamond {
		w += o.DiamondPad
	}
	w, h = max(w, o.MinW), max(h, o.MinH)
	return Size{W: even(w), H: h, Lines: lineList(lines, n.Label)}
}

func fixedRatio(s model.Shape) bool {
	return s == model.ShapeDiamond || s == model.ShapeCircle || s == model.ShapeHexagon
}

// lineList drops the line list of a label that stayed a single line
// identical to the label as written; otherwise the caller keeps it (S1: a
// wrapped, explicit or space-collapsed label carries a list that differs
// from the raw label, so a renderer draws what was measured).
func lineList(lines []string, label string) []string {
	if len(lines) == 1 && lines[0] == label {
		return nil
	}
	return lines
}

// Label measures an edge label or a cardinality as one line (S10): in the
// label font on screen, one cell per rune by one row in cells.
func Label(text string, o Options) (w, h float64) {
	if o.Cells {
		return runes(text), 1
	}
	return o.Measure(text, o.LabelFont.Size, o.LabelFont.Family)
}

// Title measures a group title as one line in the group font (S9); in
// cells, one per rune by one row.
func Title(text string, o Options) (w, h float64) {
	if o.Cells {
		return runes(text), 1
	}
	return o.Measure(text, o.GroupFont.Size, o.GroupFont.Family)
}

func runes(s string) float64 { return float64(utf8.RuneCountInString(s)) }

// even rounds a cell count up to an even number, which puts the center on
// a cell boundary.
func even(cells float64) float64 { return math.Ceil(cells/2) * 2 }
