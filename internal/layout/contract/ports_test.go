package contract

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/oxforge/diago/internal/model"
)

// fan is a DOWN scene: a feeds decision d (a diamond or a circle, 80 x 46 or
// 46 x 46 at (100,143): vertices top (100,120), bottom (100,166), left and
// right at y=143); b and c sit below it.
func fan(shape model.Shape, edges ...model.PositionedEdge) *model.PositionedGraph {
	w := 80.0
	if shape == model.ShapeCircle {
		w = 46
	}
	return layout(240, 300, []model.PositionedNode{
		rect("a", 100, 40),
		shaped("d", shape, 100, 143, w, 46),
		rect("b", 60, 260),
		rect("c", 180, 260),
	}, edges...)
}

var (
	aToTop     = wire("a", "d", 100, 60, 100, 120)
	dLeftToB   = wire("d", "b", 60, 143, 40, 143, 40, 240)
	dRightToC  = wire("d", "c", 140, 143, 180, 143, 180, 240)
	dBottomToB = wire("d", "b", 100, 166, 100, 200, 60, 200, 60, 240)
	dBottomToC = wire("d", "c", 100, 166, 100, 200, 180, 200, 180, 240)
)

func TestC8_CleanDecision(t *testing.T) {
	vs := check(fan(model.ShapeDiamond, aToTop, dLeftToB, dRightToC))
	for _, v := range vs {
		assert.NotContains(t, v.Rule, "C8", "%v", v)
	}
}

func TestC8_1_VerticesOnly(t *testing.T) {
	midEdge := wire("d", "b", 80, 131.5, 80, 100, 40, 100, 40, 240)
	assert.True(t, has(check(fan(model.ShapeDiamond, aToTop, midEdge)), "C8.1", "d->b#0"))
}

func TestC8_2_ForwardEdgesEnterTheInVertex(t *testing.T) {
	intoLeft := wire("a", "d", 100, 60, 100, 80, 40, 80, 40, 143, 60, 143)
	assert.True(t, has(check(fan(model.ShapeDiamond, intoLeft)), "C8.2", "a->d#0"))
	vs := Check(fan(model.ShapeDiamond, intoLeft), Options{Limits: ScreenLimits(), Direction: model.Auto})
	assert.False(t, has(vs, "C8.2", "a->d#0"), "Auto skips the direction-dependent clauses")
}

func TestC8_3_NothingLeavesTheInVertex(t *testing.T) {
	outOfTop := wire("d", "b", 100, 120, 100, 100, 10, 100, 10, 260, 20, 260)
	assert.True(t, has(check(fan(model.ShapeDiamond, outOfTop)), "C8.3", "d->b#0"))
}

func TestC8_4_BackEdgesUseCrossAxisVertices(t *testing.T) {
	intoBottom := wire("b", "d", 60, 240, 60, 200, 100, 200, 100, 166)
	intoRight := wire("b", "d", 60, 240, 60, 200, 160, 200, 160, 143, 140, 143)
	assert.True(t, has(check(fan(model.ShapeDiamond, intoBottom)), "C8.4", "b->d#0"))
	assert.False(t, has(check(fan(model.ShapeDiamond, intoRight)), "C8.4", "b->d#0"))
}

func TestC8_5_OwnVertices(t *testing.T) {
	t.Run("diamond: two edges share the bottom vertex", func(t *testing.T) {
		assert.True(t, has(check(fan(model.ShapeDiamond, dBottomToB, dBottomToC)), "C8.5", "d->c#0"))
	})
	t.Run("circle: a fan-out toward one side may merge", func(t *testing.T) {
		assert.False(t, has(check(fan(model.ShapeCircle, dBottomToB, dBottomToC)), "C8.5", "d->c#0"))
	})
	t.Run("circle: a partial merge is not a fan-out", func(t *testing.T) {
		rightToC := wire("d", "c", 123, 143, 180, 143, 180, 240)
		rightToC.ID = "d->c#1"
		assert.True(t, has(check(fan(model.ShapeCircle, dBottomToB, dBottomToC, rightToC)), "C8.5", "d->c#0"))
	})
	t.Run("circle: a back edge leaving beside a merged fan-out", func(t *testing.T) {
		toA := wire("d", "a", 123, 143, 200, 143, 200, 40, 140, 40)
		assert.False(t, has(check(fan(model.ShapeCircle, dBottomToB, dBottomToC, toA)), "C8.5", "d->c#0"))
	})
	t.Run("more than three edges may share", func(t *testing.T) {
		e3, e4 := dBottomToB, dBottomToC
		e3.ID, e4.ID = "d->b#1", "d->c#1"
		vs := check(fan(model.ShapeDiamond, dBottomToB, dBottomToC, e3, e4))
		for _, v := range vs {
			assert.NotEqual(t, "C8.5", v.Rule, "%v", v)
		}
	})
}

// TestC8_5_CountsOnlyForwardEdges pins C8.5's count: only the outgoing
// forward edges must leave from their own vertices, and a back edge that
// leaves the node keeps to C8.4 alone, sharing a side vertex with another
// back edge or a forward edge, and counting toward neither three nor four.
func TestC8_5_CountsOnlyForwardEdges(t *testing.T) {
	toA := func(id string, xy ...float64) model.PositionedEdge {
		e := wire("d", "a", xy...)
		e.ID = id
		return e
	}
	dLeftToA := toA("d->a#0", 60, 143, 40, 143, 40, 40, 60, 40)
	dRightToA := toA("d->a#0", 140, 143, 160, 143, 160, 40, 140, 40)
	dRightToA1 := toA("d->a#1", 140, 143, 160, 143, 160, 40, 140, 40)
	dRightToA2 := toA("d->a#2", 140, 143, 160, 143, 160, 40, 140, 40)
	dRightToC1 := dRightToC
	dRightToC1.ID = "d->c#1"
	tests := []struct {
		name  string
		edges []model.PositionedEdge
		want  []string // the edges C8.5 reports
	}{
		{"two back edges share a side vertex beside a forward exit", []model.PositionedEdge{dBottomToB, dRightToA, dRightToA1}, nil},
		{"three back edges leave", []model.PositionedEdge{dLeftToA, dRightToA1, dRightToA2}, nil},
		{"a back edge shares a side vertex with a forward exit", []model.PositionedEdge{dBottomToB, dRightToC, dRightToA}, nil},
		{"three forward edges beside a back edge are no four", []model.PositionedEdge{dBottomToB, dBottomToC, dRightToC1, dLeftToA}, []string{"d->c#0"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, v := range check(fan(model.ShapeDiamond, append([]model.PositionedEdge{aToTop}, tt.edges...)...)) {
				assert.NotEqual(t, "C8.4", v.Rule, "the back edges keep to C8.4: %v", v)
				if v.Rule == "C8.5" {
					got = append(got, v.Subject)
				}
			}
			assert.Equal(t, tt.want, got)
		})
	}
	t.Run("Auto knows no forward edge", func(t *testing.T) {
		vs := Check(fan(model.ShapeDiamond, dBottomToB, dBottomToC), Options{Limits: ScreenLimits(), Direction: model.Auto})
		assert.False(t, has(vs, "C8.5", "d->c#0"))
	})
}

func TestEdgeKind(t *testing.T) {
	upper, lower := rect("u", 60, 40), rect("l", 60, 140)
	leftN, rightN := rect("l", 60, 40), rect("r", 180, 40)
	tests := []struct {
		dir      model.Direction
		from, to model.PositionedNode
		want     kind
	}{
		{model.Down, upper, lower, forward},
		{model.Down, lower, upper, back},
		{model.Up, lower, upper, forward},
		{model.Up, upper, lower, back},
		{model.Right, leftN, rightN, forward},
		{model.Right, rightN, leftN, back},
		{model.Left, rightN, leftN, forward},
		{model.Left, leftN, rightN, back},
		{model.Down, leftN, rightN, lateral},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, edgeKind(tt.from, tt.to, tt.dir), "%v %s->%s", tt.dir, tt.from.ID, tt.to.ID)
	}
	assert.Equal(t, top, inVertex(model.Down))
	assert.Equal(t, bottom, inVertex(model.Up))
	assert.Equal(t, left, inVertex(model.Right))
	assert.Equal(t, right, inVertex(model.Left))
}

func TestCheck_RuleOrder(t *testing.T) {
	intoLeft := wire("a", "d", 100, 60, 100, 80, 40, 80, 40, 143, 60, 143)
	intoLeft.ID = "a->d#early"
	midEdge := wire("d", "b", 80, 131.5, 80, 100, 40, 100, 40, 240)
	midEdge.ID = "d->b#late"
	vs := check(fan(model.ShapeDiamond, intoLeft, midEdge))
	var ruleIds []string
	for _, v := range vs {
		if v.Rule == "C8.1" || v.Rule == "C8.2" {
			ruleIds = append(ruleIds, v.Rule)
		}
	}
	assert.True(t, len(ruleIds) >= 2, "expected at least 2 C8 violations, got %d", len(ruleIds))
	for i := 1; i < len(ruleIds); i++ {
		assert.LessOrEqual(t, ruleIds[i-1], ruleIds[i], "rule order violated: %s should not come after %s", ruleIds[i-1], ruleIds[i])
	}

	// A scene mixing C2.2, a C8 rule and C11 exercises an id ("C10", "C11")
	// that sorts before the C2.x family as a string, so a lexical order
	// check would pass by accident; sorted() must compare RuleIDs index.
	pg := fan(model.ShapeDiamond, intoLeft, midEdge)
	pg.Edges = append(pg.Edges, wire("a", "z", 100, 60, 100, 90)) // z is unknown: C2.2
	pg.Nodes[3].X = pg.Nodes[2].X                                 // c now overlaps b: C11
	vs2 := check(pg)
	rank := make(map[string]int, len(RuleIDs))
	for i, id := range RuleIDs {
		rank[id] = i
	}
	seen := map[string]bool{}
	for _, v := range vs2 {
		seen[v.Rule] = true
	}
	assert.True(t, seen["C2.2"], "expected a C2.2 violation, got %v", vs2)
	assert.True(t, seen["C8.1"] || seen["C8.2"], "expected a C8 violation, got %v", vs2)
	assert.True(t, seen["C11"], "expected a C11 violation, got %v", vs2)
	for i := 1; i < len(vs2); i++ {
		assert.LessOrEqual(t, rank[vs2[i-1].Rule], rank[vs2[i].Rule],
			"rule order violated: %s should not come after %s", vs2[i-1].Rule, vs2[i].Rule)
	}
}
