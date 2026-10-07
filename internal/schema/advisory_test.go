package schema

import (
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRules_SortedAndComplete(t *testing.T) {
	assert.True(t, sort.StringsAreSorted(Rules), "Rules must be sorted")
	for _, r := range []string{
		RuleDeepNesting, RuleDuplicateEdge, RuleEdgeWithoutID, RuleEmptyGroup, RuleFlatEdgeRanked,
		RuleGodClass, RuleIsolatedClass, RuleIsolatedNode, RuleLongLabel, RuleNoEntry,
		RuleOverlongMember, RuleOversizedClassDiagram, RuleRemovedField, RuleSeqTooManyParticipants,
		RuleShapeSoup, RuleTextLabelDropped, RuleTooLarge, RuleUnbreakableToken, RuleUnknownField,
		RuleUnlabeledAltSection, RuleUnlabeledBranch, RuleVagueEdgeLabel, RuleVagueMessageLabel,
	} {
		assert.Contains(t, Rules, r)
	}
	assert.Len(t, Rules, 23)
}

func TestSortAdvisories_RuleThenNaturalFieldThenMessage(t *testing.T) {
	in := []Advisory{
		{Rule: "long-label", Field: "nodes[10]", Message: "b"},
		{Rule: "long-label", Field: "nodes[9]", Message: "a"},
		{Rule: "empty-group", Field: "groups[0]", Message: "c"},
		{Rule: "long-label", Field: "", Message: "z"},
		{Rule: "long-label", Field: "nodes[9]", Message: "0"},
	}
	SortAdvisories(in)
	got := make([]string, len(in))
	for i, a := range in {
		got[i] = a.Rule + "|" + a.Field + "|" + a.Message
	}
	assert.Equal(t, []string{
		"empty-group|groups[0]|c",
		"long-label||z",
		"long-label|nodes[9]|0",
		"long-label|nodes[9]|a",
		"long-label|nodes[10]|b",
	}, got)
}

func TestNaturalCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"nodes[2]", "nodes[10]", -1},
		{"nodes[10]", "nodes[2]", 1},
		{"nodes[2]", "nodes[2]", 0},
		{"", "a", -1},
		{"a", "", 1},
		{"edges[1].id", "edges[1]", 1},
		{"nodes[02]", "nodes[2]", 0},
		{"before.nodes[1]", "after.nodes[1]", 1},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, naturalCompare(c.a, c.b), "%q vs %q", c.a, c.b)
	}
}

func TestAdvisoryString(t *testing.T) {
	assert.Equal(t, "long-label nodes[3]: too long", Advisory{Rule: "long-label", Field: "nodes[3]", Message: "too long"}.String())
	assert.Equal(t, "too-large: 25 nodes", Advisory{Rule: "too-large", Message: "25 nodes"}.String())
}

func TestLabelAdvisories_Boundaries(t *testing.T) {
	long60 := strings.Repeat("ab ", 20) // 60 runes, every token short
	long61 := long60 + "c"
	tok24 := "abcdefghijklmnopqrstuvwx"
	tok25 := tok24 + "y"
	require.Len(t, []rune(long60), 60)
	require.Len(t, []rune(tok24), 24)

	assert.Empty(t, labelAdvisories("nodes[0]", `node "x" label`, "the node", long60))
	got := labelAdvisories("nodes[0]", `node "x" label`, "the node", long61)
	require.Len(t, got, 1)
	assert.Equal(t, RuleLongLabel, got[0].Rule)
	assert.Equal(t, "nodes[0]", got[0].Field)
	assert.Equal(t, `node "x" label is 61 characters; over 60 reads poorly even wrapped, shorten it or move detail elsewhere`, got[0].Message)

	assert.Empty(t, labelAdvisories("nodes[0]", `node "x" label`, "the node", "ok "+tok24))
	got = labelAdvisories("nodes[0]", `node "x" label`, "the node", "ok "+tok25+" and "+tok24+"zz")
	require.Len(t, got, 1)
	assert.Equal(t, RuleUnbreakableToken, got[0].Rule)
	assert.Equal(t, `node "x" label has an unbreakable 26-character token "`+tok24+`zz"; wrapping only breaks at whitespace, so anything over 24 widens the node past the cap, shorten it or add separators`, got[0].Message)

	// Runes, not bytes: 30 two-byte runes is 30 characters.
	assert.Empty(t, labelAdvisories("nodes[0]", "s", "the node", "ééééé ééééé ééééé ééééé ééééé ééééé"))
}

func TestIsVague(t *testing.T) {
	for _, v := range []string{"uses", " Has ", "IS", "does", "calls", "handles", "data", "flow"} {
		assert.True(t, isVague(v), v)
	}
	assert.False(t, isVague("validates order"))
	assert.False(t, isVague(""))
}

func TestIgnored(t *testing.T) {
	set, err := Ignored([]byte(`{"type":"flow","ignore":["too-large","no-entry"]}`))
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"too-large": true, "no-entry": true}, set)
	set, err = Ignored([]byte(`{"type":"flow"}`))
	require.NoError(t, err)
	assert.Empty(t, set)
	_, err = Ignored([]byte(`{`))
	assert.Error(t, err)
}

func TestParse_IgnoreValidation(t *testing.T) {
	_, err := ParseFlow([]byte(`{"type":"flow","nodes":[{"id":"a","label":"A"}],"edges":[],"ignore":["too-large","nope","too-large"]}`))
	var ve ValidationErrors
	require.ErrorAs(t, err, &ve)
	require.Len(t, ve, 1)
	assert.Equal(t, "ignore[1]", ve[0].Field)
	assert.Equal(t, `unknown advisory rule "nope"; see diago check`, ve[0].Message)

	_, err = ParseFlow([]byte(`{"type":"flow","nodes":[{"id":"a","label":"A"}],"edges":[],"ignore":["too-large","too-large"]}`))
	assert.NoError(t, err, "duplicates are allowed")

	_, err = ParseSequence([]byte(`{"type":"sequence","actors":[{"id":"a","label":"A"}],"interactions":[],"ignore":["deep-nesting","x"]}`))
	require.ErrorAs(t, err, &ve)
	require.Len(t, ve, 1)
	assert.Equal(t, "ignore[1]", ve[0].Field)
}

func TestValidateIgnore_RetiredHintRulesAccepted(t *testing.T) {
	// Specs written before hints were removed may still silence the two
	// hint rules; those names validate but match nothing.
	assert.Empty(t, validateIgnore([]string{"duplicate-hint", "hint-no-effect"}))
	assert.NotEmpty(t, validateIgnore([]string{"no-such-rule"}))
	assert.NotContains(t, Rules, "duplicate-hint")
	assert.NotContains(t, Rules, "hint-no-effect")
}
