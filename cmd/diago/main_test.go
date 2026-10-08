package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

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
	for _, want := range []string{"Mermaid", ".mmd", "REV:PATH", "diago diff [flags] PATH", "diago diff [flags] OLD NEW"} {
		assert.Contains(t, out, want)
	}
	for verb, want := range map[string][]string{
		"render": {"usage: diago render [flags] [SPEC]", "Mermaid", "REV:PATH", "  -format string", "  -previous string"},
		"diff":   {"usage: diago diff [flags] PATH | OLD NEW", "Mermaid", "REV:PATH", "  -format string"},
		"check":  {"usage: diago check [flags] [SPEC]", "Mermaid", "REV:PATH", "  -strict"},
		"import": {"usage: diago import [SPEC]", "REV:PATH"},
	} {
		_, _, stderr := runCLI(t, "", verb, "-h")
		for _, w := range want {
			assert.Contains(t, stderr, w, verb)
		}
	}
}

const mmdFlow = "---\ntitle: Hello\n---\ngraph TD\n  a[Start] --> b([Stop])\n  a --- c\n"

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

func TestRun_RenderMermaid(t *testing.T) {
	// stdin, sniffed by header
	code, out, stderr := runCLI(t, mmdFlow, "render", "-format", "text")
	require.Equal(t, 0, code, stderr)
	assert.True(t, strings.HasPrefix(out, "   Hello\n\n") || strings.Contains(out, "Hello\n\n"), out)
	assert.Contains(t, out, "Start")
	// reports come first on stderr, before any warning or advisory line
	assert.True(t, strings.HasPrefix(stderr, "warning: import line 5: stadium \"b\" rendered as rounded\n"), stderr)

	// a .mmd path
	path := writeTemp(t, "d.mmd", mmdFlow)
	code, out, _ = runCLI(t, "", "render", path)
	require.Equal(t, 0, code)
	assert.Contains(t, out, ">Hello</text>")

	// a .mmd path whose content is not Mermaid is an error, not JSON
	bad := writeTemp(t, "x.mmd", twoNodeSpec)
	code, _, stderr = runCLI(t, "", "render", bad)
	assert.Equal(t, 1, code)
	assert.Equal(t, "diago render: line 1: not a Mermaid source\n", stderr)

	// a parse error from render
	code, _, stderr = runCLI(t, "graph TD\n  a --> b\n  ???\n", "render", "-format", "svg")
	assert.Equal(t, 1, code)
	assert.Equal(t, "diago render: line 3: expected a node reference near \"???\"\n", stderr)

	// check on a .mmd
	code, out, stderr = runCLI(t, "", "check", path)
	assert.Equal(t, 0, code)
	assert.Empty(t, out)
	assert.Contains(t, stderr, "warning: import line 5:")
}

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

func TestRun_PreviousMermaid(t *testing.T) {
	prev := writeTemp(t, "prev.mmd", "graph TD\n  a --> b\n")
	code, out, stderr := runCLI(t, "graph TD\n  a --> b --> c\n", "render", "-previous", prev)
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, out, "<svg")
	prev = writeTemp(t, "prev2.mmd", "graph TD\n  a --> b\n  c([C])\n")
	_, _, stderr = runCLI(t, "graph TD\n  a --> b --> c\n", "render", "-previous", prev)
	assert.Contains(t, stderr, "warning: import (previous) line 3: stadium \"c\" rendered as rounded\n")
}

const vagueSpec = `{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":[{"from":"a","to":"b","label":"uses"}]}`

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
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "error:")

	code, _, stderr = runCLI(t, "", "check", "a.json", "b.json")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "usage: diago check")
}

func TestRun_RenderPrintsAdvisories(t *testing.T) {
	for _, args := range [][]string{{"render"}, {"render", "-format", "text"}} {
		code, out, stderr := runCLI(t, vagueSpec, args...)
		require.Equal(t, 0, code)
		assert.NotEmpty(t, out)
		assert.Contains(t, stderr, "warning: vague-edge-label edges[0]: edge a->b label \"uses\" says little")
	}
	_, _, stderr := runCLI(t, twoNodeSpec, "render")
	assert.Empty(t, stderr)
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

func TestRun_Previous(t *testing.T) {
	_, first, _ := runCLI(t, twoNodeSpec, "render")
	prevPath := t.TempDir() + "/v1.svg"
	require.NoError(t, os.WriteFile(prevPath, []byte(first), 0o644))
	code, second, stderr := runCLI(t, twoNodeSpec, "render", "-previous", prevPath)
	require.Equal(t, 0, code, stderr)
	assert.Equal(t, first, second)

	// A bad previous is a structured validation error, exit 1.
	badPath := t.TempDir() + "/bad.txt"
	require.NoError(t, os.WriteFile(badPath, []byte("nope"), 0o644))
	code, _, stderr = runCLI(t, twoNodeSpec, "render", "-previous", badPath)
	assert.Equal(t, 1, code)
	var env struct {
		Error struct {
			Type   string              `json:"type"`
			Errors []map[string]string `json:"errors"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal([]byte(stderr), &env), stderr)
	assert.Equal(t, "validation_error", env.Error.Type)
	assert.Equal(t, "previous", env.Error.Errors[0]["field"])

	// Sequence: discarded with a warning, exit 0.
	seq := `{"type":"sequence","actors":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"interactions":[{"from":"a","to":"b","label":"hi"}]}`
	code, _, stderr = runCLI(t, seq, "render", "-previous", prevPath)
	assert.Equal(t, 0, code)
	assert.Contains(t, stderr, "warning: -previous ignored")
}

// TestRun_PreviousOnASequence: a sequence render discards its -previous
// with a warning whether it is the previous sequence spec or its SVG, in
// every format; a file that is neither is still a validation error.
func TestRun_PreviousOnASequence(t *testing.T) {
	seq := `{"type":"sequence","actors":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"interactions":[{"from":"a","to":"b","label":"hi"}]}`
	_, seqSVG, _ := runCLI(t, seq, "render")
	dir := t.TempDir()
	specPath, svgPath, badPath := dir+"/v1.json", dir+"/v1.svg", dir+"/bad.json"
	require.NoError(t, os.WriteFile(specPath, []byte(seq), 0o644))
	require.NoError(t, os.WriteFile(svgPath, []byte(seqSVG), 0o644))
	require.NoError(t, os.WriteFile(badPath, []byte(`{"type":"sequence","actors":[]}`), 0o644))
	for _, prev := range []string{specPath, svgPath} {
		for _, format := range []string{"svg", "text"} {
			code, out, stderr := runCLI(t, seq, "render", "-format", format, "-previous", prev)
			require.Equal(t, 0, code, "%s %s: %s", prev, format, stderr)
			assert.NotEmpty(t, out)
			assert.Equal(t, "warning: -previous ignored: sequence diagrams have no layout freedom\n", stderr)
		}
	}
	code, _, stderr := runCLI(t, seq, "render", "-previous", badPath)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, `"field": "previous"`)
}

// flatByProfileSpec has a flat edge, e6 (edges[6]), that the screen
// layout ranks and the text layout routes (S9, Flat edges).
const flatByProfileSpec = `{"type":"flow","direction":"DOWN","nodes":[{"id":"start","label":"Start","shape":"circle"},` +
	`{"id":"d1","label":"Valid?","shape":"diamond"},{"id":"d2","label":"Cached?","shape":"diamond"},{"id":"ok","label":"Serve"},` +
	`{"id":"err","label":"Reject"},{"id":"fetch","label":"Fetch"},{"id":"stop","label":"Stop","shape":"circle"}],` +
	`"edges":[{"from":"start","to":"d1","id":"e0"},{"from":"d1","to":"d2","label":"yes","id":"e1"},` +
	`{"from":"d1","to":"err","label":"no","id":"e2"},{"from":"d2","to":"ok","label":"yes","id":"e3"},` +
	`{"from":"d2","to":"fetch","label":"no","id":"e4"},{"from":"ok","to":"stop","id":"e5"},` +
	`{"from":"d1","to":"d2","label":"retry","flat":true,"id":"e6"},{"from":"err","to":"fetch","flat":true,"id":"e7"}]}`

// flatByThemeSpec has a flat edge, edges[0], that the default theme's
// screen layout ranks, n3 between its ends, and the sketch theme's routes.
const flatByThemeSpec = `{"type":"flow","direction":"DOWN","nodes":[{"id":"n1","label":"Node 1 long name","shape":"hexagon"},` +
	`{"id":"n3","label":"Node 3","shape":"hexagon"},{"id":"n8","label":"Node 8","shape":"rounded"}],` +
	`"edges":[{"from":"n1","to":"n8","flat":true}],"ignore":["isolated-node"]}`

// TestRun_PreviousSpecLaidOutAsTheRender: a -previous spec is laid out
// as the render lays out its own spec, under the text profile for a text
// render and under the screen profile of the render's theme otherwise
// (S13), -theme or else the spec's theme field, so a render anchored on its
// own spec draws what the fresh render draws and keeps only the fallbacks
// that layout has: none in text or under sketch, and on screen e6's, as a
// kept fallback.
func TestRun_PreviousSpecLaidOutAsTheRender(t *testing.T) {
	sketchField := strings.Replace(flatByThemeSpec, `{"type":"flow",`, `{"type":"flow","theme":"sketch",`, 1)
	for _, tt := range []struct {
		name, spec string
		flags      []string
		warn       string // the flat-edge-ranked line anchored, "" for none
	}{
		{"text", flatByProfileSpec, []string{"-format", "text"}, ""},
		{"sketch", flatByThemeSpec, []string{"-theme", "sketch"}, ""},
		{"sketch field", sketchField, nil, ""},
		{"screen", flatByProfileSpec, nil,
			"warning: flat-edge-ranked edges[6]: kept as an ordinary edge from the previous layout (-previous); a render without it tries a side route again\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			prev := writeTemp(t, "v1.json", tt.spec)
			code, fresh, _ := runCLI(t, tt.spec, append([]string{"render"}, tt.flags...)...)
			require.Equal(t, 0, code)
			code, anchored, stderr := runCLI(t, tt.spec, append([]string{"render", "-previous", prev}, tt.flags...)...)
			require.Equal(t, 0, code, stderr)
			assert.Equal(t, fresh, anchored, "anchored on its own spec, the render is the fresh one")
			var lines []string
			for _, line := range strings.SplitAfter(stderr, "\n") {
				if strings.Contains(line, "flat-edge-ranked") {
					lines = append(lines, line)
				}
			}
			assert.Equal(t, tt.warn, strings.Join(lines, ""))
		})
	}
}

// TestRun_UnknownSpecTheme: a theme the spec names that does not load is a
// validation error on its field, exit 1, with or without a -previous spec
// laid out under it; an unknown -theme is an invocation error, exit 2,
// reported as the flag's, not as the -previous spec's; text art takes no
// theme.
func TestRun_UnknownSpecTheme(t *testing.T) {
	spec := strings.Replace(twoNodeSpec, `{"type":"flow",`, `{"type":"flow","theme":"nope",`, 1)
	prev := writeTemp(t, "v1.json", twoNodeSpec)
	for _, tt := range []struct {
		name  string
		stdin string
		args  []string
		code  int
		field string
		msg   string // the whole of stderr, for an invocation error
	}{
		{"render", spec, []string{"render"}, 1, "theme", ""},
		{"previous spec", spec, []string{"render", "-previous", prev}, 1, "theme", ""},
		{"text", spec, []string{"render", "-format", "text", "-previous", prev}, 0, "", ""},
		{"flag", twoNodeSpec, []string{"render", "-theme", "nope", "-previous", prev}, 2, "", "error: theme \"nope\" not found\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			code, _, stderr := runCLI(t, tt.stdin, tt.args...)
			require.Equal(t, tt.code, code, stderr)
			if tt.msg != "" {
				assert.Equal(t, tt.msg, stderr)
			}
			if tt.field == "" {
				return
			}
			var env struct {
				Error struct {
					Errors []map[string]string `json:"errors"`
				} `json:"error"`
			}
			require.NoError(t, json.Unmarshal([]byte(stderr), &env), stderr)
			require.Len(t, env.Error.Errors, 1)
			assert.Equal(t, tt.field, env.Error.Errors[0]["field"])
		})
	}
}

// buildCLI builds the CLI binary and returns its path.
func buildCLI(t *testing.T) string {
	t.Helper()
	bin := t.TempDir() + "/diago"
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "failed to build CLI: %s", string(out))
	return bin
}

func TestCLIValidInput(t *testing.T) {
	bin := buildCLI(t)

	input := `{
		"type": "flow",
		"direction": "DOWN",
		"nodes": [
			{"id": "a", "label": "A"},
			{"id": "b", "label": "B"}
		],
		"edges": [{"from": "a", "to": "b"}]
	}`

	cmd := exec.Command(bin, "render")
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.Output()
	require.NoError(t, err, "CLI should exit 0 for valid input")

	svg := string(out)
	assert.True(t, strings.HasPrefix(svg, "<?xml"), "output should start with XML declaration")
	assert.Contains(t, svg, `xmlns="http://www.w3.org/2000/svg"`)
}

func TestCLIInvalidInput(t *testing.T) {
	bin := buildCLI(t)

	input := `{
		"type": "flow",
		"nodes": [{"id": "a", "label": "A", "shape": "octagon"}]
	}`

	cmd := exec.Command(bin, "render")
	cmd.Stdin = strings.NewReader(input)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	err := cmd.Run()

	// Should exit non-zero.
	require.Error(t, err)
	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok, "error should be ExitError")
	assert.Equal(t, 1, exitErr.ExitCode(), "validation error should exit with code 1")

	// Stderr should contain structured JSON error.
	var errResp struct {
		Error struct {
			Type   string `json:"type"`
			Errors []struct {
				Field   string `json:"field"`
				Message string `json:"message"`
			} `json:"errors"`
		} `json:"error"`
	}
	err = json.Unmarshal([]byte(stderr.String()), &errResp)
	require.NoError(t, err, "stderr should be valid JSON: %s", stderr.String())
	assert.Equal(t, "validation_error", errResp.Error.Type)
	assert.NotEmpty(t, errResp.Error.Errors, "should have at least one error")
}

func TestCLIFlatSelfLoopValidationError(t *testing.T) {
	bin := buildCLI(t)

	input := `{
		"type": "flow",
		"nodes": [{"id": "a", "label": "A"}],
		"edges": [{"from": "a", "to": "a", "flat": true}]
	}`

	cmd := exec.Command(bin, "render")
	cmd.Stdin = strings.NewReader(input)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	err := cmd.Run()

	require.Error(t, err)
	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok, "error should be ExitError")
	assert.Equal(t, 1, exitErr.ExitCode(), "validation error should exit with code 1")

	var errResp struct {
		Error struct {
			Type   string `json:"type"`
			Errors []struct {
				Field   string `json:"field"`
				Message string `json:"message"`
			} `json:"errors"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal([]byte(stderr.String()), &errResp), "stderr should be valid JSON: %s", stderr.String())
	assert.Equal(t, "validation_error", errResp.Error.Type)
	found := false
	for _, e := range errResp.Error.Errors {
		if e.Field == "edges[0].flat" {
			found = true
			assert.Contains(t, e.Message, "a flat edge cannot be a self-loop")
		}
	}
	assert.True(t, found, "expected an error on edges[0].flat, got %+v", errResp.Error.Errors)
}

func TestCLIInvalidJSON(t *testing.T) {
	bin := buildCLI(t)

	cmd := exec.Command(bin, "render")
	cmd.Stdin = strings.NewReader("not json at all")
	err := cmd.Run()
	require.Error(t, err)
	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok)
	assert.Equal(t, 2, exitErr.ExitCode(), "internal error should exit with code 2")
}

func TestCLIEmptyStdin(t *testing.T) {
	bin := buildCLI(t)

	cmd := exec.Command(bin, "render")
	cmd.Stdin = strings.NewReader("")
	err := cmd.Run()
	require.Error(t, err, "empty stdin should fail")
}

func TestCLISequenceDiagram(t *testing.T) {
	bin := buildCLI(t)

	input := `{
		"type": "sequence",
		"actors": [
			{"id": "client", "label": "Client"},
			{"id": "server", "label": "Server"}
		],
		"interactions": [
			{"from": "client", "to": "server", "label": "request"},
			{"from": "server", "to": "client", "label": "response", "style": "dashed"}
		]
	}`

	cmd := exec.Command(bin, "render")
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.Output()
	require.NoError(t, err, "CLI should exit 0 for valid sequence diagram")

	svg := string(out)
	assert.True(t, strings.HasPrefix(svg, "<?xml"), "output should start with XML declaration")
	assert.Contains(t, svg, `xmlns="http://www.w3.org/2000/svg"`)
	assert.Contains(t, svg, "Client", "actor label should appear in SVG")
	assert.Contains(t, svg, "Server", "actor label should appear in SVG")
	assert.Contains(t, svg, "request", "interaction label should appear in SVG")
}

func TestCLISequenceValidationError(t *testing.T) {
	bin := buildCLI(t)

	// Reference an actor that doesn't exist.
	input := `{
		"type": "sequence",
		"actors": [{"id": "a", "label": "A"}],
		"interactions": [{"from": "a", "to": "unknown_actor", "label": "msg"}]
	}`

	cmd := exec.Command(bin, "render")
	cmd.Stdin = strings.NewReader(input)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	err := cmd.Run()

	require.Error(t, err)
	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok)
	assert.Equal(t, 1, exitErr.ExitCode(), "validation error should exit with code 1")

	var errResp struct {
		Error struct {
			Type   string `json:"type"`
			Errors []struct {
				Field   string `json:"field"`
				Message string `json:"message"`
			} `json:"errors"`
		} `json:"error"`
	}
	jsonErr := json.Unmarshal([]byte(stderr.String()), &errResp)
	require.NoError(t, jsonErr, "stderr should be valid JSON: %s", stderr.String())
	assert.Equal(t, "validation_error", errResp.Error.Type)
	assert.NotEmpty(t, errResp.Error.Errors)
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

func TestRun_ClassRenderCheckAndPrevious(t *testing.T) {
	spec := `{"type":"class","classes":[{"id":"a"},{"id":"b"}],"relations":[{"from":"b","to":"a","kind":"inheritance"}]}`
	var out, errb bytes.Buffer
	code := run([]string{"render", "-format", "text"}, strings.NewReader(spec), &out, &errb)
	assert.Equal(t, 0, code, errb.String())
	assert.Contains(t, out.String(), "△")
	out.Reset()
	code = run([]string{"check"}, strings.NewReader(`{"type":"class","classes":[{"id":"a"},{"id":"b"}]}`), &out, &errb)
	assert.Equal(t, 0, code)
	assert.Contains(t, out.String(), "isolated-class classes[0]:")
	dir := t.TempDir()
	prev := filepath.Join(dir, "prev.json")
	require.NoError(t, os.WriteFile(prev, []byte(spec), 0o644))
	out.Reset()
	errb.Reset()
	code = run([]string{"render", "-previous", prev}, strings.NewReader(spec), &out, &errb)
	assert.Equal(t, 0, code, errb.String())
	assert.Contains(t, out.String(), "<svg")
	flowPrev := filepath.Join(dir, "flow.json")
	require.NoError(t, os.WriteFile(flowPrev, []byte(`{"type":"flow","nodes":[{"id":"a","label":"A"}],"edges":[]}`), 0o644))
	errb.Reset()
	code = run([]string{"render", "-previous", flowPrev}, strings.NewReader(spec), &out, &errb)
	assert.Equal(t, 1, code)
	assert.Contains(t, errb.String(), "a previous spec must be a class diagram")
	// A spec with no type of its own must report its own error, not a
	// -previous mismatch against a blank type.
	errb.Reset()
	out.Reset()
	code = run([]string{"render", "-previous", flowPrev}, strings.NewReader(`{"nodes":[]}`), &out, &errb)
	// The spec's own "unknown diagram type" error surfaces through the plain
	// error path (exit 2), not as a -previous mismatch against a blank type.
	assert.Equal(t, 2, code)
	assert.Contains(t, errb.String(), "unknown diagram type")
	assert.NotContains(t, errb.String(), "must be a  diagram")
}

func TestRender_StyleFlagRemoved(t *testing.T) {
	spec := `{"type":"flow","nodes":[{"id":"a","label":"A"}],"edges":[]}`
	code, _, stderr := runCLI(t, spec, "render", "-style", "lawful")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (unknown flag); stderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, "flag provided but not defined: -style") {
		t.Errorf("stderr = %q, want the flag package's unknown-flag message", stderr)
	}
}

func TestDiff_StyleFlagRemoved(t *testing.T) {
	code, _, stderr := runCLI(t, "", "diff", "-style", "lawful", "a.json", "b.json")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (unknown flag); stderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, "flag provided but not defined: -style") {
		t.Errorf("stderr = %q, want the flag package's unknown-flag message", stderr)
	}
}

// TestRender_RemovedFieldsIgnored pins the layered-engine Phase 0 promise
// end-to-end: a spec still carrying the removed style/alignment/hints keys
// renders byte-identically to the same spec with those keys stripped, and
// only differs in the removed-field advisories on stderr.
func TestRender_RemovedFieldsIgnored(t *testing.T) {
	tests := []struct {
		name       string
		withFields string
		stripped   string
		wantStderr string
	}{
		{
			name: "flow",
			withFields: `{"type":"flow","style":"neutral","alignment":"balanced",` +
				`"hints":[{"type":"below","node":"b","relative_to":"a"}],` +
				`"nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],` +
				`"edges":[{"from":"a","to":"b"}]}`,
			stripped: `{"type":"flow",` +
				`"nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],` +
				`"edges":[{"from":"a","to":"b"}]}`,
			wantStderr: "warning: removed-field alignment: alignment was removed; the field is ignored\n" +
				"warning: removed-field hints: layout hints were removed; the field is ignored\n" +
				"warning: removed-field style: diago is orthogonal-only; the field is ignored\n",
		},
		{
			name: "class",
			withFields: `{"type":"class","style":"lawful",` +
				`"classes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],` +
				`"relations":[{"from":"a","to":"b"}]}`,
			stripped: `{"type":"class",` +
				`"classes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],` +
				`"relations":[{"from":"a","to":"b"}]}`,
			wantStderr: "warning: removed-field style: diago is orthogonal-only; the field is ignored\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			codeWith, outWith, stderrWith := runCLI(t, tt.withFields, "render")
			require.Equal(t, 0, codeWith, stderrWith)
			codeStripped, outStripped, stderrStripped := runCLI(t, tt.stripped, "render")
			require.Equal(t, 0, codeStripped, stderrStripped)

			assert.Equal(t, outStripped, outWith, "removed fields must not change rendered output")
			assert.Equal(t, tt.wantStderr, stderrWith, "stderr must contain exactly the removed-field advisories")
			assert.NotContains(t, stderrStripped, "removed-field", "a spec without the removed keys must not warn about them")
		})
	}
}
