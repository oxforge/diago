package svg

import (
	"strings"
	"testing"

	"github.com/oxforge/diago/internal/model"
	"github.com/oxforge/diago/internal/theme"
	"github.com/stretchr/testify/assert"
)

func diffSequence() *model.PositionedSequence {
	return &model.PositionedSequence{
		Width: 400, Height: 300,
		Actors: []model.PositionedActor{
			{ID: "u", Label: "User", BoxX: 20, BoxY: 20, BoxW: 80, BoxH: 40, LineX: 60, LineTop: 60, LineBottom: 280},
			{ID: "w", Label: "Web", BoxX: 180, BoxY: 20, BoxW: 80, BoxH: 40, LineX: 220, LineTop: 60, LineBottom: 280},
		},
		Interactions: []model.PositionedInteraction{
			{From: "u", To: "w", Label: "login", Y: 100, FromX: 60, ToX: 220},
			{From: "w", To: "u", Label: "audit", Style: model.InteractionAsync, Y: 140, FromX: 220, ToX: 60, Color: "red"},
			{From: "u", To: "w", Label: "logout", Y: 200, FromX: 60, ToX: 220},
		},
		Activations: []model.PositionedActivation{{ActorID: "w", X: 215, Y: 100, Width: 10, Height: 40}},
		Fragments: []model.PositionedFragment{{
			Type: model.FragmentAlt, X: 40, Y: 80, Width: 240, Height: 150,
			Sections: []model.PositionedFragmentSection{{Label: "ok", Y: 80}, {Label: "locked", Y: 180}},
		}},
	}
}

func seqStatusOpts(th theme.Theme, classes map[string]string) *RenderOptions {
	return &RenderOptions{
		Class:       func(kind, id string) string { return classes[kind+":"+id] },
		Styles:      SequenceDiffStyles(th),
		MarkerFills: map[string]string{"added": th.Diff.Added, "changed": th.Diff.Changed, "removed": th.Diff.Removed},
		StatusPaint: true,
	}
}

func TestRenderSequenceWithOptions_NilIsRenderSequence(t *testing.T) {
	th := theme.DefaultTheme()
	s := diffSequence()
	assert.Equal(t, RenderSequence(s, th), RenderSequenceWithOptions(s, th, nil))
	assert.NotContains(t, RenderSequence(s, th), "class=")
	assert.NotContains(t, RenderSequence(s, th), `id="lifeline-`)
}

func TestRenderSequenceWithOptions_ClassesAndGroups(t *testing.T) {
	th := theme.DefaultTheme()
	out := RenderSequenceWithOptions(diffSequence(), th, seqStatusOpts(th, map[string]string{
		"actor:u": "dimmed", "lifeline:u": "dimmed", "actor:w": "added", "lifeline:w": "added",
		"interaction:0": "changed", "interaction:1": "added", "interaction:2": "dimmed",
		"fragment:0": "changed", "section:0/1": "added",
	}))

	assert.Contains(t, out, `<g id="actor-w" class="added">`)
	assert.Contains(t, out, `<g id="lifeline-w" class="added">`)
	assert.Contains(t, out, `<g id="interaction-0" class="changed">`)
	assert.Contains(t, out, `<g id="fragment-0" class="changed">`)
	assert.Contains(t, out, `<g id="fragment-0-section-0">`, "a same section carries no class")
	assert.Contains(t, out, `<g id="fragment-0-section-1" class="added">`)
	assert.Contains(t, out, ".added line { stroke: "+th.Diff.Added)
	assert.Contains(t, out, ".dimmed { opacity: 0.5; }")
	// The activation of w sits inside w's lifeline group.
	lifeline := out[strings.Index(out, `id="lifeline-w"`):]
	lifeline = lifeline[:strings.Index(lifeline, "</g>")]
	assert.Contains(t, lifeline, `width="10"`, "activation rect inside the lifeline group")
}

func TestRenderSequenceWithOptions_StatusMarkers(t *testing.T) {
	th := theme.DefaultTheme()
	out := RenderSequenceWithOptions(diffSequence(), th, seqStatusOpts(th, map[string]string{
		"interaction:0": "changed", "interaction:1": "added", "interaction:2": "dimmed",
	}))

	assert.Contains(t, out, `id="diago-arrow-changed"`)
	assert.Contains(t, out, `id="diago-arrow-open-added"`)
	assert.Contains(t, out, `marker-end="url(#diago-arrow-changed)"`)
	assert.Contains(t, out, `marker-end="url(#diago-arrow-open-added)"`, "async added edge uses the open status marker despite its user color")
	assert.Contains(t, out, `marker-end="url(#diago-arrow)"`, "dimmed edge keeps the base marker")
	assert.Less(t, strings.Index(out, `id="diago-arrow-added"`), strings.Index(out, `id="diago-arrow-changed"`), "sorted emission")
}

func TestSequenceDiffStyles_ExtendsFlowStyles(t *testing.T) {
	th := theme.DefaultTheme()
	s := SequenceDiffStyles(th)
	assert.True(t, strings.HasPrefix(s, DiffStyles(th)))
	assert.Contains(t, s, ".removed line { stroke: "+th.Diff.Removed+"; stroke-dasharray: 6 4; }")
}

func TestSequenceDiffStyles_SectionStatusOutranksItsFragment(t *testing.T) {
	th := theme.DefaultTheme()
	s := SequenceDiffStyles(th)
	// A section's group sits inside its fragment's: .changed line and
	// .added line tie on specificity, so the later one would win.
	for _, rule := range []string{
		".changed .added line { stroke: " + th.Diff.Added + "; }",
		".changed .added text { fill: " + th.Diff.Added + "; }",
		".changed .removed line { stroke: " + th.Diff.Removed + "; }",
		".changed .removed text { fill: " + th.Diff.Removed + "; }",
	} {
		assert.Contains(t, s, rule)
	}
	assert.Less(t, strings.Index(s, ".changed line {"), strings.Index(s, ".changed .added line {"))
}

func TestRenderSequenceWithOptions_LabelBackingTakesNoStatusStroke(t *testing.T) {
	th := theme.DefaultTheme()
	out := RenderSequenceWithOptions(diffSequence(), th, seqStatusOpts(th, map[string]string{
		"interaction:0": "changed", "interaction:1": "added",
	}))
	for _, id := range []string{"interaction-0", "interaction-1"} {
		grp := out[strings.Index(out, `<g id="`+id+`"`):]
		grp = grp[:strings.Index(grp, "</g>")]
		assert.Contains(t, grp, `<rect class="backing" `, "%s: the label's backing is excluded from the status stroke", id)
	}
	assert.Contains(t, out, ".added rect.backing, .changed rect.backing, .removed rect.backing { stroke: none; }")
	assert.NotContains(t, RenderSequence(diffSequence(), th), `class="backing"`, "a plain render stays byte-identical")
}

func TestRenderSequenceWithOptions_StatusPaint(t *testing.T) {
	th := theme.DefaultTheme()
	s := diffSequence()
	s.Actors[0].Color = "red" // u, unchanged
	dc := theme.DeriveColors(th.Colors["red"], th.Background, theme.ElementNode)
	out := RenderSequenceWithOptions(s, th, seqStatusOpts(th, map[string]string{
		"actor:u": "dimmed", "lifeline:u": "dimmed", "interaction:1": "dimmed",
	}))
	actor := groupOf(t, out, "actor-u")
	assert.Contains(t, actor, `fill="`+dc.Fill+`"`, "the actor keeps its own fill")
	assert.Contains(t, actor, `stroke="`+th.Actor.Stroke+`"`)
	assert.NotContains(t, actor, dc.Text)
	assert.Contains(t, groupOf(t, out, "lifeline-u"), `stroke="`+th.Actor.Stroke+`"`, "the lifeline drops the actor's color")
	it := groupOf(t, out, "interaction-1")
	assert.Contains(t, it, `stroke="`+th.Edge.Stroke+`"`, "the red interaction drops its color")
	assert.Contains(t, it, `marker-end="url(#diago-arrow-open)"`)

	// Control: the plain render keeps the actor's outline.
	assert.Contains(t, RenderSequence(s, th), `stroke="`+dc.Stroke+`"`)
}
