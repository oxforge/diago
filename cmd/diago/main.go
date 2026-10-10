// Command diago renders diagrams from JSON or Mermaid specs to stdout. Its
// verbs are render, diff, check and import: "diago help" prints the usage,
// "diago help <verb>" a verb's usage and flags, and the CLI reference is at
// https://diago.dev/docs/reference/cli/.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/oxforge/diago/internal/schema"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// specHelp says what a SPEC argument may be; the top-level usage and every
// verb's usage print it.
const specHelp = `SPEC is a JSON spec or a Mermaid source: a .mmd or .mermaid path, or a body
whose first meaningful line (after a leading --- frontmatter block, blank
lines and %% comments) is a flowchart, graph, sequenceDiagram or classDiagram
header.
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
  diago help [verb]              print this usage, or a verb's usage and flags
  diago --version                print diago's version

` + specHelp + `Without SPEC, or with -, render, check and import read stdin.
`

// run runs the verb args names. A write to stdout that fails is an error,
// exit 2, whatever the verb returned: the artifact did not arrive whole.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	out := &stickyWriter{w: stdout}
	code := dispatch(args, stdin, out, stderr)
	if out.err != nil {
		fmt.Fprintf(stderr, "error: write output: %v\n", out.err)
		return 2
	}
	return code
}

// stickyWriter is stdout as the verbs see it: it keeps the first write
// error for run to report.
type stickyWriter struct {
	w   io.Writer
	err error
}

func (s *stickyWriter) Write(p []byte) (int, error) {
	n, err := s.w.Write(p)
	if err != nil && s.err == nil {
		s.err = err
	}
	return n, err
}

// dispatch runs the verb args names, writing its output to stdout. It returns
// the exit code: 0 ok, 1 validation error (structured JSON on stderr), 2
// usage, read, git or internal error.
func dispatch(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	if isHelpFlag(args[0]) {
		fmt.Fprint(stdout, usage)
		return 0
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
	case "help":
		return runHelp(args[1:], stdout, stderr)
	case "--version", "-version", "version":
		switch {
		case len(args) == 2 && isHelpFlag(args[1]):
			fmt.Fprint(stdout, versionUsage)
			return 0
		case len(args) > 1:
			fmt.Fprint(stderr, versionUsage)
			return 2
		}
		fmt.Fprintf(stdout, "diago %s\n", version())
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

// runHelp prints the top-level usage, or with a verb that verb's usage and
// flags, on stdout.
func runHelp(args []string, stdout, stderr io.Writer) int {
	if len(args) > 1 {
		fmt.Fprint(stderr, "usage: diago help [verb]\n")
		return 2
	}
	if len(args) == 0 || args[0] == "help" || isHelpFlag(args[0]) {
		fmt.Fprint(stdout, usage)
		return 0
	}
	switch args[0] {
	case "render", "diff", "check", "import":
		return dispatch([]string{args[0], "-h"}, nil, stdout, stderr)
	case "version", "--version", "-version":
		fmt.Fprint(stdout, versionUsage)
		return 0
	}
	fmt.Fprintf(stderr, "diago help: unknown command %q\n%s", args[0], usage)
	return 2
}

// isHelpFlag reports whether arg asks for help: -h, --h, -help or --help,
// the spellings the flag package reads among a verb's flags.
func isHelpFlag(arg string) bool {
	return arg == "-h" || arg == "--h" || arg == "-help" || arg == "--help"
}

// parseVerb parses a verb's flags wherever they stand (parseArgs). Help
// asked for with -h, --h, -help or --help prints the verb's usage and
// flags on stdout, exit code 0; a bad flag prints the flag package's
// message, then the usage and flags, on stderr, exit code 2. done reports
// that the verb must return code.
func parseVerb(fs *flag.FlagSet, verbUsage string, args []string, stdout, stderr io.Writer) (paths []string, code int, done bool) {
	fs.SetOutput(stderr)
	fs.Usage = func() {} // printed below, where the outcome decides
	paths, err := parseArgs(fs, args)
	switch {
	case errors.Is(err, flag.ErrHelp):
		printUsage(fs, verbUsage, stdout)
		return nil, 0, true
	case err != nil:
		printUsage(fs, verbUsage, stderr)
		return nil, 2, true
	}
	return paths, 0, false
}

// printUsage writes a verb's usage text and its flags' defaults to w.
func printUsage(fs *flag.FlagSet, verbUsage string, w io.Writer) {
	fmt.Fprint(w, verbUsage)
	out := fs.Output()
	fs.SetOutput(w)
	fs.PrintDefaults()
	fs.SetOutput(out)
}

const versionUsage = "usage: diago --version\n\nPrints \"diago <version>\": a release tag, or a pseudo-version naming the commit\ndiago was built from; (devel) when the binary carries no version.\n"

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

// reportError writes err to stderr and returns the exit code: 1 for a
// structured schema.ValidationErrors JSON envelope, 2 for a plain message.
func reportError(stderr io.Writer, err error) int {
	if errors.Is(err, context.DeadlineExceeded) {
		fmt.Fprintf(stderr, "error: the render took longer than %s and was stopped: split the diagram or simplify it\n", renderTimeout)
		return 2
	}
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
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		enc.Encode(out) //nolint:errcheck
		return 1
	}
	fmt.Fprintf(stderr, "error: %v\n", err)
	return 2
}
