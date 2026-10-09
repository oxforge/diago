package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun_Check(t *testing.T) {
	code, out, stderr := runCLI(t, vagueSpec, "check")
	assert.Equal(t, 0, code)
	assert.Equal(t, `vague-edge-label edges[0]: edge a->b label "uses" says little; prefer a specific verb phrase like "validates order" or "publishes event"`+"\n", out)
	assert.Empty(t, stderr)

	code, out, _ = runCLI(t, vagueSpec, "check", "-strict")
	assert.Equal(t, 1, code)
	assert.Contains(t, out, "vague-edge-label")

	code, out, _ = runCLI(t, twoNodeSpec, "check", "-strict")
	assert.Equal(t, 0, code)
	assert.Empty(t, out)

	// A whole-diagram finding has no field.
	noEntry := `{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":[{"from":"a","to":"b"},{"from":"b","to":"a"}]}`
	_, out, _ = runCLI(t, noEntry, "check")
	assert.True(t, strings.HasPrefix(out, "no-entry: flow has no clear entry"), out)

	// Path, and flags after the path.
	p := writeTemp(t, "v.json", vagueSpec)
	code, out, _ = runCLI(t, "", "check", p, "-strict")
	assert.Equal(t, 1, code)
	assert.Contains(t, out, "vague-edge-label")
}

func TestRun_CheckJSON(t *testing.T) {
	code, out, _ := runCLI(t, vagueSpec, "check", "-json")
	assert.Equal(t, 0, code)
	var got struct {
		Warnings []struct{ Rule, Message, Field string } `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	require.Len(t, got.Warnings, 1)
	assert.Equal(t, "vague-edge-label", got.Warnings[0].Rule)
	assert.Equal(t, "edges[0]", got.Warnings[0].Field)

	code, out, _ = runCLI(t, twoNodeSpec, "check", "-json")
	assert.Equal(t, 0, code)
	assert.Equal(t, "{\n  \"warnings\": []\n}\n", out)
}

func TestRun_CheckErrors(t *testing.T) {
	code, out, stderr := runCLI(t, `{"type":"flow","nodes":[],"edges":[{"from":"x","to":"y"}]}`, "check")
	assert.Equal(t, 1, code)
	assert.Empty(t, out)
	assert.Contains(t, stderr, `"type": "validation_error"`)

	code, _, stderr = runCLI(t, `{"type":"flow","nodes":[{"id":"a","label":"A"}],"edges":[],"ignore":["nope"]}`, "check")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, `"field": "ignore[0]"`)

	code, _, stderr = runCLI(t, `{`, "check")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, `"field": ""`)

	code, _, stderr = runCLI(t, "", "check", "a.json", "b.json")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "usage: diago check")
}

func TestRun_CheckStrictRejectsAnUnknownSpecTheme(t *testing.T) {
	code, _, stderr := runCLI(t, `{"type":"flow","theme":"nope","nodes":[{"id":"a","label":"A"}],"edges":[]}`, "check", "-strict")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, `"field": "theme"`)
}
