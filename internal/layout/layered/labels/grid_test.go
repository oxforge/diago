package labels

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/model"
)

// meets reports whether the closed extents of two boxes meet: every box
// with a nonzero term in a spot's cost meets it (S10).
func meets(a, b Rect) bool {
	return a.X <= b.X+b.W && b.X <= a.X+a.W && a.Y <= b.Y+b.H && b.Y <= a.Y+a.H
}

func TestGrid_NearHoldsEveryBoxThatMeetsTheQueryOnceInOrder(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	box := func() Rect {
		r := Rect{rng.Float64()*400 - 50, rng.Float64()*300 - 50, rng.Float64() * 60, rng.Float64() * 60}
		switch rng.Intn(3) {
		case 0:
			r.W = 0 // a vertical segment
		case 1:
			r.H = 0 // a horizontal one
		}
		return r
	}
	for _, side := range []float64{0, 7, 25, 1000} {
		g := newGrid(Rect{0, 0, 300, 200}, side)
		var boxes []Rect
		for i := range 300 {
			b := box()
			boxes = append(boxes, b)
			g.add(i, b)
		}
		for range 500 {
			q := box()
			got := g.near(q.X, q.Y, q.X+q.W, q.Y+q.H)
			assert.True(t, slices.IsSorted(got), "side %v: ascending", side)
			assert.Equal(t, len(got), len(slices.Compact(slices.Clone(got))), "side %v: each once", side)
			for i, b := range boxes {
				if meets(q, b) {
					assert.Contains(t, got, i, "side %v: box %d meets %v", side, i, q)
				}
			}
		}
	}
}

func TestGrid_AnUnboundedQueryHoldsEveryBox(t *testing.T) {
	g := newGrid(Rect{0, 0, 100, 100}, 10)
	for i := range 4 {
		g.add(i, Rect{float64(i) * 60, float64(i) * 60, 5, 5})
	}
	inf := math.Inf(1)
	require.Equal(t, []int{0, 1, 2, 3}, g.near(-inf, -inf, inf, inf))
}

func TestGrid_TouchingHoldsEveryBoxThatMeetsTheQuery(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	g := newGrid(Rect{0, 0, 300, 200}, 20)
	var boxes []Rect
	for i := range 200 {
		b := Rect{rng.Float64() * 300, rng.Float64() * 200, rng.Float64() * 80, 0}
		boxes = append(boxes, b)
		g.add(i, b)
	}
	for range 200 {
		q := Rect{rng.Float64() * 300, rng.Float64() * 200, rng.Float64() * 90, rng.Float64() * 90}
		got := g.touching(q.X, q.Y, q.X+q.W, q.Y+q.H)
		for i, b := range boxes {
			if meets(q, b) {
				assert.Contains(t, got, i, "box %d meets %v", i, q)
			}
		}
	}
}

// crowd is a random dense scene from seed: n node boxes in two rows and m
// edges from the upper row to the lower, down, across and down, each
// labelled and some with cardinalities or an adornment, two copying
// another's labels (S12), and two groups. In the text profile every
// coordinate and extent is whole.
func crowd(seed int64, n, m int, text bool) (nodes []Rect, groups []Rect, edges []Edge) {
	rng := rand.New(rand.NewSource(seed))
	unit, gap := 1.0, 40.0
	if text {
		unit, gap = 8, 5
	}
	size := func(w, h float64) Size {
		if text {
			return Size{math.Round(w / unit), 1}
		}
		return Size{w, h}
	}
	for i := range n {
		x := float64(i/2) * 3 * gap
		y := float64(i%2) * 7 * gap
		nodes = append(nodes, Rect{x, y, 2 * gap, gap})
	}
	groups = []Rect{{-unit, -unit, 6 * gap, 9 * gap}, {6 * gap, -unit, 6 * gap, 9 * gap}}
	for i := range m {
		a, b := nodes[2*rng.Intn(n/2)], nodes[2*rng.Intn(n/2)+1]
		x1, x2 := a.X+math.Floor(rng.Float64()*a.W), b.X+math.Floor(rng.Float64()*b.W)
		y := a.Y + a.H + math.Floor(rng.Float64()*(b.Y-a.Y-a.H))
		pts := []model.Point{{X: x1, Y: a.Y + a.H}, {X: x1, Y: y}, {X: x2, Y: y}, {X: x2, Y: b.Y}}
		if x1 == x2 {
			pts = []model.Point{pts[0], pts[3]}
		}
		e := Edge{ID: fmt.Sprintf("e%d", i), Points: pts, Label: size(10+rng.Float64()*80, 14), Source: 2 * rng.Intn(n/2)}
		switch rng.Intn(4) {
		case 0:
			e.From = size(10, 14)
		case 1:
			e.To, e.Adorned = size(24, 14), true
		}
		edges = append(edges, e)
	}
	edges[0].Copies, edges[2].Copies = []int{1}, []int{3}
	return nodes, groups, edges
}

func TestPlace_TheGridsChangeNoPlacement(t *testing.T) {
	// the grids only spare a spot's cost the boxes far from it: the same
	// crowds placed with grids of one cell each, a full scan of every box,
	// take the same spots, resolved or not (S10)
	screen := Options{Gap: 6, CardStep: 10, EnvDepth: 14, EnvHalf: 7, OwnSlack: 2}
	text := Options{Gap: 1, CardStep: 1, EnvDepth: 1, EnvHalf: 1, Pad: 1, Text: true}
	for _, tc := range []struct {
		name string
		o    Options
		dir  model.Direction
	}{{"screen", screen, model.Down}, {"screen sideways", screen, model.Right}, {"text", text, model.Down}, {"text sideways", text, model.Left}} {
		t.Run(tc.name, func(t *testing.T) {
			o := tc.o
			o.Dir = tc.dir
			unresolved := 0
			for seed := int64(1); seed <= 4; seed++ {
				nodes, groups, edges := crowd(seed, 16, 60, o.Text)
				pinned := make([]bool, len(nodes))
				want := placeWith(context.Background(), nodes, pinned, groups, edges, o, false)
				got := placeWith(context.Background(), nodes, pinned, groups, edges, o, true)
				require.Equal(t, want, got, "seed %d", seed)
				for _, p := range got {
					if p.LabelUnresolved {
						unresolved++
					}
				}
			}
			assert.Positive(t, unresolved, "the crowds leave labels scoring their whole ladders")
		})
	}
}
