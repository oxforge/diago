package labels

import (
	"context"
	"math"

	"github.com/oxforge/diago/internal/layoutdbg"
)

// noise is the float tolerance of a title's fit, a millionth of a unit: a
// title may end exactly a gap from a blocked span; and of a text spot's
// snap to whole cells (S10).
const noise = 1e-6

// Span is an interval of a group's title band, from the group's left side
// in the output frame.
type Span struct{ Lo, Hi float64 }

// Band is one group's title band (S10): the group's width and its title's,
// both along the band, and the spans covered by whatever crosses the
// title's row.
type Band struct {
	ID      string
	Width   float64
	Title   float64
	Blocked []Span
}

// TitleOptions carries the title values from the Config (S14).
type TitleOptions struct {
	Inset float64 // a slot's distance from its group's left or right side
	Gap   float64 // the clear space a title keeps from a blocked span
	Text  bool    // the text profile: whole cells
}

// Title is where band b's title starts, from its group's left side (S10),
// and whether it is blocked: the left slot, Inset from the left side, when
// it is free; else the right slot, Inset from the right side; else the
// middle of the widest free span that holds it, the leftmost of equals. A
// spot is free when it keeps Gap from every blocked span; in the text
// profile a span covers the whole cells it is drawn in and the middle
// rounds down to a whole cell. Failing all three, the title stays in its
// left slot, blocked, recorded as group_title_blocked.
func Title(ctx context.Context, b Band, o TitleOptions) (float64, bool) {
	left, right := o.Inset, b.Width-o.Inset-b.Title
	free := []Span{{o.Inset, b.Width - o.Inset}}
	for _, s := range b.Blocked {
		if o.Text {
			lo := math.Floor(s.Lo + noise)
			s = Span{lo, math.Max(math.Ceil(s.Hi-noise), lo+1)}
		}
		free = cut(free, Span{s.Lo - o.Gap, s.Hi + o.Gap})
	}
	fits := func(x float64) bool {
		for _, f := range free {
			if x >= f.Lo-noise && x+b.Title <= f.Hi+noise {
				return true
			}
		}
		return false
	}
	if fits(left) {
		return left, false
	}
	if fits(right) {
		return right, false
	}
	widest := -1
	for i, f := range free {
		if f.Hi-f.Lo >= b.Title-noise && (widest < 0 || f.Hi-f.Lo > free[widest].Hi-free[widest].Lo+noise) {
			widest = i
		}
	}
	if widest >= 0 {
		x := (free[widest].Lo + free[widest].Hi - b.Title) / 2
		if o.Text {
			x = math.Floor(x + noise)
		}
		return x, false
	}
	layoutdbg.Decision(ctx, "group_title_blocked",
		"phase", "labels", "module", "diago", "spec_ref", "S10",
		"group", b.ID, "width", b.Width, "title", b.Title)
	return left, true
}

// cut removes span s from the free spans.
func cut(free []Span, s Span) []Span {
	out := make([]Span, 0, len(free)+1)
	for _, f := range free {
		if s.Hi <= f.Lo || s.Lo >= f.Hi {
			out = append(out, f)
			continue
		}
		if s.Lo > f.Lo {
			out = append(out, Span{f.Lo, s.Lo})
		}
		if s.Hi < f.Hi {
			out = append(out, Span{s.Hi, f.Hi})
		}
	}
	return out
}
