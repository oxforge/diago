package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseEdgeDirection_None(t *testing.T) {
	d, err := ParseEdgeDirection("none")
	require.NoError(t, err)
	assert.Equal(t, EdgeNone, d)
	assert.Equal(t, "none", d.String())
	b, err := json.Marshal(d)
	require.NoError(t, err)
	assert.Equal(t, `"none"`, string(b))
}
