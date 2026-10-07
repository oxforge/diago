package excalidraw

import (
	"testing"

	"github.com/oxforge/diago/internal/model"
	"github.com/stretchr/testify/assert"
)

func TestArrowheads_None(t *testing.T) {
	assert.Nil(t, arrowheadStart(model.EdgeNone))
	assert.Nil(t, arrowheadEnd(model.EdgeNone))
}
