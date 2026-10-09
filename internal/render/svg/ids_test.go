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

// Distinct ids pass through unchanged, one that reads like a numbered
// claim included.
func TestIDSet_ADistinctIDPassesThroughUnchanged(t *testing.T) {
	s := idSet{}
	for _, id := range []string{"edge-a-b-2", "edge-a-b", "group-g", "group-title-g", "node-a"} {
		assert.Equal(t, id, s.claim(id))
	}
}
