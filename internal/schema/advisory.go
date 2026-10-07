package schema

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// Advisory is one finding of Check: a spec that parses but reads badly.
// Field is a JSON path in the spec, in the same dialect as
// ValidationError.Field (nodes[2].label); it is empty for findings about the
// whole diagram (too-large, no-entry, shape-soup, seq-too-many-participants).
// Findings are returned to the caller and never logged.
type Advisory struct {
	Rule    string `json:"rule"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

// String is the CLI line: "<rule> <field>: <message>", or "<rule>: <message>"
// when the finding has no field.
func (a Advisory) String() string {
	if a.Field != "" {
		return a.Rule + " " + a.Field + ": " + a.Message
	}
	return a.Rule + ": " + a.Message
}

// CheckOptions tunes rules that depend on how the spec is being used.
type CheckOptions struct {
	// Anchored is true under render -previous and diff: it enables
	// edge-without-id, which only matters when ids are keyed on.
	Anchored bool
}

// Rule names. Rules lists them sorted; the ignore field is validated
// against that list.
const (
	RuleDeepNesting            = "deep-nesting"
	RuleDuplicateEdge          = "duplicate-edge"
	RuleEdgeWithoutID          = "edge-without-id"
	RuleEmptyGroup             = "empty-group"
	RuleFlatEdgeRanked         = "flat-edge-ranked"
	RuleGodClass               = "god-class"
	RuleIsolatedClass          = "isolated-class"
	RuleIsolatedNode           = "isolated-node"
	RuleLongLabel              = "long-label"
	RuleNoEntry                = "no-entry"
	RuleOverlongMember         = "overlong-member"
	RuleOversizedClassDiagram  = "oversized-class-diagram"
	RuleRemovedField           = "removed-field"
	RuleSeqTooManyParticipants = "seq-too-many-participants"
	RuleShapeSoup              = "shape-soup"
	RuleTextLabelDropped       = "text-label-dropped"
	RuleTooLarge               = "too-large"
	RuleUnbreakableToken       = "unbreakable-token"
	RuleUnknownField           = "unknown-field"
	RuleUnlabeledAltSection    = "unlabeled-alt-section"
	RuleUnlabeledBranch        = "unlabeled-branch"
	RuleVagueEdgeLabel         = "vague-edge-label"
	RuleVagueMessageLabel      = "vague-message-label"
)

// Rules is every rule name, sorted.
var Rules = []string{
	RuleDeepNesting,
	RuleDuplicateEdge,
	RuleEdgeWithoutID,
	RuleEmptyGroup,
	RuleFlatEdgeRanked,
	RuleGodClass,
	RuleIsolatedClass,
	RuleIsolatedNode,
	RuleLongLabel,
	RuleNoEntry,
	RuleOverlongMember,
	RuleOversizedClassDiagram,
	RuleRemovedField,
	RuleSeqTooManyParticipants,
	RuleShapeSoup,
	RuleTextLabelDropped,
	RuleTooLarge,
	RuleUnbreakableToken,
	RuleUnknownField,
	RuleUnlabeledAltSection,
	RuleUnlabeledBranch,
	RuleVagueEdgeLabel,
	RuleVagueMessageLabel,
}

var ruleSet = func() map[string]bool {
	m := make(map[string]bool, len(Rules))
	for _, r := range Rules {
		m[r] = true
	}
	return m
}()

// retiredRules are rule names that existed before a feature was removed.
// An ignore list may still name them: they validate and match nothing, so
// a spec written for an older diago keeps rendering.
var retiredRules = map[string]bool{
	"duplicate-hint": true, // hints removed in Phase 0 of the layered engine
	"hint-no-effect": true,
}

// Advisory thresholds. They are neat's, so the two tools agree on what
// "too big" means.
const (
	MaxLabelRunes   = 60
	MaxTokenRunes   = 24
	MaxNodes        = 20
	MaxShapes       = 5
	MaxParticipants = 8
	MaxNesting      = 3
	MaxClassMembers = 15
	MaxMemberRunes  = 40
)

var vagueLabels = map[string]bool{
	"uses": true, "has": true, "is": true, "does": true,
	"calls": true, "handles": true, "data": true, "flow": true,
}

// isVague reports whether a label, trimmed and lower-cased, is one of the
// words that say nothing about the relationship.
func isVague(label string) bool {
	return vagueLabels[strings.ToLower(strings.TrimSpace(label))]
}

// validateIgnore rejects ignore entries that name no rule.
func validateIgnore(ignore []string) ValidationErrors {
	var errs ValidationErrors
	for i, r := range ignore {
		if !ruleSet[r] && !retiredRules[r] {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("ignore[%d]", i),
				Message: fmt.Sprintf("unknown advisory rule %q; see diago check", r),
			})
		}
	}
	return errs
}

func ignoreSet(ignore []string) map[string]bool {
	set := make(map[string]bool, len(ignore))
	for _, r := range ignore {
		set[r] = true
	}
	return set
}

// dropIgnored removes every finding whose rule is in ignored.
func dropIgnored(advs []Advisory, ignored map[string]bool) []Advisory {
	if len(ignored) == 0 {
		return advs
	}
	out := advs[:0]
	for _, a := range advs {
		if !ignored[a.Rule] {
			out = append(out, a)
		}
	}
	return out
}

// Ignored returns the spec's ignore set. Callers that append findings after
// Check ran (the pipeline's text-label-dropped) use it to honour the list.
func Ignored(data []byte) (map[string]bool, error) {
	var probe struct {
		Ignore []string `json:"ignore"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("check: invalid JSON: %w", err)
	}
	return ignoreSet(probe.Ignore), nil
}

// SortAdvisories orders findings by rule, then field in natural order
// (nodes[9] before nodes[10]), then message. The order is what makes
// check output diffable between runs.
func SortAdvisories(a []Advisory) {
	sort.SliceStable(a, func(i, j int) bool {
		if a[i].Rule != a[j].Rule {
			return a[i].Rule < a[j].Rule
		}
		if c := naturalCompare(a[i].Field, a[j].Field); c != 0 {
			return c < 0
		}
		return a[i].Message < a[j].Message
	})
}

// naturalCompare compares two strings byte-wise except that runs of
// digits compare numerically (leading zeros ignored). Returns -1, 0 or 1.
func naturalCompare(a, b string) int {
	for a != "" && b != "" {
		if isDigit(a[0]) && isDigit(b[0]) {
			an, bn := digitRun(a), digitRun(b)
			ta, tb := strings.TrimLeft(a[:an], "0"), strings.TrimLeft(b[:bn], "0")
			if len(ta) != len(tb) {
				if len(ta) < len(tb) {
					return -1
				}
				return 1
			}
			if ta != tb {
				if ta < tb {
					return -1
				}
				return 1
			}
			a, b = a[an:], b[bn:]
			continue
		}
		if a[0] != b[0] {
			if a[0] < b[0] {
				return -1
			}
			return 1
		}
		a, b = a[1:], b[1:]
	}
	switch {
	case a == "" && b == "":
		return 0
	case a == "":
		return -1
	default:
		return 1
	}
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// digitRun returns the length of the leading run of digits in s.
func digitRun(s string) int {
	n := 0
	for n < len(s) && isDigit(s[n]) {
		n++
	}
	return n
}

// labelAdvisories returns the long-label and unbreakable-token findings for
// one label. subject reads like `node "x" label`; widens names what a
// too-wide token widens ("the node", "the group", "the diagram").
func labelAdvisories(field, subject, widens, text string) []Advisory {
	var out []Advisory
	if n := utf8.RuneCountInString(text); n > MaxLabelRunes {
		out = append(out, Advisory{
			Rule:  RuleLongLabel,
			Field: field,
			Message: fmt.Sprintf("%s is %d characters; over %d reads poorly even wrapped, shorten it or move detail elsewhere",
				subject, n, MaxLabelRunes),
		})
	}
	worst, worstLen := "", 0
	for _, tok := range strings.Fields(text) {
		if n := utf8.RuneCountInString(tok); n > MaxTokenRunes && n > worstLen {
			worst, worstLen = tok, n
		}
	}
	if worst != "" {
		out = append(out, Advisory{
			Rule:  RuleUnbreakableToken,
			Field: field,
			Message: fmt.Sprintf("%s has an unbreakable %d-character token %q; wrapping only breaks at whitespace, so anything over %d widens %s past the cap, shorten it or add separators",
				subject, worstLen, worst, MaxTokenRunes, widens),
		})
	}
	return out
}
