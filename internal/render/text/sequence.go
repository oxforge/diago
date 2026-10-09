package text

import (
	"sort"
	"strings"

	"github.com/oxforge/diago/internal/model"
)

// RenderSequence converts a text-profile PositionedSequence into text art.
// Every endpoint tied to an actor resolves through one shared, once-rounded
// column per actor (colOf), never through a rounded FromX/ToX.
func RenderSequence(ps *model.PositionedSequence, showActivations bool) Result {
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
		return Result{Art: g.String()}
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
	return Result{Art: titledArt, TopRow: top - titleLines}
}

// drawFragment paints the double-line frame, ╪ at every lifeline on frame
// and divider rows, the dividers, and the header rows: ══ type [guard] ══
// from the top-left corner and ╌╌ [guard] ╌╌ from each divider's left end,
// written over any lifeline they cross.
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
	pierceRow := func(row int) {
		for _, c := range pierce {
			g.setGlyph(c, row, '╪')
		}
	}
	// header writes text on row after FragmentHeaderMargin border runes,
	// then that many more, never past the frame's right side.
	header := func(row int, border rune, text string) {
		run := []rune(strings.Repeat(string(border), model.FragmentHeaderMargin) + text +
			strings.Repeat(string(border), model.FragmentHeaderMargin))
		for i, r := range run {
			if x+1+i >= right {
				break
			}
			g.setGlyph(x+1+i, row, r)
		}
	}

	pierceRow(y)
	pierceRow(bottom)
	first := ""
	if len(f.Sections) > 0 {
		first = f.Sections[0].Label
	}
	header(y, '═', model.FragmentHeader(f.Type, first))
	for _, s := range f.Sections[min(1, len(f.Sections)):] {
		row := px2row(s.Y)
		g.setGlyph(x, row, '╟')
		g.setGlyph(right, row, '╢')
		for cx := x + 1; cx < right; cx++ {
			g.setGlyph(cx, row, '╌')
		}
		pierceRow(row)
		if s.Label != "" {
			header(row, '╌', model.FragmentGuard(s.Label))
		}
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
		text := []rune(" " + m.Label + " ")
		start := (fromCol+toCol)/2 - len(text)/2
		lo, hi := min(fromCol, toCol), max(fromCol, toCol)
		for i, r := range text {
			// the label never covers the wire's ends: the source junction and the head
			if x := start + i; x > lo && x < hi {
				g.setGlyph(x, y, r)
			}
		}
	}
}
