package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/source/sourcetest"
)

func TestRun_DiffLatestChange(t *testing.T) {
	r := sourcetest.New(t)
	c1 := r.Commit("v1", map[string]string{"arch.json": diffOld})
	r.Commit("v2", map[string]string{"arch.json": diffNew})
	code, out, stderr := runCLI(t, "", "diff", r.Path("arch.json"), "-format", "text")
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, out, "\n\narch.json @ "+c1[:7]+" → arch.json\nadded: ")
	assert.Contains(t, out, "added: node C")
}

func TestRun_DiffRevisions(t *testing.T) {
	r := sourcetest.New(t)
	c1 := r.Commit("v1", map[string]string{"docs/arch.json": diffOld})
	c2 := r.Commit("v2", map[string]string{"docs/arch.json": diffNew})
	t.Chdir(r.Path("docs"))
	code, out, stderr := runCLI(t, "", "diff", "HEAD~1:arch.json", "HEAD:arch.json", "-format", "text")
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, out, "arch.json @ HEAD~1 ("+c1[:7]+") → arch.json @ HEAD ("+c2[:7]+")\n")
	assert.Contains(t, out, "added: node C")

	// A revision against the file on disk, which went back to v1.
	r.Write(map[string]string{"docs/arch.json": diffOld})
	code, out, stderr = runCLI(t, "", "diff", "HEAD:arch.json", "arch.json", "-format", "text")
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, out, "arch.json @ HEAD ("+c2[:7]+") → arch.json\n")
	assert.Contains(t, out, "removed: node C")
}

func TestRun_DiffCaptionNamesBaseNames(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "old.json"), []byte(diffOld), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "new.json"), []byte(diffNew), 0o644))
	_, abs, _ := runCLI(t, "", "diff", filepath.Join(dir, "old.json"), filepath.Join(dir, "new.json"), "-format", "text")
	t.Chdir(dir)
	_, rel, _ := runCLI(t, "", "diff", "old.json", "./new.json", "-format", "text")
	assert.Equal(t, abs, rel, "the caption does not depend on how the paths are spelled")
	assert.Contains(t, abs, "\n\nold.json → new.json\n")
	_, svg, _ := runCLI(t, "", "diff", "old.json", "new.json")
	assert.Contains(t, svg, ">old.json → new.json</text>")
}

func TestRun_RenderCheckAndImportAtARevision(t *testing.T) {
	r := sourcetest.New(t)
	r.Commit("v1", map[string]string{"d.mmd": "graph TD\n  a[Start] --> b[Stop]\n"})
	r.Write(map[string]string{"d.mmd": "no longer Mermaid"})
	arg := "HEAD:" + r.Path("d.mmd")
	code, out, stderr := runCLI(t, "", "render", "-format", "text", arg)
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, out, "Start")
	code, out, stderr = runCLI(t, "", "import", arg)
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, out, `"type": "flow"`)
	code, _, stderr = runCLI(t, "", "check", arg)
	assert.Equal(t, 0, code, stderr)
}

func TestRun_PreviousAtARevision(t *testing.T) {
	r := sourcetest.New(t)
	r.Commit("v1", map[string]string{"x.json": twoNodeSpec})
	path := r.Path("x.json")
	_, want, _ := runCLI(t, "", "render", "-previous", path, path)
	code, got, stderr := runCLI(t, "", "render", "-previous", "HEAD:"+path, path)
	require.Equal(t, 0, code, stderr)
	assert.Equal(t, want, got)

	code, _, stderr = runCLI(t, twoNodeSpec, "render", "-previous", "-")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "error: -previous does not read stdin")
}

func TestRun_GitErrors(t *testing.T) {
	r := sourcetest.New(t)
	r.Commit("v1", map[string]string{"x.json": twoNodeSpec})
	outside := writeTemp(t, "z.json", twoNodeSpec)
	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"render, unknown revision":  {[]string{"render", "nope:" + r.Path("x.json")}, `error: read spec: unknown revision "nope"`},
		"check, missing at HEAD":    {[]string{"check", "HEAD:" + r.Path("y.json")}, "y.json does not exist at HEAD ("},
		"diff, no earlier version":  {[]string{"diff", r.Path("x.json")}, "x.json has no earlier version in git to compare with"},
		"diff, not in a repository": {[]string{"diff", outside}, "z.json is not in a git repository"},
		"diff, unknown old":         {[]string{"diff", "nope:" + r.Path("x.json"), r.Path("x.json")}, `error: read old: unknown revision "nope"`},
	} {
		t.Run(name, func(t *testing.T) {
			code, out, stderr := runCLI(t, "", tc.args...)
			assert.Equal(t, 2, code)
			assert.Empty(t, out)
			assert.Contains(t, stderr, tc.want)
		})
	}
}

// TestRun_DiffPatchIsNotASpec: the unified-diff second argument is gone; a
// patch file now fails as the non-spec it is.
func TestRun_DiffPatchIsNotASpec(t *testing.T) {
	old := writeTemp(t, "spec.json", diffOld)
	patch := writeTemp(t, "change.patch", "--- a/spec.json\n+++ b/spec.json\n@@ -1,1 +1,1 @@\n-x\n+y\n")
	code, _, stderr := runCLI(t, "", "diff", old, patch, "-format", "text")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, `"field": "after"`)
	assert.Contains(t, stderr, "invalid JSON")
}

// TestRun_DiffEmptyArgument: an empty argument (an unset shell variable,
// diago diff "$OLD" new.json) is refused, not read as stdin.
func TestRun_DiffEmptyArgument(t *testing.T) {
	neu := writeTemp(t, "new.json", diffNew)
	for _, args := range [][]string{{"diff", "", neu}, {"diff", neu, ""}, {"diff", ""}} {
		code, out, stderr := runCLI(t, "", args...)
		assert.Equal(t, 2, code, args)
		assert.Empty(t, out, args)
		assert.Contains(t, stderr, "diago diff: an empty argument is not a spec", args)
	}
}

// TestRun_DiffOneRevisionArgument: one REV:PATH argument is not a file on
// disk; the error says how to compare that revision with the file.
func TestRun_DiffOneRevisionArgument(t *testing.T) {
	r := sourcetest.New(t)
	r.Commit("v1", map[string]string{"arch.json": diffOld})
	t.Chdir(r.Dir)
	code, out, stderr := runCLI(t, "", "diff", "HEAD~1:arch.json")
	assert.Equal(t, 2, code)
	assert.Empty(t, out)
	assert.Contains(t, stderr, "diago diff HEAD~1:arch.json arch.json")
}
