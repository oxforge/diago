package diff

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/oxforge/diago/internal/model"
)

// carrier is a carrier of one scope per entry of scopes, with no reversals.
func carrier(scopes map[string]model.ScopeHints) *model.LayoutHints {
	h := model.NewLayoutHints()
	for id, s := range scopes {
		h.Scopes[id] = s
	}
	return h
}

func TestAnchor_OrderTakesBothSides(t *testing.T) {
	// Before's layer 1: b, removed m, a->c's dummy. After's: b, added n,
	// the dummy, added q. m stays beside b, n goes before the dummy, its
	// right neighbour in after, and q at the end; layer 2 gains w after c.
	before := carrier(map[string]model.ScopeHints{"": {
		Layers: map[string]int{"a": 0, "b": 1, "m": 1, "c": 2},
		Order:  map[string]int{"a": 0, "b": 0, "m": 1, "a->c#0@1": 2, "c": 0},
	}})
	after := carrier(map[string]model.ScopeHints{"": {
		Layers: map[string]int{"a": 0, "b": 1, "n": 1, "q": 1, "c": 2, "w": 2},
		Order:  map[string]int{"a": 0, "b": 0, "n": 1, "a->c#0@1": 2, "q": 3, "c": 0, "w": 1},
	}})
	got := UnionGraph{}.Anchor(before, after)
	assert.Equal(t, map[string]int{"a": 0, "b": 0, "m": 1, "n": 2, "a->c#0@1": 3, "q": 4, "c": 0, "w": 1}, got.Scopes[""].Order)
	assert.Equal(t, map[string]int{"a": 0, "b": 1, "m": 1, "c": 2}, got.Scopes[""].Layers, "layers are before's alone")
}

func TestAnchor_KeptOrderIsBefores(t *testing.T) {
	// After swapped x and y; the kept ones keep before's order, and n
	// goes before y, its right neighbour in after.
	before := carrier(map[string]model.ScopeHints{"": {
		Layers: map[string]int{"x": 0, "y": 0},
		Order:  map[string]int{"x": 0, "y": 1},
	}})
	after := carrier(map[string]model.ScopeHints{"": {
		Layers: map[string]int{"x": 0, "y": 0, "n": 0},
		Order:  map[string]int{"n": 0, "y": 1, "x": 2},
	}})
	got := UnionGraph{}.Anchor(before, after)
	assert.Equal(t, map[string]int{"x": 0, "n": 1, "y": 2}, got.Scopes[""].Order)
}

func TestAnchor_AlignsAfterLayers(t *testing.T) {
	// After added t above a, so its layers sit one below before's: n and
	// t->c's slots land on before's numbering, t on the layer above a.
	before := carrier(map[string]model.ScopeHints{"": {
		Layers: map[string]int{"a": 0, "b": 1, "c": 2},
		Order:  map[string]int{"a": 0, "b": 0, "a->c#0@1": 1, "c": 0},
	}})
	after := carrier(map[string]model.ScopeHints{"": {
		Layers: map[string]int{"t": 0, "a": 1, "b": 2, "n": 2, "c": 3},
		Order: map[string]int{
			"t": 0, "a": 0, "t->c#0@1": 1,
			"n": 0, "b": 1, "a->c#0@2": 2, "t->c#0@2": 3, "c": 0,
		},
	}})
	got := UnionGraph{}.Anchor(before, after)
	assert.Equal(t, map[string]int{
		"t": 0, "a": 0, "t->c#0@0": 1,
		"n": 0, "b": 1, "a->c#0@1": 2, "t->c#0@1": 3, "c": 0,
	}, got.Scopes[""].Order)
}

func TestAnchor_ShiftTieGoesNearestZeroThenLower(t *testing.T) {
	// a says shift -1, b says +1, c says 0 twice over with d: 0 wins.
	// Without c and d, -1 and +1 tie and the lower one, -1, wins.
	for _, tc := range []struct {
		name          string
		before, after map[string]int
		want          string
	}{
		{"most agree", map[string]int{"a": 0, "b": 2, "c": 4, "d": 5}, map[string]int{"a": 1, "b": 1, "c": 4, "d": 5}, "e@3"},
		{"tie", map[string]int{"a": 0, "b": 2}, map[string]int{"a": 1, "b": 1}, "e@2"},
	} {
		before := carrier(map[string]model.ScopeHints{"": {Layers: tc.before, Order: map[string]int{}}})
		after := carrier(map[string]model.ScopeHints{"": {Layers: tc.after, Order: map[string]int{"e@3": 0}}})
		got := UnionGraph{}.Anchor(before, after)
		assert.Contains(t, got.Scopes[""].Order, tc.want, tc.name)
	}
}

func TestAnchor_RenamesARemovedEdgesSlots(t *testing.T) {
	// The union renamed removed edge e to e__removed, after's own e taking
	// its id: before's slot is e__removed's, and after's e is new.
	before := carrier(map[string]model.ScopeHints{"": {
		Layers: map[string]int{"b": 1},
		Order:  map[string]int{"b": 0, "e@1": 1},
	}})
	after := carrier(map[string]model.ScopeHints{"": {
		Layers: map[string]int{"b": 1},
		Order:  map[string]int{"e@1": 0, "b": 1},
	}})
	u := UnionGraph{RemovedEdgeIDs: map[string]string{"e": "e__removed"}}
	got := u.Anchor(before, after)
	assert.Equal(t, map[string]int{"e@1": 0, "b": 1, "e__removed@1": 2}, got.Scopes[""].Order)
}

func TestAnchor_ScopeOnlyAfterHoldsGivesItsOrder(t *testing.T) {
	before := carrier(map[string]model.ScopeHints{"": {Layers: map[string]int{"g": 0}, Order: map[string]int{"g": 0}}})
	after := carrier(map[string]model.ScopeHints{
		"":  {Layers: map[string]int{"g": 0}, Order: map[string]int{"g": 0}},
		"g": {Layers: map[string]int{"x": 0, "y": 0}, Order: map[string]int{"y": 0, "x": 1}},
	})
	got := UnionGraph{}.Anchor(before, after)
	assert.Equal(t, map[string]int{"y": 0, "x": 1}, got.Scopes["g"].Order)
	assert.Empty(t, got.Scopes["g"].Layers)
}

func TestAnchor_KeepsBeforesDecisionsAndInputs(t *testing.T) {
	// An unchanged pair merges to before's carrier; the inputs stay as
	// they were, and reversed and ranked are before's.
	scope := func() model.ScopeHints {
		return model.ScopeHints{
			Layers: map[string]int{"a": 0, "b": 1, "c": 1},
			Order:  map[string]int{"a": 0, "c": 0, "b": 1, "a->d#0@1": 2},
		}
	}
	before := carrier(map[string]model.ScopeHints{"": scope(), "g": scope()})
	before.Reversed, before.Ranked = []string{"r1"}, []string{"f1"}
	after := carrier(map[string]model.ScopeHints{"": scope(), "g": scope()})
	after.Reversed = []string{"r2"}
	got := UnionGraph{}.Anchor(before, after)
	want := carrier(map[string]model.ScopeHints{"": scope(), "g": scope()})
	want.Reversed, want.Ranked = []string{"r1"}, []string{"f1"}
	assert.Equal(t, want, got)
	assert.Equal(t, want, before, "before is not changed")
	got.Scopes[""].Order["a"] = 9
	got.Reversed[0] = "x"
	assert.Equal(t, 0, before.Scopes[""].Order["a"], "the result shares no map with before")
	assert.Equal(t, "r1", before.Reversed[0], "the result shares no slice with before")
}
