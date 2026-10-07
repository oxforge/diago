package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mustCheck runs Check and fails the test on error.
func mustCheck(t *testing.T, spec string, opts CheckOptions) []Advisory {
	t.Helper()
	advs, err := Check([]byte(spec), opts)
	require.NoError(t, err)
	return advs
}

// keys returns "rule field" per finding, in order.
func keys(advs []Advisory) []string {
	out := make([]string, len(advs))
	for i, a := range advs {
		out[i] = a.Rule + " " + a.Field
	}
	return out
}

func TestCheck_Errors(t *testing.T) {
	_, err := Check([]byte(`{`), CheckOptions{})
	assert.Error(t, err)
	_, err = Check([]byte(`{"type":"bogus"}`), CheckOptions{})
	assert.ErrorContains(t, err, `unknown diagram type: "bogus"`)
}

func TestCheck_UnknownFieldAndIgnore(t *testing.T) {
	advs := mustCheck(t, `{"type":"flow","nodes":[{"id":"a","label":"A","colour":"red"}],"edges":[]}`, CheckOptions{})
	assert.Equal(t, []string{"unknown-field nodes[0].colour"}, keys(advs))

	advs = mustCheck(t, `{"type":"flow","ignore":["unknown-field"],"nodes":[{"id":"a","label":"A","colour":"red"}],"edges":[]}`, CheckOptions{})
	assert.Empty(t, advs)

	advs = mustCheck(t, `{"type":"sequence","actors":[{"id":"a","label":"A","colour":"red"}],"interactions":[]}`, CheckOptions{})
	assert.Equal(t, []string{"unknown-field actors[0].colour"}, keys(advs))
}
