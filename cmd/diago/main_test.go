package main

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const twoNodeSpec = `{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":[{"from":"a","to":"b"}]}`

func runCLI(t *testing.T, stdin string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errb strings.Builder
	code = run(args, strings.NewReader(stdin), &out, &errb)
	return code, out.String(), errb.String()
}

func TestRun_VerbRequired(t *testing.T) {
	code, out, stderr := runCLI(t, twoNodeSpec)
	assert.Equal(t, 2, code)
	assert.Empty(t, out, "no verb draws nothing")
	assert.True(t, strings.HasPrefix(stderr, "usage: diago <verb>"), stderr)

	code, out, stderr = runCLI(t, twoNodeSpec, "-format", "text")
	assert.Equal(t, 2, code)
	assert.Empty(t, out)
	assert.True(t, strings.HasPrefix(stderr, "diago: a verb is required\nusage: diago <verb>"), stderr)

	for _, h := range []string{"-h", "--help", "help"} {
		code, out, stderr = runCLI(t, "", h)
		assert.Equal(t, 0, code, h)
		assert.True(t, strings.HasPrefix(out, "usage: diago <verb>"), h)
		assert.Empty(t, stderr, h)
	}
}

func TestRun_UnknownVerb(t *testing.T) {
	code, _, stderr := runCLI(t, "", "frobnicate")
	assert.Equal(t, 2, code)
	assert.True(t, strings.HasPrefix(stderr, "diago: unknown command \"frobnicate\"\nusage: diago <verb>"), stderr)
	assert.Contains(t, stderr, "diago render [flags] [SPEC]")
	assert.Contains(t, stderr, "diago import [SPEC]")

	existing := writeTemp(t, "spec", twoNodeSpec)
	for _, arg := range []string{"x.json", "x.mmd", "x.MERMAID", existing} {
		code, _, stderr = runCLI(t, "", arg)
		assert.Equal(t, 2, code, arg)
		assert.Contains(t, stderr, "did you mean: diago render "+arg+"\n", arg)
	}
}

func TestRun_UsageNamesMermaidAndRevisions(t *testing.T) {
	_, out, _ := runCLI(t, "", "help")
	for _, want := range []string{"Mermaid", ".mmd", "first meaningful line", "REV:PATH", "diago diff [flags] PATH", "diago diff [flags] OLD NEW"} {
		assert.Contains(t, out, want)
	}
	for verb, want := range map[string][]string{
		"render": {"usage: diago render [flags] [SPEC]", "Mermaid", "REV:PATH", "  -format string", "  -previous string"},
		"diff":   {"usage: diago diff [flags] PATH | OLD NEW", "Mermaid", "REV:PATH", "  -format string"},
		"check":  {"usage: diago check [flags] [SPEC]", "Mermaid", "REV:PATH", "  -strict"},
		"import": {"usage: diago import [SPEC]", "REV:PATH"},
	} {
		code, out, _ := runCLI(t, "", verb, "-h")
		assert.Equal(t, 0, code, verb)
		for _, w := range want {
			assert.Contains(t, out, w, verb)
		}
	}
}

const mmdFlow = "---\ntitle: Hello\n---\ngraph TD\n  a[Start] --> b([Stop])\n  a --- c\n"

const vagueSpec = `{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":[{"from":"a","to":"b","label":"uses"}]}`

func TestRun_PositionalSpecPath(t *testing.T) {
	path := t.TempDir() + "/spec.json"
	require.NoError(t, os.WriteFile(path, []byte(twoNodeSpec), 0o644))
	code, out, _ := runCLI(t, "", "render", path)
	require.Equal(t, 0, code)
	assert.True(t, strings.HasPrefix(out, "<?xml"))
	code, _, stderr := runCLI(t, "", "render", path, "extra")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "at most one spec path")
	code, _, stderr = runCLI(t, "", "render", path, "-format", "text", "extra")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "at most one spec path")
}

// TestRun_FlagsAnywhere: every verb takes its flags before, between or
// after its paths, "-" is stdin wherever it stands, and "--" ends the
// flags.
func TestRun_FlagsAnywhere(t *testing.T) {
	path := writeTemp(t, "spec.json", twoNodeSpec)
	_, want, _ := runCLI(t, "", "render", "-format", "text", path)
	for name, args := range map[string][]string{
		"render, flag after":   {"render", path, "-format", "text"},
		"render, flags around": {"render", "-format", "text", path, "-theme", "dark"},
		"render, stdin first":  {"render", "-", "-format", "text"},
		"render, after --":     {"render", "-format", "text", "--", path},
	} {
		t.Run(name, func(t *testing.T) {
			code, out, stderr := runCLI(t, twoNodeSpec, args...)
			require.Equal(t, 0, code, stderr)
			assert.Equal(t, want, out)
		})
	}

	old, neu := writeTemp(t, "old.json", twoNodeSpec), writeTemp(t, "new.json", diffNew)
	code, out, stderr := runCLI(t, "", "diff", old, "-format", "text", neu)
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, out, "added: node C")

	vague := writeTemp(t, "v.json", vagueSpec)
	code, out, _ = runCLI(t, "", "check", "-json", vague, "-strict")
	assert.Equal(t, 1, code)
	assert.Contains(t, out, `"warnings"`)

	mmd := writeTemp(t, "d.mmd", "graph TD\n  a --> b\n")
	code, out, stderr = runCLI(t, "", "import", mmd, "--")
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, out, `"type": "flow"`)
}

// TestBinary_ExitCodes builds the binary once and runs it: the one thing the
// in-process runs above cannot see is main(), which hands run's code to
// os.Exit, so the exit status and the streams of a real process are checked
// here for a spec that draws and for one that does not validate.
func TestBinary_ExitCodes(t *testing.T) {
	bin := buildCLI(t)

	cmd := exec.Command(bin, "render")
	cmd.Stdin = strings.NewReader(twoNodeSpec)
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	require.NoError(t, cmd.Run(), stderr.String())
	assert.True(t, strings.HasPrefix(stdout.String(), "<?xml"))

	cmd = exec.Command(bin, "render")
	cmd.Stdin = strings.NewReader(`{"type":"flow","nodes":[{"id":"a","label":"A","shape":"octagon"}]}`)
	stdout.Reset()
	stderr.Reset()
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	var exitErr *exec.ExitError
	require.ErrorAs(t, cmd.Run(), &exitErr)
	assert.Equal(t, 1, exitErr.ExitCode())
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), `"validation_error"`)
}

// buildCLI builds the diago binary into a temporary directory and returns
// its path.
func buildCLI(t *testing.T) string {
	t.Helper()
	bin := t.TempDir() + "/diago"
	out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput()
	require.NoError(t, err, "failed to build CLI: %s", out)
	return bin
}

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	return p
}

const diffOld = `{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":[{"from":"a","to":"b"}]}
`
const diffNew = `{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"},{"id":"c","label":"C"}],"edges":[{"from":"a","to":"b"},{"from":"b","to":"c"}]}
`

// shortDeadline runs the test with a deadline already past when a render starts.
func shortDeadline(t *testing.T) {
	t.Helper()
	old := renderTimeout
	renderTimeout = time.Nanosecond
	t.Cleanup(func() { renderTimeout = old })
}

func TestRun_ARenderPastItsDeadlineExits2(t *testing.T) {
	shortDeadline(t)
	code, out, stderr := runCLI(t, twoNodeSpec, "render")
	assert.Equal(t, 2, code)
	assert.Empty(t, out)
	assert.Contains(t, stderr, "took longer than")
}

func TestRun_ADiffPastItsDeadlineExits2(t *testing.T) {
	shortDeadline(t)
	old, neu := writeTemp(t, "old.json", diffOld), writeTemp(t, "new.json", diffNew)
	code, out, stderr := runCLI(t, "", "diff", old, neu)
	assert.Equal(t, 2, code)
	assert.Empty(t, out)
	assert.Contains(t, stderr, "took longer than")
}

// slowReader is a stdin that waits delay before its first read.
type slowReader struct {
	r     io.Reader
	delay time.Duration
	slept bool
}

func (s *slowReader) Read(p []byte) (int, error) {
	if !s.slept {
		time.Sleep(s.delay)
		s.slept = true
	}
	return s.r.Read(p)
}

// TestRun_TheDeadlineStartsAfterTheRead: reading the spec spends none of
// the render's time, so a spec read slower than the deadline still renders.
func TestRun_TheDeadlineStartsAfterTheRead(t *testing.T) {
	old := renderTimeout
	renderTimeout = 500 * time.Millisecond
	t.Cleanup(func() { renderTimeout = old })
	var out, errb strings.Builder
	stdin := &slowReader{r: strings.NewReader(twoNodeSpec), delay: 600 * time.Millisecond}
	code := run([]string{"render", "-format", "text"}, stdin, &out, &errb)
	require.Equal(t, 0, code, errb.String())
	assert.NotEmpty(t, out.String())
}

func TestRun_CheckHasNoDeadline(t *testing.T) {
	shortDeadline(t)
	code, _, _ := runCLI(t, twoNodeSpec, "check")
	assert.Equal(t, 0, code)
}

// failWriter is a stdout whose every write fails, as on a full disk.
type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("no space left on device") }

// A verb whose output cannot be written exits 2 and says so, whatever it
// would have returned: a check -strict that found something included.
func TestRun_AFailedStdoutWriteExits2(t *testing.T) {
	old, neu := writeTemp(t, "old.json", diffOld), writeTemp(t, "new.json", diffNew)
	isolated := `{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":[]}`
	for _, tc := range []struct {
		name, stdin string
		args        []string
	}{
		{"render", twoNodeSpec, []string{"render"}},
		{"diff", "", []string{"diff", old, neu}},
		{"check -json", twoNodeSpec, []string{"check", "-json"}},
		{"check -strict", isolated, []string{"check", "-strict"}},
		{"import", "flowchart LR\n  a --> b\n", []string{"import"}},
		{"help", "", []string{"-h"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var errb strings.Builder
			code := run(tc.args, strings.NewReader(tc.stdin), failWriter{}, &errb)
			assert.Equal(t, 2, code)
			assert.Contains(t, errb.String(), "error: write output: no space left on device")
		})
	}
}
