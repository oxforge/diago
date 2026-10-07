package sequence

import (
	"sort"

	"github.com/oxforge/diago/internal/model"
)

// computeFragments converts Fragment specs into PositionedFragments.
// X spans from the leftmost "over" actor's lifeline minus padding to the rightmost plus padding.
// Y spans from the first section's start interaction Y minus padding to the last section's end Y plus padding.
//
// Fragments are processed smallest-span-first so that when a larger (outer) fragment
// is computed, it can expand its bounds to fully contain any already-computed inner
// fragments, adding visual padding between nested borders.
//
// Precondition: all fragment section Start/End indices must be valid indexes into
// interactionY. This is guaranteed by schema validation (see schema.ParseSequence).
func computeFragments(
	fragments []model.Fragment,
	interactionY []float64,
	actorIndex map[string]int,
	centerX []float64,
	posActors []model.PositionedActor,
	p Profile,
) []model.PositionedFragment {
	if len(fragments) == 0 {
		return nil
	}

	// Build an index mapping so we can process in span order but return in
	// declaration order.
	type indexedFragment struct {
		origIdx int
		frag    model.Fragment
		span    int // total interaction range span across all sections
	}
	indexed := make([]indexedFragment, 0, len(fragments))
	for i, f := range fragments {
		if len(f.Sections) == 0 {
			continue
		}
		if len(f.Over) == 0 {
			continue
		}
		// Span = (last section end) - (first section start).
		span := f.Sections[len(f.Sections)-1].End - f.Sections[0].Start
		indexed = append(indexed, indexedFragment{origIdx: i, frag: f, span: span})
	}

	// Sort by span ascending (smallest/innermost first).
	sort.Slice(indexed, func(i, j int) bool {
		return indexed[i].span < indexed[j].span
	})

	// Compute positioned fragments in span order, collecting results keyed by
	// original index so inner fragments are available when computing outer ones.
	computed := make(map[int]model.PositionedFragment, len(indexed))

	for _, entry := range indexed {
		f := entry.frag

		// Find leftmost and rightmost actor X coordinates.
		// Account for actor box half-widths at the extremes.
		leftActorIdx := actorIndex[f.Over[0]]
		rightActorIdx := actorIndex[f.Over[0]]
		for _, id := range f.Over {
			idx := actorIndex[id]
			if centerX[idx] < centerX[leftActorIdx] {
				leftActorIdx = idx
			}
			if centerX[idx] > centerX[rightActorIdx] {
				rightActorIdx = idx
			}
		}
		// Compute the widest section label so the fragment's left edge
		// extends far enough to fit labels without overlapping lifelines.
		maxSectionLabelW := 0.0
		for _, sec := range f.Sections {
			if sec.Label != "" {
				lw, _ := p.MeasureText("["+sec.Label+"]", p.Fonts.FragLabelSize, p.Fonts.Family)
				if lw > maxSectionLabelW {
					maxSectionLabelW = lw
				}
			}
		}
		// Left edge must leave room for the section label + padding.
		leftFromActor := posActors[leftActorIdx].BoxX - p.FragmentPaddingX
		leftFromLabel := centerX[leftActorIdx] - maxSectionLabelW - p.SectionLabelPadding
		leftEdge := leftFromActor
		if maxSectionLabelW > 0 && leftFromLabel < leftEdge {
			leftEdge = leftFromLabel
		}
		rightEdge := posActors[rightActorIdx].BoxX + posActors[rightActorIdx].BoxW + p.FragmentPaddingX

		// Determine top Y from first section's start interaction.
		firstStart := f.Sections[0].Start
		topY := interactionY[firstStart] - p.FragmentPaddingY

		// Determine bottom Y from last section's end interaction.
		lastSection := f.Sections[len(f.Sections)-1]
		bottomY := interactionY[lastSection.End] + p.FragmentPaddingY

		// Expand bounds to contain any already-computed inner fragments that fall
		// within this fragment's interaction range, adding padding between borders.
		for _, inner := range computed {
			// Check if inner fragment's Y range overlaps ours.
			innerTop := inner.Y
			innerBottom := inner.Y + inner.Height
			if innerTop >= topY && innerBottom <= bottomY {
				// Inner is already contained, expand if needed with extra padding.
				if innerTop-p.FragmentPaddingY < topY {
					topY = innerTop - p.FragmentPaddingY
				}
				if innerBottom+p.FragmentPaddingY > bottomY {
					bottomY = innerBottom + p.FragmentPaddingY
				}
			} else if innerTop < bottomY && innerBottom > topY {
				// Inner overlaps our range — expand to contain it fully.
				if innerTop-p.FragmentPaddingY < topY {
					topY = innerTop - p.FragmentPaddingY
				}
				if innerBottom+p.FragmentPaddingY > bottomY {
					bottomY = innerBottom + p.FragmentPaddingY
				}
			}
		}

		// Build section dividers: first section's Y = fragment top; subsequent sections have a divider.
		posSections := make([]model.PositionedFragmentSection, len(f.Sections))
		posSections[0] = model.PositionedFragmentSection{
			Label: f.Sections[0].Label,
			Y:     topY,
		}
		for j := 1; j < len(f.Sections); j++ {
			dividerY := interactionY[f.Sections[j].Start] - p.DividerInset
			posSections[j] = model.PositionedFragmentSection{
				Label: f.Sections[j].Label,
				Y:     dividerY,
			}
		}

		pf := model.PositionedFragment{
			Type:     f.Type,
			X:        leftEdge,
			Y:        topY,
			Width:    rightEdge - leftEdge,
			Height:   bottomY - topY,
			Sections: posSections,
		}
		computed[entry.origIdx] = pf
	}

	// Return in declaration order.
	result := make([]model.PositionedFragment, 0, len(computed))
	for i := 0; i < len(fragments); i++ {
		if pf, ok := computed[i]; ok {
			result = append(result, pf)
		}
	}

	return result
}
