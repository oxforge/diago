package text

import (
	"strings"
	"testing"

	"github.com/oxforge/diago/internal/model"
)

// cellNode builds a node whose box covers cells [col, col+w) x [row, row+h).
func cellNode(id, label string, shape model.Shape, col, row, w, h int) model.PositionedNode {
	return model.PositionedNode{
		ID: id, Label: label, Shape: shape,
		X: float64(col*8) + float64(w*8)/2, Y: float64(row*16) + float64(h*16)/2,
		Width: float64(w * 8), Height: float64(h * 16),
	}
}

func rows(t *testing.T, r Result) []string {
	t.Helper()
	return strings.Split(strings.TrimRight(r.Art, "\n"), "\n")
}

func mustRender(t *testing.T, pg *model.PositionedGraph) Result {
	t.Helper()
	r, err := Render(pg)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	return r
}

// A vertical chain: a (rows 0-2) -> b (rows 5-7), wire in column 3.
func chainGraph() *model.PositionedGraph {
	a := cellNode("a", "A", model.ShapeRect, 0, 0, 7, 3)
	b := cellNode("b", "B", model.ShapeRect, 0, 5, 7, 3)
	return &model.PositionedGraph{
		Nodes: []model.PositionedNode{a, b},
		Edges: []model.PositionedEdge{{
			From: "a", To: "b", Direction: model.EdgeForward, Style: model.EdgeSolid,
			Points: []model.Point{{X: 28, Y: 48}, {X: 28, Y: 80}},
		}},
		Width: 56, Height: 128,
	}
}

func TestRender_ChainStaysInOneColumn(t *testing.T) {
	r := mustRender(t, chainGraph())
	lines := rows(t, r)
	// rows 3 and 4 are the gap: wire then arrowhead, both in column 3
	if []rune(lines[3])[3] != '│' {
		t.Fatalf("row 3 col 3 = %q, want │\n%s", []rune(lines[3])[3], r.Art)
	}
	if []rune(lines[4])[3] != '▼' {
		t.Fatalf("row 4 col 3 = %q, want ▼\n%s", []rune(lines[4])[3], r.Art)
	}
	if !strings.HasPrefix(lines[5], "┌─────┐") {
		t.Fatalf("target top border damaged: %q", lines[5])
	}
}

func TestRender_SiblingsShareTopRow(t *testing.T) {
	a := cellNode("a", "A", model.ShapeRect, 0, 0, 7, 3)
	b := cellNode("b", "B", model.ShapeRect, 9, 0, 7, 3)
	r := mustRender(t, &model.PositionedGraph{Nodes: []model.PositionedNode{a, b}, Width: 128, Height: 48})
	lines := rows(t, r)
	if lines[0] != "┌─────┐  ┌─────┐" {
		t.Fatalf("top row = %q", lines[0])
	}
}

func TestRender_NoTrailingWhitespaceNoUndefinedGlyph(t *testing.T) {
	r := mustRender(t, chainGraph())
	for i, l := range rows(t, r) {
		if strings.HasSuffix(l, " ") {
			t.Fatalf("row %d has trailing whitespace: %q", i, l)
		}
		if strings.ContainsRune(l, 0) || strings.ContainsRune(l, '�') {
			t.Fatalf("row %d has an undefined glyph: %q", i, l)
		}
	}
}

func TestRender_CrossingRendersVerticalThrough(t *testing.T) {
	// horizontal wire on row 2 from col 0..8, vertical wire in col 4 rows 0..4, crossing at (4,2)
	pg := &model.PositionedGraph{
		Nodes: []model.PositionedNode{
			cellNode("l", "L", model.ShapeRect, 0, 6, 7, 3), cellNode("r", "R", model.ShapeRect, 10, 6, 7, 3),
			cellNode("t", "T", model.ShapeRect, 1, 8, 7, 3),
		},
		Edges: []model.PositionedEdge{
			{From: "l", To: "r", Direction: model.EdgeForward, Points: []model.Point{{X: 4, Y: 40}, {X: 68, Y: 40}},
				Crossings: []model.Crossing{{SegmentIndex: 0, T: 0.5}}},
			{From: "t", To: "t", Direction: model.EdgeForward, Points: []model.Point{{X: 36, Y: 8}, {X: 36, Y: 72}}},
		},
		Width: 160, Height: 200,
	}
	r := mustRender(t, pg)
	lines := rows(t, r)
	if []rune(lines[2])[4] != '│' {
		t.Fatalf("crossing cell = %q, want │ (vertical wins)\n%s", []rune(lines[2])[4], r.Art)
	}
	if strings.ContainsRune(lines[2], '┼') {
		t.Fatalf("crossing must not render ┼:\n%s", r.Art)
	}
}

func TestRender_DiamondDecoratesMiddleLineOnly(t *testing.T) {
	d := cellNode("d", "Check\nAgain", model.ShapeDiamond, 0, 0, 15, 4)
	d.Lines = []string{"Check", "Again"}
	r := mustRender(t, &model.PositionedGraph{Nodes: []model.PositionedNode{d}, Width: 120, Height: 64})
	art := r.Art
	if strings.Count(art, "◇") != 2 {
		t.Fatalf("want exactly one ◇ pair:\n%s", art)
	}
	lines := rows(t, r)
	if !strings.Contains(lines[1], "◇ Check ◇") || strings.Contains(lines[2], "◇") {
		t.Fatalf("decoration must be on the middle (first of two) line only:\n%s", art)
	}
}

func TestRender_SideArrivalFlushOnMiddleRow(t *testing.T) {
	// a at cols 0-6 rows 0-2, b at cols 12-18 rows 0-2; wire arrives on b's left face.
	a := cellNode("a", "A", model.ShapeRect, 0, 0, 7, 3)
	b := cellNode("b", "B", model.ShapeRect, 12, 0, 7, 3)
	pg := &model.PositionedGraph{
		Nodes: []model.PositionedNode{a, b},
		Edges: []model.PositionedEdge{{From: "a", To: "b", Direction: model.EdgeForward,
			Points: []model.Point{{X: 56, Y: 24}, {X: 96, Y: 24}}}},
		Width: 160, Height: 48,
	}
	r := mustRender(t, pg)
	lines := rows(t, r)
	if []rune(lines[1])[11] != '►' {
		t.Fatalf("arrowhead should sit flush at col 11 on the middle row, got %q\n%s", []rune(lines[1])[11], r.Art)
	}
	if []rune(lines[1])[12] != '│' {
		t.Fatalf("target border must be intact at col 12:\n%s", r.Art)
	}
}

func TestRender_BothAndBackwardHeads(t *testing.T) {
	g := chainGraph()
	g.Edges[0].Direction = model.EdgeBoth
	r := mustRender(t, g)
	lines := rows(t, r)
	if []rune(lines[3])[3] != '▲' || []rune(lines[4])[3] != '▼' {
		t.Fatalf("both: want ▲ at row 3 and ▼ at row 4\n%s", r.Art)
	}
	g.Edges[0].Direction = model.EdgeBackward
	r = mustRender(t, g)
	lines = rows(t, r)
	if []rune(lines[3])[3] != '▲' || []rune(lines[4])[3] != '│' {
		t.Fatalf("backward: want ▲ at row 3 only\n%s", r.Art)
	}
}

func TestRender_AnUnresolvedLabelTakesTheLadder(t *testing.T) {
	// seeded on the wire, a label the labels stage left unresolved moves
	// along the renderer's ladder to the first empty run of cells beside it
	g := chainGraph()
	g.Height = 160
	g.Nodes[1] = cellNode("b", "B", model.ShapeRect, 0, 7, 7, 3)
	g.Edges[0].Points = []model.Point{{X: 28, Y: 48}, {X: 28, Y: 112}}
	g.Edges[0].Label = "yes"
	g.Edges[0].LabelPos = &model.Point{X: 28, Y: 80}
	g.Edges[0].LabelUnresolved = true
	r := mustRender(t, g)
	if len(r.DroppedLabels) != 0 {
		t.Fatalf("label should fit: %+v", r.DroppedLabels)
	}
	if !strings.Contains(r.Art, "│ yes") && !strings.Contains(r.Art, "│yes") {
		t.Fatalf("label not beside the wire:\n%s", r.Art)
	}
}

func TestRender_LabelThatFitsNowhereIsReported(t *testing.T) {
	g := chainGraph()
	g.Edges[0].Label = "this label is far too long for a two-row gap"
	g.Edges[0].LabelPos = &model.Point{X: 28, Y: 64}
	g.Edges[0].LabelUnresolved = true
	r := mustRender(t, g)
	if len(r.DroppedLabels) != 1 || r.DroppedLabels[0].From != "a" || r.DroppedLabels[0].To != "b" {
		t.Fatalf("want one dropped label, got %+v\n%s", r.DroppedLabels, r.Art)
	}
	if strings.Contains(r.Art, "far too long") {
		t.Fatalf("dropped label must not be drawn:\n%s", r.Art)
	}
}

func TestRender_GroupTitleSurvivesWireCrossing(t *testing.T) {
	// group covers cols 0-10 rows 0-7 with title "G"; node n inside (rows 3-5), one padding row,
	// border row 7; s below the group; the wire from s's top face rises through the group's
	// bottom side at col 5 and arrives on n's bottom face.
	n := cellNode("n", "N", model.ShapeRect, 2, 3, 7, 3)
	pg := &model.PositionedGraph{
		Nodes:  []model.PositionedNode{n, cellNode("s", "S", model.ShapeRect, 3, 10, 7, 3)},
		Groups: []model.PositionedGroup{{ID: "g", Label: "G", X: 0, Y: 0, Width: 88, Height: 128, Contains: []string{"n"}}},
		Edges: []model.PositionedEdge{{From: "s", To: "n", Direction: model.EdgeForward,
			Points: []model.Point{{X: 52, Y: 160}, {X: 52, Y: 136}, {X: 44, Y: 136}, {X: 44, Y: 96}}}},
		Width: 160, Height: 224,
	}
	r := mustRender(t, pg)
	lines := rows(t, r)
	if !strings.HasPrefix(lines[0], "┌─ G ─") {
		t.Fatalf("title damaged: %q", lines[0])
	}
	if []rune(lines[7])[5] != '┼' {
		t.Fatalf("wire through the group bottom side should be ┼, got %q\n%s", []rune(lines[7])[5], r.Art)
	}
	if []rune(lines[6])[5] != '▲' {
		t.Fatalf("arrowhead should sit in the padding row under n, got %q\n%s", []rune(lines[6])[5], r.Art)
	}
}

// TestRender_GroupTitleAtItsOffset pins a group title where the layout
// placed it: its first rune LabelOffset from the frame's left side, a blank
// on either side, and a wire through the renderer's default slot left
// whole.
func TestRender_GroupTitleAtItsOffset(t *testing.T) {
	// s above the group (rows 0-2); the group covers cols 0-10 rows 4-11,
	// its title "G" placed in col 7; n inside (rows 7-9). The wire from s
	// down to n crosses the group's top side in col 3, the default slot's
	// first rune.
	pg := &model.PositionedGraph{
		Nodes: []model.PositionedNode{cellNode("s", "S", model.ShapeRect, 0, 0, 7, 3), cellNode("n", "N", model.ShapeRect, 2, 7, 7, 3)},
		Groups: []model.PositionedGroup{{ID: "g", Label: "G", X: 0, Y: 64, Width: 88, Height: 128, Contains: []string{"n"},
			LabelWidth: 8, LabelHeight: 16, LabelOffset: 56}},
		Edges: []model.PositionedEdge{{From: "s", To: "n", Direction: model.EdgeForward,
			Points: []model.Point{{X: 28, Y: 48}, {X: 28, Y: 112}}}},
		Width: 160, Height: 224,
	}
	r := mustRender(t, pg)
	top := []rune(rows(t, r)[4])
	if got := string(top[6:9]); got != " G " {
		t.Fatalf("title should sit in cols 6-8, got %q\n%s", got, r.Art)
	}
	if top[3] != '┼' {
		t.Fatalf("the wire through the top side should be ┼, got %q\n%s", top[3], r.Art)
	}
}

func TestRender_NonAxisAlignedSegmentIsAnError(t *testing.T) {
	g := chainGraph()
	g.Edges[0].Points = []model.Point{{X: 28, Y: 48}, {X: 60, Y: 80}}
	if _, err := Render(g); err == nil {
		t.Fatal("want an error for a diagonal segment")
	}
}

func TestRender_ADirectEdgeKeepsItsRow(t *testing.T) {
	// a: cols 0-6 rows 0-2; b: cols 12-18 rows 2-4. The wire runs along
	// row 2, a's bottom border row and b's top one: it is drawn there, from
	// a's corner to its arrowhead flush beside b's, with no jog (S10 Text
	// cells)
	a := cellNode("a", "A", model.ShapeRect, 0, 0, 7, 3)
	b := cellNode("b", "B", model.ShapeRect, 12, 2, 7, 3)
	pg := &model.PositionedGraph{
		Nodes: []model.PositionedNode{a, b},
		Edges: []model.PositionedEdge{{From: "a", To: "b", Direction: model.EdgeForward,
			Points: []model.Point{{X: 56, Y: 40}, {X: 96, Y: 40}}}},
		Width: 160, Height: 80,
	}
	r := mustRender(t, pg)
	lines := rows(t, r)
	if got := string([]rune(lines[2])[6:13]); got != "┘────►┌" {
		t.Fatalf("row 2 = %q, want the wire from a's corner to b's:\n%s", got, r.Art)
	}
	for _, row := range []int{1, 3} {
		if strings.ContainsAny(lines[row], "─►") {
			t.Fatalf("row %d holds a run of the wire:\n%s", row, r.Art)
		}
	}
}
