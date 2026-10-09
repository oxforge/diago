package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/oxforge/diago/internal/mermaid"
	"github.com/oxforge/diago/internal/source"
)

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
