package text

import (
	"strings"
	"testing"
)

func TestCharGrid_JunctionTable(t *testing.T) {
	// Every NESW combination resolves to its junction glyph.
	cases := []struct {
		name string
		draw func(g *charGrid)
		want rune
	}{
		{"N|S", func(g *charGrid) { g.vline(0, 2, 1, styleSolid) }, '│'},
		{"E|W", func(g *charGrid) { g.hline(0, 2, 1, styleSolid) }, '─'},
		{"N|E", func(g *charGrid) { g.vline(0, 1, 1, styleSolid); g.hline(1, 2, 1, styleSolid) }, '└'},
		{"N|W", func(g *charGrid) { g.vline(0, 1, 1, styleSolid); g.hline(0, 1, 1, styleSolid) }, '┘'},
		{"S|E", func(g *charGrid) { g.vline(1, 2, 1, styleSolid); g.hline(1, 2, 1, styleSolid) }, '┌'},
		{"S|W", func(g *charGrid) { g.vline(1, 2, 1, styleSolid); g.hline(0, 1, 1, styleSolid) }, '┐'},
		{"N|E|S", func(g *charGrid) { g.vline(0, 2, 1, styleSolid); g.hline(1, 2, 1, styleSolid) }, '├'},
		{"N|S|W", func(g *charGrid) { g.vline(0, 2, 1, styleSolid); g.hline(0, 1, 1, styleSolid) }, '┤'},
		{"E|S|W", func(g *charGrid) { g.hline(0, 2, 1, styleSolid); g.vline(1, 2, 1, styleSolid) }, '┬'},
		{"N|E|W", func(g *charGrid) { g.hline(0, 2, 1, styleSolid); g.vline(0, 1, 1, styleSolid) }, '┴'},
		{"N|E|S|W", func(g *charGrid) { g.hline(0, 2, 1, styleSolid); g.vline(0, 2, 1, styleSolid) }, '┼'},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := newCharGrid(3, 3)
			tc.draw(g)
			if got := g.runeAt(1, 1); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestCharGrid_GlyphWinsOverFlags(t *testing.T) {
	g := newCharGrid(3, 1)
	g.hline(0, 2, 0, styleSolid)
	g.setGlyph(1, 0, 'X')
	if got := g.String(); got != "─X─\n" {
		t.Fatalf("got %q", got)
	}
}

func TestCharGrid_StyledRuns(t *testing.T) {
	g := newCharGrid(3, 3)
	g.hline(0, 2, 0, styleDashed)
	g.vline(0, 2, 0, styleDotted)
	g.hline(0, 2, 2, styleThick)
	lines := strings.Split(strings.TrimRight(g.String(), "\n"), "\n")
	// (0,0) is a junction of two styles: light solid corner. (1,0) pure dashed, (0,1) pure dotted, (1,2) pure thick.
	if lines[0] != "┌╌╌" {
		t.Fatalf("row 0 = %q", lines[0])
	}
	if []rune(lines[1])[0] != '┊' {
		t.Fatalf("(0,1) = %q, want ┊", []rune(lines[1])[0])
	}
	if lines[2] != "└━━" {
		t.Fatalf("row 2 = %q", lines[2])
	}
}

func TestCharGrid_SolidPlusDashedFallsBackToLight(t *testing.T) {
	g := newCharGrid(3, 1)
	g.hline(0, 2, 0, styleSolid)
	g.hline(0, 2, 0, styleDashed)
	if got := g.String(); got != "───\n" {
		t.Fatalf("got %q", got)
	}
}

func TestCharGrid_ThickKeepsHeavyGlyphThroughCrossing(t *testing.T) {
	g := newCharGrid(3, 3)
	g.hline(0, 2, 1, styleThick)
	g.vline(0, 2, 1, styleSolid)
	g.setGlyph(1, 1, '┃') // crossing rule: the vertical wins the cell (Task 5 stamps this)
	lines := strings.Split(strings.TrimRight(g.String(), "\n"), "\n")
	if lines[1] != "━┃━" {
		t.Fatalf("row 1 = %q", lines[1])
	}
}

func TestCharGrid_BoxFramesAndLabel(t *testing.T) {
	g := newCharGrid(7, 3)
	g.box(0, 0, 7, 3, []string{"hi"}, boxFrame{})
	if got := g.String(); got != "┌─────┐\n│ hi  │\n└─────┘\n" {
		t.Fatalf("plain box:\n%s", got)
	}
	g = newCharGrid(7, 3)
	g.box(0, 0, 7, 3, nil, boxFrame{Rounded: true})
	if got := g.String(); got != "╭─────╮\n│     │\n╰─────╯\n" {
		t.Fatalf("rounded box:\n%s", got)
	}
	g = newCharGrid(7, 3)
	g.box(0, 0, 7, 3, nil, boxFrame{Top: '═'})
	if got := g.String(); got != "╒═════╕\n│     │\n└─────┘\n" {
		t.Fatalf("cylinder box:\n%s", got)
	}
}

func TestCharGrid_BoxLabelClipsAtBorders(t *testing.T) {
	g := newCharGrid(5, 3)
	g.box(0, 0, 5, 3, []string{"a", "b", "c", "d"}, boxFrame{})
	lines := strings.Split(strings.TrimRight(g.String(), "\n"), "\n")
	if len(lines) != 3 || lines[0] != "┌───┐" || lines[2] != "└───┘" {
		t.Fatalf("borders overwritten:\n%s", g.String())
	}
}

func TestCharGrid_FrameEmbedsTitleAndLetsWiresThrough(t *testing.T) {
	g := newCharGrid(9, 5)
	g.frame(0, 0, 9, 5, "T", 3)
	g.vline(0, 4, 6, styleSolid) // a wire crossing top and bottom sides
	lines := strings.Split(strings.TrimRight(g.String(), "\n"), "\n")
	if lines[0] != "┌─ T ─┬─┐" {
		t.Fatalf("top = %q", lines[0])
	}
	if lines[4] != "└─────┴─┘" {
		t.Fatalf("bottom = %q", lines[4])
	}
	if lines[2] != "│     │ │" {
		t.Fatalf("mid = %q", lines[2])
	}
}

func TestCharGrid_FrameTruncatesTitleToRoomRightOfColumn(t *testing.T) {
	// at = 6 sits far right of a 12-wide frame; the room right of it is
	// 0 + 12 - 3 - 6 = 3 runes, so "ABCDEFGH" truncates to "ABC", written
	// " ABC " from column 5, leaving the frame's own "─" in column 10 and
	// the right corner "┐" in column 11.
	g := newCharGrid(12, 3)
	g.frame(0, 0, 12, 3, "ABCDEFGH", 6)
	lines := strings.Split(strings.TrimRight(g.String(), "\n"), "\n")
	if lines[0] != "┌──── ABC ─┐" {
		t.Fatalf("top = %q", lines[0])
	}
}

func TestCharGrid_StringTrimsAndDropsBlankRows(t *testing.T) {
	g := newCharGrid(5, 4)
	g.setGlyph(1, 1, 'x')
	if got := g.String(); got != " x\n" {
		t.Fatalf("got %q", got)
	}
}

func TestCharGrid_IsEmptyAndGlyphAt(t *testing.T) {
	g := newCharGrid(3, 2)
	if !g.isEmpty(1, 1) {
		t.Fatal("fresh cell should be empty")
	}
	g.hline(0, 2, 0, styleSolid)
	if g.isEmpty(1, 0) {
		t.Fatal("a cell with line flags is not empty")
	}
	g.setGlyph(2, 1, 'z')
	if g.isEmpty(2, 1) || g.glyphAt(2, 1) != 'z' {
		t.Fatal("glyph cell should report its glyph and not be empty")
	}
	if g.glyphAt(9, 9) != 0 || g.isEmpty(9, 9) {
		t.Fatal("out of bounds: glyphAt 0, isEmpty false")
	}
}
