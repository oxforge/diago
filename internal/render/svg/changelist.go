package svg

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/oxforge/diago/internal/font"
	"github.com/oxforge/diago/internal/model"
	"github.com/oxforge/diago/internal/theme"
)

// ChangeLine is one row of a diff render's change list.
type ChangeLine struct {
	Color  string // the status color: the sample's stroke and the text's fill
	Dashed bool   // the sample is dashed (a removed element)
	Wire   bool   // the sample is a short wire (an edge, relation or message), else a box
	Text   string
}

// Change list sample geometry, centered in the legend's sample column.
const (
	changeBoxW   = 18.0
	changeBoxH   = 12.0
	changeStroke = 2.0
)

// changeText is a line's text as drawn: the value arrow of a changed field
// falls back to "->" when the theme's label font has no glyph for it (the
// sketch theme's Pangolin), so the row is neither drawn nor measured in a
// substitute face.
func changeText(text string, th theme.Theme) string {
	if font.HasGlyph(th.Edge.LabelFont.Family, '→') {
		return text
	}
	return strings.ReplaceAll(text, "→", "->")
}

// changeListSize returns the change list's required width and height, with
// the class legend's metrics: one row per line.
func changeListSize(lines []ChangeLine, th theme.Theme) (w, h float64) {
	if len(lines) == 0 {
		return 0, 0
	}
	widest := 0.0
	for _, l := range lines {
		lw, _ := font.MeasureText(changeText(l.Text, th), th.Edge.LabelFont.Size, th.Edge.LabelFont.Family)
		widest = max(widest, lw)
	}
	return legendLabelX + widest + legendRightPad, legendPad + float64(len(lines))*legendRow + legendPad
}

// renderChangeList draws one row per line below top: a sample in the line's
// color (a box, or a wire for a connection), dashed for a removed element
// and drawn by hand in a sketch theme, then the text in the same color. It
// carries no status class, so the diff styles never fade or dash it, and
// takes no distortion filter, as the legend's samples.
func renderChangeList(lines []ChangeLine, top float64, th theme.Theme) SVGGroup {
	g := SVGGroup{ID: "changes"}
	sketch := th.Style == "sketch"
	for i, l := range lines {
		cy := top + legendPad + float64(i)*legendRow + legendRow/2
		dash := ""
		if l.Dashed {
			dash = "4,3"
		}
		seed := "change-" + strconv.Itoa(i)
		var sample Element
		switch {
		case l.Wire && sketch:
			sample = Path{D: sketchPolyline([]model.Point{{X: legendSampleX1, Y: cy}, {X: legendSampleX2, Y: cy}}, seed),
				Fill: "none", Stroke: l.Color, StrokeWidth: changeStroke, StrokeDash: dash}
		case l.Wire:
			sample = Line{X1: legendSampleX1, Y1: cy, X2: legendSampleX2, Y2: cy, Stroke: l.Color, StrokeWidth: changeStroke, StrokeDash: dash}
		case sketch:
			x := (legendSampleX1 + legendSampleX2 - changeBoxW) / 2
			sample = Path{D: sketchRect(x, cy-changeBoxH/2, changeBoxW, changeBoxH, 2, seed),
				Fill: "none", Stroke: l.Color, StrokeWidth: changeStroke, StrokeDash: dash}
		default:
			x := (legendSampleX1 + legendSampleX2 - changeBoxW) / 2
			sample = Rect{X: x, Y: cy - changeBoxH/2, Width: changeBoxW, Height: changeBoxH, Rx: 2,
				Fill: "none", Stroke: l.Color, StrokeWidth: changeStroke, StrokeDash: dash}
		}
		g.Children = append(g.Children, sample, Text{X: legendLabelX, Y: cy, Content: changeText(l.Text, th),
			FontFamily: th.Edge.LabelFont.Family, FontSize: fmt.Sprintf("%g", th.Edge.LabelFont.Size),
			FontWeight: fmt.Sprintf("%d", th.Edge.LabelFont.Weight), Fill: l.Color, DominantBaseline: "central"})
	}
	return g
}
