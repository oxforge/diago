// Command diago reads a JSON diagram spec and writes a rendered diagram.
// The diagram type is auto-detected from the "type" field in the JSON spec.
// Supported types: "flow", "sequence", "class".
//
// Usage:
//
//	diago render [flags] [spec.json]
//	diago [flags] [spec.json]   (bare form: implicit render; stdin when no path)
//	diago diff <old.json> <new.json|change.patch> [flags]
//	diago check [-strict] [-json] [spec.json]
//	diago import [diagram.mmd]
//
// The bare form is dispatched to "render" whenever the first argument is a
// flag (or there are no arguments at all), and it accepts the same flags and
// the same optional positional spec path as "render" does — so
// "diago -theme midnight spec.json" reads spec.json, not stdin. Only when the
// first argument is NOT a flag must it be a verb: a bare positional path with
// no flag ahead of it, e.g. "diago spec.json", is an unknown-command error;
// use "diago render spec.json" instead. Either way, the spec is read from the
// positional path when one is given, or from stdin when it is omitted or "-".
//
//	cat spec.json | go run ./cmd/diago
//	go run ./cmd/diago -theme midnight spec.json
//	go run ./cmd/diago render spec.json
//	go run ./cmd/diago render -previous v1.svg v2.json
//	go run ./cmd/diago diff v1.json v2.json -format text
//
// "diff" renders the union of two specs of the same diagram type, with each
// node, edge, and group marked added, removed, changed, or unchanged. The
// second argument is a spec path, or a unified diff of the first argument
// (detected by a ".patch"/".diff" extension or a "diff ", "--- ", or "@@ "
// prefix), applied before parsing. Neither argument accepts stdin ("-").
//
// "check" parses the spec and prints one advisory per line, "<rule> <field>:
// <message>" (no field for whole-diagram findings), exit 0; -strict exits 1
// when any finding remains after the spec's "ignore" list; -json prints
// {"warnings": [...]} instead. It never lays out, so text-label-dropped
// surfaces on render only. Every render and diff
// prints the same lines on stderr prefixed "warning: ".
//
// Every spec argument (and stdin) may be a Mermaid source instead of JSON:
// a path ending in .mmd or .mermaid, or a body whose first meaningful line
// is a "graph"/"flowchart", "sequenceDiagram" or "classDiagram" header, is
// translated by internal/mermaid before parsing. This holds for render,
// check, both diff arguments and -previous. Lossy translations print
// "warning: import line <n>: <message>" on stderr; a rejected line prints
// "diago <verb>: line <n>: <message>" and exits 1.
//
// "import" prints the translated JSON spec on stdout and the same warnings
// on stderr; a JSON or otherwise non-Mermaid input is refused (exit 1).
//
// On validation error, a structured JSON error is printed to stderr and the
// process exits with code 1. On internal or usage error, a plain message is
// printed to stderr and the process exits with code 2.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/oxforge/diago/internal/diff"
	"github.com/oxforge/diago/internal/layoutdbg"
	"github.com/oxforge/diago/internal/mermaid"
	"github.com/oxforge/diago/internal/model"
	"github.com/oxforge/diago/internal/pipeline"
	pngrender "github.com/oxforge/diago/internal/render/png"
	"github.com/oxforge/diago/internal/schema"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run dispatches the verb and returns the exit code: 0 ok, 1 validation
// error (structured JSON on stderr), 2 internal or usage error.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return runRender(args, stdin, stdout, stderr) // bare form: implicit render
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
	default:
		fmt.Fprintf(stderr, "diago: unknown command %q\nusage: diago render [flags] [spec.json]\n       diago [flags] [spec.json]   (bare form: implicit render; stdin when no path)\n       diago diff <old.json> <new.json|change.patch> [flags]\n       diago check [-strict] [-json] [spec.json]\n       diago import [diagram.mmd]\n", args[0])
		return 2
	}
}

func runRender(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	fs.SetOutput(stderr)
	themeName := fs.String("theme", "", "theme name (default, dark, midnight, sketch). Overrides spec's theme field.")
	format := fs.String("format", "svg", "output format: svg, png (requires resvg), text (alias: txt), drawio, or excalidraw (flow only)")
	scale := fs.Float64("scale", 0, "PNG scale factor (default 2.0). Ignored if --width is set.")
	width := fs.Int("width", 0, "PNG output width in pixels. Overrides --scale.")
	debug := fs.Bool("debug", false, "emit JSON Lines layout/routing debug log to stderr")
	previousPath := fs.String("previous", "", "anchor the layout on a previous render: an SVG rendered by diago, a layout carrier JSON, or the previous spec")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 1 {
		fmt.Fprintf(stderr, "diago render: at most one spec path (default stdin, '-' for stdin)\n")
		return 2
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

	input, reports, err := readSource(fs.Arg(0), stdin)
	if err != nil {
		return reportSourceError(stderr, "render", err)
	}
	writeReports(stderr, reports, "")

	var previous *model.LayoutHints
	var prevReports []mermaid.Report
	if *previousPath != "" {
		data, err := os.ReadFile(*previousPath)
		if err != nil {
			fmt.Fprintf(stderr, "error: read -previous: %v\n", err)
			return 2
		}
		data, prevReports, err = translate(data, *previousPath)
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

// readSpec reads the spec from path, or from stdin when path is "" or "-".
func readSpec(path string, stdin io.Reader) ([]byte, error) {
	if path == "" || path == "-" {
		b, err := io.ReadAll(stdin)
		if err != nil {
			return nil, fmt.Errorf("failed to read stdin: %w", err)
		}
		return b, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read spec: %w", err)
	}
	return b, nil
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

// readSource reads a spec (path, "-" or stdin) and translates Mermaid.
func readSource(path string, stdin io.Reader) ([]byte, []mermaid.Report, error) {
	raw, err := readSpec(path, stdin)
	if err != nil {
		return nil, nil, err
	}
	return translate(raw, path)
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

const diffUsage = "usage: diago diff <old.json> <new.json|change.patch> [-format svg|png|text] [-theme name] [-scale n] [-width px] [-debug]\n"

// runDiff renders the union of two specs with per-element status. The
// second argument may be a unified diff of the first (ingot passes git diff
// output), applied strictly before parsing.
func runDiff(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("diff", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, diffUsage) }
	themeName := fs.String("theme", "", "theme name (default, dark, midnight, sketch). Overrides the new spec's theme field.")
	format := fs.String("format", "svg", "output format: svg, png (requires resvg), or text (alias: txt)")
	scale := fs.Float64("scale", 0, "PNG scale factor (default 2.0). Ignored if --width is set.")
	width := fs.Int("width", 0, "PNG output width in pixels. Overrides --scale.")
	debug := fs.Bool("debug", false, "emit JSON Lines layout/routing debug log to stderr")
	// Flags may follow the two paths: parse, then re-parse the remainder.
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) >= 2 {
		if err := fs.Parse(rest[2:]); err != nil {
			return 2
		}
		if fs.NArg() != 0 {
			fmt.Fprint(stderr, diffUsage)
			return 2
		}
		rest = rest[:2]
	}
	if len(rest) != 2 {
		fmt.Fprint(stderr, diffUsage)
		return 2
	}
	oldPath, newPath := rest[0], rest[1]
	if oldPath == "-" || newPath == "-" {
		fmt.Fprintf(stderr, "diago diff: stdin is not accepted; pass two file paths\n%s", diffUsage)
		return 2
	}
	baseCtx := context.Background()
	if *debug {
		logger := slog.New(slog.NewJSONHandler(stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
		baseCtx = layoutdbg.NewContext(baseCtx, logger)
	}
	oldData, err := os.ReadFile(oldPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: read old: %v\n", err)
		return 2
	}
	newData, err := os.ReadFile(newPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: read new: %v\n", err)
		return 2
	}
	applied := false
	if isPatch(newPath, newData) {
		patched, err := diff.ApplyUnifiedDiff(oldData, newData, filepath.Base(oldPath))
		if err != nil {
			var pe *diff.PatchError
			if errors.As(err, &pe) {
				return reportError(stderr, schema.ValidationErrors{{Field: "new", Message: err.Error()}})
			}
			return reportError(stderr, err)
		}
		newData = patched
		applied = true
	}
	oldData, oldReports, err := translate(oldData, oldPath)
	if err != nil {
		return reportSourceError(stderr, "diff", err)
	}
	writeReports(stderr, oldReports, "before")
	newHint := newPath
	if applied {
		newHint = oldPath
	}
	newData, newReports, err := translate(newData, newHint)
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
		Advisories: &advisories,
	})
	if err != nil {
		return reportError(stderr, err)
	}
	stdout.Write(data) //nolint:errcheck
	writeAdvisories(stderr, advisories, "warning: ")
	return 0
}

// isPatch reports whether the second diff argument is a unified diff
// rather than a spec: by extension, or by a diff/---/@@ prefix.
func isPatch(path string, data []byte) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".patch", ".diff":
		return true
	}
	head := bytes.TrimLeft(data, " \t\r\n")
	return bytes.HasPrefix(head, []byte("diff ")) || bytes.HasPrefix(head, []byte("--- ")) || bytes.HasPrefix(head, []byte("@@ "))
}

const checkUsage = "usage: diago check [-strict] [-json] [spec.json]\n"

// runCheck parses the spec, then prints schema.Check's findings. It never
// lays out.
func runCheck(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, checkUsage) }
	strict := fs.Bool("strict", false, "exit 1 when any finding remains after the spec's ignore list")
	asJSON := fs.Bool("json", false, "print {\"warnings\": [...]} instead of one line per finding")
	// Flags may follow the path: parse, then re-parse the remainder.
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) >= 1 {
		if err := fs.Parse(rest[1:]); err != nil {
			return 2
		}
		if fs.NArg() != 0 {
			fmt.Fprint(stderr, checkUsage)
			return 2
		}
		rest = rest[:1]
	}
	path := ""
	if len(rest) == 1 {
		path = rest[0]
	}
	input, reports, err := readSource(path, stdin)
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

const importUsage = "usage: diago import [diagram.mmd]\n"

// runImport translates a Mermaid source (path or stdin) into a diago JSON
// spec on stdout, with the importer's reports on stderr.
func runImport(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, importUsage) }
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 1 {
		fmt.Fprint(stderr, importUsage)
		return 2
	}
	raw, err := readSpec(fs.Arg(0), stdin)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
	res, err := mermaid.Parse(raw)
	if err != nil {
		return reportSourceError(stderr, "import", err)
	}
	stdout.Write(res.JSON) //nolint:errcheck
	writeReports(stderr, res.Reports, "")
	return 0
}
