package scripts_test

import (
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
