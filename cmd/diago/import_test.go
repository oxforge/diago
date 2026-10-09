package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun_ImportVerb(t *testing.T) {
	code, out, stderr := runCLI(t, mmdFlow, "import")
	require.Equal(t, 0, code, stderr)
	assert.True(t, strings.HasPrefix(out, "{\n  \"type\": \"flow\",\n  \"title\": \"Hello\",\n"), out)
	assert.Contains(t, out, `"direction": "none"`)
	assert.True(t, strings.HasSuffix(out, "}\n"))
	assert.Equal(t, "warning: import line 5: stadium \"b\" rendered as rounded\n", stderr)

	// a path argument, and the JSON is a valid spec for check
	path := writeTemp(t, "d.mmd", mmdFlow)
	code, out2, _ := runCLI(t, "", "import", path)
	require.Equal(t, 0, code)
	assert.Equal(t, out, out2)
	code, _, stderr = runCLI(t, out, "check")
	assert.Equal(t, 0, code, stderr)

	// JSON input is refused
	code, out, stderr = runCLI(t, twoNodeSpec, "import")
	assert.Equal(t, 1, code)
	assert.Empty(t, out)
	assert.Equal(t, "diago import: line 1: not a Mermaid source\n", stderr)

	// a rejected line
	code, _, stderr = runCLI(t, "graph TD\n  a --> b\n  classDef x fill:#fff\n", "import")
	assert.Equal(t, 1, code)
	assert.Equal(t, "diago import: line 3: \"classDef\" is not supported by diago's flowchart subset\n", stderr)

	// usage
	code, _, stderr = runCLI(t, "", "import", "a.mmd", "b.mmd")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "usage: diago import [SPEC]")
}
