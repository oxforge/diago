package diff

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// PatchError reports a malformed patch, a context mismatch, or a missing
// section. The message names the failing hunk.
type PatchError struct{ Msg string }

func (e *PatchError) Error() string { return "patch: " + e.Msg }

type hunk struct {
	oldStart, oldCount int      // 1-based; oldCount 0 means insertion after oldStart
	newStart, newCount int      // 1-based; carried only for the hunk label
	lines              []string // body lines with their ' ', '-', '+' markers
}

type section struct {
	path  string
	hunks []hunk
}

var hunkRE = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// splitSections groups hunks by their "+++" path ("" for bare hunks).
// Bodies are consumed by the header counts, never by marker scanning, so a
// plain diff -u concatenation cannot swallow the next section's "---" line.
func splitSections(lines []string) ([]section, error) {
	var sections []section
	var cur *section
	for i := 0; i < len(lines); {
		line := lines[i]
		switch {
		case strings.HasPrefix(line, "+++ "):
			path := strings.SplitN(line[4:], "\t", 2)[0]
			path = strings.TrimPrefix(path, "b/")
			sections = append(sections, section{path: path})
			cur = &sections[len(sections)-1]
			i++
		case hunkRE.MatchString(line):
			m := hunkRE.FindStringSubmatch(line)
			if cur == nil {
				sections = append(sections, section{})
				cur = &sections[len(sections)-1]
			}
			h := hunk{
				oldStart: atoi(m[1]),
				oldCount: countOr1(m[2]),
				newStart: atoi(m[3]),
				newCount: countOr1(m[4]),
			}
			remOld, remNew := h.oldCount, h.newCount
			i++
			for i < len(lines) && (remOld > 0 || remNew > 0) {
				b := lines[i]
				switch {
				case strings.HasPrefix(b, "\\"):
					i++ // "\ No newline at end of file": tolerated, not counted
					continue
				case strings.HasPrefix(b, " "):
					remOld--
					remNew--
				case strings.HasPrefix(b, "-"):
					remOld--
				case strings.HasPrefix(b, "+"):
					remNew--
				default:
					return nil, &PatchError{fmt.Sprintf("malformed hunk body at patch line %d", i+1)}
				}
				h.lines = append(h.lines, b)
				i++
			}
			if i < len(lines) && strings.HasPrefix(lines[i], "\\") {
				i++
			}
			if remOld != 0 || remNew != 0 {
				label := fmt.Sprintf("hunk %d (@@ -%d,%d +%d,%d @@)", len(cur.hunks)+1, h.oldStart, h.oldCount, h.newStart, h.newCount)
				return nil, &PatchError{label + ": body has fewer lines than its header declares"}
			}
			cur.hunks = append(cur.hunks, h)
		default:
			i++ // "diff --git", "--- ", "index ", noise between sections
		}
	}
	return sections, nil
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

func countOr1(s string) int {
	if s == "" {
		return 1
	}
	return atoi(s)
}

// ApplyUnifiedDiff applies patch to old strictly: every hunk at its stated
// position with exact context and deletion matches, no fuzz, no offset
// search. Lines are split on "\n" keeping any "\r", so CRLF content needs
// a CRLF patch. A multi-file patch needs fileName (exact path or "/"-suffix
// match) to pick its section.
func ApplyUnifiedDiff(old, patch []byte, fileName string) ([]byte, error) {
	all, err := splitSections(strings.Split(string(patch), "\n"))
	if err != nil {
		return nil, err
	}
	var sections []section
	for _, s := range all {
		if len(s.hunks) > 0 {
			sections = append(sections, s)
		}
	}
	if len(sections) == 0 {
		return nil, &PatchError{"patch contains no hunks"}
	}
	sec := sections[0]
	if len(sections) > 1 {
		found := false
		for _, s := range sections {
			if fileName != "" && (s.path == fileName || strings.HasSuffix(s.path, "/"+fileName)) {
				sec, found = s, true
				break
			}
		}
		if !found {
			names := make([]string, len(sections))
			for i, s := range sections {
				names[i] = s.path
				if names[i] == "" {
					names[i] = "<bare>"
				}
			}
			name := fileName
			if name == "" {
				name = "<no fileName given>"
			}
			return nil, &PatchError{fmt.Sprintf("multi-file patch: no section matches %q (sections: %s)", name, strings.Join(names, ", "))}
		}
	}

	src := strings.Split(string(old), "\n")
	var out []string
	cursor := 0
	for hi, h := range sec.hunks {
		label := fmt.Sprintf("hunk %d (@@ -%d,%d +%d,%d @@)", hi+1, h.oldStart, h.oldCount, h.newStart, h.newCount)
		start := h.oldStart - 1
		if h.oldCount == 0 {
			start = h.oldStart
		}
		if start < cursor {
			return nil, &PatchError{label + ": hunks overlap or are out of order"}
		}
		if start > len(src) {
			return nil, &PatchError{label + ": start beyond end of input"}
		}
		out = append(out, src[cursor:start]...)
		cursor = start
		for _, body := range h.lines {
			text := body[1:]
			if strings.HasPrefix(body, "+") {
				out = append(out, text)
				continue
			}
			if cursor >= len(src) || src[cursor] != text {
				found := "<end of file>"
				if cursor < len(src) {
					found = strconv.Quote(src[cursor])
				} else {
					found = strconv.Quote(found)
				}
				return nil, &PatchError{fmt.Sprintf("%s: expected line %d to be %s, found %s", label, cursor+1, strconv.Quote(text), found)}
			}
			if strings.HasPrefix(body, " ") {
				out = append(out, text)
			}
			cursor++
		}
	}
	out = append(out, src[cursor:]...)
	return []byte(strings.Join(out, "\n")), nil
}
