package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/oxforge/diago/internal/model"
)

func TestMeasureChurn(t *testing.T) {
	carrier := func(layers, order map[string]int) *model.LayoutHints {
		h := model.NewLayoutHints()
		h.Scopes[""] = model.ScopeHints{Layers: layers, Order: order}
		return h
	}
	before := &model.PositionedGraph{
		Nodes:       []model.PositionedNode{rect("a", 100, 50), rect("b", 60, 150), rect("c", 160, 150), rect("gone", 260, 150)},
		LayoutHints: carrier(map[string]int{"a": 0, "b": 1, "c": 1, "gone": 1}, map[string]int{"a": 0, "b": 0, "c": 1, "gone": 2}),
	}
	// a moves 30 px right, the median; b and c trade places, b 130 px
	// right and c 70 px left, both 100 px off the median, and c 16 px
	// down. gone is gone, and new takes its place at the layer's end.
	after := &model.PositionedGraph{
		Nodes:       []model.PositionedNode{rect("a", 130, 50), rect("b", 190, 150), rect("c", 90, 166), rect("new", 300, 150)},
		LayoutHints: carrier(map[string]int{"a": 0, "b": 1, "c": 1, "new": 1}, map[string]int{"a": 0, "c": 0, "b": 1, "new": 2}),
	}
	c := MeasureChurn(before, after, model.Down)
	assert.Equal(t, Churn{Across: 100, Along: 16, Relayered: 0, Swapped: 1}, c)
	assert.Equal(t, "across=100 along=16 relayered=0 swapped=1", c.String())

	sideways := MeasureChurn(before, after, model.Right)
	assert.Equal(t, [2]float64{16, 100}, [2]float64{sideways.Across, sideways.Along}, "x runs along the flow under RIGHT")

	// A layer added above them all moves every survivor alike: none moves
	// against the others (C18.4), and b and c still trade places.
	for id := range after.LayoutHints.Scopes[""].Layers {
		after.LayoutHints.Scopes[""].Layers[id]++
	}
	shifted := MeasureChurn(before, after, model.Down)
	assert.Equal(t, [2]int{0, 1}, [2]int{shifted.Relayered, shifted.Swapped}, "a layer added above")

	after.LayoutHints.Scopes[""].Layers["c"] = 3
	assert.Equal(t, 1, MeasureChurn(before, after, model.Down).Relayered)
	assert.Equal(t, Churn{}, MeasureChurn(&model.PositionedGraph{}, &model.PositionedGraph{}, model.Down), "no survivors, no carriers")
}

func TestRelayered(t *testing.T) {
	carrier := func(scopes map[string]map[string]int) *model.LayoutHints {
		h := model.NewLayoutHints()
		for name, layers := range scopes {
			h.Scopes[name] = model.ScopeHints{Layers: layers, Order: map[string]int{}}
		}
		return h
	}
	before := carrier(map[string]map[string]int{"": {"a": 0, "b": 1, "c": 2, "gone": 3}, "g": {"x": 1, "y": 2}})
	cases := []struct {
		name  string
		after map[string]map[string]int
		want  []string
	}{
		{"unchanged", map[string]map[string]int{"": {"a": 0, "b": 1, "c": 2}, "g": {"x": 1, "y": 2}}, nil},
		{"a layer above the root's", map[string]map[string]int{"": {"new": 0, "a": 1, "b": 2, "c": 3}, "g": {"x": 1, "y": 2}}, nil},
		{"c against a and b", map[string]map[string]int{"": {"a": 0, "b": 1, "c": 3}, "g": {"x": 1, "y": 2}}, []string{"/c"}},
		{"a tie goes to the smallest move", map[string]map[string]int{"": {"a": 0, "b": 1, "c": 2}, "g": {"x": 0, "y": 2}}, []string{"g/y"}},
		{"a vanished scope", map[string]map[string]int{"": {"a": 0, "b": 1, "c": 2}}, nil},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, Relayered(before, carrier(tc.after)), tc.name)
	}
	assert.Nil(t, Relayered(nil, before))
}

func TestNewlyRanked(t *testing.T) {
	carrier := func(ranked ...string) *model.LayoutHints {
		h := model.NewLayoutHints()
		h.Ranked = ranked
		return h
	}
	flat := []string{"a->b#0", "c->d#0", "e->f#0"}
	cases := []struct {
		name          string
		before, after *model.LayoutHints
		want          []string
	}{
		{"no fallback", carrier(), carrier(), nil},
		{"the same fallbacks", carrier("a->b#0"), carrier("a->b#0"), nil},
		{"two newly fall back", carrier("a->b#0"), carrier("a->b#0", "e->f#0", "c->d#0"), []string{"c->d#0", "e->f#0"}},
		{"one routes again", carrier("a->b#0"), carrier(), nil},
		// x->y was no flat edge of before's graph: before's layout ranked
		// it as an ordinary edge, so ranking it now changes nothing
		{"an edge that was not flat", carrier(), carrier("x->y#0"), nil},
		{"no carriers", nil, nil, nil},
		// a fresh layout's fallbacks are all new against no previous one
		{"no previous carrier", nil, carrier("c->d#0", "a->b#0"), []string{"a->b#0", "c->d#0"}},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, NewlyRanked(tc.before, tc.after, flat), tc.name)
	}
}
