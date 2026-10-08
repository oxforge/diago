// Command diago reads a diagram spec and writes the rendered diagram to
// stdout. The diagram type is read from the spec's "type" field: "flow",
// "sequence" or "class".
//
// Usage:
//
//	diago render [flags] [SPEC]
//	diago diff [flags] PATH
//	diago diff [flags] OLD NEW
//	diago check [-strict] [-json] [SPEC]
//	diago import [SPEC]
//
// Every call names its verb: "diago", "diago spec.json" and "diago -format
// text spec.json" print the usage on stderr and exit 2, while "diago -h",
// "diago --help" and "diago help" print it on stdout and exit 0. Every verb
// takes its flags before, between or after its arguments, and "--" ends the
// flags.
//
// A SPEC is a path; "-" or nothing for stdin (render, check and import); or
// REV:PATH, the file as it was at a git revision (internal/source): a name
// that exists on disk is a file, and any other name with a colon is
// REV:PATH, PATH relative to the current directory. Every SPEC may be JSON or
// a Mermaid source: a path ending in .mmd or .mermaid, or a body whose first
// meaningful line is a "graph"/"flowchart", "sequenceDiagram" or
// "classDiagram" header, is translated by internal/mermaid before parsing.
// This holds for render, check, import, both diff arguments and -previous.
// Lossy translations print "warning: import line <n>: <message>" on stderr;
// a rejected line prints "diago <verb>: line <n>: <message>" and exits 1.
//
//	go run ./cmd/diago render spec.json
//	go run ./cmd/diago render diagram.mmd
//	go run ./cmd/diago render -previous v1.svg v2.json
//	go run ./cmd/diago diff arch.json
//	go run ./cmd/diago diff HEAD~1:arch.json arch.json -format text
//
// "diff" renders the union of two specs of the same diagram type, each
// element marked added, removed, changed or unchanged, with a change list
// whose first line names both sides. With one PATH it diffs the file's
// latest change in git: the newest committed version that differs from the
// file on disk, against the file on disk. Neither argument accepts stdin.
//
// "check" parses the spec and prints one advisory per line, "<rule> <field>:
// <message>" (no field for whole-diagram findings), exit 0; -strict exits 1
// when any finding remains after the spec's "ignore" list; -json prints
// {"warnings": [...]} instead. It never lays out, so text-label-dropped
// surfaces on render only. Every render and diff prints the same lines on
// stderr prefixed "warning: ".
//
// "import" prints the translated JSON spec on stdout and the importer's
// warnings on stderr; a JSON or otherwise non-Mermaid input is refused
// (exit 1).
//
// On validation error, a structured JSON error is printed to stderr and the
// process exits with code 1. On a usage, read, git or internal error, a
// plain message is printed to stderr and the process exits with code 2.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/oxforge/diago/internal/layoutdbg"
	"github.com/oxforge/diago/internal/mermaid"
	"github.com/oxforge/diago/internal/model"
	"github.com/oxforge/diago/internal/pipeline"
	pngrender "github.com/oxforge/diago/internal/render/png"
	"github.com/oxforge/diago/internal/schema"
	"github.com/oxforge/diago/internal/source"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// specHelp says what a SPEC argument may be; the top-level usage and every
// verb's usage print it.
const specHelp = `SPEC is a JSON spec or a Mermaid source: a .mmd or .mermaid path, or a body
whose first line is a flowchart, graph, sequenceDiagram or classDiagram header.
REV:PATH reads PATH as it was at a git revision, e.g. HEAD~1:arch.json.
`

// usage is the top-level usage: "diago -h" prints it on stdout, a missing or
// unknown verb on stderr.
const usage = `usage: diago <verb> [flags] [args]

  diago render [flags] [SPEC]    draw a diagram to stdout
  diago diff [flags] PATH        draw the latest change of a file in git
  diago diff [flags] OLD NEW     draw what changed between two specs
  diago check [flags] [SPEC]     print advisories without drawing
  diago import [SPEC]            translate Mermaid into diago JSON

` + specHelp + `Without SPEC, or with -, render, check and import read stdin.
Run "diago <verb> -h" for a verb's flags.
`

// run dispatches the verb and returns the exit code: 0 ok, 1 validation
// error (structured JSON on stderr), 2 usage, read, git or internal error.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "render":
		return runRender(args[1:], stdin, stdout, stderr)
	case "diff":
		return runDiff(args[1:], stdout, stderr)
	case "check":
		return runCheck(args[1:], stdin, stdout, stderr)
	case "import":
		return runImport(args[1:], stdin, stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return 0
	}
	if strings.HasPrefix(args[0], "-") {
		fmt.Fprintf(stderr, "diago: a verb is required\n%s", usage)
		return 2
	}
	fmt.Fprintf(stderr, "diago: unknown command %q\n%s", args[0], usage)
	if looksLikeSpec(args[0]) {
		fmt.Fprintf(stderr, "did you mean: diago render %s\n", args[0])
	}
	return 2
}

// looksLikeSpec reports whether an unknown verb is probably a spec meant for
// render: a name with a spec's extension, or an existing file.
func looksLikeSpec(arg string) bool {
	switch strings.ToLower(filepath.Ext(arg)) {
	case ".json", ".mmd", ".mermaid":
		return true
	}
	_, err := os.Stat(arg)
	return err == nil
}

const renderUsage = "usage: diago render [flags] [SPEC]\n\n" + specHelp + "Without SPEC, or with -, the spec is read from stdin.\n\n"

// parseArgs parses fs's flags wherever they stand among args, before,
// between or after the positional arguments, and returns the positionals
// in order. "-" is a positional (stdin), and "--" ends the flags: what
// follows it is positional. (A flag whose value is the literal "--" is read
// as the terminator.)
func parseArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return pos, nil
		}
		// Parse stops before a positional or right after "--".
		if n := len(args) - len(rest); n > 0 && args[n-1] == "--" {
			return append(pos, rest...), nil
		}
		pos = append(pos, rest[0])
		args = rest[1:]
	}
}

func runRender(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, renderUsage); fs.PrintDefaults() }
	themeName := fs.String("theme", "", "theme name (default, dark, midnight, sketch). Overrides spec's theme field.")
	format := fs.String("format", "svg", "output format: svg, png (requires resvg), text (alias: txt), drawio [beta], or excalidraw [beta] (flow only)")
	scale := fs.Float64("scale", 0, "PNG scale factor (default 2.0). Ignored if --width is set.")
	width := fs.Int("width", 0, "PNG output width in pixels. Overrides --scale.")
	debug := fs.Bool("debug", false, "emit JSON Lines layout/routing debug log to stderr")
	previousPath := fs.String("previous", "", "anchor the layout on a previous render: an SVG rendered by diago, a layout carrier JSON, or the previous spec")
	paths, err := parseArgs(fs, args)
	if err != nil {
		return 2
	}
	if len(paths) > 1 {
		fmt.Fprintf(stderr, "diago render: at most one spec path (default stdin, '-' for stdin)\n")
		return 2
	}
	path := ""
	if len(paths) == 1 {
		path = paths[0]
	}

	// Build a debug logger if --debug is set; otherwise the helper's
	// no-op logger is used and Decision call sites short-circuit.
	baseCtx := context.Background()
	if *debug {
		logger := slog.New(slog.NewJSONHandler(stderr, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))
		baseCtx = layoutdbg.NewContext(baseCtx, logger)
	}

	input, reports, err := readSource(baseCtx, "spec", path, stdin)
	if err != nil {
		return reportSourceError(stderr, "render", err)
	}
	writeReports(stderr, reports, "")

	var previous *model.LayoutHints
	if *previousPath != "" {
		if *previousPath == "-" {
			fmt.Fprintf(stderr, "error: -previous does not read stdin; pass a file or REV:PATH\n")
			return 2
		}
		data, prevReports, err := readSource(baseCtx, "-previous", *previousPath, nil)
		if err != nil {
			return reportSourceError(stderr, "render", err)
		}
		writeReports(stderr, prevReports, "previous")
		// The anchor must be of the same diagram type as the spec being
		// rendered. A spec that is not valid JSON, or whose own type is
		// missing or unknown, is left to the pipeline below so it is
		// reported as the spec error it is rather than as a -previous type
		// mismatch naming a type the spec never had.
		var probe struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(input, &probe) == nil && knownDiagramType(probe.Type) {
			renderTheme, err := pipeline.RenderTheme(input, *format, *themeName)
			if err != nil {
				return reportError(stderr, err)
			}
			previous, err = pipeline.ParsePrevious(baseCtx, data, probe.Type, *format, renderTheme)
			if err != nil {
				return reportError(stderr, err)
			}
		}
	}

	ctx, cancel := context.WithTimeout(baseCtx, 10*time.Second)
	defer cancel()
	var warnings []string
	var advisories []schema.Advisory
	data, renderErr := pipeline.RenderWithOptions(ctx, input, pipeline.Options{
		Theme:  *themeName,
		Format: *format,
		PNG: pngrender.Options{
			Width: *width,
			Scale: *scale,
		},
		Warnings:   &warnings,
		Previous:   previous,
		Advisories: &advisories,
	})
	if renderErr != nil {
		return reportError(stderr, renderErr)
	}
	stdout.Write(data) //nolint:errcheck
	for _, w := range warnings {
		fmt.Fprintf(stderr, "warning: %s\n", w)
	}
	writeAdvisories(stderr, advisories, "warning: ")
	return 0
}

// isMermaidPath reports whether path names a Mermaid source by extension.
func isMermaidPath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mmd", ".mermaid":
		return true
	}
	return false
}

// translate returns raw unchanged when it is not Mermaid (by extension or
// header), else the translated diago JSON with the importer's reports. A
// Mermaid-named path whose body is not Mermaid is a *mermaid.ParseError.
func translate(raw []byte, path string) ([]byte, []mermaid.Report, error) {
	if !isMermaidPath(path) && mermaid.Sniff(raw) == "" {
		return raw, nil, nil
	}
	res, err := mermaid.Parse(raw)
	if err != nil {
		return nil, nil, err
	}
	return res.JSON, res.Reports, nil
}

// readSource resolves a spec argument (a path, "" or "-" for stdin, or
// REV:PATH) through source.Read and translates Mermaid, keyed on the
// resolved name. what names the argument in a read error ("spec",
// "-previous").
func readSource(ctx context.Context, what, arg string, stdin io.Reader) ([]byte, []mermaid.Report, error) {
	s, err := source.Read(ctx, arg, stdin)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", what, err)
	}
	return translate(s.Data, s.Name)
}

// reportSourceError prints a read or translation failure: a Mermaid parse
// error is the author's input, "diago <verb>: line <n>: <message>" with
// exit 1; anything else is a plain "error:" line with exit 2.
func reportSourceError(stderr io.Writer, verb string, err error) int {
	var pe *mermaid.ParseError
	if errors.As(err, &pe) {
		fmt.Fprintf(stderr, "diago %s: %v\n", verb, pe)
		return 1
	}
	fmt.Fprintf(stderr, "error: %v\n", err)
	return 2
}

// writeReports prints the importer's reports, one per line, as soon as the
// translation they describe succeeds: before rendering and before any
// advisory. side names the input for verbs with more than one ("before",
// "after", "previous"); "" for the only input.
func writeReports(w io.Writer, reports []mermaid.Report, side string) {
	for _, r := range reports {
		if side != "" {
			fmt.Fprintf(w, "warning: import (%s) line %d: %s\n", side, r.Line, r.Message)
			continue
		}
		fmt.Fprintf(w, "warning: import line %d: %s\n", r.Line, r.Message)
	}
}

// reportError writes err to stderr and returns the exit code: 1 for a
// structured schema.ValidationErrors JSON envelope, 2 for a plain message.
// knownDiagramType reports whether a probed spec type is one the pipeline
// can anchor. Anything else (missing, misspelled) belongs to the spec's own
// validation error, not to a -previous mismatch message.
func knownDiagramType(t string) bool {
	switch t {
	case "flow", "sequence", "class":
		return true
	}
	return false
}

func reportError(stderr io.Writer, err error) int {
	var ve schema.ValidationErrors
	if errors.As(err, &ve) {
		out := struct {
			Error struct {
				Type   string                   `json:"type"`
				Errors []schema.ValidationError `json:"errors"`
			} `json:"error"`
		}{}
		out.Error.Type = "validation_error"
		out.Error.Errors = []schema.ValidationError(ve)
		enc := json.NewEncoder(stderr)
		enc.SetIndent("", "  ")
		enc.Encode(out) //nolint:errcheck
		return 1
	}
	fmt.Fprintf(stderr, "error: %v\n", err)
	return 2
}

const diffUsage = `usage: diago diff [flags] PATH | OLD NEW

PATH alone draws the file's latest change in git: the newest committed version
that differs from the file on disk, against the file on disk. OLD and NEW are
each a path or REV:PATH (the file at a git revision, e.g. HEAD~1:arch.json),
JSON or Mermaid (.mmd).

`

// runDiff renders the union of two specs with per-element status: OLD and
// NEW, each a path or REV:PATH, or one PATH against its latest committed
// change (source.Latest). The change list's caption names both sides.
func runDiff(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("diff", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, diffUsage); fs.PrintDefaults() }
	themeName := fs.String("theme", "", "theme name (default, dark, midnight, sketch). Overrides the new spec's theme field.")
	format := fs.String("format", "svg", "output format: svg, png (requires resvg), or text (alias: txt)")
	scale := fs.Float64("scale", 0, "PNG scale factor (default 2.0). Ignored if --width is set.")
	width := fs.Int("width", 0, "PNG output width in pixels. Overrides --scale.")
	debug := fs.Bool("debug", false, "emit JSON Lines layout/routing debug log to stderr")
	rest, err := parseArgs(fs, args)
	if err != nil {
		return 2
	}
	if len(rest) < 1 || len(rest) > 2 {
		fmt.Fprint(stderr, diffUsage)
		return 2
	}
	if slices.Contains(rest, "") {
		fmt.Fprintf(stderr, "diago diff: an empty argument is not a spec; pass file paths\n%s", diffUsage)
		return 2
	}
	if slices.Contains(rest, "-") {
		fmt.Fprintf(stderr, "diago diff: stdin is not accepted; pass file paths\n%s", diffUsage)
		return 2
	}
	baseCtx := context.Background()
	if *debug {
		logger := slog.New(slog.NewJSONHandler(stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
		baseCtx = layoutdbg.NewContext(baseCtx, logger)
	}
	var before, after source.Spec
	if len(rest) == 1 {
		if hint := revisionHint(rest[0]); hint != "" {
			fmt.Fprintf(stderr, "error: %s\n", hint)
			return 2
		}
		if before, after, err = source.Latest(baseCtx, rest[0]); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 2
		}
	} else {
		if before, err = source.Read(baseCtx, rest[0], nil); err != nil {
			fmt.Fprintf(stderr, "error: read old: %v\n", err)
			return 2
		}
		if after, err = source.Read(baseCtx, rest[1], nil); err != nil {
			fmt.Fprintf(stderr, "error: read new: %v\n", err)
			return 2
		}
	}
	oldData, oldReports, err := translate(before.Data, before.Name)
	if err != nil {
		return reportSourceError(stderr, "diff", err)
	}
	writeReports(stderr, oldReports, "before")
	newData, newReports, err := translate(after.Data, after.Name)
	if err != nil {
		return reportSourceError(stderr, "diff", err)
	}
	writeReports(stderr, newReports, "after")
	ctx, cancel := context.WithTimeout(baseCtx, 10*time.Second)
	defer cancel()
	var advisories []schema.Advisory
	data, err := pipeline.RenderDiff(ctx, oldData, newData, pipeline.DiffOptions{
		Format: *format, Theme: *themeName,
		PNG:        pngrender.Options{Width: *width, Scale: *scale},
		Caption:    pipeline.DiffCaption(before.Label, after.Label),
		Advisories: &advisories,
	})
	if err != nil {
		return reportError(stderr, err)
	}
	stdout.Write(data) //nolint:errcheck
	writeAdvisories(stderr, advisories, "warning: ")
	return 0
}

// revisionHint explains a lone REV:PATH argument, which is not the file on
// disk that one-argument diff takes: it names the call that compares that
// revision with the file. "" when arg is a file, or no REV:PATH.
func revisionHint(arg string) string {
	if _, err := os.Stat(arg); err == nil {
		return ""
	}
	rev, path, ok := strings.Cut(arg, ":")
	if !ok || rev == "" || path == "" {
		return ""
	}
	return fmt.Sprintf("diago diff with one argument diffs a file on disk against its latest change; to compare %s with the file, pass both: diago diff %s %s", arg, arg, path)
}

const checkUsage = "usage: diago check [flags] [SPEC]\n\n" + specHelp + "Without SPEC, or with -, the spec is read from stdin.\n\n"

// runCheck parses the spec, then prints schema.Check's findings. It never
// lays out.
func runCheck(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, checkUsage); fs.PrintDefaults() }
	strict := fs.Bool("strict", false, "exit 1 when any finding remains after the spec's ignore list")
	asJSON := fs.Bool("json", false, "print {\"warnings\": [...]} instead of one line per finding")
	rest, err := parseArgs(fs, args)
	if err != nil {
		return 2
	}
	if len(rest) > 1 {
		fmt.Fprint(stderr, checkUsage)
		return 2
	}
	path := ""
	if len(rest) == 1 {
		path = rest[0]
	}
	input, reports, err := readSource(context.Background(), "spec", path, stdin)
	if err != nil {
		return reportSourceError(stderr, "check", err)
	}
	writeReports(stderr, reports, "")
	if err := parseSpec(input); err != nil {
		return reportError(stderr, err)
	}
	advs, err := schema.Check(input, schema.CheckOptions{})
	if err != nil {
		return reportError(stderr, err)
	}
	if *asJSON {
		out := struct {
			Warnings []schema.Advisory `json:"warnings"`
		}{Warnings: advs}
		if out.Warnings == nil {
			out.Warnings = []schema.Advisory{}
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		enc.Encode(out) //nolint:errcheck
	} else {
		writeAdvisories(stdout, advs, "")
	}
	if *strict && len(advs) > 0 {
		return 1
	}
	return 0
}

// parseSpec validates a spec of either type without rendering it.
func parseSpec(data []byte) error {
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return fmt.Errorf("check: invalid JSON: %w", err)
	}
	switch probe.Type {
	case "flow":
		_, err := schema.ParseFlow(data)
		return err
	case "sequence":
		_, err := schema.ParseSequence(data)
		return err
	case "class":
		_, err := schema.ParseClass(data)
		return err
	default:
		return fmt.Errorf("check: unknown diagram type: %q", probe.Type)
	}
}

// writeAdvisories prints one line per finding, each prefixed (render and
// diff use "warning: ", check uses "").
func writeAdvisories(w io.Writer, advs []schema.Advisory, prefix string) {
	for _, a := range advs {
		fmt.Fprintf(w, "%s%s\n", prefix, a.String())
	}
}

const importUsage = "usage: diago import [SPEC]\n\nSPEC is a Mermaid source; REV:PATH reads it as it was at a git revision.\nWithout SPEC, or with -, it is read from stdin.\n"

// runImport translates a Mermaid source (path or stdin) into a diago JSON
// spec on stdout, with the importer's reports on stderr.
func runImport(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, importUsage) }
	paths, err := parseArgs(fs, args)
	if err != nil {
		return 2
	}
	if len(paths) > 1 {
		fmt.Fprint(stderr, importUsage)
		return 2
	}
	path := ""
	if len(paths) == 1 {
		path = paths[0]
	}
	s, err := source.Read(context.Background(), path, stdin)
	if err != nil {
		fmt.Fprintf(stderr, "error: read spec: %v\n", err)
		return 2
	}
	res, err := mermaid.Parse(s.Data)
	if err != nil {
		return reportSourceError(stderr, "import", err)
	}
	stdout.Write(res.JSON) //nolint:errcheck
	writeReports(stderr, res.Reports, "")
	return 0
}
