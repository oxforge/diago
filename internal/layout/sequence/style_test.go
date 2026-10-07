package sequence

import (
	"testing"

	"github.com/oxforge/diago/internal/model"
	"github.com/stretchr/testify/assert"
)

func stubMeasure(text string, size float64, family string) (float64, float64) {
	// Simple: 7px per character width, 14px height.
	return float64(len(text)) * 7, 14
}

func TestNormalizeSequence_AdjustsMargins(t *testing.T) {
	ps := &model.PositionedSequence{
		Width:  100,
		Height: 100,
		Actors: []model.PositionedActor{{ID: "a", Label: "A", BoxX: 10, BoxY: 10, BoxW: 20, BoxH: 20, LineX: 20, LineTop: 30, LineBottom: 90}},
	}
	NormalizeSequence(ps, stubMeasure, LayoutFonts{Family: "Inter", LabelSize: 12})
	// Actor box should be shifted so that minX=40 (margin)
	assert.Equal(t, 40.0, ps.Actors[0].BoxX)
	assert.Equal(t, 40.0, ps.Actors[0].BoxY)
}

func TestNormalizeSequence_SelfInteractionLabelExtendsBounds(t *testing.T) {
	// Self-interaction with label "process" positioned to the right of a self-loop.
	// SelfPoints[1].X is the rightmost point of the loop.
	// The label is rendered at SelfPoints[1].X + 6 with anchor "start",
	// so it extends rightward by the full text width.
	ps := &model.PositionedSequence{
		Actors: []model.PositionedActor{
			{ID: "a", BoxX: 40, BoxY: 40, BoxW: 80, BoxH: 30, LineX: 80, LineTop: 70, LineBottom: 200},
		},
		Interactions: []model.PositionedInteraction{
			{
				From:   "a",
				To:     "a",
				Label:  "process",
				IsSelf: true,
				Y:      100,
				FromX:  80,
				ToX:    80,
				SelfPoints: []model.Point{
					{X: 80, Y: 90},
					{X: 120, Y: 90},
					{X: 120, Y: 110},
					{X: 80, Y: 110},
				},
			},
		},
	}

	fonts := LayoutFonts{Family: "Inter", LabelSize: 12}
	NormalizeSequence(ps, stubMeasure, fonts)

	// "process" is 7 chars * 7px = 49px wide.
	// Label starts at SelfPoints[1].X + 6 = 126 (after shift).
	// Label right edge = 126 + 49 = 175.
	// Width should be >= 175 + 40 (right margin).
	// The actor box right edge is at 120. Without label, width would be
	// (120-40) + 2*40 = 160. With label, must be larger.
	if ps.Width <= 160 {
		t.Errorf("width should include self-interaction label: got %v, want > 160", ps.Width)
	}
}

func TestNormalizeSequence_NormalInteractionLabelExtendsBounds(t *testing.T) {
	// Two actors close together, but interaction has a wide label.
	// Label is centered between FromX and ToX with anchor "middle",
	// so it extends ±halfWidth from the center.
	ps := &model.PositionedSequence{
		Actors: []model.PositionedActor{
			{ID: "a", BoxX: 40, BoxY: 40, BoxW: 60, BoxH: 30, LineX: 70, LineTop: 70, LineBottom: 200},
			{ID: "b", BoxX: 120, BoxY: 40, BoxW: 60, BoxH: 30, LineX: 150, LineTop: 70, LineBottom: 200},
		},
		Interactions: []model.PositionedInteraction{
			{
				From:  "a",
				To:    "b",
				Label: "a]very long interaction label text here",
				Y:     100,
				FromX: 70,
				ToX:   150,
			},
		},
	}

	fonts := LayoutFonts{Family: "Inter", LabelSize: 12}
	NormalizeSequence(ps, stubMeasure, fonts)

	// Label: 39 chars * 7px = 273px wide. Center at (70+150)/2 = 110.
	// Left edge: 110 - 273/2 = -26.5. Right edge: 110 + 273/2 = 246.5.
	// Actor bounds: 40 to 180. Label extends well beyond.
	// Width should accommodate the label right edge + margin.
	if ps.Width <= 220 {
		t.Errorf("width should include wide interaction label: got %v, want > 220", ps.Width)
	}
}
