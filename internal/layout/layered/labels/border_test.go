package labels

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/layoutdbg"
	"github.com/oxforge/diago/internal/model"
)

// In the text profile, cells: a and b side by side, rows 5..8, joined by
// a wire along the boundary above row 7, drawn in row 7 under DOWN. The
// wire has no vertical run, so the label slides beside it (S10).
var (
	sideBoxes = []Rect{{0, 5, 9, 4}, {20, 5, 9, 4}}
	sideWire  = []model.Point{{X: 9, Y: 7}, {X: 20, Y: 7}}
)

func TestPlace_TextAcrossAWireAlongItsRowALabelTakesTheNextRow(t *testing.T) {
	// across a wire along its row a label takes the row next to the
	// cells the wire is drawn in: above it first, row 6 under DOWN (S10)
	e := Edge{ID: "e", Points: sideWire, Label: Size{5, 1}}
	got := place(t, sideBoxes, nil, text(model.Down), e)
	assert.Equal(t, &Rect{12, 6, 5, 1}, got[0].Label, "row 6, next to row 7, where the wire is drawn")
	assert.False(t, got[0].LabelUnresolved)

	// f's wire along row 6 takes the row above: the label takes the row
	// below, row 8, next to row 7 too
	f := Edge{ID: "f", Points: []model.Point{{X: 10, Y: 6.5}, {X: 19, Y: 6.5}}}
	got = place(t, sideBoxes, nil, text(model.Down), e, f)
	assert.Equal(t, &Rect{12, 8, 5, 1}, got[0].Label, "row 8, the row below the wire's")

	// under UP the frame turns the engine's y over, and the wire on the
	// boundary is drawn in the cell before it, row 6: the label takes
	// row 5, next to it
	got = place(t, sideBoxes, nil, text(model.Up), e)
	assert.Equal(t, &Rect{12, 5, 5, 1}, got[0].Label, "UP: row 5, next to row 6, where the wire is drawn")
}

func TestPlace_TextBesideAWireAcrossItsRowALabelKeepsABlankColumn(t *testing.T) {
	// under RIGHT the frame turns the engine's x into rows and its y into
	// columns, so these boxes and this wire, cellBoxes and cellDown with
	// the axes swapped, draw what those draw under DOWN: a above b and a
	// wire down column 10 of the output, along the engine's x, from row 3
	// to row 9. Beside that wire, which crosses the label's row, the label
	// keeps a blank column, as under DOWN
	// (TestPlace_TextSnapsASpotToWholeCells), not the row next to a wire
	// along its row (S10). The test pins that outcome under RIGHT, not one
	// code path: the label gap keeps the column blank, and so does the
	// label pad's blank before and after the box, each without the other
	nodes := []Rect{{0, 6, 3, 9}, {9, 6, 3, 9}}
	wire := []model.Point{{X: 3, Y: 10.5}, {X: 9, Y: 10.5}}
	got := place(t, nodes, nil, text(model.Right), Edge{ID: "e", Points: wire, Label: Size{1, 5}})
	assert.Equal(t, &Rect{5, 4, 1, 5}, got[0].Label, "row 5, columns 4..8: column 9 blank between the label and the wire's column 10")
	assert.False(t, got[0].LabelUnresolved)
}

func TestPlace_TextARingSlidesAlongItsRun(t *testing.T) {
	// under RIGHT, e leaves a along the row of cell x 10 (a run along the
	// engine's y, from y 9 to 15.5), turns and enters b. The spots beside
	// the run's middle, y 9.75..14.75, round to y 9..13 and need the
	// blank before them, y 8, which is a's last cell: blocked, on both
	// sides. In the text profile the first ring also slides along the
	// run, a cell at a time, and the spot one cell on, y 10..14, is
	// clear: the label keeps the row next to its wire rather than taking
	// the next ring out (S10)
	route := []model.Point{{X: 10.5, Y: 9}, {X: 10.5, Y: 15.5}, {X: 4.5, Y: 15.5}, {X: 4.5, Y: 18}}
	nodes := []Rect{{9, 0, 3, 9}, {3, 18, 3, 3}}
	got := place(t, nodes, nil, text(model.Right), Edge{ID: "e", Points: route, Label: Size{1, 5}})
	assert.Equal(t, &Rect{11, 10, 1, 5}, got[0].Label)
	assert.False(t, got[0].LabelUnresolved)
}

// frames places e beside cellDown among cellBoxes and a blocker left of
// the wire, rows 3..8, with group frames gs (S10).
func frames(t *testing.T, o Options, gs []Rect, e Edge) Placed {
	t.Helper()
	nodes := append(cellBoxes[:2:2], Rect{2, 3, 4, 6})
	got := Place(context.Background(), nodes, make([]bool, len(nodes)), gs, []Edge{e}, o)
	require.Len(t, got, 1)
	return got[0]
}

func TestPlace_TextALabelCoversAFrameSideOnlyWhenItsRingHoldsNoClearSpot(t *testing.T) {
	// g's left side runs down column 13, across the label's row. Left of
	// the wire the blocker takes every spot of the first ring; right of
	// it every spot covers the side. With no spot of the ring clear, the
	// first that is clear but for the side wins, and the renderer writes
	// the label over the side (S10)
	e := Edge{ID: "e", Points: cellDown, Label: Size{5, 1}}
	g := []Rect{{13, 1, 10, 10}}
	got := frames(t, text(model.Down), g, e)
	assert.Equal(t, &Rect{12, 5, 5, 1}, got.Label, "the first ring's first spot, over column 13")
	assert.False(t, got.LabelUnresolved)

	// without the blocker the ring holds a clear spot left of the wire,
	// which wins over the one on the side
	unblocked := Place(context.Background(), cellBoxes, make([]bool, 2), g, []Edge{e}, text(model.Down))
	assert.Equal(t, &Rect{4, 5, 5, 1}, unblocked[0].Label, "the side right, the clear spot left: left")
}

func TestPlace_TextALabelsBlanksNeverFallOnAFrame(t *testing.T) {
	// g's left side runs down column 17, where the first ring's right
	// spots, columns 12..16, need their blank: they are blocked, not
	// clear but for the side. The second ring's right spot, columns
	// 13..17, covers the side with its own cells, its blanks at 12 and
	// 18 clear, and wins (S10)
	got := frames(t, text(model.Down), []Rect{{17, 1, 10, 10}}, Edge{ID: "e", Points: cellDown, Label: Size{5, 1}})
	assert.Equal(t, &Rect{13, 5, 5, 1}, got.Label)
	assert.False(t, got.LabelUnresolved)
}

func TestInk_TextAFrameSideAlongTheRowBlocksOneAcrossItDoesNot(t *testing.T) {
	// g spans x 10..19 and y 10..15 in the engine's frame. Under DOWN a
	// label's row runs along x: g's top and bottom sides run along it,
	// and with their corners they are ink; its left and right sides cross
	// it, and a label may cover them (border) but its blanks may not.
	// Under RIGHT a label's row runs along y and the roles swap (S10)
	g := []Rect{{10, 10, 10, 6}}
	for _, tc := range []struct {
		name        string
		dir         model.Direction
		s           Rect
		ink, border float64
	}{
		{"DOWN: on the top side", model.Down, Rect{12, 10, 3, 1}, 5, 0},
		{"DOWN: on the top left corner, a cell of both sides", model.Down, Rect{8, 10, 3, 1}, 2, 1},
		{"DOWN: over the left side", model.Down, Rect{8, 12, 4, 1}, 0, 1},
		{"DOWN: its blank on the left side", model.Down, Rect{11, 12, 3, 1}, 1, 0},
		{"RIGHT: over the top side", model.Right, Rect{12, 8, 1, 4}, 0, 1},
		{"RIGHT: on the left side, its blank after it on the bottom side", model.Right, Rect{10, 12, 1, 3}, 6, 0},
		{"RIGHT: its blank on the top side", model.Right, Rect{12, 11, 1, 3}, 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &placer{o: text(tc.dir)}
			p.drawCells(g)
			p.index(false)
			assert.Equal(t, tc.ink, p.ink(tc.s), "ink")
			assert.Equal(t, tc.border, p.border(tc.s), "border")
		})
	}
}

func TestPlace_TextALabelStaysInsideItsHomeGroup(t *testing.T) {
	// a and b lie in h, whose left side runs down column 9 and whose
	// right side down column 24; the wire runs down column 11 and a
	// blocker inside h, columns 13..14 of rows 3..8, takes the first ring
	// right of it. Left of the wire the first ring's spot, columns 5..9,
	// covers h's left side: for an edge of no group it wins, but an edge
	// with both ends in h keeps its label inside h's frame, and it takes
	// the second ring right of the wire, slid up the run to row 2, clear
	// of the blocker and of a (S10)
	nodes := []Rect{{10, 0, 3, 3}, {10, 9, 3, 3}, {13, 3, 2, 6}}
	wire := []model.Point{{X: 11.5, Y: 3}, {X: 11.5, Y: 9}}
	h := Rect{9, -1, 16, 14}
	placeIn := func(home *Rect) Placed {
		got := Place(context.Background(), nodes, make([]bool, 3), []Rect{h}, []Edge{{ID: "e", Points: wire, Label: Size{5, 1}, Home: home}}, text(model.Down))
		return got[0]
	}
	assert.Equal(t, &Rect{5, 5, 5, 1}, placeIn(nil).Label, "no home: over h's left side")
	got := placeIn(&h)
	assert.Equal(t, &Rect{14, 2, 5, 1}, got.Label, "home h: inside its frame")
	assert.False(t, got.LabelUnresolved)
}

// TestPlace_TextACopysLabelStaysInsideItsOwnHomeGroup pins S10's home
// group on a congruence's copies (S12): e copies its label to c, whose
// wire, nodes, blocker and home group are e's moved 30 columns right, as
// TestPlace_TextALabelStaysInsideItsHomeGroup lays e out. Each box is
// costed against its own edge's home, so both labels take the second
// ring right of their wires, inside their own frames; costed against e's
// home, every spot of c's would lie outside it, and the ladder would run
// again without the home rule.
func TestPlace_TextACopysLabelStaysInsideItsOwnHomeGroup(t *testing.T) {
	const dx = 30
	nodes := []Rect{{10, 0, 3, 3}, {10, 9, 3, 3}, {13, 3, 2, 6}, {10 + dx, 0, 3, 3}, {10 + dx, 9, 3, 3}, {13 + dx, 3, 2, 6}}
	h, hc := Rect{9, -1, 16, 14}, Rect{9 + dx, -1, 16, 14}
	edges := []Edge{
		{ID: "e", Points: []model.Point{{X: 11.5, Y: 3}, {X: 11.5, Y: 9}}, Label: Size{5, 1}, Copies: []int{1}, Home: &h},
		{ID: "c", Points: []model.Point{{X: 11.5 + dx, Y: 3}, {X: 11.5 + dx, Y: 9}}, Label: Size{5, 1}, Home: &hc},
	}
	got := Place(context.Background(), nodes, make([]bool, len(nodes)), []Rect{h, hc}, edges, text(model.Down))
	require.Len(t, got, 2)
	assert.Equal(t, &Rect{14, 2, 5, 1}, got[0].Label, "e: inside its home")
	assert.Equal(t, &Rect{14 + dx, 2, 5, 1}, got[1].Label, "c: inside its own home")
	assert.False(t, got[0].LabelUnresolved)
	assert.False(t, got[1].LabelUnresolved)
}

func TestPlace_TextALabelLeavesAHomeGroupItCannotFitIn(t *testing.T) {
	// a and b lie in h, columns 9..13, whose frame leaves three columns
	// inside, 10..12, the wire's own: no spot of the ladder fits a label
	// five wide there, clear or clear but for a border. The ladder runs
	// again without the home rule, and the label takes the spot an edge of
	// no group takes, resolved, instead of the least overlap (S10)
	nodes := []Rect{{10, 0, 3, 3}, {10, 9, 3, 3}}
	wire := []model.Point{{X: 11.5, Y: 3}, {X: 11.5, Y: 9}}
	h := Rect{9, -1, 5, 14}
	placeIn := func(home *Rect) Placed {
		got := Place(context.Background(), nodes, make([]bool, 2), []Rect{h}, []Edge{{ID: "e", Points: wire, Label: Size{5, 1}, Home: home}}, text(model.Down))
		return got[0]
	}
	free := placeIn(nil)
	require.NotNil(t, free.Label)
	require.False(t, free.LabelUnresolved, "no home: resolved")
	got := placeIn(&h)
	assert.Equal(t, free.Label, got.Label, "home h: where no home puts it")
	assert.False(t, got.LabelUnresolved)
}

func TestPlace_TextANestedGroupsSideIsNoHomeFrame(t *testing.T) {
	// as above, but h is wider, columns 1..24, and holds a nested group n,
	// columns 3..9, whose right side runs down column 9. The edge's home
	// is h, not n: the first ring's left spot, columns 5..9, may cover
	// n's side, and with no spot of the ring clear (the blocker takes
	// the right ones) it wins as the spot clear but for a border (S10)
	nodes := []Rect{{10, 0, 3, 3}, {10, 9, 3, 3}, {13, 3, 2, 6}}
	wire := []model.Point{{X: 11.5, Y: 3}, {X: 11.5, Y: 9}}
	h := Rect{1, -1, 24, 14}
	n := Rect{3, 1, 7, 10}
	got := Place(context.Background(), nodes, make([]bool, 3), []Rect{h, n}, []Edge{{ID: "e", Points: wire, Label: Size{5, 1}, Home: &h}}, text(model.Down))
	assert.Equal(t, &Rect{5, 5, 5, 1}, got[0].Label, "over n's right side, inside h")
	assert.False(t, got[0].LabelUnresolved)
}

func TestPlace_ScreenIgnoresFramesAndHomes(t *testing.T) {
	// on screen a group frame blocks nothing and a home group binds
	// nothing: the label sits a gap right of the wire, as with neither
	o := Options{Gap: 1, CardStep: 1, EnvDepth: 1, EnvHalf: 1}
	h := Rect{13, 1, 10, 10}
	got := Place(context.Background(), cellBoxes, make([]bool, 2), []Rect{h}, []Edge{{ID: "e", Points: cellDown, Label: Size{5, 1}, Home: &Rect{0, 0, 12, 12}}}, o)
	assert.Equal(t, &Rect{11.5, 5.5, 5, 1}, got[0].Label)
}

func TestPlace_LogsALabelOnABorder(t *testing.T) {
	var buf bytes.Buffer
	ctx := layoutdbg.NewContext(context.Background(), slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	nodes := append(cellBoxes[:2:2], Rect{2, 3, 4, 6})
	Place(ctx, nodes, make([]bool, 3), []Rect{{13, 1, 10, 10}}, []Edge{{ID: "e", Points: cellDown, Label: Size{5, 1}}}, text(model.Down))
	out := buf.String()
	assert.Contains(t, out, `"decision":"label_on_border"`)
	assert.Contains(t, out, `"spec_ref":"S10"`)
	assert.Equal(t, 1, strings.Count(out, "label_on_border"))
}

func TestPlace_TextWithoutALabelGapEnds(t *testing.T) {
	// a ring's slide along its run steps one gap at a time while the box
	// still reaches the run: with no gap it takes no step, as the slide at
	// the ladder's end does, instead of stepping forever (S10)
	o := text(model.Down)
	o.Gap = 0
	done := make(chan []Placed, 1)
	go func() {
		done <- Place(context.Background(), cellBoxes, make([]bool, 2), nil, []Edge{{ID: "e", Points: cellDown, Label: Size{5, 1}}}, o)
	}()
	select {
	case got := <-done:
		require.Len(t, got, 1)
		assert.NotNil(t, got[0].Label)
	case <-time.After(5 * time.Second):
		t.Fatal("the ladder never ends")
	}
}
