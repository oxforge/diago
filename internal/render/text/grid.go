package text

import (
	"slices"
	"strings"
)

// Direction flags per cell.
const (
	flagN uint8 = 1
	flagE uint8 = 2
	flagS uint8 = 4
	flagW uint8 = 8
)

// lineStyle bits. Solid claims its own bit so a cell shared by a solid run
// and a styled run has no pure-style entry and falls back to light solid.
type lineStyle uint8

const (
	styleDashed lineStyle = 1
	styleDotted lineStyle = 2
	styleThick  lineStyle = 4
	styleSolid  lineStyle = 8
)

// styledRuns maps a pure single style to its [horizontal, vertical] glyphs.
var styledRuns = map[lineStyle][2]rune{
	styleDashed: {'╌', '╎'},
	styleDotted: {'┈', '┊'},
	styleThick:  {'━', '┃'},
}

// lineChars is the junction table indexed by the NESW flag bitmask.
var lineChars = [16]rune{
	' ', '│', '─', '└', '│', '│', '┌', '├',
	'─', '┘', '─', '┴', '┐', '┤', '┬', '┼',
}

// boxFrame selects a node box's corner and top-edge decoration.
type boxFrame struct {
	Rounded bool
	Top     rune // 0 = '─'; '═' = cylinder cap
}

// charGrid is a character grid with NESW line flags per cell, glyph
// overrides, and style bits (a port of neat's CharGrid). Lines OR their
// direction flags into cells so junctions and crossings resolve from the
// table; glyphs (borders, labels, arrowheads) always win over flags.
type charGrid struct {
	cols, rows int
	flags      []uint8
	glyphs     []rune
	style      []uint8
	frames     []uint8 // per cell, the flags the group frames set, kept once they are drawn (keepFrames); nil before
}

func newCharGrid(cols, rows int) *charGrid {
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	n := cols * rows
	return &charGrid{cols: cols, rows: rows, flags: make([]uint8, n), glyphs: make([]rune, n), style: make([]uint8, n)}
}

func (g *charGrid) idx(x, y int) (int, bool) {
	if x < 0 || y < 0 || x >= g.cols || y >= g.rows {
		return 0, false
	}
	return y*g.cols + x, true
}

func (g *charGrid) hline(x1, x2, y int, st lineStyle) {
	lo, hi := x1, x2
	if lo > hi {
		lo, hi = hi, lo
	}
	for x := lo; x <= hi; x++ {
		i, ok := g.idx(x, y)
		if !ok {
			continue
		}
		var f uint8
		if x > lo {
			f |= flagW
		}
		if x < hi {
			f |= flagE
		}
		g.flags[i] |= f
		g.style[i] |= uint8(st)
	}
}

func (g *charGrid) vline(y1, y2, x int, st lineStyle) {
	lo, hi := y1, y2
	if lo > hi {
		lo, hi = hi, lo
	}
	for y := lo; y <= hi; y++ {
		i, ok := g.idx(x, y)
		if !ok {
			continue
		}
		var f uint8
		if y > lo {
			f |= flagN
		}
		if y < hi {
			f |= flagS
		}
		g.flags[i] |= f
		g.style[i] |= uint8(st)
	}
}

func (g *charGrid) setGlyph(x, y int, r rune) {
	if i, ok := g.idx(x, y); ok {
		g.glyphs[i] = r
	}
}

func (g *charGrid) glyphAt(x, y int) rune {
	if i, ok := g.idx(x, y); ok {
		return g.glyphs[i]
	}
	return 0
}

func (g *charGrid) isEmpty(x, y int) bool {
	i, ok := g.idx(x, y)
	return ok && g.glyphs[i] == 0 && g.flags[i] == 0
}

// keepFrames records the flags the group frames have set, once they are
// drawn and before anything else is (frameSide).
func (g *charGrid) keepFrames() { g.frames = slices.Clone(g.flags) }

// frameSide reports whether the only ink in cell (x, y) is a group
// frame's vertical side, which crosses a label's row: no glyph, and the
// frame's own flags, north and south, with nothing drawn into the cell
// since (S10 Text cells).
func (g *charGrid) frameSide(x, y int) bool {
	i, ok := g.idx(x, y)
	return ok && g.frames != nil && g.glyphs[i] == 0 && g.frames[i] == flagN|flagS && g.flags[i] == flagN|flagS
}

func (g *charGrid) writeText(x, y int, s string) {
	for i, r := range []rune(s) {
		g.setGlyph(x+i, y, r)
	}
}

// runeAt resolves one cell exactly as String does.
func (g *charGrid) runeAt(x, y int) rune {
	i, ok := g.idx(x, y)
	if !ok {
		return ' '
	}
	if g.glyphs[i] != 0 {
		return g.glyphs[i]
	}
	f := g.flags[i]
	ch := lineChars[f&15]
	if pair, styled := styledRuns[lineStyle(g.style[i])]; styled && f != 0 {
		if f&(flagN|flagS) == 0 {
			ch = pair[0]
		} else if f&(flagE|flagW) == 0 {
			ch = pair[1]
		}
	}
	return ch
}

// box draws a filled node box: border glyphs by frame, interior cleared
// with spaces, label lines block-centered with row 1 as the ceiling so a
// too-tall block clips at the borders instead of overwriting them.
func (g *charGrid) box(x, y, w, h int, lines []string, frame boxFrame) {
	tl, tr, bl, br := '┌', '┐', '└', '┘'
	top := '─'
	switch {
	case frame.Rounded:
		tl, tr, bl, br = '╭', '╮', '╰', '╯'
	case frame.Top == '═':
		tl, tr = '╒', '╕'
		top = '═'
	}
	right, bottom := x+w-1, y+h-1
	for cx := x; cx <= right; cx++ {
		for cy := y; cy <= bottom; cy++ {
			ch := ' '
			switch {
			case cx == x && cy == y:
				ch = tl
			case cx == right && cy == y:
				ch = tr
			case cx == x && cy == bottom:
				ch = bl
			case cx == right && cy == bottom:
				ch = br
			case cy == y:
				ch = top
			case cy == bottom:
				ch = '─'
			case cx == x || cx == right:
				ch = '│'
			}
			g.setGlyph(cx, cy, ch)
		}
	}
	if len(lines) == 0 {
		return
	}
	startRow := y + max(1, (h-len(lines))/2)
	for li, line := range lines {
		row := startRow + li
		if row >= bottom {
			break
		}
		rs := []rune(line)
		if len(rs) > w-2 {
			rs = rs[:max(0, w-2)]
		}
		start := x + max(1, (w-len(rs))/2)
		g.writeText(start, row, string(rs))
	}
}

// frame draws a group frame: sides as solid line flags (so wires can punch
// through and resolve to junctions), corners and the embedded title as
// glyphs (which always win), its first rune in column at, a blank on
// either side. The title is truncated to the room right of its column, so
// it stops two columns short of the right border regardless of at; for the
// default column (three columns in) that is the frame's own width minus
// six. The interior is untouched.
func (g *charGrid) frame(x, y, w, h int, title string, at int) {
	right, bottom := x+w-1, y+h-1
	g.hline(x, right, y, styleSolid)
	g.hline(x, right, bottom, styleSolid)
	g.vline(y, bottom, x, styleSolid)
	g.vline(y, bottom, right, styleSolid)
	g.setGlyph(x, y, '┌')
	g.setGlyph(right, y, '┐')
	g.setGlyph(x, bottom, '└')
	g.setGlyph(right, bottom, '┘')
	if title != "" {
		rs := []rune(title)
		room := max(0, x+w-3-at)
		if len(rs) > room {
			rs = rs[:room]
		}
		g.writeText(at-1, y, " "+string(rs)+" ")
	}
}

// String resolves every cell (glyph wins, then junction table, then styled
// substitute for pure runs), right-trims rows, drops leading and trailing
// blank rows, and ends with exactly one newline.
func (g *charGrid) String() string {
	art, _ := g.render()
	return art
}

// render is String plus the number of leading blank rows it dropped, so a
// caller can map art rows back to grid rows.
func (g *charGrid) render() (string, int) {
	lines := make([]string, 0, g.rows)
	for y := 0; y < g.rows; y++ {
		var sb strings.Builder
		for x := 0; x < g.cols; x++ {
			sb.WriteRune(g.runeAt(x, y))
		}
		lines = append(lines, strings.TrimRight(sb.String(), " "))
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	dropped := 0
	for len(lines) > 0 && lines[0] == "" {
		lines = lines[1:]
		dropped++
	}
	if len(lines) == 0 {
		return "", dropped
	}
	return strings.Join(lines, "\n") + "\n", dropped
}
