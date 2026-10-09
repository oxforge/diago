package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun_DiffMermaid(t *testing.T) {
	old := writeTemp(t, "old.mmd", "graph TD\n  a --> b\n")
	neu := writeTemp(t, "new.mmd", "graph TD\n  a --> b --> c([C])\n")
	code, out, stderr := runCLI(t, "", "diff", old, neu, "-format", "text")
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, out, "added: node C")
	assert.Contains(t, stderr, "warning: import (after) line 2: stadium \"c\" rendered as rounded\n")

	// a parse error on the before side
	broken := writeTemp(t, "broken.mmd", "graph TD\n  ???\n")
	code, _, stderr = runCLI(t, "", "diff", broken, neu)
	assert.Equal(t, 1, code)
	assert.Equal(t, "diago diff: line 2: expected a node reference near \"???\"\n", stderr)
}

func TestRun_DiffPrintsPrefixedAdvisories(t *testing.T) {
	old := writeTemp(t, "old.json", vagueSpec)
	nw := writeTemp(t, "new.json", `{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":[{"id":"e","from":"a","to":"b","label":"publishes event"}]}`)
	code, _, stderr := runCLI(t, "", "diff", old, nw, "-format", "text")
	require.Equal(t, 0, code)
	assert.Contains(t, stderr, "warning: edge-without-id before.edges[0]:")
	assert.Contains(t, stderr, "warning: vague-edge-label before.edges[0]:")
	assert.NotContains(t, stderr, "after.")
}

func TestRun_DiffSVGAndText(t *testing.T) {
	old, neu := writeTemp(t, "old.json", diffOld), writeTemp(t, "new.json", diffNew)
	code, out, stderr := runCLI(t, "", "diff", old, neu)
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, out, `class="added"`)
	code, out, stderr = runCLI(t, "", "diff", old, neu, "-format", "text")
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, out, "+ C")
	assert.Contains(t, out, "added: node C")
}

func TestRun_DiffErrors(t *testing.T) {
	old := writeTemp(t, "old.json", diffOld)
	seq := writeTemp(t, "seq.json", `{"type":"sequence","actors":[{"id":"a","label":"A"}],"interactions":[]}`)
	code, _, stderr := runCLI(t, "", "diff", old, seq)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, `"field": "after.type"`)
	// One path is its latest change in git; outside a repository that fails.
	code, _, stderr = runCLI(t, "", "diff", old)
	assert.Equal(t, 2, code)
	assert.True(t, strings.HasPrefix(stderr, "error: "), stderr)
	code, _, stderr = runCLI(t, "", "diff", old, "-")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "stdin")
	code, out, _ := runCLI(t, "", "diff", old, old, "-format", "text")
	assert.Equal(t, 0, code)
	assert.NotContains(t, out, "added:")

	// Flags before both paths still work.
	neu := writeTemp(t, "new.json", diffNew)
	code, out, stderr = runCLI(t, "", "diff", "-format", "text", old, neu)
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, out, "added: node C")

	// Surplus positionals are rejected, not silently dropped.
	code, _, stderr = runCLI(t, "", "diff", old, old, old)
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "usage: diago diff")

	// A bad flag after both paths prints the positional usage line.
	code, _, stderr = runCLI(t, "", "diff", old, neu, "-bogus")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "usage: diago diff")
}

func TestRun_DiffWithAnUnsupportedFormatIsAUsageError(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.json")
	require.NoError(t, os.WriteFile(p, []byte(twoNodeSpec), 0o644))
	code, _, stderr := runCLI(t, "", "diff", "-format", "bogus", p, p)
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "error: diff: unsupported format")
}

// A text-art diff draws no theme, but the theme the new spec names must load
// (exit 1, on after.theme), and an unknown -theme is a usage error (exit 2).
func TestRun_DiffTextChecksTheTheme(t *testing.T) {
	old, neu := writeTemp(t, "old.json", diffOld), writeTemp(t, "new.json", diffNew)
	themed := writeTemp(t, "themed.json", strings.Replace(diffNew, "{", `{"theme":"nope",`, 1))
	code, _, stderr := runCLI(t, "", "diff", "-format", "text", old, themed)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, `"field": "after.theme"`)
	code, _, stderr = runCLI(t, "", "diff", "-format", "text", "-theme", "nope", old, neu)
	assert.Equal(t, 2, code)
	assert.True(t, strings.HasPrefix(stderr, "error: "), stderr)
}
