package text

import (
	"sort"

	"github.com/oxforge/diago/internal/model"
)

// RenderSequence converts a text-profile PositionedSequence into text art.
// Every endpoint tied to an actor resolves through one shared, once-rounded
// column per actor (colOf), never through a rounded FromX/ToX.
func RenderSequence(ps *model.PositionedSequence, showActivations bool) (Result, error) {
	cols := px2col(ps.Width) + 1
	rows := px2row(ps.Height) + 1
	// The footer boxes are a text-only feature: make room for them below
	// the lifelines.
	for _, a := range ps.Actors {
		if r := px2row(a.LineBottom+a.BoxH) + 1; r > rows {
			rows = r
		}
	}
	g := newCharGrid(cols, rows)
	if len(ps.Actors) == 0 {
		return Result{Art: g.String()}, nil
	}

	colOf := make(map[string]int, len(ps.Actors))
	for _, a := range ps.Actors {
		colOf[a.ID] = px2col(a.LineX)
	}

	// 1. Lifelines.
	for _, a := range ps.Actors {
		g.vline(px2row(a.LineTop), px2row(a.LineBottom)-1, colOf[a.ID], styleSolid)
	}

	// 2. Activation bars.
	active := map[string]map[int]bool{}
	if showActivations {
		for _, act := range ps.Activations {
			col := colOf[act.ActorID]
			rowsOf := active[act.ActorID]
			if rowsOf == nil {
				rowsOf = map[int]bool{}
				active[act.ActorID] = rowsOf
			}
			for y := px2row(act.Y); y <= px2row(act.Y+act.Height)-1; y++ {
				g.setGlyph(col, y, '┃')
				rowsOf[y] = true
			}
		}
	}

	// 3. Fragments, outer first.
	frags := make([]model.PositionedFragment, len(ps.Fragments))
	copy(frags, ps.Fragments)
	sort.SliceStable(frags, func(i, j int) bool { return frags[i].Width*frags[i].Height > frags[j].Width*frags[j].Height })
	for _, f := range frags {
		drawFragment(g, f, colOf)
	}

	// 4. Messages.
	for _, m := range ps.Interactions {
		drawMessage(g, m, colOf, active)
	}

	// 5. Actor headers top and bottom, last so they win.
	for _, a := range ps.Actors {
		w := px2col(a.BoxX+a.BoxW) - px2col(a.BoxX)
		h := px2row(a.BoxY+a.BoxH) - px2row(a.BoxY)
		left := colOf[a.ID] - w/2
		g.box(left, px2row(a.BoxY), w, h, []string{a.Label}, boxFrame{})
		g.box(left, px2row(a.LineBottom), w, h, []string{a.Label}, boxFrame{})
	}

	art, top := g.render()
	titledArt, titleLines := withTitle(art, ps.Title)
	return Result{Art: titledArt, TopRow: top - titleLines}, nil
}

func fragmentTab(t model.FragmentType) string { return " " + t.String() + " " }

// drawFragment paints the double-line frame, the tab slid right past any
// pierced lifeline, ╪ at every lifeline on frame and divider rows, and the
// section dividers with their guards.
func drawFragment(g *charGrid, f model.PositionedFragment, colOf map[string]int) {
	// The frame's top row is the row above the first message (f.Y sits one
	// row above it); the bottom edge f.Y+f.Height is one row below the last
	// message, so the closing line is drawn ON that row (inclusive), never
	// through the message.
	x, y := px2col(f.X), px2row(f.Y)
	w := px2col(f.X+f.Width) - x
	right, bottom := x+w-1, px2row(f.Y+f.Height)
	for cx := x; cx <= right; cx++ {
		g.setGlyph(cx, y, '═')
		g.setGlyph(cx, bottom, '═')
	}
	for cy := y + 1; cy < bottom; cy++ {
		g.setGlyph(x, cy, '║')
		g.setGlyph(right, cy, '║')
	}
	g.setGlyph(x, y, '╔')
	g.setGlyph(right, y, '╗')
	g.setGlyph(x, bottom, '╚')
	g.setGlyph(right, bottom, '╝')

	var pierce []int
	for _, c := range colOf {
		if c > x && c < right {
			pierce = append(pierce, c)
		}
	}
	sort.Ints(pierce)
	pierceRow := func(row int, protect map[int]bool) {
		for _, c := range pierce {
			if !protect[c] {
				g.setGlyph(c, row, '╪')
			}
		}
	}
	writeTracked := func(start, row int, text string) map[int]bool {
		used := map[int]bool{}
		for i, r := range []rune(text) {
			g.setGlyph(start+i, row, r)
			if r != ' ' {
				used[start+i] = true
			}
		}
		return used
	}
	slide := func(base int, text string) int {
		n := len([]rune(text))
		maxStart := right - n
		// Every character, padding spaces included, must avoid a pierced
		// column, so a slid tab or guard never reads glued to a ╪.
		for start := base; start <= maxStart; start++ {
			ok := true
			for i := range []rune(text) {
				for _, c := range pierce {
					if c == start+i {
						ok = false
					}
				}
			}
			if ok {
				return start
			}
		}
		// No collision-free start: keep the text inside the frame at least.
		return max(x+1, min(base, maxStart))
	}

	tab := fragmentTab(f.Type)
	tabStart := slide(x+2, tab)
	used := writeTracked(tabStart, y, tab)
	pierceRow(y, used)
	pierceRow(bottom, nil)

	for i, s := range f.Sections {
		if i == 0 {
			continue // the first section shares the frame's top row
		}
		row := px2row(s.Y)
		g.setGlyph(x, row, '╟')
		g.setGlyph(right, row, '╢')
		for cx := x + 1; cx < right; cx++ {
			g.setGlyph(cx, row, '╌')
		}
		var guardUsed map[int]bool
		if s.Label != "" {
			text := " [" + s.Label + "] "
			guardUsed = writeTracked(slide(x+2, text), row, text)
		}
		pierceRow(row, guardUsed)
	}
	// First section's guard sits after the tab on the top row.
	if len(f.Sections) > 0 && f.Sections[0].Label != "" {
		text := " [" + f.Sections[0].Label + "] "
		start := slide(tabStart+len([]rune(tab)), text)
		writeTracked(start, y, text)
	}
}

// drawMessage paints one message: a run between the two columns, a head at
// the destination, a ┠/┨ departure when leaving an active bar, and the label
// centered on the run.
func drawMessage(g *charGrid, m model.PositionedInteraction, colOf map[string]int, active map[string]map[int]bool) {
	filled := m.Style != model.InteractionAsync
	st := styleSolid
	if m.Style == model.InteractionDashed {
		st = styleDashed
	}
	if m.IsSelf && len(m.SelfPoints) == 4 {
		col := colOf[m.From]
		yTop, yBottom := px2row(m.SelfPoints[0].Y), px2row(m.SelfPoints[2].Y)
		xRight := px2col(m.SelfPoints[1].X)
		g.hline(col, xRight, yTop, st)
		g.vline(yTop, yBottom, xRight, st)
		g.hline(xRight, col, yBottom, st)
		if filled {
			g.setGlyph(col, yBottom, '◄')
		} else {
			g.setGlyph(col, yBottom, '◁')
		}
		if m.Label != "" {
			g.writeText(xRight+1, (yTop+yBottom)/2, " "+m.Label)
		}
		return
	}
	fromCol, toCol := colOf[m.From], colOf[m.To]
	y := px2row(m.Y)
	g.hline(fromCol, toCol, y, st)
	dir := 1
	if toCol < fromCol {
		dir = -1
	}
	switch {
	case filled && dir == 1:
		g.setGlyph(toCol, y, '►')
	case filled:
		g.setGlyph(toCol, y, '◄')
	case dir == 1:
		g.setGlyph(toCol, y, '▷')
	default:
		g.setGlyph(toCol, y, '◁')
	}
	if active[m.From][y] {
		if dir == 1 {
			g.setGlyph(fromCol, y, '┠')
		} else {
			g.setGlyph(fromCol, y, '┨')
		}
	}
	if m.Label != "" {
		text := " " + m.Label + " "
		mid := (fromCol + toCol) / 2
		g.writeText(mid-len([]rune(text))/2, y, text)
	}
}
