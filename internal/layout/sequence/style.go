package sequence

import (
	"math"

	"github.com/oxforge/diago/internal/model"
)

// NormalizeSequence shifts all sequence diagram elements so that
// the content starts at (margin, margin), ensuring equal margins on all sides.
// measureText and fonts are used to compute label extents for bounds calculation.
func NormalizeSequence(
	ps *model.PositionedSequence,
	measureText func(string, float64, string) (float64, float64),
	fonts LayoutFonts,
) {
	p := ScreenProfile(fonts)
	p.MeasureText = measureText
	NormalizeSequenceWithProfile(ps, p)
}

// NormalizeSequenceWithProfile shifts all sequence diagram elements so that
// the content starts at (margin, margin), ensuring equal margins on all
// sides. NormalizeSequence is the screen-profile wrapper.
func NormalizeSequenceWithProfile(ps *model.PositionedSequence, p Profile) {
	margin := p.NormalizeMargin

	if len(ps.Actors) == 0 {
		return
	}

	// Find content bounding box.
	minX, minY := math.MaxFloat64, math.MaxFloat64
	maxX, maxY := -math.MaxFloat64, -math.MaxFloat64
	for _, a := range ps.Actors {
		minX = min(minX, a.BoxX)
		minY = min(minY, a.BoxY)
		maxX = max(maxX, a.BoxX+a.BoxW)
		maxY = max(maxY, a.LineBottom)
	}
	for _, it := range ps.Interactions {
		if it.IsSelf {
			for _, pt := range it.SelfPoints {
				minX = min(minX, pt.X)
				maxX = max(maxX, pt.X)
			}
		}
	}
	for _, f := range ps.Fragments {
		minX = min(minX, f.X)
		minY = min(minY, f.Y)
		maxX = max(maxX, f.X+f.Width)
		maxY = max(maxY, f.Y+f.Height)
	}
	for _, act := range ps.Activations {
		minX = min(minX, act.X)
		maxX = max(maxX, act.X+act.Width)
	}

	// Include interaction label extents in bounds.
	for _, it := range ps.Interactions {
		if it.Label == "" {
			continue
		}
		lw, _ := p.MeasureText(it.Label, p.Fonts.LabelSize, p.Fonts.Family)
		if it.IsSelf && len(it.SelfPoints) >= 2 {
			// Self-interaction label: anchor="start" at SelfPoints[1].X + gap.
			labelLeft := it.SelfPoints[1].X + p.SelfLabelGap
			maxX = max(maxX, labelLeft+lw)
		} else {
			// Normal interaction label: anchor="middle" centered between FromX and ToX.
			cx := (it.FromX + it.ToX) / 2
			hw := lw / 2
			minX = min(minX, cx-hw)
			maxX = max(maxX, cx+hw)
		}
	}

	if p.TextMode() {
		minX = math.Floor(minX/p.CellW) * p.CellW
		minY = math.Floor(minY/p.CellH) * p.CellH
		maxX = math.Ceil(maxX/p.CellW) * p.CellW
		maxY = math.Ceil(maxY/p.CellH) * p.CellH
	}

	dx := margin - minX
	dy := margin - minY

	if dx == 0 && dy == 0 {
		ps.Width = (maxX - minX) + 2*margin
		ps.Height = (maxY - minY) + 2*margin
		return
	}

	// Shift actors.
	for i := range ps.Actors {
		ps.Actors[i].BoxX += dx
		ps.Actors[i].BoxY += dy
		ps.Actors[i].LineX += dx
		ps.Actors[i].LineTop += dy
		ps.Actors[i].LineBottom += dy
	}

	// Shift interactions.
	for i := range ps.Interactions {
		ps.Interactions[i].Y += dy
		ps.Interactions[i].FromX += dx
		ps.Interactions[i].ToX += dx
		for j := range ps.Interactions[i].SelfPoints {
			ps.Interactions[i].SelfPoints[j].X += dx
			ps.Interactions[i].SelfPoints[j].Y += dy
		}
	}

	// Shift fragments.
	for i := range ps.Fragments {
		ps.Fragments[i].X += dx
		ps.Fragments[i].Y += dy
		for j := range ps.Fragments[i].Sections {
			ps.Fragments[i].Sections[j].Y += dy
		}
	}

	// Shift activations.
	for i := range ps.Activations {
		ps.Activations[i].X += dx
		ps.Activations[i].Y += dy
	}

	// Recompute bounds.
	ps.Width = (maxX - minX) + 2*margin
	ps.Height = (maxY - minY) + 2*margin
}
