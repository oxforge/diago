package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestRun_PreviousMermaid(t *testing.T) {
	prev := writeTemp(t, "prev.mmd", "graph TD\n  a --> b\n")
	code, out, stderr := runCLI(t, "graph TD\n  a --> b --> c\n", "render", "-previous", prev)
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, out, "<svg")
	prev = writeTemp(t, "prev2.mmd", "graph TD\n  a --> b\n  c([C])\n")
	_, _, stderr = runCLI(t, "graph TD\n  a --> b --> c\n", "render", "-previous", prev)
	assert.Contains(t, stderr, "warning: import (previous) line 3: stadium \"c\" rendered as rounded\n")
}

// TestRun_APreviousThatCannotBeReadComesFirst: the -previous file is read,
// and a Mermaid one translated, before the render checks the spec, so a
// file that is missing or does not translate is reported even beside a
// wrong spec.
func TestRun_APreviousThatCannotBeReadComesFirst(t *testing.T) {
	badField := `{"type":"flow","nodes":[{"id":"a","label":"A","shape":"octagon"}],"edges":[]}`
	missing := filepath.Join(t.TempDir(), "missing.json")
	code, _, stderr := runCLI(t, badField, "render", "-previous", missing)
	assert.Equal(t, 2, code, stderr)
	assert.True(t, strings.HasPrefix(stderr, "error: read -previous: "), stderr)
	broken := writeTemp(t, "prev.mmd", "graph TD\n  a -->\n")
	code, _, stderr = runCLI(t, badField, "render", "-previous", broken)
	assert.Equal(t, 1, code, stderr)
	assert.True(t, strings.HasPrefix(stderr, "diago render: line 2: "), stderr)
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
// laid out under it, and before the -previous file is read as one; an
// unknown -theme is an invocation error, exit 2, reported as the flag's,
// not as the -previous spec's; text art checks it too.
func TestRun_UnknownSpecTheme(t *testing.T) {
	spec := strings.Replace(twoNodeSpec, `{"type":"flow",`, `{"type":"flow","theme":"nope",`, 1)
	prev := writeTemp(t, "v1.json", twoNodeSpec)
	bad := writeTemp(t, "bad.txt", "nope")
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
		{"text", spec, []string{"render", "-format", "text", "-previous", prev}, 1, "theme", ""},
		{"text, bad previous", spec, []string{"render", "-format", "text", "-previous", bad}, 1, "theme", ""},
		{"svg, bad previous", spec, []string{"render", "-previous", bad}, 1, "theme", ""},
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

// TestRun_PreviousAfterTheSpec: the spec is checked before its -previous
// file is read as one, so a wrong spec, its fields or its format, is
// reported as a render without -previous reports it, whatever the file; the
// file's own error comes only for a sound spec.
func TestRun_PreviousAfterTheSpec(t *testing.T) {
	badField := `{"type":"flow","nodes":[{"id":"a","label":"A","shape":"octagon"}],"edges":[]}`
	bad := writeTemp(t, "bad.txt", "nope")
	badSpec := writeTemp(t, "v1.json", `{"type":"flow","nodes":[{"id":"a","label":"A"}],"edges":[{"from":"a","to":"zz"}]}`)
	for _, tt := range []struct {
		name, spec string
		flags      []string
	}{
		{"a field", badField, nil},
		{"a field in text", badField, []string{"-format", "text"}},
		{"a field and the theme flag", badField, []string{"-theme", "nope"}},
		{"the format", twoNodeSpec, []string{"-format", "bogus"}},
		{"an export of a class diagram", `{"type":"class","classes":[{"id":"a"}]}`, []string{"-format", "drawio"}},
		{"a class field", `{"type":"class","classes":[{"id":"a"}],"relations":[{"from":"a","to":"zz"}]}`, nil},
		{"a sequence field", `{"type":"sequence","actors":[{"id":"a","label":"A"}],"interactions":[{"from":"a","to":"zz","label":"m"}]}`, nil},
	} {
		for _, prev := range []string{bad, badSpec} {
			t.Run(tt.name+" "+filepath.Base(prev), func(t *testing.T) {
				wantCode, _, wantErr := runCLI(t, tt.spec, append([]string{"render"}, tt.flags...)...)
				require.NotEqual(t, 0, wantCode, "the spec alone is wrong")
				code, _, stderr := runCLI(t, tt.spec, append([]string{"render", "-previous", prev}, tt.flags...)...)
				assert.Equal(t, wantCode, code)
				assert.Equal(t, wantErr, stderr)
			})
		}
	}
	code, _, stderr := runCLI(t, twoNodeSpec, "render", "-previous", bad)
	require.Equal(t, 1, code, stderr)
	assert.Contains(t, stderr, `"field": "previous"`)
}

// TestRun_RenderExitCodes: what each kind of input does to the exit code and
// the stderr envelope of render: a spec that is valid draws and exits 0, a
// validation error, a spec that is not JSON or is empty among them, exits 1
// with a structured error.
func TestRun_RenderExitCodes(t *testing.T) {
	type fieldErr struct {
		Field   string `json:"field"`
		Message string `json:"message"`
	}
	for _, tt := range []struct {
		name    string
		input   string
		code    int
		drawn   []string // in stdout when the render succeeds
		field   string   // the field of a validation error
		message string
	}{
		{"flow", `{"type":"flow","direction":"DOWN","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":[{"from":"a","to":"b"}]}`,
			0, []string{`xmlns="http://www.w3.org/2000/svg"`, "A", "B"}, "", ""},
		{"sequence", `{"type":"sequence","actors":[{"id":"client","label":"Client"},{"id":"server","label":"Server"}],` +
			`"interactions":[{"from":"client","to":"server","label":"request"},{"from":"server","to":"client","label":"response","style":"dashed"}]}`,
			0, []string{`xmlns="http://www.w3.org/2000/svg"`, "Client", "Server", "request"}, "", ""},
		{"unknown shape", `{"type":"flow","nodes":[{"id":"a","label":"A","shape":"octagon"}]}`, 1, nil, "nodes[0].shape", ""},
		{"flat self-loop", `{"type":"flow","nodes":[{"id":"a","label":"A"}],"edges":[{"from":"a","to":"a","flat":true}]}`,
			1, nil, "edges[0].flat", "a flat edge cannot be a self-loop"},
		{"unknown actor", `{"type":"sequence","actors":[{"id":"a","label":"A"}],"interactions":[{"from":"a","to":"unknown_actor","label":"msg"}]}`,
			1, nil, "", ""},
		{"not JSON", "not json at all", 1, nil, "", "invalid JSON"},
		{"empty stdin", "", 1, nil, "", "invalid JSON"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			code, out, stderr := runCLI(t, tt.input, "render")
			require.Equal(t, tt.code, code, stderr)
			if tt.code == 0 {
				assert.True(t, strings.HasPrefix(out, "<?xml"))
				for _, want := range tt.drawn {
					assert.Contains(t, out, want)
				}
				return
			}
			assert.Empty(t, out)
			if tt.code == 2 {
				assert.True(t, strings.HasPrefix(stderr, "error: "), stderr)
				return
			}
			var env struct {
				Error struct {
					Type   string     `json:"type"`
					Errors []fieldErr `json:"errors"`
				} `json:"error"`
			}
			require.NoError(t, json.Unmarshal([]byte(stderr), &env), stderr)
			assert.Equal(t, "validation_error", env.Error.Type)
			require.NotEmpty(t, env.Error.Errors)
			if tt.field != "" {
				assert.Equal(t, tt.field, env.Error.Errors[0].Field)
			}
			assert.Contains(t, env.Error.Errors[0].Message, tt.message)
		})
	}
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
	// The spec's own missing-type error surfaces as a validation error on
	// "type", not as a -previous mismatch against a blank type.
	assert.Equal(t, 1, code)
	assert.Contains(t, errb.String(), "\"field\": \"type\"")
	assert.NotContains(t, errb.String(), "must be a  diagram")
}

// TestRun_RemovedStyleFlag: render and diff take no -style flag; each
// reports it as unknown, exit 2.
func TestRun_RemovedStyleFlag(t *testing.T) {
	spec := `{"type":"flow","nodes":[{"id":"a","label":"A"}],"edges":[]}`
	for _, args := range [][]string{{"render", "-style", "lawful"}, {"diff", "-style", "lawful", "a.json", "b.json"}} {
		code, out, stderr := runCLI(t, spec, args...)
		assert.Equal(t, 2, code, args)
		assert.Empty(t, out, args)
		assert.Contains(t, stderr, "flag provided but not defined: -style", args)
	}
}

// TestRender_RemovedFieldsIgnored pins end-to-end that a spec carrying the
// retired style/alignment/hints keys renders byte-identically to the same
// spec with those keys stripped, and only differs in the removed-field
// advisories on stderr.
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

func TestRun_UnknownThemeFlagFailsInTextToo(t *testing.T) {
	code, _, stderr := runCLI(t, twoNodeSpec, "render", "-format", "text", "-theme", "nope")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "error:")
}

// TestRun_FieldsBeforeTheTheme: a spec with a field error, rendered with a
// theme that does not load, the flag's or its own, reports the field error
// in every format.
func TestRun_FieldsBeforeTheTheme(t *testing.T) {
	for _, sp := range []struct {
		kind, spec, field string
		formats           []string
	}{
		{"flow", `{"type":"flow","nodes":[{"id":"a","label":"A","shape":"octagon"}],"edges":[]}`, "nodes[0].shape", []string{"svg", "text", "drawio"}},
		{"class", `{"type":"class","classes":[{"id":"a"}],"relations":[{"from":"a","to":"zz"}]}`, "relations[0].to", []string{"svg", "text"}},
		{"sequence", `{"type":"sequence","actors":[{"id":"a","label":"A"}],"interactions":[{"from":"a","to":"zz","label":"m"}]}`, "interactions[0].to", []string{"svg", "text"}},
	} {
		themed := strings.Replace(sp.spec, `{"type":"`+sp.kind+`",`, `{"type":"`+sp.kind+`","theme":"nope",`, 1)
		for _, tt := range []struct {
			name, spec string
			flags      []string
		}{
			{"the theme flag", sp.spec, []string{"-theme", "nope"}},
			{"the spec's theme", themed, nil},
		} {
			for _, format := range sp.formats {
				t.Run(sp.kind+" "+tt.name+" "+format, func(t *testing.T) {
					code, _, stderr := runCLI(t, tt.spec, append([]string{"render", "-format", format}, tt.flags...)...)
					require.Equal(t, 1, code, stderr)
					assert.Contains(t, stderr, `"field": "`+sp.field+`"`)
				})
			}
		}
	}
}

// A theme name with a path is rejected: the spec's on its theme field
// (exit 1), the -theme flag's as a usage error (exit 2). A spec whose
// theme is empty renders with the default.
func TestRun_AThemeNameWithAPathIsRejected(t *testing.T) {
	code, _, stderr := runCLI(t, `{"type":"flow","theme":"../x","nodes":[{"id":"a","label":"A"}],"edges":[]}`, "render")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, `"field": "theme"`)
	assert.Contains(t, stderr, "plain file name")
	code, _, stderr = runCLI(t, twoNodeSpec, "render", "-theme", "../x")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "plain file name")
	code, out, _ := runCLI(t, `{"type":"flow","theme":"","nodes":[{"id":"a","label":"A"}],"edges":[]}`, "render")
	assert.Equal(t, 0, code)
	assert.Contains(t, out, "<svg")
}

// The CLI's JSON keeps <, > and & as written: a validation error's message
// and check -json's findings.
func TestRun_JSONKeepsAngleBracketsAndAmpersands(t *testing.T) {
	code, _, stderr := runCLI(t, `{"type":"flow","nodes":[{"id":"a","label":"A"}],"edges":[{"from":"a","to":"List<T>"}]}`, "render")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, `List<T>`)
	assert.NotContains(t, stderr, `\u003c`)
	// The finding names the node by its id, which carries the &.
	code, out, _ := runCLI(t, `{"type":"flow","nodes":[{"id":"a&b","label":"A"},{"id":"c","label":"C"}],"edges":[]}`, "check", "-json")
	assert.Equal(t, 0, code)
	assert.Contains(t, out, `a&b`)
	assert.NotContains(t, out, `\u0026`)
}

// A spec's stray format and store keys are unknown fields: check reports
// each, render warns on each and still draws the spec, and both exit 0.
func TestRun_StrayFormatAndStoreKeysWarn(t *testing.T) {
	spec := `{"type":"flow","format":"png","store":false,"nodes":[{"id":"a","label":"A"}],"edges":[]}`
	code, out, _ := runCLI(t, spec, "check")
	assert.Equal(t, 0, code)
	assert.Contains(t, out, "unknown-field format")
	assert.Contains(t, out, "unknown-field store")
	code, out, stderr := runCLI(t, spec, "render")
	assert.Equal(t, 0, code)
	assert.Contains(t, out, "<svg")
	assert.Contains(t, stderr, "warning: unknown-field format")
	assert.Contains(t, stderr, "warning: unknown-field store")
}

// A direction must be one the schema lists: UNDEFINED is rejected on the
// direction field, while an empty or missing direction still means AUTO.
func TestRun_DirectionUndefinedIsRejected(t *testing.T) {
	code, _, stderr := runCLI(t, `{"type":"flow","direction":"UNDEFINED","nodes":[{"id":"a","label":"A"}],"edges":[]}`, "render")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, `"field": "direction"`)
	for _, spec := range []string{
		`{"type":"flow","direction":"","nodes":[{"id":"a","label":"A"}],"edges":[]}`,
		`{"type":"flow","nodes":[{"id":"a","label":"A"}],"edges":[]}`,
	} {
		code, out, _ := runCLI(t, spec, "render")
		assert.Equal(t, 0, code, spec)
		assert.Contains(t, out, "<svg", spec)
	}
}
