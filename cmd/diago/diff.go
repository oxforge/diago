package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"

	"github.com/oxforge/diago/internal/layoutdbg"
	"github.com/oxforge/diago/internal/pipeline"
	pngrender "github.com/oxforge/diago/internal/render/png"
	"github.com/oxforge/diago/internal/schema"
	"github.com/oxforge/diago/internal/source"
)

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
	themeName := fs.String("theme", "", "theme name (default, dark, midnight, sketch). Overrides the new spec's theme field.")
	format := fs.String("format", "svg", "output format: svg, png (requires resvg), or text (alias: txt)")
	scale := fs.Float64("scale", 0, "PNG scale factor (default 2.0). Ignored if --width is set.")
	width := fs.Int("width", 0, "PNG output width in pixels. Overrides --scale.")
	debug := fs.Bool("debug", false, "emit JSON Lines layout/routing debug log to stderr")
	rest, code, done := parseVerb(fs, diffUsage, args, stdout, stderr)
	if done {
		return code
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
	var err error
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
	ctx, cancel := context.WithTimeout(baseCtx, renderTimeout)
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
	stdout.Write(data) //nolint:errcheck // run reports a failed write (stickyWriter)
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
	rev, path, ok := source.SplitRevision(arg)
	if !ok || rev == "" || path == "" {
		return ""
	}
	return fmt.Sprintf("diago diff with one argument diffs a file on disk against its latest change; to compare %s with the file, pass both: diago diff %s %s", arg, arg, path)
}
