// Package sequence implements the timeline layout engine for sequence diagrams.
// The layout is a strict vertical timeline: actors are positioned left-to-right,
// interactions are spaced top-to-bottom in declaration order.
package sequence

import (
	"math"

	"github.com/oxforge/diago/internal/model"
)

// LayoutFonts holds the font family and sizes used during sequence diagram layout.
type LayoutFonts struct {
	Family        string
	HeaderSize    float64
	LabelSize     float64
	FragLabelSize float64
}

// Layout computes the positions for all actors, interactions, activation boxes,
// and fragments in a sequence diagram.
// measureText is used to compute the rendered size of text labels.
// fonts specifies the font family and sizes for actor headers, interaction labels,
// and fragment section labels.
func Layout(
	diagram model.SequenceDiagram,
	measureText func(string, float64, string) (float64, float64),
	fonts LayoutFonts,
) *model.PositionedSequence {
	p := ScreenProfile(fonts)
	p.MeasureText = measureText
	return LayoutWithProfile(diagram, p)
}

// LayoutOptions parameterizes LayoutWithOptions.
type LayoutOptions struct {
	// InactiveInteractions marks interactions (by index) that keep their
	// row but neither open nor close an activation bar: a diff render's
	// removed messages. nil, or shorter than the list, means all active.
	InactiveInteractions []bool
}

// LayoutWithProfile computes the positions for all actors, interactions,
// activation boxes, and fragments in a sequence diagram under the given
// Profile. It is LayoutWithOptions with no options.
func LayoutWithProfile(diagram model.SequenceDiagram, p Profile) *model.PositionedSequence {
	return LayoutWithOptions(diagram, p, LayoutOptions{})
}

// LayoutWithOptions is LayoutWithProfile with an activation mask.
func LayoutWithOptions(diagram model.SequenceDiagram, p Profile, opts LayoutOptions) *model.PositionedSequence {
	if len(diagram.Actors) == 0 {
		return &model.PositionedSequence{}
	}

	actorIndex := make(map[string]int, len(diagram.Actors))
	for i, a := range diagram.Actors {
		actorIndex[a.ID] = i
	}
	plans := planFragments(diagram.Fragments, diagram.Interactions, actorIndex, p)

	// 1. Position actors.
	posActors, centerX, actorBoxWidths, maxHeaderH := layoutActors(diagram, actorIndex, plans, p)

	// 2. Position interactions vertically and route them.
	posInteractions, interactionY := layoutInteractions(diagram, actorIndex, centerX, maxHeaderH, p)

	// 3. Compute activation boxes (if enabled).
	var posActivations []model.PositionedActivation
	if diagram.Activations {
		posActivations = computeActivations(diagram.Interactions, interactionY, actorIndex, posActors, p, opts.InactiveInteractions)
	}

	// 4. Compute fragment bounds.
	posFragments := computeFragments(plans, diagram.Interactions, interactionY, actorIndex, centerX, posActors, p)

	// 5. Finalize dimensions and lifelines.
	width, height := calculateDimensions(
		diagram, posActors, posInteractions, posFragments,
		actorIndex, centerX, actorBoxWidths, interactionY, maxHeaderH,
		p,
	)

	return &model.PositionedSequence{
		Title:        diagram.Title,
		Actors:       posActors,
		Interactions: posInteractions,
		Fragments:    posFragments,
		Activations:  posActivations,
		Width:        width,
		Height:       height,
	}
}

type actorSize struct {
	w, h float64
}

// layoutActors computes the header box sizes and X-coordinates for each
// actor: far enough apart for their boxes, the labels of the messages
// between neighbors and, on screen, the fragment headers (plans).
func layoutActors(
	diagram model.SequenceDiagram,
	actorIndex map[string]int,
	plans []fragmentPlan,
	p Profile,
) ([]model.PositionedActor, []float64, []float64, float64) {
	sizes := make([]actorSize, len(diagram.Actors))
	maxHeaderH := 0.0
	for i, a := range diagram.Actors {
		tw, th := p.MeasureText(a.Label, p.Fonts.HeaderSize, p.Fonts.Family)
		w := tw + p.ActorPaddingX*2
		h := th + p.ActorPaddingY*2
		if w < p.ActorMinWidth {
			w = p.ActorMinWidth
		}
		if h < p.ActorMinHeight {
			h = p.ActorMinHeight
		}
		// In text mode, the box is centered on the lifeline (BoxX = centerX -
		// w/2), so w must be an even number of cells or the half-width would
		// land off-grid and drag centerX, which the text profile keeps on a
		// cell boundary, off-grid too.
		if p.TextMode() {
			evenCell := 2 * p.CellW
			w = math.Ceil(w/evenCell) * evenCell
		}
		sizes[i] = actorSize{w, h}
		if h > maxHeaderH {
			maxHeaderH = h
		}
	}

	centerX := make([]float64, len(diagram.Actors))
	centerX[0] = p.LeftMargin + sizes[0].w/2

	for i := 1; i < len(diagram.Actors); i++ {
		minGap := (sizes[i-1].w+sizes[i].w)/2.0 + p.ActorBoxGap
		if minGap < p.ActorSpacing {
			minGap = p.ActorSpacing
		}
		centerX[i] = centerX[i-1] + minGap
	}

	maxLabelW := make([]float64, len(diagram.Actors))
	for _, it := range diagram.Interactions {
		if it.From == it.To {
			continue
		}
		fi, ti := actorIndex[it.From], actorIndex[it.To]
		if fi > ti {
			fi, ti = ti, fi
		}
		lw, _ := p.MeasureText(it.Label, p.Fonts.LabelSize, p.Fonts.Family)
		if ti == fi+1 && lw > maxLabelW[fi] {
			maxLabelW[fi] = lw
		}
	}

	for i := 1; i < len(diagram.Actors); i++ {
		neededForLabel := maxLabelW[i-1] + p.ActorPaddingX*2
		minCenter := centerX[i-1] + neededForLabel
		if centerX[i] < minCenter {
			diff := minCenter - centerX[i]
			for j := i; j < len(diagram.Actors); j++ {
				centerX[j] += diff
			}
		}
	}

	boxW := make([]float64, len(sizes))
	for i, sz := range sizes {
		boxW[i] = sz.w
	}
	spaceForFragmentHeaders(plans, diagram.Interactions, actorIndex, boxW, centerX, p)

	actorBoxY := p.TopMargin
	posActors := make([]model.PositionedActor, len(diagram.Actors))
	actorBoxWidths := make([]float64, len(diagram.Actors))
	for i, a := range diagram.Actors {
		bw := sizes[i].w
		bh := maxHeaderH
		bx := centerX[i] - bw/2
		posActors[i] = model.PositionedActor{
			ID:      a.ID,
			Label:   a.Label,
			BoxX:    bx,
			BoxY:    actorBoxY,
			BoxW:    bw,
			BoxH:    bh,
			LineX:   centerX[i],
			LineTop: actorBoxY + bh,
			Color:   a.Color,
		}
		actorBoxWidths[i] = bw
	}

	return posActors, centerX, actorBoxWidths, maxHeaderH
}

// layoutInteractions computes the vertical positions and point coordinates for interactions.
func layoutInteractions(
	diagram model.SequenceDiagram,
	actorIndex map[string]int,
	centerX []float64,
	maxHeaderH float64,
	p Profile,
) ([]model.PositionedInteraction, []float64) {
	// fragmentSectionStarts counts the sections that start at each
	// interaction, over every fragment: each one draws its own header row
	// (a frame top or a divider) above that message, so frames opening on
	// the same message each reserve their room.
	fragmentSectionStarts := make(map[int]int)
	// fragmentEnds counts the fragments whose last section ends at each
	// interaction: every one of them closes a frame below that message, and
	// nested frames close on successive rows.
	fragmentEnds := make(map[int]int)
	for _, f := range diagram.Fragments {
		for _, sec := range f.Sections {
			fragmentSectionStarts[sec.Start]++
		}
		if len(f.Sections) > 0 {
			fragmentEnds[f.Sections[len(f.Sections)-1].End]++
		}
	}

	firstInteractionY := p.TopMargin + maxHeaderH + p.InteractionSpacingY
	interactionY := make([]float64, len(diagram.Interactions))
	currentY := firstInteractionY
	for i, it := range diagram.Interactions {
		currentY += float64(fragmentSectionStarts[i]) * p.FragmentHeaderHeight
		interactionY[i] = currentY
		if it.From == it.To {
			currentY += p.SelfLoopHeight + p.InteractionSpacingY
		} else {
			currentY += p.InteractionSpacingY
		}
		currentY += float64(fragmentEnds[i]) * p.FragmentFooterHeight
	}

	posInteractions := make([]model.PositionedInteraction, len(diagram.Interactions))
	for i, it := range diagram.Interactions {
		y := interactionY[i]
		fromIdx := actorIndex[it.From]
		toIdx := actorIndex[it.To]
		fromX := centerX[fromIdx]
		toX := centerX[toIdx]

		pi := model.PositionedInteraction{
			From:  it.From,
			To:    it.To,
			Label: it.Label,
			Style: it.Style,
			Y:     y,
			Color: it.Color,
		}

		if it.From == it.To {
			pi.IsSelf = true
			lx := centerX[fromIdx]
			pi.SelfPoints = []model.Point{
				{X: lx, Y: y},
				{X: lx + p.SelfLoopWidth, Y: y},
				{X: lx + p.SelfLoopWidth, Y: y + p.SelfLoopHeight},
				{X: lx, Y: y + p.SelfLoopHeight},
			}
			pi.FromX = lx
			pi.ToX = lx
		} else {
			pi.IsSelf = false
			pi.FromX = fromX
			pi.ToX = toX
		}

		posInteractions[i] = pi
	}

	return posInteractions, interactionY
}

// calculateDimensions determines the total diagram size and updates lifeline bottoms.
func calculateDimensions(
	diagram model.SequenceDiagram,
	posActors []model.PositionedActor,
	posInteractions []model.PositionedInteraction,
	posFragments []model.PositionedFragment,
	actorIndex map[string]int,
	centerX []float64,
	actorBoxWidths []float64,
	interactionY []float64,
	maxHeaderH float64,
	p Profile,
) (float64, float64) {
	firstInteractionY := p.TopMargin + maxHeaderH + p.InteractionSpacingY
	maxY := firstInteractionY
	if len(interactionY) > 0 {
		last := interactionY[len(interactionY)-1]
		lastIt := diagram.Interactions[len(diagram.Interactions)-1]
		if lastIt.From == lastIt.To {
			maxY = last + p.SelfLoopHeight
		} else {
			maxY = last
		}
	}

	for _, pf := range posFragments {
		bottom := pf.Y + pf.Height
		if bottom > maxY {
			maxY = bottom
		}
	}

	lineBottom := maxY + p.BottomMargin
	for i := range posActors {
		posActors[i].LineBottom = lineBottom
	}

	rightmostX := centerX[len(centerX)-1] + actorBoxWidths[len(actorBoxWidths)-1]/2
	for _, it := range diagram.Interactions {
		if it.From == it.To {
			rightmostX = max(rightmostX, selfMessageRight(it, centerX[actorIndex[it.From]], p))
		}
	}

	for _, pf := range posFragments {
		fragRight := pf.X + pf.Width
		if fragRight > rightmostX {
			rightmostX = fragRight
		}
	}

	totalWidth := rightmostX + p.RightMargin
	totalHeight := lineBottom + p.BottomMargin/2

	return totalWidth, totalHeight
}
