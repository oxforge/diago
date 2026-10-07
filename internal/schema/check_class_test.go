package schema

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func classAdvisories(t *testing.T, spec string, opts CheckOptions) []Advisory {
	t.Helper()
	advs, err := Check([]byte(spec), opts)
	require.NoError(t, err)
	return advs
}

func TestCheckClass_GodClass(t *testing.T) {
	var members []string
	for i := 0; i < 17; i++ {
		members = append(members, fmt.Sprintf(`{"visibility":"+","text":"m%d()"}`, i))
	}
	spec := `{"type":"class","classes":[{"id":"order","methods":[` + strings.Join(members, ",") + `]}]}`
	advs := classAdvisories(t, spec, CheckOptions{})
	require.Len(t, advs, 1)
	assert.Equal(t, Advisory{Rule: RuleGodClass, Field: "classes[0]",
		Message: `class "order" has 17 members (advice threshold: 15); it is likely doing too much, split its responsibilities into smaller classes`}, advs[0])
}

func TestCheckClass_Oversized(t *testing.T) {
	var classes []string
	for i := 0; i < 23; i++ {
		classes = append(classes, fmt.Sprintf(`{"id":"c%d"}`, i))
	}
	var rels []string
	for i := 1; i < 23; i++ {
		rels = append(rels, fmt.Sprintf(`{"from":"c0","to":"c%d"}`, i))
	}
	spec := `{"type":"class","classes":[` + strings.Join(classes, ",") + `],"relations":[` + strings.Join(rels, ",") + `]}`
	advs := classAdvisories(t, spec, CheckOptions{})
	require.Len(t, advs, 1)
	assert.Equal(t, RuleOversizedClassDiagram, advs[0].Rule)
	assert.Equal(t, "", advs[0].Field)
	assert.Equal(t, "23 classes is a lot for one diagram (advice threshold: 20); consider decomposing into linked sub-diagrams", advs[0].Message)
}

func TestCheckClass_Isolated(t *testing.T) {
	spec := `{"type":"class","classes":[{"id":"a"},{"id":"b"},{"id":"audit"}],"relations":[{"from":"a","to":"b"}]}`
	advs := classAdvisories(t, spec, CheckOptions{})
	require.Len(t, advs, 1)
	assert.Equal(t, Advisory{Rule: RuleIsolatedClass, Field: "classes[2]",
		Message: `class "audit" has no relations; connect it to the diagram or remove it`}, advs[0])
	assert.Empty(t, classAdvisories(t, `{"type":"class","classes":[{"id":"only"}]}`, CheckOptions{}), "a single class is never isolated")
}

func TestCheckClass_OverlongMember(t *testing.T) {
	spec := `{"type":"class","classes":[{"id":"ledger","attributes":[{"visibility":"-","text":"x: int"}],
	  "methods":[{"visibility":"+","text":"reconcile(ledger: Ledger, at: Date): Report"}]}]}`
	advs := classAdvisories(t, spec, CheckOptions{})
	require.Len(t, advs, 1)
	assert.Equal(t, Advisory{Rule: RuleOverlongMember, Field: "classes[0].methods[0].text",
		Message: `member "+ reconcile(ledger: Ledger, at: Date): Report" is 45 runes long (advice threshold: 40); it widens the box, shorten the signature or its types`}, advs[0])
}

func TestCheckClass_EdgeWithoutIDOnlyAnchored(t *testing.T) {
	spec := `{"type":"class","classes":[{"id":"order"},{"id":"item"}],"relations":[{"from":"order","to":"item","kind":"composition"},{"id":"r","from":"item","to":"order"}]}`
	assert.Empty(t, classAdvisories(t, spec, CheckOptions{}))
	advs := classAdvisories(t, spec, CheckOptions{Anchored: true})
	require.Len(t, advs, 1)
	assert.Equal(t, Advisory{Rule: RuleEdgeWithoutID, Field: "relations[0]",
		Message: "relation order->item has no id; anchoring and diffs key on ids, so give it one that survives edits"}, advs[0])
}

func TestCheckClass_PackagesAndUnknownField(t *testing.T) {
	spec := `{"type":"class","theme":"dark","clases":[],"classes":[{"id":"a","attrs":[]}],"packages":[{"id":"p","label":"P","contains":[]}]}`
	advs := classAdvisories(t, spec, CheckOptions{})
	got := make([]string, len(advs))
	for i, a := range advs {
		got[i] = a.String()
	}
	assert.Equal(t, []string{
		`empty-group packages[0]: group "p" contains nothing and renders as an empty box`,
		`unknown-field clases: unknown key "clases" is ignored; did you mean "classes"?`,
		`unknown-field classes[0].attrs: unknown key "attrs" is ignored`,
	}, got)
}

func TestCheckClass_NoFlowRulesLeak(t *testing.T) {
	// A 70-rune class label and a vague relation label: neither long-label
	// nor vague-edge-label runs on a class document.
	spec := `{"type":"class","classes":[{"id":"a","label":"` + strings.Repeat("x", 70) + `"},{"id":"b"}],"relations":[{"from":"a","to":"b","label":"uses"}]}`
	assert.Empty(t, classAdvisories(t, spec, CheckOptions{}))
}

func TestCheckClass_IgnoreApplies(t *testing.T) {
	spec := `{"type":"class","classes":[{"id":"a"},{"id":"b"}],"ignore":["isolated-class"]}`
	assert.Empty(t, classAdvisories(t, spec, CheckOptions{}))
}
