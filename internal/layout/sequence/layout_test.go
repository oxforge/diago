package sequence_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	seqlayout "github.com/oxforge/diago/internal/layout/sequence"
	"github.com/oxforge/diago/internal/model"
	"github.com/oxforge/diago/internal/schema"
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

// --- Fragment frames: header band, coverage, self-messages, nesting, gutter ---

var fragFonts = seqlayout.LayoutFonts{Family: "Inter", HeaderSize: 14, LabelSize: 12, FragLabelSize: 11}

type namedProfile struct {
	name string
	p    seqlayout.Profile
}

func screenAndText() []namedProfile {
	return []namedProfile{{"screen", seqlayout.ScreenProfile(fragFonts)}, {"text", seqlayout.TextProfile()}}
}

// layOut parses spec and lays it out, normalized, under p.
func layOut(t *testing.T, spec string, p seqlayout.Profile) *model.PositionedSequence {
	t.Helper()
	d, err := schema.ParseSequence([]byte(spec))
	require.NoError(t, err)
	ps := seqlayout.LayoutWithProfile(*d, p)
	seqlayout.NormalizeSequenceWithProfile(ps, p)
	return ps
}

// box is an axis-aligned rectangle by its corners.
type box struct{ x0, y0, x1, y1 float64 }

func (b box) intersects(o box) bool {
	return b.x0 < o.x1 && o.x0 < b.x1 && b.y0 < o.y1 && o.y0 < b.y1
}

// within reports whether b lies inside o with at least inset on every side.
func (b box) within(o box, inset float64) bool {
	const eps = 1e-9
	return b.x0-o.x0 >= inset-eps && o.x1-b.x1 >= inset-eps && b.y0-o.y0 >= inset-eps && o.y1-b.y1 >= inset-eps
}

func frameBox(f model.PositionedFragment) box { return box{f.X, f.Y, f.X + f.Width, f.Y + f.Height} }

func tabBox(f model.PositionedFragment) box {
	return box{f.X, f.Y, f.X + f.TabWidth, f.Y + f.TabHeight}
}

// guardBox is the box of a section's guard text, measured with the profile.
func guardBox(s model.PositionedFragmentSection, p seqlayout.Profile) box {
	_, h := p.MeasureText("["+s.Label+"]", p.Fonts.FragLabelSize, p.Fonts.Family)
	return box{s.LabelX, s.LabelY - h/2, s.LabelX + s.LabelWidth, s.LabelY + h/2}
}

// messageLabelBox is the box of a message's label, measured with the
// profile: centered on the arrow, or right of a self-message's loop.
func messageLabelBox(it model.PositionedInteraction, p seqlayout.Profile) (box, bool) {
	if it.Label == "" {
		return box{}, false
	}
	w, h := p.MeasureText(it.Label, p.Fonts.LabelSize, p.Fonts.Family)
	if it.IsSelf {
		left := it.SelfPoints[1].X + p.SelfLabelGap
		cy := (it.SelfPoints[0].Y + it.SelfPoints[2].Y) / 2
		return box{left, cy - h/2, left + w, cy + h/2}, true
	}
	cx := (it.FromX + it.ToX) / 2
	return box{cx - w/2, it.Y - h/2, cx + w/2, it.Y + h/2}, true
}

// nestTop is how far below an outer frame's line an inner frame's top must
// sit: the outer's header row and a gap on screen, one row in text art.
func nestTop(p seqlayout.Profile, outer model.PositionedFragment) float64 {
	return max(p.FragmentPaddingY, outer.TabHeight+p.FragmentGuardGap)
}

const reproSpec = `{"type":"sequence","actors":[{"id":"c","label":"Client"},{"id":"s","label":"Shop"},{"id":"w","label":"Warehouse"}],
"interactions":[{"from":"c","to":"s","label":"order"},{"from":"s","to":"w","label":"reserve"},
{"from":"w","to":"s","label":"ok"},{"from":"s","to":"c","label":"out of stock"}],
"fragments":[{"type":"alt","over":["s","w"],"sections":[{"label":"in stock","start":1,"end":2},{"label":"out of stock","start":3,"end":3}]}]}`

// headerSpec has a three-section alt whose sections start on a plain
// message, a self-message and a message to an actor outside over, and an
// opt nested in its first section.
const headerSpec = `{"type":"sequence","actors":[{"id":"u","label":"User"},{"id":"w","label":"Web App"},{"id":"a","label":"API"}],
"interactions":[{"from":"u","to":"w","label":"submit the order form"},{"from":"w","to":"a","label":"create order"},
{"from":"a","to":"w","label":"201 created"},{"from":"w","to":"w","label":"validate the cart"},{"from":"w","to":"u","label":"rejected"},
{"from":"a","to":"u","label":"timed out"}],
"fragments":[{"type":"alt","over":["w","a"],"sections":[{"label":"accepted","start":1,"end":2},{"label":"invalid cart","start":3,"end":4},{"start":5,"end":5}]},
{"type":"opt","over":["w","a"],"sections":[{"label":"created","start":2,"end":2}]}]}`

// A section's line (the frame top or its divider) sits at least
// SectionTopInset above the section's first message, and that message's
// label clears the section's guard and the operator tab.
func TestLayoutFragment_SectionHeaderClearsItsFirstMessage(t *testing.T) {
	p := seqlayout.ScreenProfile(fragFonts)
	ps := layOut(t, headerSpec, p)
	starts := [][]int{{1, 3, 5}, {2}}
	require.Len(t, ps.Fragments, len(starts))
	for fi, f := range ps.Fragments {
		require.Len(t, f.Sections, len(starts[fi]))
		assert.Positive(t, f.TabWidth, "fragment %d has a tab", fi)
		assert.Positive(t, f.TabHeight, "fragment %d has a tab", fi)
		for k, s := range f.Sections {
			m := ps.Interactions[starts[fi][k]]
			assert.GreaterOrEqual(t, m.Y-s.Y, p.SectionTopInset,
				"fragment %d section %d: first message at %g, line at %g", fi, k, m.Y, s.Y)
			lb, ok := messageLabelBox(m, p)
			require.True(t, ok)
			if k == 0 {
				assert.False(t, lb.intersects(tabBox(f)), "fragment %d: the tab %+v meets the label %+v of %q", fi, tabBox(f), lb, m.Label)
			}
			if s.Label != "" {
				assert.Positive(t, s.LabelWidth, "fragment %d section %d guard width", fi, k)
				assert.False(t, lb.intersects(guardBox(s, p)), "fragment %d section %d: the guard %+v meets the label %+v of %q", fi, k, guardBox(s, p), lb, m.Label)
			}
		}
	}
}

// The frame covers both ends of every message in its sections,
// Client included though over leaves it out, and no message label lands
// on a guard.
func TestLayoutFragment_FrameCoversEveryMessageInItsSections(t *testing.T) {
	for _, np := range screenAndText() {
		t.Run(np.name, func(t *testing.T) {
			ps := layOut(t, reproSpec, np.p)
			require.Len(t, ps.Fragments, 1)
			f := ps.Fragments[0]
			for i := 1; i <= 3; i++ {
				m := ps.Interactions[i]
				assert.Less(t, f.X, min(m.FromX, m.ToX), "message %d %q starts inside the frame", i, m.Label)
				assert.Greater(t, f.X+f.Width, max(m.FromX, m.ToX), "message %d %q ends inside the frame", i, m.Label)
			}
			if np.name != "screen" {
				return
			}
			for i, m := range ps.Interactions {
				lb, ok := messageLabelBox(m, np.p)
				if !ok {
					continue
				}
				for k, s := range f.Sections {
					assert.False(t, lb.intersects(guardBox(s, np.p)), "message %d %q meets section %d's guard", i, m.Label, k)
				}
			}
		})
	}
}

// selfSpec ends an alt's first section, and the alt itself, on a
// self-message, and puts a self-message alone in an opt over its actor.
const selfSpec = `{"type":"sequence","actors":[{"id":"u","label":"User"},{"id":"w","label":"Web App"},{"id":"a","label":"API"}],
"interactions":[{"from":"u","to":"w","label":"open"},{"from":"w","to":"a","label":"fetch"},
{"from":"w","to":"w","label":"render the whole list"},{"from":"a","to":"w","label":"stale"},{"from":"w","to":"w","label":"cache it"},
{"from":"w","to":"w","label":"render list"},{"from":"u","to":"w","label":"done"}],
"fragments":[{"type":"alt","over":["w","a"],"sections":[{"label":"fresh","start":1,"end":2},{"label":"stale","start":3,"end":4}]},
{"type":"opt","over":["w"],"sections":[{"label":"items found","start":5,"end":5}]}]}`

// A self-message's loop and its label lie inside its frame, and
// above the next section's divider.
func TestLayoutFragment_FrameHoldsASelfMessageAndItsLabel(t *testing.T) {
	ranges := [][2]int{{1, 4}, {5, 5}}
	for _, np := range screenAndText() {
		t.Run(np.name, func(t *testing.T) {
			ps := layOut(t, selfSpec, np.p)
			require.Len(t, ps.Fragments, len(ranges))
			for fi, f := range ps.Fragments {
				fb := frameBox(f)
				for i := ranges[fi][0]; i <= ranges[fi][1]; i++ {
					m := ps.Interactions[i]
					if !m.IsSelf {
						continue
					}
					loop := box{m.SelfPoints[0].X, m.SelfPoints[0].Y, m.SelfPoints[1].X, m.SelfPoints[2].Y}
					assert.True(t, loop.within(fb, 1e-6), "fragment %d: the loop of %q %+v sticks out of the frame %+v", fi, m.Label, loop, fb)
					lb, _ := messageLabelBox(m, np.p)
					assert.True(t, lb.within(fb, 1e-6), "fragment %d: the label of %q %+v sticks out of the frame %+v", fi, m.Label, lb, fb)
				}
			}
			alt := ps.Fragments[0]
			self := ps.Interactions[2]
			lb, _ := messageLabelBox(self, np.p)
			assert.Less(t, max(self.SelfPoints[2].Y, lb.y1), alt.Sections[1].Y, "the loop ending the first section stays above the divider")
		})
	}
}

// nestedCase is an outer frame with one inner frame; divider is the outer's
// section starting on the inner's first message, or -1.
type nestedCase struct {
	name         string
	spec         string
	outer, inner int
	divider      int
}

var nestedCases = []nestedCase{
	{"an opt over fewer actors", `{"type":"sequence","actors":[{"id":"a","label":"A"},{"id":"b","label":"B"},{"id":"c","label":"C"}],
		"interactions":[{"from":"a","to":"b","label":"one"},{"from":"b","to":"c","label":"two"},{"from":"c","to":"b","label":"three","style":"dashed"},{"from":"b","to":"a","label":"four","style":"dashed"}],
		"fragments":[{"type":"loop","over":["a","b","c"],"sections":[{"label":"each","start":0,"end":3}]},
		{"type":"opt","over":["b","c"],"sections":[{"label":"maybe","start":1,"end":2}]}]}`, 0, 1, -1},
	{"an opt over the same actors with a longer guard", `{"type":"sequence","actors":[{"id":"c","label":"Client"},{"id":"s","label":"Server"}],
		"interactions":[{"from":"c","to":"s","label":"submit job"},{"from":"c","to":"s","label":"poll status"},{"from":"s","to":"c","label":"result","style":"dashed"}],
		"fragments":[{"type":"loop","over":["c","s"],"sections":[{"label":"until done","start":1,"end":2}]},
		{"type":"opt","over":["c","s"],"sections":[{"label":"job finished","start":2,"end":2}]}]}`, 0, 1, -1},
	{"two frames starting on the same message", `{"type":"sequence","actors":[{"id":"a","label":"A"},{"id":"b","label":"B"}],
		"interactions":[{"from":"a","to":"b","label":"hello"},{"from":"a","to":"b","label":"one"},{"from":"b","to":"a","label":"two"},{"from":"a","to":"b","label":"three"}],
		"fragments":[{"type":"loop","over":["a","b"],"sections":[{"label":"each","start":1,"end":3}]},
		{"type":"opt","over":["a","b"],"sections":[{"label":"first only","start":1,"end":2}]}]}`, 0, 1, -1},
	{"an opt starting the outer's second section", `{"type":"sequence","actors":[{"id":"a","label":"A"},{"id":"b","label":"B"}],
		"interactions":[{"from":"a","to":"b","label":"one"},{"from":"b","to":"a","label":"two"},{"from":"a","to":"b","label":"three"},{"from":"b","to":"a","label":"four"}],
		"fragments":[{"type":"alt","over":["a","b"],"sections":[{"label":"ok","start":0,"end":1},{"label":"retry","start":2,"end":3}]},
		{"type":"opt","over":["a","b"],"sections":[{"label":"again","start":2,"end":3}]}]}`, 0, 1, 1},
}

// An outer frame contains each inner frame with FragmentPaddingX left and
// right, FragmentPaddingY below and its header row above; a divider of
// the outer above an inner frame starting there keeps the same room; and
// the outer's guards and tab stay off the inner frame.
func TestLayoutFragment_NestedFrameIsInsetInItsParent(t *testing.T) {
	for _, c := range nestedCases {
		for _, np := range screenAndText() {
			t.Run(c.name+"/"+np.name, func(t *testing.T) {
				p := np.p
				ps := layOut(t, c.spec, p)
				outer, inner := ps.Fragments[c.outer], ps.Fragments[c.inner]
				ob, ib := frameBox(outer), frameBox(inner)
				const eps = 1e-9
				assert.GreaterOrEqual(t, ib.x0-ob.x0, p.FragmentPaddingX-eps, "left inset: outer %+v inner %+v", ob, ib)
				assert.GreaterOrEqual(t, ob.x1-ib.x1, p.FragmentPaddingX-eps, "right inset: outer %+v inner %+v", ob, ib)
				assert.GreaterOrEqual(t, ob.y1-ib.y1, p.FragmentPaddingY-eps, "bottom inset: outer %+v inner %+v", ob, ib)
				assert.GreaterOrEqual(t, ib.y0-ob.y0, nestTop(p, outer)-eps, "top inset: outer %+v inner %+v", ob, ib)
				if c.divider >= 0 {
					d := outer.Sections[c.divider]
					assert.GreaterOrEqual(t, ib.y0-d.Y, nestTop(p, outer)-eps, "the divider at %g sits above the inner top %g", d.Y, ib.y0)
				}
				if np.name == "screen" {
					assert.False(t, tabBox(outer).intersects(ib), "the outer's tab meets the inner frame")
					for k, s := range outer.Sections {
						if s.Label != "" {
							assert.False(t, guardBox(s, p).intersects(ib), "the outer's guard %d meets the inner frame", k)
						}
					}
				}
			})
		}
	}
}

// Every frame starting on a message reserves its own header room: two
// frames opening on the same message sit as far below the previous one
// as a lone frame does.
func TestLayoutFragment_FramesOpeningTogetherEachGetHeaderRoom(t *testing.T) {
	c := nestedCases[2]
	for _, np := range screenAndText() {
		t.Run(np.name, func(t *testing.T) {
			p := np.p
			ps := layOut(t, c.spec, p)
			prev := ps.Interactions[0]
			lone := p.InteractionSpacingY + p.FragmentHeaderHeight - p.SectionTopInset
			assert.GreaterOrEqual(t, ps.Fragments[c.outer].Y-prev.Y, lone-1e-9,
				"the outer frame opens %g below the previous message, a lone frame %g", ps.Fragments[c.outer].Y-prev.Y, lone)
		})
	}
}

// The tab and every guard lie inside the frame and left of the leftmost
// lifeline the frame spans; the first guard sits right of the tab.
func TestLayoutFragment_HeaderSitsLeftOfTheLeftmostSpannedLifeline(t *testing.T) {
	p := seqlayout.ScreenProfile(fragFonts)
	for name, spec := range map[string]string{"repro": reproSpec, "header": headerSpec, "self": selfSpec, "nested": nestedCases[1].spec} {
		t.Run(name, func(t *testing.T) {
			ps := layOut(t, spec, p)
			for fi, f := range ps.Fragments {
				leftmost := f.X + f.Width
				for _, a := range ps.Actors {
					if a.LineX > f.X && a.LineX < f.X+f.Width {
						leftmost = min(leftmost, a.LineX)
					}
				}
				assert.Less(t, f.X+f.TabWidth, leftmost, "fragment %d: the tab ends left of the lifeline at %g", fi, leftmost)
				for k, s := range f.Sections {
					if s.Label == "" {
						continue
					}
					assert.Greater(t, s.LabelX, f.X, "fragment %d section %d: the guard starts inside the frame", fi, k)
					assert.Less(t, s.LabelX+s.LabelWidth, leftmost, "fragment %d section %d: the guard ends left of the lifeline at %g", fi, k, leftmost)
					if k == 0 {
						assert.GreaterOrEqual(t, s.LabelX, f.X+f.TabWidth, "fragment %d: the first guard sits right of the tab", fi)
					}
				}
			}
		})
	}
}

// showcaseSpec has frames whose span starts right of the first actor: a
// loop over web..db holding a par over api..db, an opt over web alone with
// a self-message, an alt over web and api.
const showcaseSpec = `{"type":"sequence","actors":[{"id":"user","label":"User"},{"id":"web","label":"Web App"},{"id":"api","label":"API"},{"id":"db","label":"Database"}],
"interactions":[{"from":"user","to":"web","label":"open catalog"},{"from":"web","to":"api","label":"GET /items"},{"from":"api","to":"db","label":"SELECT items"},
{"from":"db","to":"api","label":"rows","style":"dashed"},{"from":"api","to":"web","label":"200 items","style":"dashed"},{"from":"web","to":"web","label":"render list"},
{"from":"user","to":"web","label":"click buy"},{"from":"web","to":"api","label":"POST /orders"},{"from":"api","to":"web","label":"409 conflict"},{"from":"api","to":"web","label":"201 created"}],
"fragments":[{"type":"loop","over":["web","db"],"sections":[{"label":"each page","start":1,"end":4}]},
{"type":"par","over":["api","db"],"sections":[{"label":"query","start":2,"end":2},{"label":"result","start":3,"end":3}]},
{"type":"opt","over":["web"],"sections":[{"label":"items found","start":5,"end":5}]},
{"type":"alt","over":["web","api"],"sections":[{"label":"out of stock","start":8,"end":8},{"label":"in stock","start":9,"end":9}]}]}`

// On screen a frame's left side, out in the gutter its header needs, and
// its right side, out past a self-message's label, keep FragmentPaddingX
// and half an activation bar clear of the lifelines beside its span: the
// actors move apart to make room.
func TestLayoutFragment_FrameClearsTheLifelinesBesideItsSpan(t *testing.T) {
	p := seqlayout.ScreenProfile(fragFonts)
	clearance := p.FragmentPaddingX + p.ActivationWidth/2
	for name, spec := range map[string]string{"showcase": showcaseSpec, "self": selfSpec, "nested": nestedCases[0].spec, "header": headerSpec} {
		t.Run(name, func(t *testing.T) {
			d, err := schema.ParseSequence([]byte(spec))
			require.NoError(t, err)
			ps := layOut(t, spec, p)
			for fi, f := range ps.Fragments {
				// The actors the frame spans: over and every actor its
				// messages touch (no case here nests a wider frame).
				leftmost, rightmost := len(d.Actors), -1
				spanned := func(id string) {
					for i, a := range d.Actors {
						if a.ID == id {
							leftmost, rightmost = min(leftmost, i), max(rightmost, i)
						}
					}
				}
				fr := d.Fragments[fi]
				for _, id := range fr.Over {
					spanned(id)
				}
				for i := fr.Sections[0].Start; i <= fr.Sections[len(fr.Sections)-1].End; i++ {
					spanned(d.Interactions[i].From)
					spanned(d.Interactions[i].To)
				}
				for _, a := range ps.Actors[:leftmost] {
					assert.GreaterOrEqual(t, f.X-a.LineX, clearance-1e-9,
						"fragment %d (%s): its left side at %g is %g right of the %s lifeline", fi, f.Type, f.X, f.X-a.LineX, a.ID)
				}
				for _, a := range ps.Actors[rightmost+1:] {
					assert.GreaterOrEqual(t, a.LineX-(f.X+f.Width), clearance-1e-9,
						"fragment %d (%s): its right side at %g is %g left of the %s lifeline", fi, f.Type, f.X+f.Width, a.LineX-(f.X+f.Width), a.ID)
				}
			}
		})
	}
}

// Three frames closing together on a message, a self-message or not, keep
// the next message and its label below the outermost frame's bottom, and
// a frame opening on that next message below it too.
func TestLayoutFragment_FramesClosingTogetherClearTheNextMessage(t *testing.T) {
	for _, self := range []bool{false, true} {
		to := "b"
		if self {
			to = "a"
		}
		spec := `{"type":"sequence","actors":[{"id":"a","label":"A"},{"id":"b","label":"B"}],
			"interactions":[{"from":"a","to":"b","label":"one"},{"from":"a","to":"` + to + `","label":"two"},{"from":"b","to":"a","label":"the next message"},{"from":"a","to":"b","label":"four"}],
			"fragments":[{"type":"loop","over":["a","b"],"sections":[{"label":"each","start":0,"end":1}]},
			{"type":"opt","over":["a","b"],"sections":[{"label":"maybe","start":1,"end":1}]},
			{"type":"break","over":["a","b"],"sections":[{"label":"stop","start":1,"end":1}]},
			{"type":"opt","over":["a","b"],"sections":[{"label":"after","start":3,"end":3}]}]}`
		for _, np := range screenAndText() {
			t.Run(fmt.Sprintf("self=%v/%s", self, np.name), func(t *testing.T) {
				ps := layOut(t, spec, np.p)
				outer := ps.Fragments[0]
				bottom := outer.Y + outer.Height
				next := ps.Interactions[2]
				lb, _ := messageLabelBox(next, np.p)
				assert.Less(t, bottom, lb.y0, "the outer frame closes at %g, above the next label at %g", bottom, lb.y0)
				assert.Less(t, bottom, ps.Fragments[3].Y, "the outer frame closes above the next frame")
			})
		}
	}
}

// A chain of coincident frames costs a pass, not a call per nesting path:
// 40 opt frames over the same two actors and message lay out at once, each
// inside the one declared before it.
func TestLayoutFragment_CoincidentFramesLayOutQuickly(t *testing.T) {
	const n = 40
	frags := make([]string, n)
	for i := range frags {
		frags[i] = `{"type":"opt","over":["a","b"],"sections":[{"label":"f","start":0,"end":0}]}`
	}
	spec := `{"type":"sequence","actors":[{"id":"a","label":"A"},{"id":"b","label":"B"},{"id":"c","label":"C"}],
		"interactions":[{"from":"a","to":"b","label":"one"},{"from":"b","to":"c","label":"two"}],
		"fragments":[` + strings.Join(frags, ",") + `]}`
	for _, np := range screenAndText() {
		t.Run(np.name, func(t *testing.T) {
			start := time.Now()
			ps := layOut(t, spec, np.p)
			assert.Less(t, time.Since(start), 2*time.Second)
			require.Len(t, ps.Fragments, n)
			for i := 1; i < n; i++ {
				o, in := ps.Fragments[i-1], ps.Fragments[i]
				assert.Less(t, o.X, in.X, "fragment %d lies inside fragment %d (left)", i, i-1)
				assert.Greater(t, o.X+o.Width, in.X+in.Width, "fragment %d lies inside fragment %d (right)", i, i-1)
				assert.Less(t, o.Y, in.Y, "fragment %d lies inside fragment %d (top)", i, i-1)
				assert.Greater(t, o.Y+o.Height, in.Y+in.Height, "fragment %d lies inside fragment %d (bottom)", i, i-1)
			}
		})
	}
}
