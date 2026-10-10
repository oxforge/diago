package scripts_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setup builds this checkout's diago into a temp dir behind a wrapper that
// logs every call's arguments, one line per call, and returns the PATH to
// run the script with and the log's path.
func setup(t *testing.T) (path, log string) {
	t.Helper()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not on PATH")
	}
	bin := t.TempDir()
	real := filepath.Join(bin, "diago-real")
	build := exec.Command("go", "build", "-o", real, "github.com/oxforge/diago/cmd/diago")
	out, err := build.CombinedOutput()
	require.NoError(t, err, "%s", out)
	log = filepath.Join(bin, "calls.log")
	wrapper := "#!" + bash + "\necho \"$*\" >> " + log + "\nexec " + real + " \"$@\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(bin, "diago"), []byte(wrapper), 0o755))
	return bin + string(os.PathListSeparator) + os.Getenv("PATH"), log
}

// TestDiagoRender_AnchorsFlowButNotSequence: a second version of a sequence
// diagram renders under --strict, with no -previous (diago would only warn
// that it ignores one), while a flow diagram's second version is still
// anchored on its first.
func TestDiagoRender_AnchorsFlowButNotSequence(t *testing.T) {
	path, log := setup(t)
	dir := t.TempDir()
	specs := map[string]string{
		"seq.v1.json":  `{"type":"sequence","actors":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"interactions":[{"from":"a","to":"b","label":"hi"}]}`,
		"seq.v2.json":  `{"type":"sequence","actors":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"interactions":[{"from":"a","to":"b","label":"hi"},{"from":"b","to":"a","label":"ok"}]}`,
		"flow.v1.json": `{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":[{"id":"ab","from":"a","to":"b"}]}`,
		"flow.v2.json": `{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"},{"id":"c","label":"C"}],"edges":[{"id":"ab","from":"a","to":"b"},{"id":"bc","from":"b","to":"c"}]}`,
	}
	for name, spec := range specs {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(spec), 0o644))
	}
	cmd := exec.Command("bash", "diago-render", dir, "--strict")
	cmd.Env = append(os.Environ(), "PATH="+path)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)

	calls, err := os.ReadFile(log)
	require.NoError(t, err)
	var seqRenders, flowRenders int
	for _, call := range strings.Split(strings.TrimSpace(string(calls)), "\n") {
		if !strings.HasPrefix(call, "render ") {
			continue
		}
		switch {
		case strings.HasSuffix(call, "seq.v2.json"):
			seqRenders++
			assert.NotContains(t, call, "-previous", "a sequence render is not anchored")
		case strings.HasSuffix(call, "flow.v2.json"):
			flowRenders++
			assert.Contains(t, call, "-previous "+filepath.Join(dir, "flow.v1.json"), "a flow render is anchored on its previous spec")
		}
	}
	assert.Equal(t, 2, seqRenders, "text and image renders of seq.v2")
	assert.Equal(t, 2, flowRenders, "text and image renders of flow.v2")
	for _, f := range []string{"seq.v2.txt", "seq.v2.svg", "seq.v1-v2.diff.txt", "flow.v2.svg", "flow.v1-v2.diff.svg"} {
		assert.FileExists(t, filepath.Join(dir, f))
	}
}

// TestDiagoRender_DiffWidthSkipsTheCaption: a diff's caption names both spec
// files; it is not art, so a long slug does not widen diff-width.
func TestDiagoRender_DiffWidthSkipsTheCaption(t *testing.T) {
	path, _ := setup(t)
	dir := t.TempDir()
	slug := "a-slug-long-enough-to-make-the-caption-the-widest-line"
	specs := map[string]string{
		slug + ".v1.json": `{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":[{"id":"ab","from":"a","to":"b"}]}`,
		slug + ".v2.json": `{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"},{"id":"c","label":"C"}],"edges":[{"id":"ab","from":"a","to":"b"},{"id":"bc","from":"b","to":"c"}]}`,
	}
	for name, spec := range specs {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(spec), 0o644))
	}
	cmd := exec.Command("bash", "diago-render", dir)
	cmd.Env = append(os.Environ(), "PATH="+path)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)

	caption := slug + ".v1.json → " + slug + ".v2.json"
	diff, err := os.ReadFile(filepath.Join(dir, slug+".v1-v2.diff.txt"))
	require.NoError(t, err)
	assert.Contains(t, string(diff), "\n"+caption+"\n", "the diff names its two versions")
	m := regexp.MustCompile(`diff-width=(\d+)`).FindStringSubmatch(string(out))
	require.NotNil(t, m, "%s", out)
	w, err := strconv.Atoi(m[1])
	require.NoError(t, err)
	assert.Less(t, w, utf8.RuneCountInString(caption), "the caption is not measured as art")
}

// script runs diago-render with args under path and returns its combined
// output and its exit code.
func script(t *testing.T, path string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command("bash", append([]string{"diago-render"}, args...)...)
	cmd.Env = append(os.Environ(), "PATH="+path)
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return string(out), exit.ExitCode()
	}
	require.NoError(t, err, "%s", out)
	return string(out), 0
}

// embedFixture writes doc as doc.md and a flow diagram's two versions into
// the diagrams folder beside it, and returns both paths.
func embedFixture(t *testing.T, doc string) (docPath, dir string) {
	t.Helper()
	root := t.TempDir()
	dir = filepath.Join(root, "diagrams")
	require.NoError(t, os.Mkdir(dir, 0o755))
	specs := map[string]string{
		"flow.v1.json": `{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":[{"id":"ab","from":"a","to":"b"}]}`,
		"flow.v2.json": `{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"},{"id":"c","label":"C"}],"edges":[{"id":"ab","from":"a","to":"b"},{"id":"bc","from":"b","to":"c"}]}`,
	}
	for name, spec := range specs {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(spec), 0o644))
	}
	docPath = filepath.Join(root, "doc.md")
	require.NoError(t, os.WriteFile(docPath, []byte(doc), 0o644))
	return docPath, dir
}

// assertNoScratch asserts that a run left none of its scratch files
// behind, beside the document or in the diagrams folder.
func assertNoScratch(t *testing.T, docPath, dir string) {
	t.Helper()
	for _, pattern := range []string{docPath + ".diago.tmp*", filepath.Join(dir, "*.tmp"), filepath.Join(dir, ".diago-body.tmp")} {
		left, err := filepath.Glob(pattern)
		require.NoError(t, err)
		assert.Empty(t, left, "scratch files matching %s", pattern)
	}
}

// TestDiagoRender_EmbedFillsEveryMarker: --embed replaces what lies between
// each begin and end marker with the rendered text art in a fence and a
// link to the image, the latest version for "slug" and the diff for
// "slug.diff", and leaves every other line of the document as it was. A
// second run changes nothing.
func TestDiagoRender_EmbedFillsEveryMarker(t *testing.T) {
	path, _ := setup(t)
	docPath, dir := embedFixture(t, "# Design\n\nIntro.\n\n"+
		"<!-- diago:begin flow -->\na stale body\nof two lines\n<!-- diago:end flow -->\n\n"+
		"Between.\n\n"+
		"<!-- diago:begin flow.diff -->\n<!-- diago:end flow.diff -->\n\nOutro.\n")
	out, code := script(t, path, dir, "--embed", docPath)
	require.Zero(t, code, "%s", out)

	body := func(base string) string {
		art, err := os.ReadFile(filepath.Join(dir, base+".txt"))
		require.NoError(t, err)
		require.True(t, strings.HasSuffix(string(art), "\n"), "the art ends its last line")
		return "```text\n" + string(art) + "```\n[" + base + ".svg](diagrams/" + base + ".svg)\n"
	}
	want := "# Design\n\nIntro.\n\n" +
		"<!-- diago:begin flow -->\n" + body("flow.v2") + "<!-- diago:end flow -->\n\n" +
		"Between.\n\n" +
		"<!-- diago:begin flow.diff -->\n" + body("flow.v1-v2.diff") + "<!-- diago:end flow.diff -->\n\nOutro.\n"
	got, err := os.ReadFile(docPath)
	require.NoError(t, err)
	assert.Equal(t, want, string(got))
	assertNoScratch(t, docPath, dir)

	out, code = script(t, path, dir, "--embed", docPath)
	require.Zero(t, code, "%s", out)
	again, err := os.ReadFile(docPath)
	require.NoError(t, err)
	assert.Equal(t, string(got), string(again), "a second run changes nothing")
}

// TestDiagoRender_EmbedLeavesTheDocumentUntouchedOnARefusal: a begin
// marker without its end marker, which would swallow the rest of the
// document, or a marker with no rendered files fails the run with exit 1
// and leaves the document byte for byte as it was, also when a marker
// before it was filled already.
func TestDiagoRender_EmbedLeavesTheDocumentUntouchedOnARefusal(t *testing.T) {
	path, _ := setup(t)
	for _, tc := range []struct {
		name, doc, want string
	}{
		{"an unterminated marker",
			"Intro.\n\n<!-- diago:begin flow -->\nthe rest of the document\n",
			"marker 'flow' in %s has no matching <!-- diago:end flow --> line; document left untouched"},
		{"a misspelled end marker",
			"<!-- diago:begin flow -->\n<!-- diago:end flwo -->\nOutro.\n",
			"marker 'flow' in %s has no matching <!-- diago:end flow --> line; document left untouched"},
		{"an unterminated marker after a filled one",
			"<!-- diago:begin flow -->\n<!-- diago:end flow -->\n\n<!-- diago:begin flow.diff -->\nOutro.\n",
			"marker 'flow.diff' in %s has no matching <!-- diago:end flow.diff --> line; document left untouched"},
		{"a marker without rendered files",
			"<!-- diago:begin flow -->\n<!-- diago:end flow -->\n\n<!-- diago:begin ghost -->\n<!-- diago:end ghost -->\n",
			"marker 'ghost' in %s has no rendered files in "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			docPath, dir := embedFixture(t, tc.doc)
			out, code := script(t, path, dir, "--embed", docPath)
			assert.Equal(t, 1, code, "%s", out)
			assert.Contains(t, out, "diago-render: "+fmt.Sprintf(tc.want, docPath))
			got, err := os.ReadFile(docPath)
			require.NoError(t, err)
			assert.Equal(t, tc.doc, string(got), "the document is untouched")
			assertNoScratch(t, docPath, dir)
		})
	}
}

// TestDiagoRender_StrictFailsOnALintWarning: diago's advisories on a
// diagram are relayed as lint lines and the run succeeds; under --strict
// the same run fails with exit 1.
func TestDiagoRender_StrictFailsOnALintWarning(t *testing.T) {
	path, _ := setup(t)
	dir := t.TempDir()
	spec := filepath.Join(dir, "lint.v1.json")
	require.NoError(t, os.WriteFile(spec, []byte(`{"type":"flow","nodes":[{"id":"a","label":"A","colour":"red"}],"edges":[]}`), 0o644))
	lint := "diago-render: lint lint.v1: unknown-field nodes[0].colour: "

	out, code := script(t, path, dir)
	assert.Zero(t, code, "%s", out)
	assert.Contains(t, out, lint)
	assert.Contains(t, out, "lint v1 width=")

	out, code = script(t, path, dir, "--strict")
	assert.Equal(t, 1, code, "%s", out)
	assert.Contains(t, out, lint)
	assert.Contains(t, out, "diago-render: lint warnings in "+spec+" (--strict)")
	assert.NotContains(t, out, " ok\n", "no diagram reports ok")
}
