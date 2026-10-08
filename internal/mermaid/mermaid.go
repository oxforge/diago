// Package mermaid translates a Mermaid flowchart, sequenceDiagram or
// classDiagram source into a diago JSON spec (Phase 5 of the review-loop
// parity program). It is a migration door for the CLI only: every
// construct with a nearest diago form is translated and reported, every
// statement with no meaning in diago is a line-numbered ParseError. The
// package depends on schema only; it emits schema.FlowSpec, SequenceSpec
// and ClassSpec, so the CLI validates the translation like any JSON. An
// unclosed block reports the line after the last statement in the
// flowchart and class dialects and the block's opening line in the
// sequence dialect.
package mermaid

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Kind is a Mermaid dialect.
type Kind string

const (
	KindFlowchart Kind = "flowchart"
	KindSequence  Kind = "sequence"
	KindClass     Kind = "class"
)

// Report is one lossy translation, tied to its source line.
type Report struct {
	Line    int
	Message string
}

// ParseError is a rejected source line.
type ParseError struct {
	Line    int
	Message string
}

func (e *ParseError) Error() string { return fmt.Sprintf("line %d: %s", e.Line, e.Message) }

// perr builds a ParseError for line n.
func perr(n int, format string, args ...any) *ParseError {
	return &ParseError{Line: n, Message: fmt.Sprintf(format, args...)}
}

// Result is a translated source.
type Result struct {
	Kind    Kind
	JSON    []byte   // the diago spec, indented with two spaces, newline-terminated
	Reports []Report // in source-line order
}

// line is one source line, 1-based, with its %% comment and surrounding
// whitespace removed. Blank lines are kept so numbering stays exact.
type line struct {
	n    int
	text string
}

// dialectParser translates the body lines (frontmatter removed) of one
// dialect into a spec struct. Each dialect file registers its parser in
// init(), so the core is testable on its own.
type dialectParser func(title string, body []line) (spec any, reports []Report, err error)

var dialects = map[Kind]dialectParser{}

var commentRE = regexp.MustCompile(`%%.*$`)

// splitLines strips a leading UTF-8 BOM, splits src on "\n", trims a
// trailing "\r", strips comments and trims whitespace.
func splitLines(src []byte) []line {
	raw := strings.Split(strings.TrimPrefix(string(src), "\ufeff"), "\n")
	out := make([]line, len(raw))
	for i, r := range raw {
		r = strings.TrimSuffix(r, "\r")
		out[i] = line{n: i + 1, text: strings.TrimSpace(commentRE.ReplaceAllString(r, ""))}
	}
	return out
}

// unquote strips one pair of surrounding double quotes from a label or a
// title, after trimming whitespace.
func unquote(s string) string {
	t := strings.TrimSpace(s)
	if len(t) >= 2 && strings.HasPrefix(t, `"`) && strings.HasSuffix(t, `"`) {
		return t[1 : len(t)-1]
	}
	return t
}

var frontmatterTitleRE = regexp.MustCompile(`^title:\s*(.*)$`)

// parseFrontmatter recognizes a block only when the first line is exactly
// "---". It supports a single "title:" key; any other key is a ParseError
// on its line and an unclosed block is a ParseError on line 1. It returns
// the title and the remaining lines with their original numbers.
func parseFrontmatter(lines []line) (title string, body []line, err error) {
	if len(lines) == 0 || lines[0].text != "---" {
		return "", lines, nil
	}
	closeAt := -1
	for i := 1; i < len(lines); i++ {
		if lines[i].text == "---" {
			closeAt = i
			break
		}
	}
	if closeAt == -1 {
		return "", nil, perr(1, `unclosed frontmatter block (missing closing "---")`)
	}
	for _, l := range lines[1:closeAt] {
		if l.text == "" {
			continue
		}
		m := frontmatterTitleRE.FindStringSubmatch(l.text)
		if m == nil {
			return "", nil, perr(l.n, `frontmatter supports only "title:", got %q`, l.text)
		}
		title = unquote(m[1])
	}
	return title, lines[closeAt+1:], nil
}

var (
	sequenceHeaderRE  = regexp.MustCompile(`^sequenceDiagram\s*$`)
	classHeaderLineRE = regexp.MustCompile(`^classDiagram\s*$`)
	flowchartHeaderRE = regexp.MustCompile(`^(graph|flowchart)\s`)
)

// sniffLines returns the dialect named by the first non-blank line, or "".
func sniffLines(body []line) Kind {
	for _, l := range body {
		if l.text == "" {
			continue
		}
		switch {
		case sequenceHeaderRE.MatchString(l.text):
			return KindSequence
		case classHeaderLineRE.MatchString(l.text):
			return KindClass
		case flowchartHeaderRE.MatchString(l.text):
			return KindFlowchart
		}
		return ""
	}
	return ""
}

// Sniff reports the dialect of the first meaningful line after an optional
// frontmatter block and any %% comments, or "" when src is not Mermaid. An
// unclosed frontmatter block is not Mermaid either.
func Sniff(src []byte) Kind {
	_, body, err := parseFrontmatter(splitLines(src))
	if err != nil {
		return ""
	}
	return sniffLines(body)
}

// Parse translates src. err is nil or a *ParseError; a source Sniff does
// not recognize is a ParseError on line 1.
func Parse(src []byte) (*Result, error) {
	title, body, err := parseFrontmatter(splitLines(src))
	if err != nil {
		return nil, err
	}
	kind := sniffLines(body)
	if kind == "" {
		return nil, perr(1, "not a Mermaid source")
	}
	parse, ok := dialects[kind]
	if !ok {
		return nil, perr(1, "the %s dialect is not supported", kind)
	}
	spec, reports, err := parse(title, body)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(reports, func(i, j int) bool { return reports[i].Line < reports[j].Line })
	js, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("mermaid: encode spec: %w", err)
	}
	return &Result{Kind: kind, JSON: append(js, '\n'), Reports: reports}, nil
}
