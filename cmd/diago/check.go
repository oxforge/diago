package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/oxforge/diago/internal/pipeline"
	"github.com/oxforge/diago/internal/schema"
)

const checkUsage = "usage: diago check [flags] [SPEC]\n\n" + specHelp + "Without SPEC, or with -, the spec is read from stdin.\n\n"

// runCheck parses the spec, then prints schema.Check's findings. It never
// lays out.
func runCheck(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	strict := fs.Bool("strict", false, "exit 1 when any finding remains after the spec's ignore list")
	asJSON := fs.Bool("json", false, "print {\"warnings\": [...]} instead of one line per finding")
	rest, code, done := parseVerb(fs, checkUsage, args, stdout, stderr)
	if done {
		return code
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
	if err := pipeline.Validate(input); err != nil {
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
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		enc.Encode(out) //nolint:errcheck // run reports a failed write (stickyWriter)
	} else {
		writeAdvisories(stdout, advs, "")
	}
	if *strict && len(advs) > 0 {
		return 1
	}
	return 0
}

// writeAdvisories prints one line per finding, each prefixed (render and
// diff use "warning: ", check uses "").
func writeAdvisories(w io.Writer, advs []schema.Advisory, prefix string) {
	for _, a := range advs {
		fmt.Fprintf(w, "%s%s\n", prefix, a.String())
	}
}
