package diff

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// driftBefore is the base the drift tests edit: an order path in a group.
const driftBefore = `{"type":"flow","direction":"DOWN",
 "nodes":[{"id":"api","label":"API"},{"id":"svc","label":"Order Service"},{"id":"db","label":"Orders DB","shape":"cylinder"}],
 "edges":[{"id":"e1","from":"api","to":"svc","label":"places order"},{"id":"e2","from":"svc","to":"db","label":"writes order"}],
 "groups":[{"id":"backend","label":"Backend","contains":["svc","db"]}]}`

// A v2 written afresh instead of copied gives the same things new ids: each
// is removed under its old id and added under its new one.
func TestGraphDrift_NewIDsForTheSameThings(t *testing.T) {
	after := parse(t, `{"type":"flow","direction":"DOWN",
	 "nodes":[{"id":"gateway","label":"API"},{"id":"svc","label":"Order Service"},{"id":"db","label":"Orders DB","shape":"cylinder"}],
	 "edges":[{"id":"api-to-svc","from":"gateway","to":"svc","label":"places order"},{"id":"e2","from":"svc","to":"db","label":"writes order"}],
	 "groups":[{"id":"core","label":"Backend","contains":["svc","db"]}]}`)
	assert.Equal(t, []Drift{
		{Kind: DriftNode, AfterID: "gateway", BeforeID: "api", Name: "API"},
		{Kind: DriftEdge, AfterID: "api-to-svc", BeforeID: "e1", Name: "API -> Order Service"},
		{Kind: DriftGroup, AfterID: "core", BeforeID: "backend", Name: "Backend"},
	}, GraphDrift(parse(t, driftBefore), after))
}

// A v2 copied from v1 and edited keeps its ids: a relabel is a change and a
// real replacement has a label of its own, so neither is drift.
func TestGraphDrift_NoneWhenIDsAreKept(t *testing.T) {
	after := parse(t, `{"type":"flow","direction":"DOWN",
	 "nodes":[{"id":"api","label":"Gateway"},{"id":"svc","label":"Order Service"},{"id":"queue","label":"Order Queue","shape":"cylinder"}],
	 "edges":[{"id":"e1","from":"api","to":"svc","label":"places order"},{"id":"e3","from":"svc","to":"queue","label":"publishes order"}],
	 "groups":[{"id":"backend","label":"Backend","contains":["svc","queue"]}]}`)
	assert.Empty(t, GraphDrift(parse(t, driftBefore), after))
}

// An edge that kept its id while one of its ends drifted is removed and
// added under that one id: it already has the right id, so only the node
// is drift.
func TestGraphDrift_NotAnEdgeThatKeptItsID(t *testing.T) {
	after := parse(t, `{"type":"flow","direction":"DOWN",
	 "nodes":[{"id":"api","label":"API"},{"id":"orders","label":"Order Service"},{"id":"db","label":"Orders DB","shape":"cylinder"}],
	 "edges":[{"id":"e1","from":"api","to":"orders","label":"places order"},{"id":"e2","from":"orders","to":"db","label":"writes order"}],
	 "groups":[{"id":"backend","label":"Backend","contains":["orders","db"]}]}`)
	assert.Equal(t, []Drift{{Kind: DriftNode, AfterID: "orders", BeforeID: "svc", Name: "Order Service"}},
		GraphDrift(parse(t, driftBefore), after))
}

// An edge without an id of its own takes the derived from->to#n, which
// shifts when a sibling between the same ends goes: it has no id to keep,
// and edge-without-id is the finding that covers it, so it is not drift.
func TestGraphDrift_NotAnEdgeWithADerivedID(t *testing.T) {
	nodes := `"nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}]`
	before := parse(t, `{"type":"flow",`+nodes+`,"edges":[{"id":"r","from":"a","to":"b","label":"retries"},{"from":"a","to":"b","label":"reads"}]}`)
	after := parse(t, `{"type":"flow",`+nodes+`,"edges":[{"from":"a","to":"b","label":"reads"}]}`)
	assert.Empty(t, GraphDrift(before, after))
}

// Several removed things under one label pair with the added ones in spec
// order, the first removed with the first added.
func TestGraphDrift_PairsInSpecOrder(t *testing.T) {
	before := parse(t, `{"type":"flow","nodes":[{"id":"q","label":"Queue"},{"id":"w1","label":"Worker"},{"id":"w2","label":"Worker"}],
	 "edges":[{"id":"a","from":"q","to":"w1"},{"id":"b","from":"q","to":"w2"}]}`)
	after := parse(t, `{"type":"flow","nodes":[{"id":"q","label":"Queue"},{"id":"x1","label":"Worker"},{"id":"x2","label":"Worker"}],
	 "edges":[{"id":"a","from":"q","to":"x1"},{"id":"b","from":"q","to":"x2"}]}`)
	assert.Equal(t, []Drift{
		{Kind: DriftNode, AfterID: "x1", BeforeID: "w1", Name: "Worker"},
		{Kind: DriftNode, AfterID: "x2", BeforeID: "w2", Name: "Worker"},
	}, GraphDrift(before, after))
}

// A class graph drifts the same way: its classes, relations and packages
// are the graph's nodes, edges and groups.
func TestGraphDrift_ClassGraph(t *testing.T) {
	before := parseClass(t, `{"type":"class","classes":[{"id":"order","label":"Order"},{"id":"item","label":"Item"}],
	 "relations":[{"id":"r1","from":"order","to":"item","kind":"composition"}],
	 "packages":[{"id":"domain","label":"Domain","contains":["order","item"]}]}`)
	after := parseClass(t, `{"type":"class","classes":[{"id":"Order","label":"Order"},{"id":"item","label":"Item"}],
	 "relations":[{"id":"order-items","from":"Order","to":"item","kind":"composition"}],
	 "packages":[{"id":"domain","label":"Domain","contains":["Order","item"]}]}`)
	assert.Equal(t, []Drift{
		{Kind: DriftNode, AfterID: "Order", BeforeID: "order", Name: "Order"},
		{Kind: DriftEdge, AfterID: "order-items", BeforeID: "r1", Name: "Order -> Item"},
	}, GraphDrift(before, after))
}

func TestSequenceDrift_AnActorUnderANewID(t *testing.T) {
	after := parseSeq(t, `{"type":"sequence",
	 "actors":[{"id":"user","label":"User"},{"id":"w","label":"Web"},{"id":"a","label":"Auth"}],
	 "interactions":[{"from":"user","to":"w","label":"login"},{"from":"w","to":"user","label":"welcome","style":"dashed"}]}`)
	assert.Equal(t, []Drift{{Kind: DriftActor, AfterID: "user", BeforeID: "u", Name: "User"}}, SequenceDrift(parseSeq(t, seqA), after))
}

func TestSequenceDrift_NoneWhenIDsAreKept(t *testing.T) {
	after := parseSeq(t, `{"type":"sequence",
	 "actors":[{"id":"u","label":"Customer"},{"id":"w","label":"Web"},{"id":"c","label":"Cache"}],
	 "interactions":[{"from":"u","to":"w","label":"login"},{"from":"w","to":"c","label":"look up session"}]}`)
	assert.Empty(t, SequenceDrift(parseSeq(t, seqA), after))
}
