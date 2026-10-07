package contract

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/oxforge/diago/internal/model"
)

func TestC11_NodeSeparation(t *testing.T) {
	tests := []struct {
		name   string
		bx, by float64
		want   bool
	}{
		{"25 px apart", 60, 105, false},
		{"20 px apart", 60, 100, false},
		{"10 px apart", 60, 90, true},
		{"overlapping", 80, 50, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pg := layout(300, 300, []model.PositionedNode{rect("a", 60, 40), rect("b", tt.bx, tt.by)})
			assert.Equal(t, tt.want, has(check(pg), "C11", "a"))
		})
	}
}

func TestC12_GroupContainment(t *testing.T) {
	grp := func(id string, x, y, w, h float64, contains []string, children ...string) model.PositionedGroup {
		return model.PositionedGroup{ID: id, X: x, Y: y, Width: w, Height: h, Contains: contains, Children: children, Depth: 1}
	}
	scene := func(groups ...model.PositionedGroup) []Violation {
		pg := layout(400, 300, []model.PositionedNode{rect("a", 60, 40), rect("b", 260, 40)})
		pg.Groups = groups
		return check(pg)
	}
	t.Run("member inside", func(t *testing.T) {
		assert.Empty(t, filter(scene(grp("g", 10, 10, 100, 60, []string{"a"})), "C12.1"))
	})
	t.Run("member sticks out", func(t *testing.T) {
		assert.True(t, has(scene(grp("g", 10, 10, 80, 60, []string{"a"})), "C12.1", "g"))
	})
	t.Run("non-member inside", func(t *testing.T) {
		assert.True(t, has(scene(grp("g", 10, 10, 100, 60, nil)), "C12.2", "g"))
	})
	t.Run("nested child inside its parent", func(t *testing.T) {
		vs := scene(grp("o", 5, 5, 120, 80, nil, "i"), grp("i", 10, 10, 100, 60, []string{"a"}))
		assert.Empty(t, filter(vs, "C12.1"))
		assert.Empty(t, filter(vs, "C12.2"), "a is a member of o through i")
	})
	t.Run("child sticks out of its parent", func(t *testing.T) {
		vs := scene(grp("o", 12, 12, 100, 60, nil, "i"), grp("i", 10, 10, 100, 60, []string{"a"}))
		assert.True(t, has(vs, "C12.1", "o"))
	})
	t.Run("unrelated groups overlap", func(t *testing.T) {
		vs := scene(grp("g", 10, 10, 100, 60, []string{"a"}), grp("h", 90, 10, 220, 60, []string{"b"}))
		assert.True(t, has(vs, "C12.2", "g"))
		assert.Empty(t, filter(vs, "C12.3"), "an overlap is C12.2's alone")
	})
	t.Run("a non-member 10 px from the box", func(t *testing.T) {
		assert.True(t, has(scene(grp("g", 10, 10, 200, 60, []string{"a"})), "C12.3", "g"), "the box ends at 210, b starts at 220")
	})
	t.Run("a non-member 12 px from the box", func(t *testing.T) {
		assert.Empty(t, filter(scene(grp("g", 10, 10, 198, 60, []string{"a"})), "C12.3"))
	})
	t.Run("unrelated groups 10 px apart", func(t *testing.T) {
		vs := scene(grp("g", 10, 10, 100, 60, []string{"a"}), grp("h", 120, 10, 190, 60, []string{"b"}))
		assert.True(t, has(vs, "C12.3", "g"))
	})
	t.Run("a child group against its parent's border", func(t *testing.T) {
		vs := scene(grp("o", 10, 10, 100, 60, nil, "i"), grp("i", 10, 10, 100, 60, []string{"a"}))
		assert.Empty(t, filter(vs, "C12.3"), "nested groups are exempt")
	})
}

func TestC13_Titles(t *testing.T) {
	titled := func(offset float64) model.PositionedGroup {
		return model.PositionedGroup{ID: "g", Label: "G", X: 20, Y: 20, Width: 200, Height: 120, Depth: 1,
			LabelWidth: 40, LabelHeight: 14, LabelOffset: offset}
	}
	scene := func(nodes []model.PositionedNode, edges ...model.PositionedEdge) []Violation {
		pg := layout(300, 200, nodes, edges...)
		// Title box: (28,29)-(68,43).
		pg.Groups = []model.PositionedGroup{titled(0)}
		return check(pg)
	}
	assert.True(t, has(scene(nil, wire("p", "q", 40, 10, 40, 100)), "C13", "p->q#0"), "wire through the title")
	assert.False(t, has(scene(nil, wire("p", "q", 70, 10, 70, 100)), "C13", "p->q#0"), "wire beside the title")
	assert.True(t, has(scene([]model.PositionedNode{rect("n", 80, 50)}), "C13", "n"), "node over the title")

	pg := layout(300, 200, nil, wire("p", "q", 40, 10, 40, 100))
	pg.Groups = []model.PositionedGroup{{ID: "g", X: 20, Y: 20, Width: 200, Height: 120, Depth: 1}}
	assert.Empty(t, filter(check(pg), "C13"), "an unlabeled group has no title")
}

// TestC13_TheTitleSitsAtItsOffset pins C13's title box where the layout
// placed it: LabelOffset from the group's left side, not the renderer's
// default inset.
func TestC13_TheTitleSitsAtItsOffset(t *testing.T) {
	scene := func(edges ...model.PositionedEdge) []Violation {
		pg := layout(300, 200, nil, edges...)
		// Title box: (140,29)-(180,43).
		pg.Groups = []model.PositionedGroup{{ID: "g", Label: "G", X: 20, Y: 20, Width: 200, Height: 120, Depth: 1,
			LabelWidth: 40, LabelHeight: 14, LabelOffset: 120}}
		return check(pg)
	}
	assert.False(t, has(scene(wire("p", "q", 40, 10, 40, 100)), "C13", "p->q#0"), "the default slot is free once the title moved")
	assert.True(t, has(scene(wire("p", "q", 160, 10, 160, 100)), "C13", "p->q#0"), "a wire through the placed title")
}

// TestC13_GroupBoxesAndTitleExtents pins the rest of C13: a group box
// other than the group's own and its ancestors' crosses no title, and a
// titled group carries its title's extents.
func TestC13_GroupBoxesAndTitleExtents(t *testing.T) {
	scene := func(groups ...model.PositionedGroup) []Violation {
		pg := layout(300, 200, nil)
		pg.Groups = groups
		return check(pg)
	}
	// g's title box: (28,29)-(68,43).
	g := model.PositionedGroup{ID: "g", Label: "G", X: 20, Y: 20, Width: 200, Height: 120, Depth: 2,
		LabelWidth: 40, LabelHeight: 14, Children: []string{"c"}}
	outer := model.PositionedGroup{ID: "o", X: 10, Y: 10, Width: 220, Height: 140, Depth: 1, Children: []string{"g"}}
	over := model.PositionedGroup{ID: "c", X: 24, Y: 24, Width: 100, Height: 60, Depth: 3}
	below := model.PositionedGroup{ID: "c", X: 36, Y: 52, Width: 100, Height: 60, Depth: 3}
	assert.True(t, has(scene(outer, g, over), "C13", "c"), "a child group over the title")
	assert.Empty(t, filter(scene(outer, g, below), "C13"), "a child group below the title; the ancestor's box holds it")

	unmeasured := model.PositionedGroup{ID: "g", Label: "G", X: 20, Y: 20, Width: 200, Height: 120, Depth: 1}
	assert.True(t, has(scene(unmeasured), "C13", "g"), "a titled group without its title's extents")
}

// TestC13_ABlockedTitleExemptsItsWires pins C13's sanctioned degradation
// (C0): a wire may cross a title the layout marked LabelBlocked, which
// every renderer draws in front of it, but a node or a foreign group box
// over it still breaks the rule, as does a missing extent.
func TestC13_ABlockedTitleExemptsItsWires(t *testing.T) {
	// g's title box: (28,29)-(68,43).
	blocked := model.PositionedGroup{ID: "g", Label: "G", X: 20, Y: 20, Width: 200, Height: 120, Depth: 1,
		LabelWidth: 40, LabelHeight: 14, LabelBlocked: true}
	scene := func(nodes []model.PositionedNode, groups []model.PositionedGroup, edges ...model.PositionedEdge) []Violation {
		pg := layout(300, 200, nodes, edges...)
		pg.Groups = append([]model.PositionedGroup{blocked}, groups...)
		return check(pg)
	}
	assert.Empty(t, filter(scene(nil, nil, wire("p", "q", 40, 10, 40, 100)), "C13"), "a wire through a blocked title")
	assert.True(t, has(scene([]model.PositionedNode{rect("n", 80, 50)}, nil), "C13", "n"), "a node over a blocked title")
	foreign := model.PositionedGroup{ID: "f", X: 24, Y: 24, Width: 100, Height: 60, Depth: 1}
	assert.True(t, has(scene(nil, []model.PositionedGroup{foreign}), "C13", "f"), "a foreign group over a blocked title")

	blocked.LabelWidth = 0
	assert.True(t, has(scene(nil, nil), "C13", "g"), "a blocked title still carries its extents")
}

// TestC13_TextTitleCoversItsBlanks pins C13's text title box: one cell
// wider on each side, over the blank the text renderer writes beside the
// title. g's title spans columns 7..11, (56,32)-(96,48); its blanks take
// columns 6 and 12.
func TestC13_TextTitleCoversItsBlanks(t *testing.T) {
	text := Options{Limits: TextLimits(model.Down), Direction: model.Down, Text: true}
	scene := func(x float64, o Options) []Violation {
		pg := layout(240, 160, nil, wire("p", "q", x, 0, x, 120))
		pg.Groups = []model.PositionedGroup{{ID: "g", Label: "G", X: 16, Y: 32, Width: 160, Height: 96, Depth: 1,
			LabelWidth: 40, LabelHeight: 16, LabelOffset: 40}}
		return filter(Check(pg, o), "C13")
	}
	assert.NotEmpty(t, scene(52, text), "a wire in the blank's column before the title")
	assert.NotEmpty(t, scene(100, text), "a wire in the blank's column after the title")
	assert.Empty(t, scene(44, text), "a wire one column further left")
	assert.Empty(t, scene(108, text), "a wire one column further right")
	screen := Options{Limits: ScreenLimits(), Direction: model.Down}
	assert.Empty(t, scene(52, screen), "on screen a title has no blanks")
}

// TestC16_TextMarginsCountWholeCells pins C16 in the text profile: a
// side column on a cell's middle, half a cell outside every box, is drawn
// in its whole cell, and the margin counts from that cell's edge.
func TestC16_TextMarginsCountWholeCells(t *testing.T) {
	pg := layout(120, 136,
		[]model.PositionedNode{rect("a", 64, 36), rect("b", 64, 100)},
		wire("b", "a", 24, 100, 20, 100, 20, 36, 24, 36))
	text := Options{Limits: TextLimits(model.Down), Direction: model.Down, Text: true}
	assert.Empty(t, filter(Check(pg, text), "C16.2"), "the column's cell starts 16 px in")
	pg.Width = 128
	assert.Equal(t, []Violation{{Rule: "C16.2", Detail: "the right margin is 24.00 px, not 16"}}, filter(Check(pg, text), "C16.2"))
}

func TestC16_Margins(t *testing.T) {
	assert.Empty(t, filter(check(chain()), "C16.1"))
	assert.Empty(t, filter(check(chain()), "C16.2"))

	near := chain()
	near.Nodes[0].X = 50 // a's left side at x=10
	assert.True(t, has(check(near), "C16.1", "a"))

	loose := chain()
	loose.Width = 140
	assert.Equal(t, []Violation{{Rule: "C16.2", Detail: "the right margin is 40.00 px, not 20"}}, filter(check(loose), "C16.2"))

	label := chain()
	label.Edges[0].Label = "wide"
	label.Edges[0].LabelPos = &model.Point{X: 90, Y: 90}
	label.Edges[0].LabelWidth, label.Edges[0].LabelHeight = 40, 14
	assert.True(t, has(check(label), "C16.1", "a->b#0"), "labels are bounded by the margin too")

	assert.Empty(t, check(layout(0, 0, nil)), "an empty layout")
}
