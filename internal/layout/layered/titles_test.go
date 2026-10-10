package layered

import (
	"context"
	"math"
	"testing"

	"github.com/oxforge/diago/internal/layout/contract"
	"github.com/oxforge/diago/internal/layout/layered/frame"
	"github.com/oxforge/diago/internal/model"
	"github.com/stretchr/testify/assert"
)

// invertOutputPoint maps a point in the OUTPUT frame back to the engine's,
// the exact inverse of frame.Apply's map (S11): DOWN keeps it, UP negates
// y, RIGHT swaps the axes, LEFT swaps them and negates the one that lands
// on x.
func invertOutputPoint(dir model.Direction, x, y float64) (float64, float64) {
	switch dir {
	case model.Up:
		return x, -y
	case model.Right:
		return y, x
	case model.Left:
		return y, -x
	}
	return x, y
}

// invertOutputBox maps an output-frame box (top-left x, y, and its width
// and height) back to an engine-frame box: it inverts two opposite
// corners and takes the low corner and the span on each axis, since a
// direction can flip an axis or swap width and height (frame.Apply, S11).
func invertOutputBox(dir model.Direction, x, y, w, h float64) (ex, ey, ew, eh float64) {
	x0, y0 := invertOutputPoint(dir, x, y)
	x1, y1 := invertOutputPoint(dir, x+w, y+h)
	return math.Min(x0, x1), math.Min(y0, y1), math.Abs(x1 - x0), math.Abs(y1 - y0)
}

// TestPlaceTitles_EveryDirectionMapsTheBand pins placeTitles' band mapping
// (S10) in every direction and both profiles. Nothing else in the suite
// crosses a title band under RIGHT or LEFT, so without it swapping RIGHT's
// and LEFT's u, flipping their v, or dropping "width = gr.Height" for a
// sideways direction would still pass every other test.
//
// The scene is described in the OUTPUT frame: one titled group at the
// origin and one vertical wire crossing the group's top side inside the
// left slot, so the left slot is blocked and the title must take the
// right slot, Width - TitleInset - LabelWidth from the group's left side.
// It is inverted into the engine's frame with invertOutputBox and
// invertOutputPoint, run through placeTitles and frame.Apply (and the
// text adapter, in text), and the result is read back in the output
// frame, where it started.
func TestPlaceTitles_EveryDirectionMapsTheBand(t *testing.T) {
	for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
		for _, profile := range []string{"screen", "text"} {
			t.Run(dir.String()+"/"+profile, func(t *testing.T) {
				cfg := screen()
				w, h, lw, lh := 200.0, 120.0, 40.0, 14.0
				wireTop, wireBottom := -20.0, 40.0
				if profile == "text" {
					cfg = TextConfig(dir)
					w, h, lw, lh = 25, 8, 5, 1
					wireTop, wireBottom = -2, 3
				}
				wireU := cfg.TitleInset + 2 // a few units into the left slot

				gx, gy, gw, gh := invertOutputBox(dir, 0, 0, w, h)
				p0x, p0y := invertOutputPoint(dir, wireU, wireTop)
				p1x, p1y := invertOutputPoint(dir, wireU, wireBottom)
				pg := &model.PositionedGraph{
					Groups: []model.PositionedGroup{{ID: "g", Label: "G", X: gx, Y: gy, Width: gw, Height: gh, LabelWidth: lw, LabelHeight: lh}},
					Edges:  []model.PositionedEdge{{ID: "w1", From: "a", To: "b", Points: []model.Point{{X: p0x, Y: p0y}, {X: p1x, Y: p1y}}}},
				}

				placeTitles(context.Background(), dir, cfg, pg)
				frame.Apply(context.Background(), pg, dir, cfg.MarginX, cfg.MarginY, cfg.Text)
				if cfg.Text {
					scale(pg, dir, cellW, cellH)
				}

				want := w - cfg.TitleInset - lw
				if cfg.Text {
					want *= cellW
				}
				assert.InDelta(t, want, pg.Groups[0].LabelOffset, 1e-6, "the right slot, from the group's left side")

				lim := contract.ScreenLimits()
				if cfg.Text {
					lim = contract.TextLimits(dir)
				}
				var c13 []contract.Violation
				for _, v := range contract.Check(pg, contract.Options{Limits: lim, Direction: dir, Text: cfg.Text}) {
					if v.Rule == "C13" {
						c13 = append(c13, v)
					}
				}
				assert.Empty(t, c13, "the wire keeps clear of the placed title")
			})
		}
	}
}
