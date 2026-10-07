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
		MarkerFills: map[string]string{"added": th.Diff.Added, "changed": th.Diff.Changed},
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
	assert.Contains(t, s, ".removed line { stroke-dasharray: 6 4; }")
}
