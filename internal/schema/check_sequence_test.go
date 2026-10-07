package schema

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seqSpec(actors, interactions string, extra ...string) string {
	s := `{"type":"sequence","actors":[` + actors + `],"interactions":[` + interactions + `]`
	for _, e := range extra {
		s += "," + e
	}
	return s + "}"
}

func TestCheckSequence_Participants(t *testing.T) {
	mk := func(n int) string {
		var a []string
		for i := 0; i < n; i++ {
			a = append(a, fmt.Sprintf(`{"id":"p%d","label":"P%d"}`, i, i))
		}
		return seqSpec(strings.Join(a, ","), `{"from":"p0","to":"p1","label":"hi"}`)
	}
	assert.Empty(t, mustCheck(t, mk(8), CheckOptions{}))
	advs := mustCheck(t, mk(9), CheckOptions{})
	require.Equal(t, []string{"seq-too-many-participants "}, keys(advs))
	assert.Equal(t, "9 participants is a lot for one sequence diagram (advice threshold: 8); consider splitting into focused diagrams or collapsing minor participants", advs[0].Message)
}

func TestCheckSequence_Labels(t *testing.T) {
	long := strings.TrimSpace(strings.Repeat("x ", 31)) // 61 runes, every token short
	tok := strings.Repeat("y", 25)
	advs := mustCheck(t, seqSpec(
		`{"id":"a","label":"`+long+`"},{"id":"b","label":"B"}`,
		`{"from":"a","to":"b","label":"calls"},{"from":"b","to":"a","label":"has `+tok+`"}`,
		`"fragments":[{"type":"loop","over":["a"],"sections":[{"label":"`+long+`","start":0,"end":1}]}]`), CheckOptions{})
	assert.Equal(t, []string{
		"long-label actors[0]",
		"long-label fragments[0].sections[0]",
		"unbreakable-token interactions[1]",
		"vague-message-label interactions[0]",
	}, keys(advs))
	assert.Contains(t, advs[0].Message, `actor "a" label is 61 characters`)
	assert.True(t, strings.HasPrefix(advs[1].Message, `section "`+long+`" of fragments[0] label is 61 characters`))
	assert.Contains(t, advs[2].Message, "message b->a at interactions[1] label has an unbreakable 25-character token")
	assert.Contains(t, advs[2].Message, "widens the diagram past the cap")
	assert.Equal(t, `message a->b at interactions[0] label "calls" says little; prefer a specific verb phrase like "validates order" or "publishes event"`, advs[3].Message)
}

func TestCheckSequence_UnlabeledAltSection(t *testing.T) {
	advs := mustCheck(t, seqSpec(`{"id":"a","label":"A"}`, `{"from":"a","to":"a","label":"x"},{"from":"a","to":"a","label":"y"}`,
		`"fragments":[{"type":"alt","over":["a"],"sections":[{"label":"ok","start":0,"end":0},{"label":" ","start":1,"end":1}]},
		             {"type":"opt","over":["a"],"sections":[{"label":"","start":0,"end":0}]}]`), CheckOptions{})
	require.Equal(t, []string{"unlabeled-alt-section fragments[0].sections[1]"}, keys(advs), "opt sections may be blank")
	assert.Equal(t, "alt section fragments[0].sections[1] has no guard label; label each section with its condition so readers know which path runs", advs[0].Message)
}

func TestCheckSequence_DeepNesting(t *testing.T) {
	var msgs []string
	for i := 0; i < 8; i++ {
		msgs = append(msgs, `{"from":"a","to":"a","label":"m"}`)
	}
	frag := func(typ, label string, lo, hi int) string {
		return fmt.Sprintf(`{"type":"%s","over":["a"],"sections":[{"label":"%s","start":%d,"end":%d}]}`, typ, label, lo, hi)
	}
	three := seqSpec(`{"id":"a","label":"A"}`, strings.Join(msgs, ","),
		`"fragments":[`+frag("loop", "outer", 0, 7)+`,`+frag("opt", "mid", 1, 6)+`,`+frag("opt", "inner", 2, 5)+`]`)
	assert.Empty(t, mustCheck(t, three, CheckOptions{}), "three levels are fine")

	four := seqSpec(`{"id":"a","label":"A"}`, strings.Join(msgs, ","),
		`"fragments":[`+frag("opt", "", 3, 4)+`,`+frag("loop", "outer", 0, 7)+`,`+frag("opt", "mid", 1, 6)+`,`+frag("opt", "inner", 2, 5)+`]`)
	advs := mustCheck(t, four, CheckOptions{})
	require.Equal(t, []string{"deep-nesting fragments[0]"}, keys(advs))
	assert.Equal(t, `fragment "opt" (fragments[0]) is nested 4 levels deep; over 3 gets hard to read, flatten it or split into a referenced diagram`, advs[0].Message)

	// Equal ranges do not nest.
	equal := seqSpec(`{"id":"a","label":"A"}`, strings.Join(msgs, ","),
		`"fragments":[`+frag("loop", "x", 0, 7)+`,`+frag("opt", "x", 0, 7)+`,`+frag("opt", "x", 0, 7)+`,`+frag("opt", "x", 0, 7)+`]`)
	assert.Empty(t, mustCheck(t, equal, CheckOptions{}))
}

func TestFragmentDepth(t *testing.T) {
	ranges := []span{{0, 9, true}, {1, 8, true}, {2, 3, true}, {5, 8, true}, {0, 0, false}}
	memo := map[int]int{}
	assert.Equal(t, 1, fragmentDepth(ranges, 0, memo))
	assert.Equal(t, 2, fragmentDepth(ranges, 1, memo))
	assert.Equal(t, 3, fragmentDepth(ranges, 2, memo))
	assert.Equal(t, 3, fragmentDepth(ranges, 3, memo))
	assert.Equal(t, 1, fragmentDepth(ranges, 4, memo), "a fragment without sections is neither container nor contained")
}
