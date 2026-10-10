package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/oxforge/diago/internal/layoutdbg"
	"github.com/oxforge/diago/internal/pipeline"
	pngrender "github.com/oxforge/diago/internal/render/png"
	"github.com/oxforge/diago/internal/schema"
)

const renderUsage = "usage: diago render [flags] [SPEC]\n\n" + specHelp + "Without SPEC, or with -, the spec is read from stdin.\n\n"

func runRender(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	themeName := fs.String("theme", "", "theme name (default, dark, midnight, sketch). Overrides spec's theme field.")
	format := fs.String("format", "svg", "output format: svg, png (requires resvg), text (alias: txt), drawio [beta], or excalidraw [beta] (flow only)")
	scale := fs.Float64("scale", 0, "PNG scale factor (default 2.0). Ignored if --width is set.")
	width := fs.Int("width", 0, "PNG output width in pixels. Overrides --scale.")
	debug := fs.Bool("debug", false, "emit JSON Lines layout/routing debug log to stderr")
	previousPath := fs.String("previous", "", "anchor the layout on a previous render: an SVG rendered by diago, a layout carrier JSON, or the previous spec")
	paths, code, done := parseVerb(fs, renderUsage, args, stdout, stderr)
	if done {
		return code
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

	var previousData []byte
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
		previousData = data
		if previousData == nil { // a file empty at a revision is a -previous all the same
			previousData = []byte{}
		}
	}

	// The deadline bounds the layout and drawing, never the reads above.
	ctx, cancel := context.WithTimeout(baseCtx, renderTimeout)
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
		Warnings:     &warnings,
		PreviousData: previousData,
		Advisories:   &advisories,
	})
	if renderErr != nil {
		return reportError(stderr, renderErr)
	}
	stdout.Write(data) //nolint:errcheck // run reports a failed write (stickyWriter)
	for _, w := range warnings {
		fmt.Fprintf(stderr, "warning: %s\n", w)
	}
	writeAdvisories(stderr, advisories, "warning: ")
	return 0
}

// renderTimeout bounds a render or diff, layout and drawing; a variable so
// tests can shorten it.
var renderTimeout = 10 * time.Second
