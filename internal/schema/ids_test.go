package schema

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parseAny parses spec by the type it declares.
func parseAny(spec string) error {
	var err error
	switch {
	case strings.Contains(spec, `"type":"flow"`):
		_, err = ParseFlow([]byte(spec))
	case strings.Contains(spec, `"type":"class"`):
		_, err = ParseClass([]byte(spec))
	default:
		_, err = ParseSequence([]byte(spec))
	}
	return err
}

// flowWithNode is a one-node flow spec whose node id is the raw JSON string
// content id.
func flowWithNode(id string) string {
	return `{"type":"flow","nodes":[{"id":"` + id + `","label":"N"}],"edges":[]}`
}

func TestIDs_RejectTheFormsLayoutsKeyOn(t *testing.T) {
	for _, tc := range []struct{ name, spec, field, msg string }{
		{"node arrow", flowWithNode(`a->b`), "nodes[0].id", `"->"`},
		{"node slot", flowWithNode(`x@1`), "nodes[0].id", `"@" and digits`},
		{"node control", flowWithNode(`a\u0007`), "nodes[0].id", "control characters"},
		{"group slot", `{"type":"flow","nodes":[{"id":"a","label":"A"}],"edges":[],"groups":[{"id":"g@2","label":"G","contains":["a"]}]}`, "groups[0].id", `"@" and digits`},
		{"class arrow", `{"type":"class","classes":[{"id":"a->b"}]}`, "classes[0].id", `"->"`},
		{"package slot", `{"type":"class","classes":[{"id":"a"}],"packages":[{"id":"p@0","label":"P","contains":["a"]}]}`, "packages[0].id", `"@" and digits`},
		{"edge control", `{"type":"flow","nodes":[{"id":"a","label":"A"}],"edges":[{"id":"e\u0007","from":"a","to":"a"}]}`, "edges[0].id", "control characters"},
		{"actor control", `{"type":"sequence","actors":[{"id":"a\u0007","label":"A"}],"interactions":[]}`, "actors[0].id", "control characters"},
		{"node U+FFFE", flowWithNode(`a\ufffe`), "nodes[0].id", "U+FFFE"},
		{"group U+FFFF", `{"type":"flow","nodes":[{"id":"a","label":"A"}],"edges":[],"groups":[{"id":"g\uffff","label":"G","contains":["a"]}]}`, "groups[0].id", "U+FFFF"},
		{"actor U+FFFE", `{"type":"sequence","actors":[{"id":"a\ufffe","label":"A"}],"interactions":[]}`, "actors[0].id", "U+FFFE"},
		{"relation U+FFFF", `{"type":"class","classes":[{"id":"a"},{"id":"b"}],"relations":[{"id":"r\uffff","from":"a","to":"b"}]}`, "relations[0].id", "U+FFFF"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var ve ValidationErrors
			require.True(t, errors.As(parseAny(tc.spec), &ve), "a validation error")
			var got []string
			for _, e := range ve {
				if e.Field == tc.field {
					got = append(got, e.Message)
				}
			}
			require.Len(t, got, 1, "one error on %s: %v", tc.field, ve)
			assert.Contains(t, got[0], tc.msg)
		})
	}
}

func TestIDs_AcceptNearForms(t *testing.T) {
	for _, spec := range []string{
		flowWithNode(`user@example`),
		flowWithNode(`v@2b`),
		flowWithNode(`a-b`),
		flowWithNode(`a>b`),
		`{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":[{"id":"a->b#0","from":"a","to":"b"}]}`,
		`{"type":"sequence","actors":[{"id":"x@1","label":"X"}],"interactions":[]}`,
	} {
		assert.NoError(t, parseAny(spec), spec)
	}
}
