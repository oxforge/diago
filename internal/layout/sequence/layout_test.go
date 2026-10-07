package sequence_test

import (
	"testing"

	seqlayout "github.com/oxforge/diago/internal/layout/sequence"
	"github.com/oxforge/diago/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixedMeasure is a deterministic text measurer that returns width based on string length.
func fixedMeasure(text string, _ float64, _ string) (float64, float64) {
	return float64(len(text)) * 8.0, 16.0
}

func twoActorDiagram(interactions []model.Interaction) model.SequenceDiagram {
	return model.SequenceDiagram{
		Actors: []model.Actor{
			{ID: "a", Label: "A"},
			{ID: "b", Label: "B"},
		},
		Interactions: interactions,
		Activations:  true,
	}
}

func TestLayoutBasicTwoActors(t *testing.T) {
	diag := twoActorDiagram([]model.Interaction{
		{From: "a", To: "b", Label: "hello"},
	})
	result := seqlayout.Layout(diag, fixedMeasure, seqlayout.LayoutFonts{Family: "Inter", HeaderSize: 14.0, LabelSize: 12.0, FragLabelSize: 11.0})
	require.NotNil(t, result)

	assert.Len(t, result.Actors, 2)
	assert.Len(t, result.Interactions, 1)

	actorA := result.Actors[0]
	actorB := result.Actors[1]

	// Actor A should be to the left of B.
	assert.Less(t, actorA.LineX, actorB.LineX, "actor A should be left of B")

	// Header boxes should be at the top.
	assert.Equal(t, actorA.BoxY, actorB.BoxY, "both actors start at same Y")

	// Lifeline starts below the header box.
	assert.Equal(t, actorA.BoxY+actorA.BoxH, actorA.LineTop)

	// There should be one interaction.
	it := result.Interactions[0]
	assert.Equal(t, "a", it.From)
	assert.Equal(t, "b", it.To)
	assert.False(t, it.IsSelf)
	assert.Greater(t, it.Y, actorA.LineTop, "interaction should be below header")

	// Dimension sanity.
	assert.Greater(t, result.Width, 0.0)
	assert.Greater(t, result.Height, 0.0)
}

func TestLayoutActorSpacing(t *testing.T) {
	// Actors should not overlap even when labels are long.
	diag := model.SequenceDiagram{
		Actors: []model.Actor{
			{ID: "a", Label: "A Very Long Actor Name"},
			{ID: "b", Label: "Another Very Long Name"},
		},
		Interactions: []model.Interaction{
			{From: "a", To: "b", Label: "request"},
		},
		Activations: false,
	}
	result := seqlayout.Layout(diag, fixedMeasure, seqlayout.LayoutFonts{Family: "Inter", HeaderSize: 14.0, LabelSize: 12.0, FragLabelSize: 11.0})
	require.NotNil(t, result)

	// The right edge of actor A should not overlap the left edge of actor B.
	actorA := result.Actors[0]
	actorB := result.Actors[1]
	aRight := actorA.BoxX + actorA.BoxW
	bLeft := actorB.BoxX
	assert.LessOrEqual(t, aRight, bLeft, "actor boxes should not overlap")
}

func TestLayoutInteractionOrdering(t *testing.T) {
	diag := twoActorDiagram([]model.Interaction{
		{From: "a", To: "b", Label: "first"},
		{From: "b", To: "a", Label: "second"},
		{From: "a", To: "b", Label: "third"},
	})
	result := seqlayout.Layout(diag, fixedMeasure, seqlayout.LayoutFonts{Family: "Inter", HeaderSize: 14.0, LabelSize: 12.0, FragLabelSize: 11.0})

	// Interactions should be strictly ordered top-to-bottom.
	y0 := result.Interactions[0].Y
	y1 := result.Interactions[1].Y
	y2 := result.Interactions[2].Y
	assert.Less(t, y0, y1, "interaction 0 should be above 1")
	assert.Less(t, y1, y2, "interaction 1 should be above 2")
}

func TestLayoutSelfMessage(t *testing.T) {
	diag := twoActorDiagram([]model.Interaction{
		{From: "a", To: "a", Label: "process()"},
	})
	result := seqlayout.Layout(diag, fixedMeasure, seqlayout.LayoutFonts{Family: "Inter", HeaderSize: 14.0, LabelSize: 12.0, FragLabelSize: 11.0})
	require.NotNil(t, result)

	it := result.Interactions[0]
	assert.True(t, it.IsSelf, "interaction should be marked as self")
	assert.Len(t, it.SelfPoints, 4, "self-message should have 4 waypoints")

	// The U-shape: first point and last point should share the same X.
	assert.Equal(t, it.SelfPoints[0].X, it.SelfPoints[3].X, "first and last X should be equal (same lifeline)")
	// Second and third point should be to the right (the rightward extension).
	assert.Greater(t, it.SelfPoints[1].X, it.SelfPoints[0].X, "second point should be right of first")
	assert.Greater(t, it.SelfPoints[2].Y, it.SelfPoints[1].Y, "third point should be below second (going down)")
}

func TestLayoutSelfMessageSpacing(t *testing.T) {
	// The interaction after a self-message should be below the self-loop bottom.
	diag := twoActorDiagram([]model.Interaction{
		{From: "a", To: "a", Label: "self"},
		{From: "a", To: "b", Label: "next"},
	})
	result := seqlayout.Layout(diag, fixedMeasure, seqlayout.LayoutFonts{Family: "Inter", HeaderSize: 14.0, LabelSize: 12.0, FragLabelSize: 11.0})

	selfIt := result.Interactions[0]
	nextIt := result.Interactions[1]
	selfBottom := selfIt.SelfPoints[3].Y
	assert.Greater(t, nextIt.Y, selfBottom, "next interaction must be below self-loop bottom")
}

func TestLayoutActivationBoxes(t *testing.T) {
	// request/response pair: actor B should get an activation box.
	diag := twoActorDiagram([]model.Interaction{
		{From: "a", To: "b", Label: "request"},
		{From: "b", To: "a", Label: "response"},
	})
	result := seqlayout.Layout(diag, fixedMeasure, seqlayout.LayoutFonts{Family: "Inter", HeaderSize: 14.0, LabelSize: 12.0, FragLabelSize: 11.0})
	require.NotNil(t, result)
	require.NotEmpty(t, result.Activations, "should have at least one activation box")

	// Find activation for actor b.
	var bActivation *model.PositionedActivation
	for i := range result.Activations {
		if result.Activations[i].ActorID == "b" {
			bActivation = &result.Activations[i]
			break
		}
	}
	require.NotNil(t, bActivation, "actor b should have an activation box")
	assert.Greater(t, bActivation.Height, 0.0, "activation box height should be positive")
}

func TestLayoutActivationsDisabled(t *testing.T) {
	diag := model.SequenceDiagram{
		Actors: []model.Actor{
			{ID: "a", Label: "A"},
			{ID: "b", Label: "B"},
		},
		Interactions: []model.Interaction{
			{From: "a", To: "b", Label: "request"},
			{From: "b", To: "a", Label: "response"},
		},
		Activations: false,
	}
	result := seqlayout.Layout(diag, fixedMeasure, seqlayout.LayoutFonts{Family: "Inter", HeaderSize: 14.0, LabelSize: 12.0, FragLabelSize: 11.0})
	assert.Empty(t, result.Activations, "no activation boxes when disabled")
}

func TestLayoutFragmentBounds(t *testing.T) {
	diag := model.SequenceDiagram{
		Actors: []model.Actor{
			{ID: "a", Label: "A"},
			{ID: "b", Label: "B"},
		},
		Interactions: []model.Interaction{
			{From: "a", To: "b", Label: "msg1"},
			{From: "b", To: "a", Label: "msg2"},
		},
		Fragments: []model.Fragment{
			{
				Type: model.FragmentOpt,
				Over: []string{"a", "b"},
				Sections: []model.FragmentSection{
					{Label: "if ready", Start: 0, End: 1},
				},
			},
		},
		Activations: false,
	}
	result := seqlayout.Layout(diag, fixedMeasure, seqlayout.LayoutFonts{Family: "Inter", HeaderSize: 14.0, LabelSize: 12.0, FragLabelSize: 11.0})
	require.Len(t, result.Fragments, 1)

	frag := result.Fragments[0]
	assert.Greater(t, frag.Width, 0.0, "fragment width should be positive")
	assert.Greater(t, frag.Height, 0.0, "fragment height should be positive")

	// Fragment should vertically bracket the interactions.
	it0 := result.Interactions[0]
	it1 := result.Interactions[1]
	assert.Less(t, frag.Y, it0.Y, "fragment top should be above first interaction")
	assert.Greater(t, frag.Y+frag.Height, it1.Y, "fragment bottom should be below last interaction")

	// Fragment should horizontally span both actors.
	actorA := result.Actors[0]
	actorB := result.Actors[1]
	assert.LessOrEqual(t, frag.X, actorA.LineX, "fragment left edge should be at or left of actor A lifeline")
	assert.GreaterOrEqual(t, frag.X+frag.Width, actorB.LineX, "fragment right edge should cover actor B lifeline")
}

func TestLayoutFragmentSections(t *testing.T) {
	diag := model.SequenceDiagram{
		Actors: []model.Actor{
			{ID: "a", Label: "A"},
			{ID: "b", Label: "B"},
		},
		Interactions: []model.Interaction{
			{From: "a", To: "b", Label: "msg1"},
			{From: "b", To: "a", Label: "msg2"},
			{From: "a", To: "b", Label: "msg3"},
			{From: "b", To: "a", Label: "msg4"},
		},
		Fragments: []model.Fragment{
			{
				Type: model.FragmentAlt,
				Over: []string{"a", "b"},
				Sections: []model.FragmentSection{
					{Label: "success", Start: 0, End: 1},
					{Label: "failure", Start: 2, End: 3},
				},
			},
		},
		Activations: false,
	}
	result := seqlayout.Layout(diag, fixedMeasure, seqlayout.LayoutFonts{Family: "Inter", HeaderSize: 14.0, LabelSize: 12.0, FragLabelSize: 11.0})
	require.Len(t, result.Fragments, 1)
	frag := result.Fragments[0]
	require.Len(t, frag.Sections, 2)

	// First section Y should equal fragment Y.
	assert.Equal(t, frag.Y, frag.Sections[0].Y, "first section Y should equal fragment Y")

	// Second section Y (divider) should be between the two section ranges.
	assert.Greater(t, frag.Sections[1].Y, frag.Sections[0].Y, "divider should be below first section top")
	assert.Less(t, frag.Sections[1].Y, frag.Y+frag.Height, "divider should be above fragment bottom")
}

func TestLayoutNestedFragments(t *testing.T) {
	diag := model.SequenceDiagram{
		Actors: []model.Actor{
			{ID: "a", Label: "A"},
			{ID: "b", Label: "B"},
		},
		Interactions: []model.Interaction{
			{From: "a", To: "b", Label: "msg1"},
			{From: "b", To: "a", Label: "msg2"},
			{From: "a", To: "b", Label: "msg3"},
			{From: "b", To: "a", Label: "msg4"},
		},
		Fragments: []model.Fragment{
			// Outer: covers all 4 interactions.
			{
				Type: model.FragmentLoop,
				Over: []string{"a", "b"},
				Sections: []model.FragmentSection{
					{Label: "retry", Start: 0, End: 3},
				},
			},
			// Inner: covers just interactions 1-2.
			{
				Type: model.FragmentOpt,
				Over: []string{"a", "b"},
				Sections: []model.FragmentSection{
					{Label: "if ready", Start: 1, End: 2},
				},
			},
		},
		Activations: false,
	}
	result := seqlayout.Layout(diag, fixedMeasure, seqlayout.LayoutFonts{Family: "Inter", HeaderSize: 14.0, LabelSize: 12.0, FragLabelSize: 11.0})
	require.Len(t, result.Fragments, 2)

	outer := result.Fragments[0]
	inner := result.Fragments[1]

	// Outer should fully contain inner.
	assert.LessOrEqual(t, outer.Y, inner.Y, "outer fragment should start above inner")
	outerBottom := outer.Y + outer.Height
	innerBottom := inner.Y + inner.Height
	assert.GreaterOrEqual(t, outerBottom, innerBottom, "outer fragment should end below inner")
}

func TestLayoutEmptyInteractions(t *testing.T) {
	diag := model.SequenceDiagram{
		Actors: []model.Actor{
			{ID: "a", Label: "A"},
			{ID: "b", Label: "B"},
		},
		Interactions: []model.Interaction{},
		Activations:  false,
	}
	result := seqlayout.Layout(diag, fixedMeasure, seqlayout.LayoutFonts{Family: "Inter", HeaderSize: 14.0, LabelSize: 12.0, FragLabelSize: 11.0})
	require.NotNil(t, result)
	assert.Len(t, result.Actors, 2)
	assert.Empty(t, result.Interactions)
	assert.Empty(t, result.Activations)
	assert.Greater(t, result.Width, 0.0)
	assert.Greater(t, result.Height, 0.0)
}
