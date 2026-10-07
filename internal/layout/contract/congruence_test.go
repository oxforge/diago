package contract

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/oxforge/diago/internal/model"
)

// siblings is a congruence of two members: s fans out to m1 and m2 (edges 0
// and 1, Z templates with spread runs at y=100), and each member has one
// internal edge, m1->n1 (edge 2, the representative) and m2->n2 (edge 3),
// 140 px apart.
func siblings() *model.PositionedGraph {
	pg := layout(400, 400,
		[]model.PositionedNode{rect("s", 200, 40), rect("m1", 100, 180), rect("m2", 240, 180), rect("n1", 100, 300), rect("n2", 240, 300)},
		wire("s", "m1", 190, 60, 190, 100, 100, 100, 100, 160),
		wire("s", "m2", 210, 60, 210, 100, 240, 100, 240, 160),
		wire("m1", "n1", 100, 200, 100, 280),
		wire("m2", "n2", 240, 200, 240, 280),
	)
	pg.Edges[2].LabelPos = &model.Point{X: 118, Y: 240}
	pg.Edges[3].LabelPos = &model.Point{X: 258, Y: 240}
	pg.AppliedCongruences = []model.AppliedCongruence{{
		RepEdges: []int{2},
		Members:  []model.AppliedCongruenceMember{{Edges: []int{3}}},
		FanOut:   []int{0, 1},
	}}
	return pg
}

func c17(pg *model.PositionedGraph) []string { return congruenceRules(check(pg)) }

// congruenceRules keeps the C17 rules of vs.
func congruenceRules(vs []Violation) []string {
	var out []string
	for _, v := range vs {
		if v.Rule == "C17.1" || v.Rule == "C17.2" || v.Rule == "C17.3" {
			out = append(out, v.Rule)
		}
	}
	return out
}

func TestC17_CleanCongruence(t *testing.T) {
	assert.Empty(t, c17(siblings()))
}

func TestC17_1_Translation(t *testing.T) {
	moved := siblings()
	moved.Edges[3].Points[1].X = 240.5
	assert.Equal(t, []string{"C17.1"}, c17(moved), "a point off the translation")

	extra := siblings()
	extra.Edges[3].Points = append(extra.Edges[3].Points[:1], model.Point{X: 240, Y: 240}, extra.Edges[3].Points[1])
	assert.Equal(t, []string{"C17.1"}, c17(extra), "a different point count")

	label := siblings()
	label.Edges[3].LabelPos.X = 259
	assert.Equal(t, []string{"C17.1"}, c17(label), "a label off the translation")

	unlabeled := siblings()
	unlabeled.Edges[3].LabelPos = nil
	assert.Equal(t, []string{"C17.1"}, c17(unlabeled), "a label on the representative only")

	labeled := siblings()
	labeled.Edges[2].LabelPos = nil
	assert.Equal(t, []string{"C17.1"}, c17(labeled), "a label on the member only")

	short := siblings()
	short.AppliedCongruences[0].Members[0].Edges = nil
	assert.Equal(t, []string{"C17.1"}, c17(short), "a member missing an edge")
}

func TestC17_1_TextLabelsMoveWithinHalfACell(t *testing.T) {
	// a label box lies on whole cells in text (C14.5), so a member label
	// an odd number of cells wider than its representative's centers
	// half a cell off the translation: 4 px across a column, 8 along a row
	text := Options{Limits: TextLimits(model.Down), Direction: model.Down, Text: true}
	for _, tc := range []struct {
		dx, dy float64
		want   bool
	}{
		{4, 0, false}, {-4, 0, false}, {4.5, 0, true}, {0, 8, false}, {0, -8, false}, {0, 8.5, true},
	} {
		pg := siblings()
		pg.Edges[3].LabelPos.X += tc.dx
		pg.Edges[3].LabelPos.Y += tc.dy
		assert.Equal(t, tc.want, slices.Contains(congruenceRules(Check(pg, text)), "C17.1"), "in text, off by (%g, %g)", tc.dx, tc.dy)
		assert.Equal(t, []string{"C17.1"}, c17(pg), "on screen, off by (%g, %g)", tc.dx, tc.dy)
	}
}

func TestC17_2_Slots(t *testing.T) {
	pg := siblings()
	pg.Edges[1].Points[2].X, pg.Edges[1].Points[3].X = 250, 250 // enters m2 10 px right of center
	assert.Equal(t, []string{"C17.2"}, c17(pg))

	uneven := siblings()
	uneven.AppliedCongruences[0].FanOut = []int{0, 1, 1}
	assert.Contains(t, c17(uneven), "C17.2", "three fan-out edges for two members")
}

// twoPerMember is siblings with two fan-out edges into each member, 20 px
// either side of its center: the fan-out runs [0 4] into m1 and [5 1] into
// m2, the outer pair on track 100 and the inner one on track 108.
func twoPerMember() *model.PositionedGraph {
	pg := siblings()
	pg.Edges[0] = wire("s", "m1", 185, 60, 185, 100, 80, 100, 80, 160)
	pg.Edges[1] = wire("s", "m2", 215, 60, 215, 100, 260, 100, 260, 160)
	pg.Edges = append(pg.Edges,
		wire("s", "m1", 195, 60, 195, 108, 120, 108, 120, 160),
		wire("s", "m2", 205, 60, 205, 108, 220, 108, 220, 160))
	pg.AppliedCongruences[0].FanOut = []int{0, 4, 5, 1}
	return pg
}

func TestC17_2_RunsPerMember(t *testing.T) {
	assert.Empty(t, c17(twoPerMember()), "the k-th edge of each run enters at one offset")

	off := twoPerMember()
	off.Edges[1].Points[2].X, off.Edges[1].Points[3].X = 262, 262
	assert.Equal(t, []string{"C17.2"}, c17(off), "the second run's second edge 2 px off")
}

func TestC17_3_FanOut(t *testing.T) {
	levels := siblings()
	levels.Edges[1].Points[1].Y, levels.Edges[1].Points[2].Y = 108, 108
	assert.Equal(t, []string{"C17.3"}, c17(levels), "a pair on different tracks")

	// A diamond's side exits: the first run across the flow axis lies
	// inside the source's box, the spread run below it.
	sides := siblings()
	sides.Edges[0] = wire("s", "m1", 160, 40, 150, 40, 150, 100, 100, 100, 100, 160)
	sides.Edges[1] = wire("s", "m2", 240, 40, 250, 40, 250, 100, 240, 100, 240, 160)
	assert.Empty(t, c17(sides), "side exits spreading on one track")
	sides.Edges[1] = wire("s", "m2", 240, 40, 250, 40, 250, 108, 240, 108, 240, 160)
	assert.Equal(t, []string{"C17.3"}, c17(sides), "side exits spreading on two tracks")
	assert.Empty(t, congruenceRules(Check(sides, Options{Limits: ScreenLimits(), Direction: model.Auto})), "no direction, no tracks")

	crossed := siblings()
	crossed.Edges[0] = wire("s", "m1", 190, 60, 190, 110, 100, 110, 100, 160)
	crossed.Edges[1] = wire("s", "m2", 210, 60, 210, 100, 180, 100, 180, 130, 240, 130, 240, 160)
	vs := c17(crossed)
	assert.Contains(t, vs, "C17.3", "the fan crosses itself")
}
