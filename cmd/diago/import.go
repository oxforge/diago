package main

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/oxforge/diago/internal/mermaid"
	"github.com/oxforge/diago/internal/source"
)

const importUsage = "usage: diago import [SPEC]\n\nSPEC is a Mermaid source; REV:PATH reads it as it was at a git revision.\nWithout SPEC, or with -, it is read from stdin.\n"

// runImport translates a Mermaid source (path or stdin) into a diago JSON
// spec on stdout, with the importer's reports on stderr.
func runImport(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	paths, code, done := parseVerb(fs, importUsage, args, stdout, stderr)
	if done {
		return code
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
	stdout.Write(res.JSON) //nolint:errcheck // run reports a failed write (stickyWriter)
	writeReports(stderr, res.Reports, "")
	return 0
}
