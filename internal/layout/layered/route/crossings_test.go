package route

import (
	"testing"

	"github.com/oxforge/diago/internal/model"
	"github.com/stretchr/testify/assert"
)

func TestCrossings(t *testing.T) {
	h := pts(0, 50, 100, 50)
	v := pts(40, 0, 40, 100)
	got := Crossings([][]model.Point{h, v}, 5)
	assert.Empty(t, got[0])
	assert.Equal(t, []model.Crossing{{SegmentIndex: 0, T: 0.5}}, got[1], "on the later edge, along its segment")

	corner := pts(3, -50, 3, 50)
	got = Crossings([][]model.Point{pts(0, 0, 100, 0), corner}, 5)
	assert.Empty(t, got[1], "within the radius of a segment end: a corner")

	got = Crossings([][]model.Point{pts(0, 0, 100, 0), pts(50, 0, 50, 100)}, 5)
	assert.Empty(t, got[1], "touching at a segment's end is no crossing")
}
