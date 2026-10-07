package schema

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// flowSpec builds a flow spec from node and edge JSON fragments.
func flowSpec(nodes, edges string, extra ...string) string {
	s := `{"type":"flow","nodes":[` + nodes + `],"edges":[` + edges + `]`
	for _, e := range extra {
		s += "," + e
	}
	return s + "}"
}

func TestCheckFlow_UnlabeledBranch(t *testing.T) {
	nodes := `{"id":"d","label":"D","shape":"diamond"},{"id":"a","label":"A"},{"id":"b","label":"B"}`
	// One outgoing edge: never fires.
	assert.Empty(t, mustCheck(t, flowSpec(nodes, `{"from":"d","to":"a"},{"from":"a","to":"b"}`), CheckOptions{}))
	// Two outgoing, both labeled: fine.
	assert.Empty(t, mustCheck(t, flowSpec(nodes, `{"from":"d","to":"a","label":"yes"},{"from":"d","to":"b","label":"no"}`), CheckOptions{}))
	// Two outgoing, one blank: fires, naming derived and explicit ids.
	advs := mustCheck(t, flowSpec(nodes, `{"from":"d","to":"a","label":"yes"},{"from":"d","to":"b","label":" "},{"id":"e3","from":"d","to":"b"}`, `"ignore":["duplicate-edge"]`), CheckOptions{})
	require.Equal(t, []string{"unlabeled-branch nodes[0]"}, keys(advs))
	assert.Equal(t, `decision "d" has unlabeled outgoing edges: d->b#0, e3; label every branch so each path's condition is clear`, advs[0].Message)
	// A rect with unlabeled fan-out is not a decision.
	assert.Empty(t, mustCheck(t, flowSpec(`{"id":"d","label":"D"},{"id":"a","label":"A"},{"id":"b","label":"B"}`, `{"from":"d","to":"a"},{"from":"d","to":"b"}`), CheckOptions{}))
	// A flat edge is not a branch: two labelled ordinary out-edges plus an
	// unlabeled flat one never fires (control: the same edge, not flat,
	// fires at line 28 above).
	assert.Empty(t, mustCheck(t, flowSpec(nodes, `{"from":"d","to":"a","label":"yes"},{"from":"d","to":"b","label":"no"},{"from":"d","to":"a","flat":true}`), CheckOptions{}))
}

func TestCheckFlow_VagueEdgeLabel(t *testing.T) {
	nodes := `{"id":"a","label":"A"},{"id":"b","label":"B"}`
	advs := mustCheck(t, flowSpec(nodes, `{"from":"a","to":"b","label":" Uses "},{"from":"b","to":"a","label":"validates order"}`, `"ignore":["no-entry"]`), CheckOptions{})
	require.Equal(t, []string{"vague-edge-label edges[0]"}, keys(advs))
	assert.Equal(t, `edge a->b label "Uses" says little; prefer a specific verb phrase like "validates order" or "publishes event"`, advs[0].Message)
}

func TestCheckFlow_LabelLength(t *testing.T) {
	long := strings.TrimSpace(strings.Repeat("a ", 31)) // 61 runes, every token short
	tok := strings.Repeat("b", 25)
	advs := mustCheck(t, flowSpec(
		`{"id":"a","label":"`+long+`"},{"id":"b","label":"x `+tok+`"}`,
		`{"from":"a","to":"b"}`,
		`"groups":[{"id":"g","label":"`+long+`","contains":["a"]}]`), CheckOptions{})
	assert.Equal(t, []string{"long-label groups[0]", "long-label nodes[0]", "unbreakable-token nodes[1]"}, keys(advs))
	assert.Contains(t, advs[0].Message, `group "g" label is 61 characters`)
	assert.Contains(t, advs[2].Message, "widens the node past the cap")
	// Exactly 60 and exactly 24 are fine.
	assert.Empty(t, mustCheck(t, flowSpec(`{"id":"a","label":"`+strings.Repeat("ab ", 20)+`"},{"id":"b","label":"`+strings.Repeat("b", 24)+`"}`, `{"from":"a","to":"b"}`), CheckOptions{}))
}

func TestCheckFlow_TooLarge(t *testing.T) {
	mk := func(n int) string {
		var nodes, edges []string
		for i := 0; i < n; i++ {
			nodes = append(nodes, fmt.Sprintf(`{"id":"n%d","label":"N%d"}`, i, i))
			if i > 0 {
				edges = append(edges, fmt.Sprintf(`{"from":"n%d","to":"n%d"}`, i-1, i))
			}
		}
		return flowSpec(strings.Join(nodes, ","), strings.Join(edges, ","), `"groups":[{"id":"g","label":"G","contains":["n0","n1"]}]`)
	}
	assert.Empty(t, mustCheck(t, mk(20), CheckOptions{}), "20 nodes plus a group is fine")
	advs := mustCheck(t, mk(21), CheckOptions{})
	require.Equal(t, []string{"too-large "}, keys(advs))
	assert.Equal(t, "21 nodes is a lot for one diagram (advice threshold: 20); consider decomposing into linked sub-diagrams", advs[0].Message)
}

func TestCheckFlow_NoEntry(t *testing.T) {
	nodes := `{"id":"a","label":"A"},{"id":"b","label":"B"}`
	advs := mustCheck(t, flowSpec(nodes, `{"from":"a","to":"b"},{"from":"b","to":"a"}`), CheckOptions{})
	require.Equal(t, []string{"no-entry "}, keys(advs))
	assert.Equal(t, "flow has no clear entry: every node has an incoming edge; add a start node or break a cycle so readers know where to begin", advs[0].Message)
	// A self-loop is not an incoming edge.
	assert.Empty(t, mustCheck(t, flowSpec(nodes, `{"from":"a","to":"b"},{"from":"a","to":"a"}`), CheckOptions{}))
	// One node, or no edges: never fires (the single self-looping node is also "touched", so no isolated-node either).
	assert.Empty(t, mustCheck(t, flowSpec(`{"id":"a","label":"A"}`, `{"from":"a","to":"a"}`), CheckOptions{}))
	// A flat edge implies no order, so it gives its target no "entry"
	// (control: the same pair, not flat, fires above).
	assert.Empty(t, mustCheck(t, flowSpec(nodes, `{"from":"a","to":"b"},{"from":"b","to":"a","flat":true}`), CheckOptions{}))
}

func TestCheckFlow_ShapeSoup(t *testing.T) {
	shapes := []string{"rect", "rounded", "circle", "diamond", "cylinder", "hexagon"}
	mk := func(n int) string {
		var nodes []string
		for i := 0; i < n; i++ {
			nodes = append(nodes, fmt.Sprintf(`{"id":"n%d","label":"N","shape":"%s"}`, i, shapes[i]))
		}
		nodes = append(nodes, `{"id":"x","label":"X"}`) // implicit rect
		return flowSpec(strings.Join(nodes, ","), `{"from":"n0","to":"x"}`, `"ignore":["isolated-node"]`)
	}
	assert.Empty(t, mustCheck(t, mk(5), CheckOptions{}), "rect, rounded, circle, diamond, cylinder plus an implicit rect is five")
	advs := mustCheck(t, mk(6), CheckOptions{})
	require.Equal(t, []string{"shape-soup "}, keys(advs))
	assert.Equal(t, "6 distinct shapes (circle, cylinder, diamond, hexagon, rect, rounded); more than 5 dilutes meaning, reserve each shape for one consistent role", advs[0].Message)
}

func TestCheckFlow_IsolatedNode(t *testing.T) {
	advs := mustCheck(t, flowSpec(`{"id":"a","label":"A"},{"id":"b","label":"B"},{"id":"c","label":"C"}`, `{"from":"a","to":"b"},{"from":"c","to":"c"}`), CheckOptions{})
	assert.Empty(t, advs, "a self-loop touches its node")
	advs = mustCheck(t, flowSpec(`{"id":"a","label":"A"},{"id":"b","label":"B"},{"id":"c","label":"C"}`, `{"from":"a","to":"b"}`), CheckOptions{})
	require.Equal(t, []string{"isolated-node nodes[2]"}, keys(advs))
	assert.Equal(t, `node "c" is connected to nothing; connect it or remove it`, advs[0].Message)
	assert.Empty(t, mustCheck(t, flowSpec(`{"id":"a","label":"A"}`, ``), CheckOptions{}), "one node is never isolated")
}

func TestCheckFlow_EmptyGroupAndDuplicateEdge(t *testing.T) {
	advs := mustCheck(t, flowSpec(`{"id":"a","label":"A"},{"id":"b","label":"B"}`,
		`{"from":"a","to":"b","label":"x"},{"from":"a","to":"b","label":" x "},{"from":"a","to":"b","label":"y"},{"from":"b","to":"a"},{"from":"b","to":"a"}`,
		`"groups":[{"id":"g","label":"G","contains":[]}],"ignore":["no-entry"]`), CheckOptions{})
	assert.Equal(t, []string{"duplicate-edge edges[1]", "duplicate-edge edges[4]", "empty-group groups[0]"}, keys(advs))
	assert.Equal(t, `edge a->b "x" repeats edges[0]; drop one or give them different labels`, advs[0].Message)
	assert.Equal(t, `edge b->a repeats edges[3]; drop one or give them different labels`, advs[1].Message)
	assert.Equal(t, `group "g" contains nothing and renders as an empty box`, advs[2].Message)
}

func TestCheckFlow_EdgeWithoutID(t *testing.T) {
	spec := flowSpec(`{"id":"a","label":"A"},{"id":"b","label":"B"}`,
		`{"from":"a","to":"b","label":"x"},{"id":"e","from":"a","to":"b","label":"y"},{"from":"b","to":"a"}`,
		`"ignore":["no-entry"]`)
	assert.Empty(t, mustCheck(t, spec, CheckOptions{}), "not anchored: silent")
	advs := mustCheck(t, spec, CheckOptions{Anchored: true})
	require.Equal(t, []string{"edge-without-id edges[0]"}, keys(advs))
	assert.Equal(t, `edge a->b "x" has no id; anchoring and diffs key on ids, so give it one that survives edits`, advs[0].Message)
}

func TestCheckFlow_DuplicateEdge(t *testing.T) {
	nodes := `{"id":"a","label":"A"},{"id":"b","label":"B"}`
	// Identical from, to, label without explicit ids: fires duplicate-edge.
	advs := mustCheck(t, flowSpec(nodes, `{"from":"a","to":"b","label":"x"},{"from":"a","to":"b","label":"x"}`, `"ignore":["no-entry"]`), CheckOptions{})
	require.Equal(t, []string{"duplicate-edge edges[1]"}, keys(advs))
	assert.Equal(t, `edge a->b "x" repeats edges[0]; drop one or give them different labels`, advs[0].Message)
	// Same pair with explicit distinct ids on both edges still produces duplicate-edge.
	advs = mustCheck(t, flowSpec(nodes, `{"id":"e1","from":"a","to":"b","label":"x"},{"id":"e2","from":"a","to":"b","label":"x"}`, `"ignore":["no-entry"]`), CheckOptions{})
	require.Equal(t, []string{"duplicate-edge edges[1]"}, keys(advs))
	assert.Equal(t, `edge a->b "x" repeats edges[0]; drop one or give them different labels`, advs[0].Message)
}
