package text

import (
	"strings"
	"testing"

	"github.com/oxforge/diago/internal/model"
)

// twoActorSeq: A at col 3 (box cols 0-6), B at col 15 (box cols 12-18);
// headers rows 1-3, lifelines from row 4; one message A->B on row 6.
func twoActorSeq() *model.PositionedSequence {
	return &model.PositionedSequence{
		Actors: []model.PositionedActor{
			{ID: "a", Label: "A", BoxX: 0, BoxY: 16, BoxW: 56, BoxH: 48, LineX: 28, LineTop: 64, LineBottom: 160},
			{ID: "b", Label: "B", BoxX: 96, BoxY: 16, BoxW: 56, BoxH: 48, LineX: 124, LineTop: 64, LineBottom: 160},
		},
		Interactions: []model.PositionedInteraction{
			{From: "a", To: "b", Label: "hi", Style: model.InteractionSolid, Y: 96, FromX: 28, ToX: 124},
		},
		Width: 176, Height: 240,
	}
}

func TestRenderSequence_SharedColumnsAndHead(t *testing.T) {
	r, err := RenderSequence(twoActorSeq(), true)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(r.Art, "\n"), "\n")
	// message row is row 6 in grid terms but String() drops the blank row 0: index 5
	row := lines[5]
	rs := []rune(row)
	if rs[15] != '►' {
		t.Fatalf("head at destination column 15, got %q\n%s", rs[15], r.Art)
	}
	if !strings.Contains(row, " hi ") {
		t.Fatalf("label on the arrow row:\n%s", r.Art)
	}
	if rs[3] != '├' {
		t.Fatalf("source junction should be ├ (lifeline N|S + arrow E), got %q\n%s", rs[3], r.Art)
	}
}

func TestRenderSequence_AsyncOpenHeadAndDashedReturn(t *testing.T) {
	ps := twoActorSeq()
	ps.Interactions[0].Style = model.InteractionAsync
	r, _ := RenderSequence(ps, true)
	if !strings.Contains(r.Art, "▷") || strings.Contains(r.Art, "►") {
		t.Fatalf("async wants an open head:\n%s", r.Art)
	}
	ps.Interactions[0].Style = model.InteractionDashed
	r, _ = RenderSequence(ps, true)
	if !strings.Contains(r.Art, "╌") {
		t.Fatalf("dashed wants ╌ runs:\n%s", r.Art)
	}
}

func TestRenderSequence_FramePiercesLifelines(t *testing.T) {
	ps := twoActorSeq()
	ps.Fragments = []model.PositionedFragment{{
		Type: model.FragmentAlt, X: 0, Y: 80, Width: 176, Height: 64,
		Sections: []model.PositionedFragmentSection{{Label: "ok", Y: 80}, {Label: "else", Y: 112}},
	}}
	r, _ := RenderSequence(ps, true)
	lines := strings.Split(strings.TrimRight(r.Art, "\n"), "\n")
	top := []rune(lines[4]) // frame top row (grid row 5)
	if top[0] != '╔' || !strings.Contains(lines[4], " alt ") {
		t.Fatalf("frame top with tab:\n%s", r.Art)
	}
	if top[15] != '╪' {
		t.Fatalf("lifeline crossing the frame top must be ╪, got %q\n%s", top[15], r.Art)
	}
	div := lines[6] // divider row (grid row 7)
	if !strings.HasPrefix(div, "╟") || !strings.Contains(div, "[else]") || !strings.HasSuffix(strings.TrimRight(div, " "), "╢") {
		t.Fatalf("divider row:\n%s", r.Art)
	}
}

func TestRenderSequence_ActivationsToggle(t *testing.T) {
	ps := twoActorSeq()
	ps.Activations = []model.PositionedActivation{{ActorID: "b", X: 120, Y: 96, Width: 8, Height: 32}}
	r, _ := RenderSequence(ps, true)
	if !strings.Contains(r.Art, "┃") {
		t.Fatalf("activation bar expected:\n%s", r.Art)
	}
	r, _ = RenderSequence(ps, false)
	if strings.Contains(r.Art, "┃") {
		t.Fatalf("activations hidden:\n%s", r.Art)
	}
}

func TestRenderSequence_SelfMessageLoop(t *testing.T) {
	ps := twoActorSeq()
	ps.Interactions = []model.PositionedInteraction{{From: "a", To: "a", Label: "tick", Style: model.InteractionSolid, IsSelf: true, Y: 96,
		FromX: 28, ToX: 28, SelfPoints: []model.Point{{X: 28, Y: 96}, {X: 60, Y: 96}, {X: 60, Y: 128}, {X: 28, Y: 128}}}}
	r, _ := RenderSequence(ps, true)
	lines := strings.Split(strings.TrimRight(r.Art, "\n"), "\n")
	if !strings.Contains(lines[5], "┐") || !strings.Contains(lines[7], "◄") || !strings.Contains(lines[7], "┘") {
		t.Fatalf("self loop shape:\n%s", r.Art)
	}
	if !strings.Contains(r.Art, "tick") {
		t.Fatalf("self label:\n%s", r.Art)
	}
}

func TestRenderSequence_HeadersTopAndBottomMatch(t *testing.T) {
	r, _ := RenderSequence(twoActorSeq(), true)
	lines := strings.Split(strings.TrimRight(r.Art, "\n"), "\n")
	if lines[0] != lines[len(lines)-3] || lines[1] != lines[len(lines)-2] {
		t.Fatalf("footer must repeat the header:\n%s", r.Art)
	}
}
