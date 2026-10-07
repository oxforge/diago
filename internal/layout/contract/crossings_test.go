package contract

import (
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
