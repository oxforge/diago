package place

import (
	"fmt"
	"math"
	"testing"

	"github.com/oxforge/diago/internal/layout/layered/lgraph"
	"github.com/oxforge/diago/internal/layout/layered/lgraph/lgraphtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlace_ParentsAlignOnTheOffCenterPortTheyLandOn(t *testing.T) {
	// t has two in-edges, so its in-face splits into thirds: a's wire lands
	// a sixth of t's width left of its center, b's a sixth right. A third
	// of 400 clears the 120 the parents need, so both run straight.
	_, _, x := placed(t, screen, map[string]float64{"t": 400}, "a->t", "b->t")
	assert.InDelta(t, x("t")-400.0/6, x("a"), 1e-6)
	assert.InDelta(t, x("t")+400.0/6, x("b"), 1e-6)
}

func TestPlace_SingleParentChildrenSitUnderTheirParents(t *testing.T) {
	// a bus fanning out, with a retry worker three layers down feeding
	// back into it: the reversed chain must not drag the only children
	// off their parents
	_, _, x := placed(t, screen, nil,
		"checkout->bus", "signup->bus",
		"bus->analytic", "bus->audit", "bus->crm", "bus->emailc", "bus->fraud", "bus->ware",
		"emailc->dlq", "dlq->retry", "retry->bus", "fraud->slack", "audit->s3")
	assert.InDelta(t, x("audit"), x("s3"), 1e-6)
	assert.InDelta(t, x("emailc"), x("dlq"), 1e-6)
	assert.InDelta(t, x("fraud"), x("slack"), 1e-6)
}

func TestPlace_MiddleParentsSitNearTheirChildren(t *testing.T) {
	specs := []string{"ceo->vp0", "ceo->vp1", "ceo->vp2"}
	for vp := range 3 {
		for k := range 3 {
			specs = append(specs, fmt.Sprintf("vp%d->t%d%d", vp, vp, k))
		}
	}
	_, _, x := placed(t, screen, nil, specs...)
	for vp := range 3 {
		mean := 0.0
		for k := range 3 {
			mean += x(fmt.Sprintf("t%d%d", vp, k)) / 3
		}
		assert.LessOrEqual(t, math.Abs(x(fmt.Sprintf("vp%d", vp))-mean), 45.0)
	}
}

func TestPlace_ACounterFlowHopIsNoAlignment(t *testing.T) {
	// login and creds share a forward and a reversed edge, so login's
	// out-face holds two ports at -15 and +15. The reversed end attaches at
	// a side vertex of creds, so only the forward wire aligns, straight into
	// the apex.
	_, _, x := placed(t, screen, map[string]float64{"login": 90, "creds": 248},
		"login->creds:diamond", "creds->login")
	assert.InDelta(t, x("login")-15, x("creds"), 1e-6)
}

// dummyOf returns g's vertex index for the dummy with id.
func dummyOf(t *testing.T, g *lgraph.Graph, id string) int {
	t.Helper()
	for v, vx := range g.Vertices {
		if vx.ID == id {
			return v
		}
	}
	require.Failf(t, "no dummy", "%s", id)
	return -1
}

// TestPlace_ABackEdgeDoesNotPullANodeWithFaces pins S7's act 1: c->b is
// reversed, so its chain runs from b through a dummy beside diamond m to
// c, and its end on b is counter-flow, which the router attaches at a side
// (S8). That hop does not pull b, so b sits where its forward exit to m
// runs straight: m under b's port for b->m, the left one of the two its
// out-face holds in the first pass (m orders before the dummy). Pulled
// toward the dummy too, b sat 28.6 px off that line, beyond Span, and
// act 2 could not align a hop into a pinned node.
func TestPlace_ABackEdgeDoesNotPullANodeWithFaces(t *testing.T) {
	g, _, x := placed(t, screen, nil, "b->m:diamond", "m->c", "c->b")
	idx := g.Index()
	require.Less(t, idx[vertexOf(t, g, "m")], idx[dummyOf(t, g, "c->b#0@1")], "m orders before the dummy")
	assert.InDelta(t, x("b")-80.0/6, x("m"), 1e-6, "b->m runs straight from b's left port")
}

// TestPlace_ABackEdgeStillPullsAPinnedNode is the same graph with b a
// diamond: a pinned node keeps the counter-flow hop as a neighbor (S7, act
// 1), so the dummy pulls b off m's axis, beyond Span, to between the two.
func TestPlace_ABackEdgeStillPullsAPinnedNode(t *testing.T) {
	g, xs, x := placed(t, screen, nil, "b:diamond->m", "m->c", "c->b")
	dummy := xs[dummyOf(t, g, "c->b#0@1")]
	require.Less(t, x("m"), dummy, "m left of the dummy")
	assert.Greater(t, x("b")-x("m"), screen.Span, "b pulled off m's axis")
	assert.Less(t, x("b"), dummy, "b between m and the dummy")
}

// TestPlace_AHopIntoACounterFlowEndAddsNoWeight pins S7's act 1 on the
// weight: b->a is reversed, a one-hop chain from a to b whose ends on
// both are counter-flow, so the hop counts in neither node's weight and
// the level places as it does without b->a. On act 1's reference, c lies
// 14.3 px off d's middle in-port, within Span, and act 2 runs c->d
// straight. Counted, the hop made a and b one heavier, the row b, c, e
// pooled nearer b's target, and c lay 25 px off, beyond Span: c->d
// stayed bent.
func TestPlace_AHopIntoACounterFlowEndAddsNoWeight(t *testing.T) {
	specs := []string{"a->b", "b->d", "c->d", "e->d"}
	_, _, twin := placed(t, screen, nil, specs...)
	_, _, x := placed(t, screen, nil, append(specs, "b->a")...)
	assert.InDelta(t, x("d"), x("c"), 1e-9, "c->d runs straight into d's middle in-port")
	for _, id := range []string{"a", "c", "d", "e"} {
		assert.InDelta(t, twin(id)-twin("b"), x(id)-x("b"), 1e-9, "%s placed as without b->a", id)
	}
}

// TestPlace_AHopIntoACounterFlowEndIsNoSpanExemptNeighbor pins S7's act 1
// on Span-exempt links: c->a is reversed, its chain runs from a through a
// dummy beside b to c, and its ends on a and c are counter-flow. So, as
// act 1 counts neighbors, c's only upper neighbor is b and b's only lower
// one is c: b->c is span-exempt. On act 1's reference it lies 41.7 px off
// straight, beyond Span, so only the exemption lines it up, and the main
// line runs straight from a's left out-port through b into c's left
// in-port. Counted among c's neighbors, the dummy's hop left b->c to the
// span rule, and c stayed where act 1 put it.
func TestPlace_AHopIntoACounterFlowEndIsNoSpanExemptNeighbor(t *testing.T) {
	g, _, x := placed(t, screen, nil, "a->b", "b->c", "c->a")
	assert.InDelta(t, x("a")-80.0/6, x("b"), 1e-9, "a->b runs straight from a's left out-port")
	assert.InDelta(t, x("c")-80.0/6, x("b"), 1e-9, "b->c runs straight into c's left in-port")
	ref := newPlacer(g, screen).reference()
	b, c := vertexOf(t, g, "b"), vertexOf(t, g, "c")
	assert.Greater(t, math.Abs(ref[b]-(ref[c]-80.0/6)), screen.Span, "b->c beyond Span on act 1's reference")
}

func TestPlace_ABranchChildsOnlyChildIsSpanExempt(t *testing.T) {
	// m's two exits are forced to its side columns, c 60 left of m. c hangs
	// below a pinned node with two lower neighbors, and its hop to its only
	// child d is span-exempt all the same: f's pull leaves d beyond the span
	// off c in the reference, and the hop still lines d up under c, while
	// m's exits keep their forcing.
	_, _, x := placed(t, screen, nil, "m:diamond->c", "m->e", "c->d", "d->f", "e->f")
	assert.InDelta(t, x("m")-60, x("c"), 1e-9, "m's left exit forced under its column")
	assert.InDelta(t, x("m")+60, x("e"), 1e-9, "m's right exit forced under its column")
	assert.InDelta(t, x("c"), x("d"), 1e-9, "d under its only parent c")
}

func TestPlace_ALongEdgeLeavingAPinnedNodeKeepsTheSpanRule(t *testing.T) {
	// m->b runs through a dummy. Its first hop ends at a pinned node, so it
	// is not span-exempt and is not straightened under m at any residual:
	// the dummy stays one of m's side exits, and both exits are forced to
	// the columns, the dummy right and a left. (b leads on to z as a does
	// to b, so neither exit is m's main line, S7's primary clause.)
	g, xs, x := placed(t, screen, nil, "m:diamond->a", "a->b", "m->b", "b->z")
	dummy := -1
	for v, vx := range g.Vertices {
		if vx.ID == "m->b#0@1" {
			dummy = v
		}
	}
	require.GreaterOrEqual(t, dummy, 0)
	assert.InDelta(t, x("m")+60, xs[dummy], 1e-9)
	assert.InDelta(t, x("m")-60, x("a"), 1e-9)
}

func TestPlace_AHopIntoANodeWithTwoParentsKeepsTheSpanRule(t *testing.T) {
	// a's only child t has a second parent, b, so a->t is not span-exempt.
	// a lands on t's left in-port (a sixth of t's width left of its center)
	// only within the span; here it is 24 off and stays so.
	_, _, x := placed(t, screen, nil, "a->t", "b->t", "b->u")
	residual := math.Abs(x("a") - (x("t") - 80.0/6))
	assert.Greater(t, residual, screen.Span)
}

// TestPlace_AlignsOnAnAnchor pins S7 (S9): a hop into an end anchored on a
// group's face runs straight when the group sits so that the anchor lies
// under the hop's upper end.
func TestPlace_AlignsOnAnAnchor(t *testing.T) {
	lv := lgraphtest.Level(t, "a", "b", "g", "a->g", "b->g")
	lv.Nodes[2].Group, lv.Nodes[2].W, lv.Nodes[2].H = true, 400, 200
	lv.Edges[0].Anchor[1] = lgraph.Anchor{On: true, At: -120}
	lv.Edges[1].Anchor[1] = lgraph.Anchor{On: true, At: 90}
	_, x := laid(t, lv, screen)
	assert.InDelta(t, x("g")-120, x("a"), 1e-6)
	assert.InDelta(t, x("g")+90, x("b"), 1e-6)
}

// TestPlace_AFacedEndOfAReversedEdgeStaysOnItsFace pins S7 (S9): counterFlow
// excludes a group's or a terminal's end, so a reversed edge's end on a
// group keeps its port link instead of being dropped as a side attachment.
// g1 also feeds c, pulling g1's reference off the exact port g2 offers, so
// only the restored align link (not the soft barycenter pull alone) can
// snap g1 back onto it.
func TestPlace_AFacedEndOfAReversedEdgeStaysOnItsFace(t *testing.T) {
	lv := lgraphtest.Level(t, "g1->g2", "g2->g1", "g1->c")
	lv.Nodes[0].Group, lv.Nodes[0].W, lv.Nodes[0].H = true, 300, 150
	lv.Nodes[1].Group, lv.Nodes[1].W, lv.Nodes[1].H = true, 300, 150
	o := screen
	o.Span = 40 // wide enough for the port's residual to qualify as a candidate
	_, x := laid(t, lv, o)
	assert.InDelta(t, x("g1")-50, x("g2"), 1e-6)
}
