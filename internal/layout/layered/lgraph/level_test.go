package lgraph_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/oxforge/diago/internal/layout/layered/lgraph/lgraphtest"
)

func TestLevel_SelfLoopAndOriented(t *testing.T) {
	lv := lgraphtest.Level(t, "a->b", "b->b")
	assert.False(t, lv.SelfLoop(0))
	assert.True(t, lv.SelfLoop(1))

	upper, lower := lv.Oriented(0, nil)
	assert.Equal(t, [2]int{0, 1}, [2]int{upper, lower})
	upper, lower = lv.Oriented(0, []bool{true, false})
	assert.Equal(t, [2]int{1, 0}, [2]int{upper, lower})
}

func TestLevel_SelfLoops(t *testing.T) {
	lv := lgraphtest.Level(t, "a->b", "b->b", "c", "b->b", "c->c")
	assert.Equal(t, []int{0, 2, 1}, lv.SelfLoops(), "a none, b two, c one")
}
