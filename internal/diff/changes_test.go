package diff

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFlowLegend_ChangedFields(t *testing.T) {
	for name, tc := range map[string]struct {
		before, after string
		class         bool
		want          string
	}{
		"edge, two fields": {
			`{"type":"flow","nodes":[{"id":"a","label":"API Gateway"},{"id":"b","label":"Postgres"}],"edges":[{"id":"q","from":"a","to":"b"}]}`,
			`{"type":"flow","nodes":[{"id":"a","label":"API Gateway"},{"id":"b","label":"Postgres"}],"edges":[{"id":"q","from":"a","to":"b","label":"read/write","style":"dashed"}]}`,
			false, `changed: edge API Gateway -> Postgres: label none → "read/write", style solid → dashed`},
		"node color": {
			`{"type":"flow","nodes":[{"id":"b","label":"Postgres"}],"edges":[]}`,
			`{"type":"flow","nodes":[{"id":"b","label":"Postgres","color":"red"}],"edges":[]}`,
			false, "changed: node Postgres: color none → red"},
		"node named by its after label": {
			`{"type":"flow","nodes":[{"id":"a","label":"API Server"}],"edges":[]}`,
			`{"type":"flow","nodes":[{"id":"a","label":"API Gateway"}],"edges":[]}`,
			false, `changed: node API Gateway: label "API Server" → "API Gateway"`},
		"class relation cardinality": {
			`{"type":"class","classes":[{"id":"c","label":"Customer"},{"id":"a","label":"Address"}],"relations":[{"id":"r","from":"c","to":"a","to_card":"1..*"}]}`,
			`{"type":"class","classes":[{"id":"c","label":"Customer"},{"id":"a","label":"Address"}],"relations":[{"id":"r","from":"c","to":"a","to_card":"0..*"}]}`,
			true, `changed: relation Customer -> Address: to_card "1..*" → "0..*"`},
		"class package move names package": {
			`{"type":"class","classes":[{"id":"c","label":"Customer"},{"id":"a","label":"Address"},{"id":"o","label":"Order"}],"packages":[{"id":"p","label":"Domain","contains":["c","a","o"]}]}`,
			`{"type":"class","classes":[{"id":"c","label":"Customer"},{"id":"a","label":"Address"},{"id":"o","label":"Order"}],"packages":[{"id":"p","label":"Domain","contains":["a","o"]}]}`,
			true, `changed: class Customer: package "Domain" → none`},
	} {
		t.Run(name, func(t *testing.T) {
			p := parse
			if tc.class {
				p = parseClass
			}
			assert.Contains(t, FlowLegend(BuildUnion(p(t, tc.before), p(t, tc.after))), tc.want)
		})
	}
}

func TestFlowChanges_StatusAndWire(t *testing.T) {
	u := BuildUnion(parse(t, specA), parse(t, `{"type":"flow","direction":"DOWN",
	 "nodes":[{"id":"d","label":"D"},{"id":"a","label":"A!"},{"id":"b","label":"B"}],
	 "edges":[{"from":"a","to":"b","label":"now"},{"from":"b","to":"d"}],
	 "groups":[{"id":"g","label":"G2","contains":["b"]}]}`))
	type sw struct {
		s    Status
		wire bool
	}
	var got []sw
	for _, c := range FlowChanges(u) {
		got = append(got, sw{c.Status, c.Wire})
	}
	assert.Equal(t, []sw{
		{Added, false}, {Added, true},
		{Removed, false}, {Removed, true},
		{Changed, false}, {Changed, true}, {Changed, false},
	}, got)
}

func TestFlowChanges_MemberLinesAreChangedBoxes(t *testing.T) {
	u := BuildUnion(
		parseClass(t, `{"type":"class","classes":[{"id":"p","label":"Payment","attributes":[{"visibility":"-","text":"amount: Money"}]}]}`),
		parseClass(t, `{"type":"class","classes":[{"id":"p","label":"Payment"}]}`))
	assert.Equal(t, []Change{
		{Status: Changed, Text: "changed: class Payment, removed attribute - amount: Money"},
	}, FlowChanges(u), "a members-only change is named by its member lines alone")
}

func TestFlowChanges_ClassWithOwnFieldsKeepsItsLine(t *testing.T) {
	u := BuildUnion(
		parseClass(t, `{"type":"class","classes":[{"id":"p","label":"Payment","attributes":[{"visibility":"-","text":"amount: Money"}]}]}`),
		parseClass(t, `{"type":"class","classes":[{"id":"p","label":"Payment","stereotype":"entity"}]}`))
	assert.Equal(t, []Change{
		{Status: Changed, Text: `changed: class Payment: stereotype none → "entity"`},
		{Status: Changed, Text: "changed: class Payment, removed attribute - amount: Money"},
	}, FlowChanges(u))
}

func TestFlowChanges_ReusedIDRemovedEdge(t *testing.T) {
	u := BuildUnion(
		parse(t, `{"type":"flow","nodes":[{"id":"w","label":"Worker"},{"id":"s","label":"SMTP"},{"id":"m","label":"Metrics"}],"edges":[{"id":"out","from":"w","to":"s"}]}`),
		parse(t, `{"type":"flow","nodes":[{"id":"w","label":"Worker"},{"id":"s","label":"SMTP"},{"id":"m","label":"Metrics"}],"edges":[{"id":"out","from":"w","to":"m"}]}`))
	assert.Equal(t, []Change{
		{Status: Added, Wire: true, Text: "added: edge Worker -> Metrics"},
		{Status: Removed, Wire: true, Text: "removed: edge Worker -> SMTP"},
	}, FlowChanges(u))
}

func TestSequenceLegend_ChangedMessageFields(t *testing.T) {
	actors := `"actors":[{"id":"u","label":"U"},{"id":"w","label":"W"}]`
	u := BuildUnionSequence(
		parseSeq(t, `{"type":"sequence",`+actors+`,"interactions":[{"from":"u","to":"w","label":"a"},{"from":"w","to":"u","label":"b"}]}`),
		parseSeq(t, `{"type":"sequence",`+actors+`,"interactions":[{"from":"u","to":"w","label":"a","style":"dashed"},{"from":"w","to":"u","label":"b","color":"red"}]}`))
	assert.Equal(t, []string{
		`changed: message U -> W "a": style solid → dashed`,
		`changed: message W -> U "b": color none → red`,
	}, SequenceLegend(u), "a message whose label did not change is named by it")
}

func TestSequenceChanges_StatusAndWire(t *testing.T) {
	u := BuildUnionSequence(parseSeq(t, loginV1), parseSeq(t, loginV2))
	type sw struct {
		s    Status
		wire bool
	}
	var got []sw
	for _, c := range SequenceChanges(u) {
		got = append(got, sw{c.Status, c.Wire})
	}
	assert.Equal(t, []sw{
		{Added, false},               // actor Audit Log
		{Added, true}, {Added, true}, // two messages
		{Added, false},                   // section [locked]
		{Changed, true}, {Changed, true}, // two messages
	}, got)
}

func TestTexts(t *testing.T) {
	assert.Equal(t, []string{"a", "b"}, Texts([]Change{{Text: "a"}, {Text: "b"}}))
	assert.Empty(t, Texts(nil))
}
