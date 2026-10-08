package sequence

import (
	"sort"

	"github.com/oxforge/diago/internal/model"
)

// fragmentPlan is the part of a fragment's geometry that needs no position:
// its message range, its actor span, the sizes of its header and the
// fragments it contains.
type fragmentPlan struct {
	origIdx     int
	frag        model.Fragment
	first, last int // the interaction range across all sections
	// leftIdx and rightIdx are the leftmost and rightmost actors the frame
	// spans: "over", every actor a message in its range touches, and the
	// span of every fragment it contains.
	leftIdx, rightIdx int
	tabW, tabH        float64   // the operator tab; zero in text art
	guardW            []float64 // each section's guard ("[label]") width; zero without a label
	// headerW is the widest header row: the tab, then the first guard
	// after FragmentGuardGap, or a later guard after FragmentGuardGap from
	// the frame's left side.
	headerW float64
	inner   []int // the plans (indexes into the plan list) this one contains
}

// planFragments plans every fragment with sections and actors, smallest
// message range first, so a plan's inner fragments precede it.
//
// A fragment contains every smaller one whose range shares a message with
// its own. The sort is by range length, ties by declaration order with the
// later-declared fragment first, so of two frames of equal length (the same
// range, or overlapping ranges) the later-declared one is the inner one, as
// an importer that lists blocks by their opening line writes them.
func planFragments(fragments []model.Fragment, interactions []model.Interaction, actorIndex map[string]int, p Profile) []fragmentPlan {
	plans := make([]fragmentPlan, 0, len(fragments))
	for i, f := range fragments {
		if len(f.Sections) == 0 || len(f.Over) == 0 {
			continue
		}
		pl := fragmentPlan{origIdx: i, frag: f, first: f.Sections[0].Start, last: f.Sections[0].End}
		for _, sec := range f.Sections[1:] {
			pl.first, pl.last = min(pl.first, sec.Start), max(pl.last, sec.End)
		}
		plans = append(plans, pl)
	}
	sort.SliceStable(plans, func(i, j int) bool {
		si, sj := plans[i].last-plans[i].first, plans[j].last-plans[j].first
		if si != sj {
			return si < sj
		}
		return plans[i].origIdx > plans[j].origIdx
	})

	for i := range plans {
		pl := &plans[i]
		f := pl.frag

		// The actor span: "over", widened to every actor a message in the
		// range touches, so no message runs out of its frame, and to every
		// inner frame's span.
		pl.leftIdx, pl.rightIdx = actorIndex[f.Over[0]], actorIndex[f.Over[0]]
		widen := func(idx int) {
			pl.leftIdx, pl.rightIdx = min(pl.leftIdx, idx), max(pl.rightIdx, idx)
		}
		for _, id := range f.Over {
			widen(actorIndex[id])
		}
		for k := pl.first; k <= pl.last; k++ {
			widen(actorIndex[interactions[k].From])
			widen(actorIndex[interactions[k].To])
		}
		for j := range i {
			if in := plans[j]; in.last >= pl.first && in.first <= pl.last {
				pl.inner = append(pl.inner, j)
				widen(in.leftIdx)
				widen(in.rightIdx)
			}
		}

		// The header. Text art draws its own inline tab and guards on the
		// frame's rows, so only the guard widths count there.
		if !p.TextMode() {
			ow, oh := p.MeasureText(f.Type.String(), p.Fonts.FragLabelSize, p.Fonts.Family)
			pl.tabW = ow + 2*p.FragmentTabPadX + p.FragmentTabCut
			pl.tabH = oh + 2*p.FragmentTabPadY
		}
		pl.guardW = make([]float64, len(f.Sections))
		for k, sec := range f.Sections {
			row := 0.0
			if k == 0 {
				row = pl.tabW
			}
			if sec.Label != "" {
				pl.guardW[k], _ = p.MeasureText("["+sec.Label+"]", p.Fonts.FragLabelSize, p.Fonts.Family)
				row += p.FragmentGuardGap + pl.guardW[k]
			}
			pl.headerW = max(pl.headerW, row)
		}
	}
	return plans
}

// headerRoom is how far below a frame's line (its top, or a divider) an
// inner frame opening there starts: the frame's header row and a gap on
// screen, one row in text art.
func (pl fragmentPlan) headerRoom(p Profile) float64 {
	return max(p.FragmentPaddingY, pl.tabH+p.FragmentGuardGap)
}

// spaceForFragmentHeaders moves actors right, on screen, until every
// frame's sides keep FragmentPaddingX and half an activation bar clear of
// the lifelines beside its span, which the frame does not cover: its left
// side out in the gutter its header rows need left of its leftmost lifeline,
// its right side out past the self-message loops and labels in its range.
// Text art keeps its spacing: its frames draw their guards on their own
// rows.
func spaceForFragmentHeaders(plans []fragmentPlan, interactions []model.Interaction, actorIndex map[string]int, boxW, centerX []float64, p Profile) {
	if p.TextMode() || len(centerX) < 2 {
		return
	}
	clearance := p.FragmentPaddingX + p.ActivationWidth/2

	// leftExt is how far a frame's left side reaches left of its leftmost
	// lifeline, as computeFragments places it once no frame crosses the
	// lifeline left of its span; needLeft is the room an actor needs from
	// its left neighbor's lifeline for the frames it starts.
	leftExt := make([]float64, len(plans))
	needLeft := make([]float64, len(centerX))
	for i, pl := range plans {
		e := boxW[pl.leftIdx]/2 + p.FragmentPaddingX
		if pl.headerW > 0 {
			e = max(e, p.SectionLabelPadding+pl.headerW)
		}
		for _, j := range pl.inner {
			if plans[j].leftIdx == pl.leftIdx {
				e = max(e, leftExt[j]+p.FragmentPaddingX)
			}
		}
		leftExt[i] = e
		needLeft[pl.leftIdx] = max(needLeft[pl.leftIdx], e+clearance)
	}

	// rightEdge is a frame's right side as computeFragments places it; it
	// reads only the lifelines up to the frame's rightmost one.
	// Memoized per plan: every call comes after the lifelines up to the
	// frame's rightmost one are final, so the value never changes, and a
	// chain of nested frames costs one pass instead of a call per path.
	rightMemo := make([]float64, len(plans))
	rightDone := make([]bool, len(plans))
	var rightEdge func(i int) float64
	rightEdge = func(i int) float64 {
		if rightDone[i] {
			return rightMemo[i]
		}
		pl := plans[i]
		r := centerX[pl.rightIdx] + boxW[pl.rightIdx]/2 + p.FragmentPaddingX
		for k := pl.first; k <= pl.last; k++ {
			if it := interactions[k]; it.From == it.To {
				r = max(r, selfMessageRight(it, centerX[actorIndex[it.From]], p)+p.FragmentPaddingX)
			}
		}
		for _, j := range pl.inner {
			r = max(r, rightEdge(j)+p.FragmentPaddingX)
		}
		rightMemo[i], rightDone[i] = r, true
		return r
	}

	// Left to right, so every lifeline a frame's right side depends on is
	// final by the time its right neighbor is placed.
	for a := 1; a < len(centerX); a++ {
		need := needLeft[a]
		for i, pl := range plans {
			if pl.rightIdx == a-1 {
				need = max(need, rightEdge(i)+clearance-centerX[a-1])
			}
		}
		if gap := centerX[a] - centerX[a-1]; gap < need {
			for j := a; j < len(centerX); j++ {
				centerX[j] += need - gap
			}
		}
	}
}

// computeFragments positions the planned fragments and returns them in
// declaration order.
//
// A frame spans its plan's actors, FragmentPaddingX beyond the outermost
// header boxes, and reaches right past every self-message loop in its range
// and that loop's label. Each section's line, the frame top for the first
// section and a divider for every later one, sits SectionTopInset above the
// section's first message; the bottom sits FragmentPaddingY below the last
// message, or below its loop when that message is a self-message.
//
// A screen layout also places the frame's header: the operator tab at the
// frame's top-left, the first guard right of the tab on the same row, and
// each later guard just below its divider, all left of the leftmost spanned
// lifeline, the frame's left side moving out to make room. Text art draws
// its own inline tab and guards on the frame's rows, so it leaves the tab
// and guard fields zero.
//
// An outer frame contains each inner one: FragmentPaddingX left and right,
// FragmentPaddingY below, and its own header room above, both at its top
// and at a divider that opens on the inner frame's first message.
//
// Precondition: all fragment section Start/End indices must be valid indexes into
// interactionY. This is guaranteed by schema validation (see schema.ParseSequence).
func computeFragments(
	plans []fragmentPlan,
	interactions []model.Interaction,
	interactionY []float64,
	actorIndex map[string]int,
	centerX []float64,
	posActors []model.PositionedActor,
	p Profile,
) []model.PositionedFragment {
	if len(plans) == 0 {
		return nil
	}

	placed := make([]model.PositionedFragment, len(plans))
	for i, pl := range plans {
		f := pl.frag

		leftEdge := posActors[pl.leftIdx].BoxX - p.FragmentPaddingX
		if pl.headerW > 0 {
			leftEdge = min(leftEdge, centerX[pl.leftIdx]-p.SectionLabelPadding-pl.headerW)
		}
		rightEdge := posActors[pl.rightIdx].BoxX + posActors[pl.rightIdx].BoxW + p.FragmentPaddingX
		for k := pl.first; k <= pl.last; k++ {
			if it := interactions[k]; it.From == it.To {
				rightEdge = max(rightEdge, selfMessageRight(it, centerX[actorIndex[it.From]], p)+p.FragmentPaddingX)
			}
		}

		topY := interactionY[pl.first] - p.SectionTopInset
		bottomY := interactionY[pl.last] + p.FragmentPaddingY
		if last := interactions[pl.last]; last.From == last.To {
			bottomY += p.SelfLoopHeight
		}
		lineY := make([]float64, len(f.Sections))
		for k, sec := range f.Sections {
			lineY[k] = interactionY[sec.Start] - p.SectionTopInset
		}

		// Contain the inner frames. Above one, the line it opens under (the
		// top, or a divider starting on the same message) keeps this
		// frame's header row clear of it.
		room := pl.headerRoom(p)
		for _, j := range pl.inner {
			ib := placed[j]
			leftEdge = min(leftEdge, ib.X-p.FragmentPaddingX)
			rightEdge = max(rightEdge, ib.X+ib.Width+p.FragmentPaddingX)
			topY = min(topY, ib.Y-room)
			bottomY = max(bottomY, ib.Y+ib.Height+p.FragmentPaddingY)
			for k := 1; k < len(f.Sections); k++ {
				if f.Sections[k].Start == plans[j].first {
					lineY[k] = min(lineY[k], ib.Y-room)
				}
			}
		}
		lineY[0] = topY

		// Sections: the first one's line is the frame top, every later
		// one's a divider. On screen the first guard shares the tab's row,
		// a later guard sits just below its divider.
		sections := make([]model.PositionedFragmentSection, len(f.Sections))
		for k, sec := range f.Sections {
			s := model.PositionedFragmentSection{Label: sec.Label, Y: lineY[k]}
			if !p.TextMode() && sec.Label != "" {
				s.LabelX = leftEdge + p.FragmentGuardGap
				if k == 0 {
					s.LabelX += pl.tabW
				}
				s.LabelY = lineY[k] + pl.tabH/2
				s.LabelWidth = pl.guardW[k]
			}
			sections[k] = s
		}

		placed[i] = model.PositionedFragment{
			Type:      f.Type,
			X:         leftEdge,
			Y:         topY,
			Width:     rightEdge - leftEdge,
			Height:    bottomY - topY,
			Sections:  sections,
			TabWidth:  pl.tabW,
			TabHeight: pl.tabH,
			TabCut:    p.FragmentTabCut,
		}
	}

	// Return in declaration order.
	order := make([]int, len(plans))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(a, b int) bool { return plans[order[a]].origIdx < plans[order[b]].origIdx })
	result := make([]model.PositionedFragment, len(plans))
	for i, k := range order {
		result[i] = placed[k]
	}
	return result
}

// selfMessageRight is the right end of a self-message drawn on the lifeline
// at x: its loop, and the label beside the loop.
func selfMessageRight(it model.Interaction, x float64, p Profile) float64 {
	right := x + p.SelfLoopWidth
	if it.Label != "" {
		lw, _ := p.MeasureText(it.Label, p.Fonts.LabelSize, p.Fonts.Family)
		right += p.SelfLabelGap + lw
	}
	return right
}
