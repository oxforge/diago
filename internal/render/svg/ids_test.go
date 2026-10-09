package svg

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIDSet_TheFirstClaimKeepsItsIDAndLaterOnesAreNumbered(t *testing.T) {
	s := idSet{}
	assert.Equal(t, "edge-a-b", s.claim("edge-a-b"))
	assert.Equal(t, "edge-a-b-2", s.claim("edge-a-b"))
	assert.Equal(t, "edge-a-b-3", s.claim("edge-a-b"))
	assert.Equal(t, "edge-a-b-2-2", s.claim("edge-a-b-2"))
	assert.Equal(t, "group-g", s.claim("group-g"))
}
