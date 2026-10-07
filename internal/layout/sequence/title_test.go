package sequence

import (
	"testing"

	"github.com/oxforge/diago/internal/font"
	"github.com/oxforge/diago/internal/model"
	"github.com/stretchr/testify/assert"
)

func TestLayout_CopiesTitle(t *testing.T) {
	d := model.SequenceDiagram{
		Title:        "Login",
		Actors:       []model.Actor{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}},
		Interactions: []model.Interaction{{From: "a", To: "b", Label: "hi"}},
		Activations:  true,
	}
	ps := Layout(d, font.MeasureText, LayoutFonts{Family: "Inter", LabelSize: 12})
	assert.Equal(t, "Login", ps.Title)
}
