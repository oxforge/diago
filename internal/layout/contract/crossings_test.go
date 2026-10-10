package contract

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/oxforge/diago/internal/model"
)

// cross is a horizontal wire over a vertical one; they cross at (100,100).
func cross(h, v []model.Crossing) []Violation {
	e1 := wire("p", "q", 20, 100, 200, 100)
	e1.Crossings = h
	e2 := wire("r", "s", 100, 20, 100, 200)
	e2.Crossings = v
	return check(layout(300, 300, nil, e1, e2))
}

func TestC15_1_RecordsAreValid(t *testing.T) {
	assert.True(t, has(cross([]model.Crossing{{SegmentIndex: 3, T: 0.4444}}, nil), "C15.1", "p->q#0"))
	assert.True(t, has(cross([]model.Crossing{{SegmentIndex: 0, T: 1.5}}, nil), "C15.1", "p->q#0"))
	assert.True(t, has(cross([]model.Crossing{{SegmentIndex: 0, T: math.NaN()}}, nil), "C15.1", "p->q#0"), "NaN lies in no range")
}

func TestC15_2_CrossingsAreRecorded(t *testing.T) {
	assert.True(t, has(cross(nil, nil), "C15.2", "p->q#0"), "no record")
	assert.Empty(t, filter(cross([]model.Crossing{{SegmentIndex: 0, T: 80.0 / 180}}, nil), "C15.2"), "recorded on the first edge")
	assert.Empty(t, filter(cross(nil, []model.Crossing{{SegmentIndex: 0, T: 0.4444}}), "C15.2"), "recorded on the second")

	// A crossing 3 px from a segment end is a corner hop the renderer drops.
	e1 := wire("p", "q", 20, 100, 200, 100)
	e2 := wire("r", "s", 100, 97, 100, 200)
	assert.Empty(t, filter(check(layout(300, 300, nil, e1, e2)), "C15.2"))
}

// TestC15_WhatIsNotFlagged pins what C15 lets pass: records at either end
// of a segment, on a route's last segment, and within 1 px of the
// crossing; a crossing no more than the 5 px hop radius from a segment
// end, where the renderer draws a corner; segments that only touch or run
// on one line; and an edge crossing itself, which is no crossing of two
// edges. Each near miss beside it is flagged.
func TestC15_WhatIsNotFlagged(t *testing.T) {
	h := wire("p", "q", 20, 100, 200, 100) // crosses v at (100,100), t = 80/180 along h
	v := wire("r", "s", 100, 20, 100, 200)
	two := func(a, b model.PositionedEdge) []Violation { return check(layout(300, 300, nil, a, b)) }
	recorded := func(e model.PositionedEdge, crs ...model.Crossing) model.PositionedEdge {
		e.Crossings = crs
		return e
	}
	tFor := func(dx float64) float64 { return (80 + dx) / 180 } // a record dx px along h from the crossing
	for _, tc := range []struct {
		name string
		vs   []Violation
		rule string
		want bool
	}{
		{"a record at t = 0", two(recorded(h, model.Crossing{SegmentIndex: 0, T: 0}, model.Crossing{SegmentIndex: 0, T: tFor(0)}), v), "C15.1", false},
		{"a record at t = 1", two(recorded(h, model.Crossing{SegmentIndex: 0, T: 1}, model.Crossing{SegmentIndex: 0, T: tFor(0)}), v), "C15.1", false},
		{"a record past t = 1", two(recorded(h, model.Crossing{SegmentIndex: 0, T: 1.01}), v), "C15.1", true},
		{"a record on a route's last segment",
			two(recorded(wire("p", "q", 20, 60, 20, 100, 200, 100), model.Crossing{SegmentIndex: 1, T: tFor(0)}), v), "C15.1", false},
		{"a record past a route's last segment",
			two(recorded(wire("p", "q", 20, 60, 20, 100, 200, 100), model.Crossing{SegmentIndex: 2, T: 0.5}), v), "C15.1", true},
		{"a record 0.9 px from the crossing", two(recorded(h, model.Crossing{SegmentIndex: 0, T: tFor(0.9)}), v), "C15.2", false},
		{"a record 1.5 px from the crossing", two(recorded(h, model.Crossing{SegmentIndex: 0, T: tFor(1.5)}), v), "C15.2", true},
		{"a crossing 5 px from a segment end", two(h, wire("r", "s", 100, 95, 100, 200)), "C15.2", false},
		{"a crossing 5.5 px from a segment end", two(h, wire("r", "s", 100, 94.5, 100, 200)), "C15.2", true},
		{"a segment ending on another", two(h, wire("r", "s", 100, 20, 100, 100)), "C15.2", false},
		{"collinear segments", two(h, wire("r", "s", 100, 100, 260, 100)), "C15.2", false},
		{"an edge crossing itself", check(layout(300, 300, nil, wire("p", "q", 20, 100, 200, 100, 200, 160, 100, 160, 100, 20))), "C15.2", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, len(filter(tc.vs, tc.rule)) > 0, "%v", filter(tc.vs, tc.rule))
		})
	}
}
