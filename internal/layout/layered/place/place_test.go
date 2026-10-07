package place

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/layout/layered/cycle"
	"github.com/oxforge/diago/internal/layout/layered/lgraph"
	"github.com/oxforge/diago/internal/layout/layered/lgraph/lgraphtest"
	"github.com/oxforge/diago/internal/layout/layered/order"
	"github.com/oxforge/diago/internal/layout/layered/ports"
	"github.com/oxforge/diago/internal/layout/layered/rank"
	"github.com/oxforge/diago/internal/layoutdbg"
	"github.com/oxforge/diago/internal/model"
)

var screen = Options{Gap: 40, Span: 16, Reach: 20, Clearance: 12, Dir: model.Down}

// placed runs S2 to S7 over a fixture level; widths overrides node widths
// by id before layering. It returns the graph, the positions Place gave it
// (so a caller needing the raw slice does not have to call Place again),
// and an id-keyed accessor.
func placed(t *testing.T, o Options, widths map[string]float64, specs ...string) (*lgraph.Graph, []float64, func(id string) float64) {
	t.Helper()
	lv := lgraphtest.Level(t, specs...)
	for i := range lv.Nodes {
		if w, ok := widths[lv.Nodes[i].ID]; ok {
			lv.Nodes[i].W = w
		}
	}
	ctx := context.Background()
	reversed := cycle.Break(ctx, lv, nil)
	g, err := lgraph.Build(lv, rank.Assign(ctx, lv, reversed, nil), reversed, 8)
	require.NoError(t, err)
	g.Layers = order.Minimize(ctx, g, false)
	x := Place(ctx, g, o)
	require.Len(t, x, len(g.Vertices))
	return g, x, func(id string) float64 {
		for i, n := range lv.Nodes {
			if n.ID == id {
				return x[i]
			}
		}
		require.Failf(t, "no node", "%s", id)
		return math.NaN()
	}
}

func TestPlace_AChainIsStraight(t *testing.T) {
	_, _, x := placed(t, screen, nil, "a->b", "b->c", "c->d")
	for _, id := range []string{"b", "c", "d"} {
		assert.Equal(t, x("a"), x(id), id)
	}
}

func TestPlace_DiamondBranchesAreSymmetric(t *testing.T) {
	_, _, x := placed(t, screen, nil, "a->b", "a->c", "b->d", "c->d")
	assert.InDelta(t, x("a"), x("d"), 1e-6)
	left, right := min(x("b"), x("c")), max(x("b"), x("c"))
	assert.InDelta(t, x("a")-left, right-x("a"), 1e-6)
}

func TestPlace_AFanIsCenteredAndEven(t *testing.T) {
	_, _, x := placed(t, screen, nil, "root->a", "root->b", "root->c", "root->d")
	assert.InDelta(t, (x("a")+x("d"))/2, x("root"), 1e-6)
	assert.InDelta(t, x("b")-x("a"), x("c")-x("b"), 1e-6)
	assert.InDelta(t, x("b")-x("a"), x("d")-x("c"), 1e-6)
}

func TestPlace_ABinaryTreeIsMirrorSymmetric(t *testing.T) {
	_, _, x := placed(t, screen, nil, "r->a", "r->b", "a->aa", "a->ab", "b->ba", "b->bb")
	axis := x("r")
	assert.InDelta(t, 2*axis, x("aa")+x("bb"), 1e-6)
	assert.InDelta(t, 2*axis, x("ab")+x("ba"), 1e-6)
	assert.InDelta(t, 2*axis, x("a")+x("b"), 1e-6)
}

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

// TestPlace_ASideExitHeadsForItsPort pins S7's side-exit forcing on the
// port an exit ends on. b has two in-ports, m->b's the left one, 80/6 in
// from its center. On the baseline b's center lies 3.33 px beyond m's
// right column (50 + 20 out), but that port 10 px inside it, so the exit
// is forced and the port lands exactly under the column, where the router
// draws an L. Judged by b's center, it was not forced, and the router
// slid the port 10 px onto the column instead.
func TestPlace_ASideExitHeadsForItsPort(t *testing.T) {
	widths := map[string]float64{"m": 100, "b": 80, "z": 60}
	specs := []string{"m:diamond->a", "m->b", "z->b"}
	_, x := placed1(t, screen, widths, specs...)
	assert.InDelta(t, x("m")+70, x("b")-80.0/6, 1e-9, "m->b's port under m's right column")
	_, got := decisions(t, screen, widths, specs...)
	assert.Contains(t, forcedEdges(got), "m->b#0")
}

// TestPlace_ExitsHeadForTheirPorts pins S7's exit headings on ports, the
// placement twin of TestRoute_ExitsHeadForTheirPorts: m's two exits both
// end on a, 400 wide, whose in-face holds their ports at its thirds,
// 66.7 either side of its center. Act 1 centers a under m and act 2
// leaves it there (neither link is within the span), so both centers lie
// on m's axis, within the band, while the ports lie beyond it, left and
// right. Headed for their ports, the exits take m's left and right
// vertices, each port beyond its side column (40 + 20 out), which the
// router moves out over it: nothing is forced, and a stays under m.
// Headed for the centers, the first would take the right vertex and the
// other the bottom, and the right one, whose port lies 126.7 short of the
// right column, would be forced, pulling a 126.7 off m's axis. (The
// primary's trigger, its port off the axis, heads the same way; a center
// exactly on the axis with its port off it is all but unreachable, so
// this case pins the heading for both.)
func TestPlace_ExitsHeadForTheirPorts(t *testing.T) {
	widths := map[string]float64{"a": 400}
	specs := []string{"m:diamond->a", "m->a"}
	_, x := placed1(t, screen, widths, specs...)
	assert.InDelta(t, x("m"), x("a"), 1e-9, "a under m")
	_, got := decisions(t, screen, widths, specs...)
	assert.Empty(t, forcedEdges(got), "both ports beyond their columns")
	for _, d := range got {
		assert.NotEqual(t, "side_exit_dropped", d["decision"], "%v", d)
	}
}

// TestPlace_AReachedPortIsNotForcedBack pins the other half: m's three
// exits take left, bottom and right, the second m->b the right vertex, and
// its port is b's right one, 20 px out from b's center. That port lies
// beyond m's right column (30 + 20 out), which the router moves out over
// it, so the exit is not forced although b's center lies inside the
// column. Judged by the center, the forcing pulled b 10 px inward to put
// the port on the column.
func TestPlace_AReachedPortIsNotForcedBack(t *testing.T) {
	widths := map[string]float64{"m": 60, "b": 120, "a": 40}
	specs := []string{"m:diamond->a", "m->b", "m->b"}
	_, x := placed1(t, screen, widths, specs...)
	require.Less(t, x("b")-x("m"), 50.0, "b's center inside m's right column")
	assert.Greater(t, x("b")+20-x("m"), 50+1.0, "m->b#1's port beyond the column")
	_, got := decisions(t, screen, widths, specs...)
	assert.NotContains(t, forcedEdges(got), "m->b#1")
}

// TestPlace_ARailedSideExitIsForcedOntoItsColumn pins S7's reach on a
// side rail: three of m's four exits head for circle n and share m's left
// vertex, where the router lands them on one column Reach out and never
// moves it out (S8). On the baseline, z pulls n 2.86 px beyond that column,
// which a lone column would move out to meet; the rail cannot, so the exit
// is forced and n's in-vertex lands exactly under it.
func TestPlace_ARailedSideExitIsForcedOntoItsColumn(t *testing.T) {
	widths := map[string]float64{"z": 80, "k": 160}
	specs := []string{"z->n:circle", "m:diamond->n", "m->n", "m->n", "m->k"}
	_, x := placed1(t, screen, widths, specs...)
	assert.InDelta(t, x("m")-60, x("n"), 1e-9, "n under m's left column")
	_, got := decisions(t, screen, widths, specs...)
	assert.Contains(t, forcedEdges(got), "m->n#0")
}

// TestPlace_ForcingIsDecidedAgainAfterTheRerun pins S7's recheck. On the
// baseline, a, squeezed against b, keeps b beyond m's right column (80 +
// 20 out), so only m->a is forced. The rerun moves a out to m's left
// column, which lets b fall back toward z, 7.86 px inside the right
// column. Decided again on the rerun's result, m->b is forced too, in round
// 1, and b lands under the column: the router draws both exits as Ls.
// (m was a circle, whose exits now merge instead, TestPlace_ACirclesExitsAreNotForced;
// as a diamond it keeps the case, which placement sees only through m's
// width.)
func TestPlace_ForcingIsDecidedAgainAfterTheRerun(t *testing.T) {
	widths := map[string]float64{"m": 160, "a": 60, "b": 160, "z": 80}
	specs := []string{"m:diamond->b:diamond", "m->a:circle", "z:diamond->b"}
	_, x := placed1(t, screen, widths, specs...)
	assert.InDelta(t, x("m")-100, x("a"), 1e-9, "a under m's left column")
	assert.InDelta(t, x("m")+100, x("b"), 1e-9, "b under m's right column")
	_, got := decisions(t, screen, widths, specs...)
	rounds := map[string]any{}
	for _, e := range got {
		if e["decision"] == "side_exit_forced" {
			rounds[e["edge"].(string)] = e["round"]
		}
	}
	assert.Equal(t, map[string]any{"m->a#0": float64(0), "m->b#0": float64(1)}, rounds)
}

// TestPlace_TheSecondPassForcesTheReplayedSideExits pins S7's prediction
// in the second pass: the router replays m->a on m's left vertex (S8,
// Second pass), although a sits on m's axis, where the exit rule would
// send it from the bottom. S7 predicts the replay, not the exit rule, and
// forces m->a, so a lands under m's left column and the replayed exit
// routes as an L instead of leaving the side vertex and looping back to
// the axis.
func TestPlace_TheSecondPassForcesTheReplayedSideExits(t *testing.T) {
	widths := map[string]float64{"m": 200, "a": 40, "b": 80}
	specs := []string{"m:diamond->a", "m->b", "z->b"}
	_, first := placed1(t, screen, widths, specs...)
	require.InDelta(t, first("m"), first("a"), 1e-9, "first pass: a on m's axis, the exit rule's bottom")
	second := screen
	second.Side = [][2]ports.Vertex{{ports.Left, ports.Bottom}, {ports.Right, ports.Bottom}, {}}
	_, x := placed1(t, second, widths, specs...)
	assert.InDelta(t, x("m")-120, x("a"), 1e-9, "a under m's left column")
}

// placed1 is placed without the raw positions.
func placed1(t *testing.T, o Options, widths map[string]float64, specs ...string) (*lgraph.Graph, func(id string) float64) {
	t.Helper()
	g, _, x := placed(t, o, widths, specs...)
	return g, x
}

// forcedEdges lists the edges of the side_exit_forced records in got.
func forcedEdges(got []map[string]any) []string {
	var out []string
	for _, e := range got {
		if e["decision"] == "side_exit_forced" {
			out = append(out, e["edge"].(string))
		}
	}
	return out
}

func TestPlace_ASideAttachedEndLeavesItsFace(t *testing.T) {
	// the second pass (S8): the router attached creds->login's end on login
	// (its chain's head) at a side face, so login's out-face holds the
	// forward port alone, in its middle, and creds aligns under login
	second := screen
	second.Side = [][2]ports.Vertex{{}, {ports.Right, ports.Bottom}}
	_, _, x := placed(t, second, map[string]float64{"login": 90, "creds": 248},
		"login->creds:diamond", "creds->login")
	assert.InDelta(t, x("login"), x("creds"), 1e-6)
}

// TestPlace_ACirclesExitsAreNotForced pins S7's side-exit forcing on a
// circle: its two or more forward exits all leave its bottom vertex, a
// fan-out merged onto one trunk (S8), so none takes a side column to line
// up under and none is forced. The children that are forced out under a
// diamond's side columns (TestPlace_TwoExitsAreForcedUnderTheSideColumns)
// stay where act 2 puts them under a circle: at their minimum separation,
// centered under it. m is 160 wide, so its side columns (100 out) lie
// beyond every child, which a diamond's forcing would pull out under them.
func TestPlace_ACirclesExitsAreNotForced(t *testing.T) {
	for _, specs := range [][]string{
		{"m:circle->yes", "m->no"},
		{"m:circle->yes", "m->maybe", "m->no"},
	} {
		widths := map[string]float64{"m": 160, "yes": 40, "maybe": 40, "no": 40}
		_, _, x := placed(t, screen, widths, specs...)
		assert.InDelta(t, (x("yes")+x("no"))/2, x("m"), 1e-6, "%v: m centered over its children", specs)
		assert.InDelta(t, 80*float64(len(specs)-1), x("no")-x("yes"), 1e-6, "%v: the children at their minimum separation", specs)
		_, got := decisions(t, screen, widths, specs...)
		for _, d := range got {
			assert.NotContains(t, []string{"side_exit_forced", "side_exit_dropped"}, d["decision"], "%v: %v", specs, d)
		}
	}
}

func TestPlace_TwoExitsAreForcedUnderTheSideColumns(t *testing.T) {
	// 40-wide children sit 40 either side of the 80-wide diamond, inside
	// its side columns at 40 + 20: forcing moves them out, so both wires
	// route as Ls
	_, _, x := placed(t, screen, map[string]float64{"yes": 40, "no": 40}, "m:diamond->yes", "m->no")
	assert.InDelta(t, x("m")-60, x("yes"), 1e-9)
	assert.InDelta(t, x("m")+60, x("no"), 1e-9)
}

func TestPlace_ThreeExitsTakeLeftBottomRight(t *testing.T) {
	_, _, x := placed(t, screen, nil, "m:diamond->a", "m->b", "m->c")
	assert.InDelta(t, x("m"), x("b"), 1e-9, "the middle exit drops straight")
	assert.InDelta(t, x("m")-x("a"), x("c")-x("m"), 1e-9)
	assert.GreaterOrEqual(t, x("m")-x("a"), 60.0, "the side children reach the columns")
}

func TestPlace_ALoneExitKeepsTheBottom(t *testing.T) {
	_, _, x := placed(t, screen, map[string]float64{"a": 40}, "m:diamond->a")
	assert.InDelta(t, x("m"), x("a"), 1e-9)
}

// TestPlace_ADiamondsPrimaryExitRunsStraight pins S7's primary clause: of
// m's two exits, only the one into main leads on (main has a forward
// out-edge, end has none), so it is the main line: it is forced straight
// under m's bottom vertex, and the other exit takes the side its next
// vertex lies on, at least the minimum separation (80 + 40) out, beyond
// m's side column (40 + 20). Symmetric, as before the clause, both
// children sat 60 either side of m. The forcing is logged as a primary's.
func TestPlace_ADiamondsPrimaryExitRunsStraight(t *testing.T) {
	for _, specs := range [][]string{
		{"m:diamond->main", "m->end", "main->next"},
		{"m:diamond->end", "m->main", "main->next"},
	} {
		_, _, x := placed(t, screen, nil, specs...)
		assert.InDelta(t, x("m"), x("main"), 1e-9, "%v: the main line under m", specs)
		assert.InDelta(t, x("m"), x("next"), 1e-9, "%v: and on through main", specs)
		assert.InDelta(t, 120, math.Abs(x("end")-x("m")), 1e-9, "%v: the other branch beside it", specs)
		_, got := decisions(t, screen, nil, specs...)
		var primary []string
		for _, d := range got {
			switch d["decision"] {
			case "side_exit_forced":
				if d["primary"] == true {
					primary = append(primary, d["edge"].(string))
					assert.Equal(t, float64(0), d["offset"], "%v", specs)
					assert.Equal(t, "S7", d["spec_ref"], "%v", specs)
				}
			case "side_exit_dropped":
				assert.Failf(t, "a forcing dropped", "%v: %v", specs, d)
			}
		}
		assert.Equal(t, []string{"m->main#0"}, primary, "%v", specs)
	}
}

// TestPlace_ASecondaryIsJudgedAfterItsPrimary pins the order of S7's
// primary clause. m is 140 wide, so its side columns lie 90 out; main and
// end are 130 wide, 170 apart at least. On the baseline m sits between
// them, 85 from each, so the exit into end has not reached its column.
// Forced together, main under m and end's port under the right column,
// the two would stand 90 apart, which the separation forbids, and
// alignment would drop both. Judged after the primary's rerun, end stands
// 170 out, beyond the column, where the router moves the column out over
// it: only the primary is forced, and nothing is dropped.
func TestPlace_ASecondaryIsJudgedAfterItsPrimary(t *testing.T) {
	widths := map[string]float64{"m": 140, "main": 130, "end": 130}
	specs := []string{"m:diamond->main", "m->end", "main->next"}
	_, x := placed1(t, screen, widths, specs...)
	assert.InDelta(t, x("m"), x("main"), 1e-9, "the main line under m")
	assert.InDelta(t, 170, math.Abs(x("end")-x("m")), 1e-9, "end at its minimum separation from main")
	_, got := decisions(t, screen, widths, specs...)
	assert.Equal(t, []string{"m->main#0"}, forcedEdges(got))
	for _, d := range got {
		assert.NotEqual(t, "side_exit_dropped", d["decision"], "%v", d)
	}
}

// TestPlace_NoPrimaryWithoutOneMainLine pins the other half of S7's primary
// clause: when both of m's exits lead on, or neither, no branch is the main
// line, and the exit rule keeps them symmetric about m's axis, each under
// its side column (40 + 20 out) as before the clause; so does a
// diamond with three exits, and one with a counter-flow end, which is not
// forced at all.
func TestPlace_NoPrimaryWithoutOneMainLine(t *testing.T) {
	widths := map[string]float64{"yes": 40, "no": 40}
	for _, specs := range [][]string{
		{"m:diamond->yes", "m->no"},
		{"m:diamond->yes", "m->no", "yes->a", "no->b"},
	} {
		_, _, x := placed(t, screen, widths, specs...)
		assert.InDelta(t, 60, x("m")-min(x("yes"), x("no")), 1e-9, "%v", specs)
		assert.InDelta(t, 60, max(x("yes"), x("no"))-x("m"), 1e-9, "%v", specs)
		_, got := decisions(t, screen, widths, specs...)
		for _, d := range got {
			assert.NotEqual(t, true, d["primary"], "%v: %v", specs, d)
		}
	}
	for _, specs := range [][]string{
		{"m:diamond->yes", "m->no", "m->maybe", "yes->a"},
		{"m:diamond->yes", "m->no", "yes->a", "no->m"},
	} {
		_, got := decisions(t, screen, widths, specs...)
		for _, d := range got {
			assert.NotEqual(t, true, d["primary"], "%v: %v", specs, d)
		}
	}
}

// TestPlace_TheSecondPassForcesAReplayedPrimary pins S7's primary clause
// in the second pass: the router's replay records the primary on m's
// bottom vertex and the other exit on its right, so S7 forces the primary
// again and main stays under m, where act 1 alone centers m between its
// two children. A replay that records the primary on a side (its forcing
// dropped in the first pass) forces no primary.
func TestPlace_TheSecondPassForcesAReplayedPrimary(t *testing.T) {
	specs := []string{"m:diamond->main", "m->end", "main->next"}
	second := screen
	second.Side = [][2]ports.Vertex{{ports.Bottom, ports.Bottom}, {ports.Right, ports.Bottom}, {}}
	_, x := placed1(t, second, nil, specs...)
	assert.InDelta(t, x("m"), x("main"), 1e-9, "main under m")
	_, got := decisions(t, second, nil, specs...)
	assert.Contains(t, forcedEdges(got), "m->main#0")

	second.Side = [][2]ports.Vertex{{ports.Left, ports.Bottom}, {ports.Bottom, ports.Bottom}, {}}
	_, got = decisions(t, second, nil, specs...)
	for _, d := range got {
		assert.NotEqual(t, true, d["primary"], "%v", d)
	}
}

// TestPlace_ALoneBottomExitRunsStraight pins S7's bottom clause: neither
// of m's exits leads on, so m has no primary, but the replay records a's
// exit on m's bottom vertex and b's on its left, where S6 orders b; c,
// a's other parent, pulls a off m's axis, beyond the span. The bottom exit
// is forced straight under m all the same, its port on a (the left of a's
// two in-ports, a sixth of a's width left of its center) under m's bottom
// vertex, logged as a bottom forcing, and
// b, judged after it, stands beyond m's left column. The positive control: without
// the replay m's exits head to opposite sides beyond the band, left and
// right, and no exit takes the bottom.
func TestPlace_ALoneBottomExitRunsStraight(t *testing.T) {
	specs := []string{"m:diamond->a", "m->b", "c->a"}
	second := screen
	second.Side = [][2]ports.Vertex{{ports.Bottom, ports.Bottom}, {ports.Left, ports.Bottom}, {}}
	_, x := placed1(t, second, nil, specs...)
	assert.InDelta(t, x("m"), x("a")-80.0/6, 1e-9, "m->a's port on a, the left of two, under m's bottom vertex")
	assert.GreaterOrEqual(t, x("m")-x("b"), 60.0, "b beyond m's left column")
	_, got := decisions(t, second, nil, specs...)
	var bottom []string
	for _, d := range got {
		switch d["decision"] {
		case "side_exit_forced":
			if d["bottom"] == true {
				bottom = append(bottom, d["edge"].(string))
				assert.Equal(t, float64(0), d["offset"])
				assert.Equal(t, false, d["primary"])
			}
		case "side_exit_dropped":
			assert.Failf(t, "a forcing dropped", "%v", d)
		}
	}
	assert.Equal(t, []string{"m->a#0"}, bottom)

	_, got = decisions(t, screen, nil, specs...)
	for _, d := range got {
		assert.NotEqual(t, true, d["bottom"], "%v", d)
	}
}

// TestPlace_ALongBottomExitTakesNoBottomClause: the replay records m->a on
// m's bottom vertex and m->b on its left, but a lies two layers down (t
// feeds it), so m->a's first hop ends at a dummy: the bottom clause takes
// no long edge, and nothing forces it. The positive control:
// TestPlace_ALoneBottomExitRunsStraight, a one-hop bottom exit.
func TestPlace_ALongBottomExitTakesNoBottomClause(t *testing.T) {
	specs := []string{"m:diamond->a", "m->b", "s->t", "t->a"}
	second := screen
	second.Side = [][2]ports.Vertex{{ports.Bottom, ports.Bottom}, {ports.Left, ports.Bottom}, {}, {}}
	_, got := decisions(t, second, nil, specs...)
	for _, d := range got {
		assert.NotEqual(t, true, d["bottom"], "%v", d)
	}
}

// TestPlace_ABottomClauseGivesWayToItsSides: m's three exits take left,
// bottom and right (TestPlace_AReachedPortIsNotForcedBack's graph), so the
// bottom clause forces m->b#0's port, b's left one, under m. Judged beside
// it, m->b#1's port, b's right one 40 px farther, falls short of m's right
// column (30 + 20 out) and is forced onto it; b cannot take both, the
// batch drops whole, and the bottom clause gives way, logged as dropped,
// with the side forcing judged beside it. Judged again without it,
// m->b#1's port lies beyond the column, where the router moves the column
// out over it: nothing more is forced on m's right, and the layout is the
// one without the clause. (TestLayout_AReplayedSideExitRoutesAsAnL lays a
// side rail out the same way, in full.) The positive control:
// TestPlace_ALoneBottomExitRunsStraight, a bottom clause that holds.
func TestPlace_ABottomClauseGivesWayToItsSides(t *testing.T) {
	widths := map[string]float64{"m": 60, "b": 120, "a": 40}
	specs := []string{"m:diamond->a", "m->b", "m->b"}
	_, x := placed1(t, screen, widths, specs...)
	assert.Greater(t, x("b")+20-x("m"), 50+1.0, "m->b#1's port beyond m's right column")
	_, got := decisions(t, screen, widths, specs...)
	var dropped []string
	for _, d := range got {
		if d["decision"] == "side_exit_dropped" {
			dropped = append(dropped, fmt.Sprintf("%s bottom=%v", d["edge"], d["bottom"]))
		}
	}
	assert.Equal(t, []string{"m->b#0 bottom=true"}, dropped)
	assert.NotContains(t, forcedEdges(got), "m->b#1")
}

// TestPlace_ADeniedDiamondTakesNoBottomClause: a diamond denied its
// primary takes its exits by the exit rule, and a bottom clause on the
// exit that was its primary would force it back; it takes none. On a base
// where main heads 10 right of m and end 200 right, the exit rule gives
// main the bottom vertex and end the right one, which end has reached:
// held, m forces its primary; denied, nothing; and m's twin without a
// main line (next taken away, so neither exit leads on) forces main by
// the bottom clause, the positive control.
func TestPlace_ADeniedDiamondTakesNoBottomClause(t *testing.T) {
	round := func(t *testing.T, denied bool, specs ...string) []forcedExit {
		lv := lgraphtest.Level(t, specs...)
		ctx := context.Background()
		reversed := cycle.Break(ctx, lv, nil)
		g, err := lgraph.Build(lv, rank.Assign(ctx, lv, reversed, nil), reversed, 8)
		require.NoError(t, err)
		g.Layers = order.Minimize(ctx, g, false)
		p := newPlacer(g, screen)
		base := make([]float64, len(g.Vertices))
		base[vertexOf(t, g, "main")] = 10
		base[vertexOf(t, g, "end")] = 200
		if denied {
			p.clause[vertexOf(t, g, "m")] = primaryDenied
		}
		return p.sideExits(base, nil, nil, 0).forced
	}
	held := round(t, false, "m:diamond->main", "m->end", "main->next")
	require.Len(t, held, 1)
	assert.True(t, held[0].primary, "held: m's primary forced")
	assert.Empty(t, round(t, true, "m:diamond->main", "m->end", "main->next"), "denied: nothing forced")
	twin := round(t, false, "m:diamond->main", "m->end")
	require.Len(t, twin, 1)
	assert.True(t, twin[0].bottom, "no main line: main forced by the bottom clause")
	assert.False(t, twin[0].primary)
}

// cascade is a cascade of two decisions: a's primary leads into b, b's
// into ok, and each has another branch, ea and eb, that leads nowhere.
var cascade = []string{"s->a:diamond", "a->b:diamond", "a->ea", "b->ok", "b->eb", "ok->done"}

// primaryChanges lists got's secondary_room, primary_denied and
// primary_restored records as "decision node edge round", a room's with
// its blocking node after its edge.
func primaryChanges(t *testing.T, got []map[string]any) []string {
	t.Helper()
	var out []string
	for _, d := range got {
		switch d["decision"] {
		case "secondary_room":
			assert.Equal(t, "S7", d["spec_ref"], "%v", d)
			out = append(out, fmt.Sprintf("%s %s %s %s %v", d["decision"], d["node"], d["edge"], d["blocking"], d["round"]))
		case "primary_denied", "primary_restored":
			assert.Equal(t, "S7", d["spec_ref"], "%v", d)
			out = append(out, fmt.Sprintf("%s %s %s %v", d["decision"], d["node"], d["edge"], d["round"]))
		}
	}
	return out
}

// TestPlace_ABlockedSecondaryGetsRoom pins S7's room on a cascade of two
// decisions, every node 80 wide. a's primary puts b under a, and a's
// other branch, ea, lands in b's row, right of b. Once b's primary puts
// ok under b too, eb keeps the node gap from ok, beyond b's right column
// (40 + 20 out), where ea stands within the node clearance of the
// column's way out. So eb gets room in the round after b's forcing: ea
// keeps the node clearance (12) beyond eb's port, eb's center (its one
// in-port), and b keeps its primary, ok under it. Denied it, as before
// the room, b fanned out symmetrically and the main line bent.
func TestPlace_ABlockedSecondaryGetsRoom(t *testing.T) {
	_, x := placed1(t, screen, nil, cascade...)
	assert.InDelta(t, x("a"), x("b"), 1e-9, "a's main line")
	assert.InDelta(t, x("b"), x("ok"), 1e-9, "b's main line")
	assert.InDelta(t, 40+12, x("ea")-x("eb"), 1e-9, "ea the node clearance beyond eb's port")
	_, got := decisions(t, screen, nil, cascade...)
	assert.Equal(t, []string{"secondary_room b b->eb#0 ea 1"}, primaryChanges(t, got))
	assert.ElementsMatch(t, []string{"a->b#0", "b->ok#0"}, forcedEdges(got), "both primaries forced and realized")
}

// TestPlace_ARoomThatDoesNotHoldFallsBackToTheDenial pins the denial as
// the room's fallback, and its undoing: m's primary puts main under m,
// and end, which z in m's row also feeds, lands beyond m's right column,
// where z blocks the column's way out, so end gets room. But z, 40 wide,
// runs straight into end, one block with it, which leaves z within the
// node clearance of m->end's port: the room does not hold, and the next
// round denies m its primary. Denied, m's exits still leave end's port
// beyond the right column with z in the way: the denial bought no L, so
// m gets its primary back, for good, and main runs under m again.
func TestPlace_ARoomThatDoesNotHoldFallsBackToTheDenial(t *testing.T) {
	widths := map[string]float64{"z": 40}
	specs := []string{"s->m:diamond", "m->main", "m->end", "z->end", "main->next"}
	_, x := placed1(t, screen, widths, specs...)
	assert.InDelta(t, x("m"), x("main"), 1e-9, "the main line under m")
	_, got := decisions(t, screen, widths, specs...)
	assert.Equal(t, []string{"secondary_room m m->end#0 z 1", "primary_denied m m->end#0 2", "primary_restored m m->end#0 1"}, primaryChanges(t, got))
}

// TestPlace_ANodeAtTheClearanceKeepsTheRowClear pins S7's row test on
// float noise, on both sides: m's side column runs out over 200 (or -200),
// and o, in m's row, keeps the node clearance (12) beyond it. Left exactly
// there, or short of it by float noise (1e-12, 1e-7), o keeps the row
// clear, as a room solved to the clearance leaves it; short by more than
// 1e-6 (1e-5, 1e-3), o blocks the column.
func TestPlace_ANodeAtTheClearanceKeepsTheRowClear(t *testing.T) {
	lv := lgraphtest.Level(t, "m:diamond", "o")
	g, err := lgraph.Build(lv, []int{0, 0}, make([]bool, len(lv.Edges)), 8)
	require.NoError(t, err)
	p := newPlacer(g, screen)
	half := g.Vertices[1].W / 2
	for _, sign := range []float64{1, -1} {
		for _, tc := range []struct {
			short  float64
			blocks bool
		}{{0, false}, {1e-12, false}, {1e-7, false}, {1e-5, true}, {1e-3, true}} {
			base := []float64{0, sign * (200 + 12 + half - tc.short)}
			assert.Equal(t, tc.blocks, len(p.blocking(base, 0, sign, sign*200)) > 0, "side %v, o %g short of the clearance", sign, tc.short)
		}
	}
}

// TestPlace_ARoomKeepsTheClearanceFromItsBlockersSide pins a room's
// separation (S7): the blocking node keeps the node clearance beyond the
// port, counted from its box plus its own farthest side column or loop leg
// on its side facing the port, on either side; the port is the next
// vertex's center plus its in-port offset, and a dummy's offset is 0.
func TestPlace_ARoomKeepsTheClearanceFromItsBlockersSide(t *testing.T) {
	lv := lgraphtest.Level(t, "n", "o")
	g, err := lgraph.Build(lv, []int{1, 0}, make([]bool, len(lv.Edges)), 8)
	require.NoError(t, err)
	s := spacing{clearance: 12, out: [][2]float64{{0, 0}, {5, 20}}}
	for _, tc := range []struct {
		name        string
		sign, port  float64
		left, right int
		gap         float64
	}{
		{"right of the port", 1, 0, 0, 1, 40 + 5 + 12},
		{"right of an off-center port", 1, 10, 0, 1, 10 + 40 + 5 + 12},
		{"left of the port", -1, 0, 1, 0, 40 + 20 + 12},
		{"left of an off-center port", -1, -10, 1, 0, 10 + 40 + 20 + 12},
	} {
		left, right, gap := s.roomSep(g, room{next: 0, other: 1, sign: tc.sign, port: tc.port})
		assert.Equal(t, [2]int{tc.left, tc.right}, [2]int{left, right}, tc.name)
		assert.InDelta(t, tc.gap, gap, 1e-9, tc.name)
	}
}

// TestAlign_RoomsThatContradictEachOtherKeepTheFirst pins act 2's rooms
// against each other: one room keeps o right of n, another o left of n,
// which no placement meets. The first is kept and the second gives way,
// so the compaction meets the first and every layer's order, a and o
// side by side above n.
func TestAlign_RoomsThatContradictEachOtherKeepTheFirst(t *testing.T) {
	lv := lgraphtest.Level(t, "a", "o", "n")
	g, err := lgraph.Build(lv, []int{0, 0, 1}, make([]bool, len(lv.Edges)), 8)
	require.NoError(t, err)
	p := newPlacer(g, screen)
	in := alignInput{g: g, space: p.space, span: screen.Span, ref: []float64{0, 100, 50}, links: p.links, weight: p.weight,
		rooms: []room{{next: 2, other: 1, sign: 1}, {next: 2, other: 1, sign: -1}}}
	x := align(in)
	assert.GreaterOrEqual(t, x[1]-x[2], 40+12-1e-9, "the first room: o the node clearance right of n")
	assert.GreaterOrEqual(t, x[1]-x[0], 80+screen.Gap-1e-9, "a left of o, the node gap apart")
}

// restarts returns the restarts got's placed record counts.
func restarts(t *testing.T, got []map[string]any) float64 {
	t.Helper()
	for _, d := range got {
		if d["decision"] == "placed" {
			return d["restarts"].(float64)
		}
	}
	require.Fail(t, "no placed record")
	return 0
}

// TestPlace_ARoundChangesEveryBlockedDiamondAtOnce pins S7's batched
// changes on two copies of each fixture above, side by side. The round
// that finds b's and d's secondaries blocked gives both their rooms, with
// no restart, and each cascade lays out as it does alone; the round that
// finds m's and m2's rooms did not hold denies both in one restart, and
// the settled round that finds those denials did not clear their rows
// gives both primaries back in another. So the restarts do not grow with
// the number of diamonds blocked side by side.
func TestPlace_ARoundChangesEveryBlockedDiamondAtOnce(t *testing.T) {
	two := append(slices.Clone(cascade), "s->c:diamond", "c->d:diamond", "c->ec", "d->ok2", "d->ed", "ok2->done2")
	_, x := placed1(t, screen, nil, two...)
	for _, v := range [][4]string{{"b", "ok", "eb", "ea"}, {"d", "ok2", "ed", "ec"}} {
		assert.InDelta(t, x(v[0]), x(v[1]), 1e-9, "%s under %s", v[1], v[0])
		assert.InDelta(t, 40+12, math.Abs(x(v[3])-x(v[2])), 1e-9, "%s the node clearance beyond %s's port", v[3], v[2])
	}
	_, got := decisions(t, screen, nil, two...)
	assert.Equal(t, []string{"secondary_room b b->eb#0 ea 1", "secondary_room d d->ed#0 ec 1"}, primaryChanges(t, got))
	assert.Equal(t, float64(0), restarts(t, got), "rooms take no restart")

	widths := map[string]float64{"z": 40, "z2": 40}
	undone := []string{"s->m:diamond", "m->main", "m->end", "z->end", "main->next",
		"s->m2:diamond", "m2->main2", "m2->end2", "z2->end2", "main2->next2"}
	_, x = placed1(t, screen, widths, undone...)
	assert.InDelta(t, x("m"), x("main"), 1e-9, "the main line under m")
	assert.InDelta(t, x("m2"), x("main2"), 1e-9, "the main line under m2")
	_, got = decisions(t, screen, widths, undone...)
	assert.Equal(t, []string{
		"secondary_room m m->end#0 z 1", "secondary_room m2 m2->end2#0 z2 1",
		"primary_denied m m->end#0 2", "primary_denied m2 m2->end2#0 2",
		"primary_restored m m->end#0 1", "primary_restored m2 m2->end2#0 1",
	}, primaryChanges(t, got))
	assert.Equal(t, float64(2), restarts(t, got), "both denials in one restart, both give-backs in another")
}

// TestPlace_TheSecondPassGivesRoomAndDeniesNoPrimary pins S7's second
// pass on a blocked secondary: replayed with b's primary on its bottom
// vertex and eb on its right, as the router draws them after the first
// pass, the cascade's b keeps its primary and eb gets its room again, ea
// the node clearance beyond eb's port. The replay fixes the vertices, so
// a denial there could only unstraighten the main line: none happens.
func TestPlace_TheSecondPassGivesRoomAndDeniesNoPrimary(t *testing.T) {
	second := screen
	second.Side = [][2]ports.Vertex{{}, {}, {ports.Right, ports.Bottom}, {}, {ports.Right, ports.Bottom}, {}}
	_, x := placed1(t, second, nil, cascade...)
	assert.InDelta(t, x("b"), x("ok"), 1e-9, "ok under b")
	assert.InDelta(t, 40+12, x("ea")-x("eb"), 1e-9, "ea the node clearance beyond eb's port")
	_, got := decisions(t, second, nil, cascade...)
	assert.Equal(t, []string{"secondary_room b b->eb#0 ea 1"}, primaryChanges(t, got))
}

// TestPlace_TheSecondPassDeniesNoPrimaryWhenARoomFails pins the second
// pass's hold on the denial where the room does not hold: replayed with
// m's primary on its bottom vertex and m->end on its right, as the router
// draws them once the first pass gave m its primary back, the narrow z
// still runs straight into end, within the clearance of m->end's port.
// end gets its room again, which does not hold, and m keeps its primary:
// the replay fixes the vertices, so no denial follows, and m->end jogs.
func TestPlace_TheSecondPassDeniesNoPrimaryWhenARoomFails(t *testing.T) {
	widths := map[string]float64{"z": 40}
	specs := []string{"s->m:diamond", "m->main", "m->end", "z->end", "main->next"}
	second := screen
	second.Side = [][2]ports.Vertex{{}, {ports.Bottom, ports.Bottom}, {ports.Right, ports.Bottom}, {}, {}}
	_, x := placed1(t, second, widths, specs...)
	assert.InDelta(t, x("m"), x("main"), 1e-9, "the main line under m")
	_, got := decisions(t, second, widths, specs...)
	assert.Equal(t, []string{"secondary_room m m->end#0 z 1"}, primaryChanges(t, got), "a room, and no denial")
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

func TestPlace_ACounterFlowEndCancelsTheForcing(t *testing.T) {
	// TestPlace_TwoExitsAreForcedUnderTheSideColumns plus a back edge from
	// no: m now has a counter-flow end, so its exits are not forced (the
	// router settles them from the realized coordinates) and both children
	// stay inside the side columns at 40 + 20.
	_, got := decisions(t, screen, map[string]float64{"yes": 40, "no": 40}, "m:diamond->yes", "m->no", "no->m")
	_, _, x := placed(t, screen, map[string]float64{"yes": 40, "no": 40}, "m:diamond->yes", "m->no", "no->m")
	assert.Less(t, x("m")-x("yes"), 60-1e-6)
	assert.Less(t, x("no")-x("m"), 60-1e-6)
	for _, e := range got {
		assert.NotContains(t, []string{"side_exit_forced", "side_exit_dropped"}, e["decision"])
		if e["decision"] == "placed" {
			assert.Equal(t, float64(0), e["forced"])
			assert.Equal(t, float64(0), e["dropped"])
		}
	}
}

// TestPlace_TheSecondPassForcesADiamondWithABackEdge pins the forcing of
// a diamond with a counter-flow end, which only the second pass gets: the
// replay records m's back edge from b on m's right vertex, a's exit on its
// left and b's on its bottom. a leads on, but m takes no primary clause:
// b's exit is forced straight under m by the bottom clause, and a, judged
// after it, stands beyond m's left column at its separation from b, where
// the router moves the column out over it. With the replay recording b's
// exit on the right vertex too, beside the back edge, that side is the
// back edge's: nothing forces b's exit there. The positive control:
// TestPlace_ACounterFlowEndCancelsTheForcing, the first pass.
func TestPlace_TheSecondPassForcesADiamondWithABackEdge(t *testing.T) {
	widths := map[string]float64{"b": 200}
	specs := []string{"s->m:diamond", "m->a", "m->b", "b->m", "a->n"}
	second := screen
	second.Side = [][2]ports.Vertex{{}, {ports.Left, ports.Bottom}, {}, {ports.Right, ports.Right}, {}}
	_, x := placed1(t, second, widths, specs...)
	assert.LessOrEqual(t, x("a"), x("m")-60, "a beyond m's left column")
	_, got := decisions(t, second, widths, specs...)
	var bottom []string
	for _, d := range got {
		switch d["decision"] {
		case "side_exit_forced":
			assert.NotEqual(t, true, d["primary"], "%v", d)
			if d["bottom"] == true {
				bottom = append(bottom, d["edge"].(string))
			}
		case "side_exit_dropped":
			assert.Failf(t, "a forcing dropped", "%v", d)
		}
	}
	assert.Equal(t, []string{"m->b#0"}, bottom, "b's exit under m's bottom vertex")

	second.Side = [][2]ports.Vertex{{}, {ports.Left, ports.Bottom}, {ports.Right, ports.Bottom}, {ports.Right, ports.Right}, {}}
	_, got = decisions(t, second, widths, specs...)
	for _, d := range got {
		if d["decision"] == "side_exit_forced" || d["decision"] == "side_exit_dropped" {
			assert.NotEqual(t, "m->b#0", d["edge"], "the back edge's side forces nothing: %v", d)
		}
	}
}

func TestPlace_TextProfileHasNoPinnedShapes(t *testing.T) {
	text := Options{Gap: 5, Span: 1, Reach: 3, Clearance: 2, Dir: model.Down, Text: true}
	_, _, x := placed(t, text, map[string]float64{"m": 10, "yes": 8, "no": 8}, "m:diamond->yes", "m->no")
	// in text the diamond is a box whose out-face spreads two ports at
	// thirds of its width; the children center under them if they fit
	// (8 + 5 = 13 apart), which a 10-wide face (ports 3.33 apart) cannot give;
	// on the character grid, within half a cell
	assert.InDelta(t, x("m"), (x("yes")+x("no"))/2, 0.5)
	assert.GreaterOrEqual(t, x("no")-x("yes"), 13-1e-9)
}

func TestPlace_ASelfLoopKeepsItsRoom(t *testing.T) {
	narrow := Options{Gap: 10, Span: 16, Reach: 20, Clearance: 12, Dir: model.Down}
	// a and b are both x's parents, so they squeeze to their minimum
	_, _, x := placed(t, narrow, nil, "a->a", "a->x", "b->x")
	assert.InDelta(t, 80+20+12, x("b")-x("a"), 1e-6, "Reach and the clearance right of a, more than the gap")

	_, _, x = placed(t, narrow, nil, "a", "b->b", "a->x", "b->x")
	assert.InDelta(t, 80+10, x("b")-x("a"), 1e-6, "b's loop runs on its right: the gap on its left stays")
}

// TestPlace_NestedSelfLoopsKeepTheirRoom pins S7's separation: right of a
// node with self-loops the gap is at least its outermost loop leg's
// distance out plus the clearance. n loops nest on a node with faces, the
// outermost Reach + (n - 1) InLaneGap out (S8); a diamond's loops share the
// column Reach out.
func TestPlace_NestedSelfLoopsKeepTheirRoom(t *testing.T) {
	narrow := Options{Gap: 10, Span: 16, Reach: 20, Clearance: 12, InLaneGap: 8, Dir: model.Down}
	// a and b are both x's parents, so they squeeze to their minimum
	_, _, x := placed(t, narrow, nil, "a->a", "a->a", "a->a", "a->x", "b->x")
	assert.InDelta(t, 80+20+2*8+12, x("b")-x("a"), 1e-6, "three nested loops right of a")

	_, _, x = placed(t, narrow, nil, "a:diamond->a", "a->a", "a->a", "a->x", "b->x")
	assert.InDelta(t, 80+20+12, x("b")-x("a"), 1e-6, "a diamond's loops run on one column")
}

// TestPlace_AVertexWithoutNeighborsRidesWithItsRow pins S7's act 1: a
// vertex without neighbors on either side keeps its minimum separation
// from the nearest vertex before it in its layer that has neighbors, or,
// first in its layer, from the nearest after it, instead of holding the
// place the packing from 0 gave it while that vertex moves away.
func TestPlace_AVertexWithoutNeighborsRidesWithItsRow(t *testing.T) {
	widths := map[string]float64{"t": 8, "a": 140, "w": 150, "iso": 190}
	built := func(layers []int, specs ...string) *lgraph.Graph {
		lv := lgraphtest.Level(t, specs...)
		for i := range lv.Nodes {
			lv.Nodes[i].W = widths[lv.Nodes[i].ID]
		}
		g, err := lgraph.Build(lv, layers, make([]bool, len(lv.Edges)), 8)
		require.NoError(t, err)
		return g
	}

	// a moves left under t, a narrow parent, as under a terminal in a
	// group's level (S9)
	g := built([]int{0, 1, 2, 1}, "t->a", "a->w", "iso")
	x := Place(context.Background(), g, screen)
	assert.InDelta(t, 70+40+95, x[3]-x[1], 1e-6, "right of a")

	// one downward projection of layer [iso a] toward t, far right of the
	// packing from 0
	g = built([]int{0, 1, 1}, "t->a", "iso")
	g.Layers[1] = []int{2, 1}
	p := newPlacer(g, screen)
	x = []float64{644, 300, 95} // t, a, iso
	p.project(x, 1, p.up)
	assert.InDelta(t, 644, x[1], 1e-6, "a under t")
	assert.InDelta(t, 95+40+70, x[1]-x[2], 1e-6, "left of a, first in its layer")
}

// TestPlace_AVertexWithOnlySelfLoopsRidesWithItsRow pins S7's projection
// for a node whose only edges are self-loops: a self-loop has no chain,
// so it gives the node no neighbor, and the node takes no part in the fit
// but rides with its row, as an edgeless one does
// (TestPlace_AVertexWithoutNeighborsRidesWithItsRow).
func TestPlace_AVertexWithOnlySelfLoopsRidesWithItsRow(t *testing.T) {
	widths := map[string]float64{"t": 8, "a": 140, "s": 190}
	lv := lgraphtest.Level(t, "t->a", "s->s", "s->s")
	for i := range lv.Nodes {
		lv.Nodes[i].W = widths[lv.Nodes[i].ID]
	}
	g, err := lgraph.Build(lv, []int{0, 1, 1}, make([]bool, len(lv.Edges)), 8)
	require.NoError(t, err)
	g.Layers[1] = []int{2, 1} // s, then a
	p := newPlacer(g, screen)
	require.True(t, p.alone(2), "s has no neighbor")

	// one downward projection of layer [s a] toward t, far right of the
	// packing from 0: s keeps left of a, at the minimum separation
	x := []float64{644, 300, 95} // t, a, s
	p.project(x, 1, p.up)
	assert.InDelta(t, 644, x[1], 1e-6, "a under t")
	assert.InDelta(t, 95+40+70, x[1]-x[2], 1e-6, "s left of a, first in its layer")
}

func TestPlace_TextBoxesSitOnTheCharacterGrid(t *testing.T) {
	text := Options{Gap: 5, Span: 1, Reach: 3, Clearance: 2, Dir: model.Right, Text: true}
	// a 3-row box (RIGHT: the cross extent is rows) and a long edge's dummy
	g, x, _ := placed(t, text, map[string]float64{"a": 3, "b": 3, "c": 3}, "a->b", "b->c", "a->c", "a->d")
	for v, vx := range g.Vertices {
		left := x[v] - vx.W/2
		assert.Equal(t, math.Floor(left), left, "%s starts on a cell boundary", vx.ID)
	}
}

func TestPlace_KeepsTheMinimumSeparation(t *testing.T) {
	rng := rand.New(rand.NewPCG(13, 17))
	shapes := []string{"", ":diamond", ":circle", ":hexagon", ":parallelogram", ":cylinder"}
	for trial := range 120 {
		n := 2 + rng.IntN(16)
		specs := make([]string, 0, 3*n)
		for i := range n {
			specs = append(specs, fmt.Sprintf("n%d%s", i, shapes[rng.IntN(len(shapes))]))
		}
		for range n + rng.IntN(2*n) {
			specs = append(specs, fmt.Sprintf("n%d->n%d", rng.IntN(n), rng.IntN(n)))
		}
		widths := map[string]float64{}
		for i := range n {
			widths[fmt.Sprintf("n%d", i)] = float64(40 + rng.IntN(160))
		}
		g, x, _ := placed(t, screen, widths, specs...)
		for l, layer := range g.Layers {
			for i := 1; i < len(layer); i++ {
				a, b := layer[i-1], layer[i]
				need := (g.Vertices[a].W+g.Vertices[b].W)/2 + screen.Gap
				assert.GreaterOrEqual(t, x[b]-x[a], need-1e-6, "trial %d layer %d: %s | %s", trial, l, g.Vertices[a].ID, g.Vertices[b].ID)
			}
		}
		for v, xv := range x {
			assert.False(t, math.IsNaN(xv) || math.IsInf(xv, 0), "trial %d vertex %d", trial, v)
		}
	}
}

// decisions runs Place with a recording layoutdbg context and returns the
// graph (ordered, so a caller can check layer preconditions) and the
// place-phase decision records as JSON objects, in emission order.
func decisions(t *testing.T, o Options, widths map[string]float64, specs ...string) (*lgraph.Graph, []map[string]any) {
	t.Helper()
	lv := lgraphtest.Level(t, specs...)
	for i := range lv.Nodes {
		if w, ok := widths[lv.Nodes[i].ID]; ok {
			lv.Nodes[i].W = w
		}
	}
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := layoutdbg.NewContext(context.Background(), logger)
	reversed := cycle.Break(ctx, lv, nil)
	g, err := lgraph.Build(lv, rank.Assign(ctx, lv, reversed, nil), reversed, 8)
	require.NoError(t, err)
	g.Layers = order.Minimize(ctx, g, false)
	Place(ctx, g, o)
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		if entry["phase"] == "place" {
			out = append(out, entry)
		}
	}
	return g, out
}

// vertexOf returns g's vertex index for real node id.
func vertexOf(t *testing.T, g *lgraph.Graph, id string) int {
	t.Helper()
	for i, n := range g.Level.Nodes {
		if n.ID == id {
			return i
		}
	}
	require.Failf(t, "no node", "%s", id)
	return -1
}

// TestPlace_ForcedExitsAreLoggedAsSideExitForced runs the same fixture as
// TestPlace_TwoExitsAreForcedUnderTheSideColumns through a recording
// layoutdbg context: both forcings land (the batch is feasible, a single
// pinned node's two side exits with nothing between them), so both are
// logged as side_exit_forced, none as side_exit_dropped, and placed's
// forced/dropped counts agree.
func TestPlace_ForcedExitsAreLoggedAsSideExitForced(t *testing.T) {
	_, got := decisions(t, screen, map[string]float64{"yes": 40, "no": 40}, "m:diamond->yes", "m->no")
	var forced, dropped, placed int
	forcedEdges := map[string]bool{}
	for _, e := range got {
		switch e["decision"] {
		case "side_exit_forced":
			forced++
			forcedEdges[e["edge"].(string)] = true
			assert.Equal(t, "S7", e["spec_ref"])
			assert.Equal(t, "diago", e["module"])
		case "side_exit_dropped":
			dropped++
		case "placed":
			placed++
			assert.Equal(t, float64(2), e["forced"])
			assert.Equal(t, float64(0), e["dropped"])
		}
	}
	assert.Equal(t, 2, forced)
	assert.Equal(t, 0, dropped)
	assert.Equal(t, 1, placed)
	assert.Equal(t, map[string]bool{"m->yes#0": true, "m->no#0": true}, forcedEdges)
}

// TestPlace_ACollidingForcedBatchIsDropped gives m's two forced children
// (a, b) a third vertex, z, ordered between them in their layer: z reaches
// that layer not through m but through its own edge to c, which rank
// tightening pulls up beside a and b since nothing else holds z back.
// Forcing a and b to m's columns joins them into m's block, 240 apart, with
// z's own block between them in layer 1. The room suffices (the three need
// 140), but within layer 1 alone the block order is cyclic (m's block, z's,
// m's again): align's feasible (blocks.go) rejects the batch as a
// block-order cycle, not as a room shortfall, and Place logs both
// predicted forcings as side_exit_dropped.
func TestPlace_ACollidingForcedBatchIsDropped(t *testing.T) {
	g, got := decisions(t, screen,
		map[string]float64{"m": 200, "a": 40, "b": 40, "z": 20},
		"m:diamond->a", "z->c", "m->b", "a->c", "b->c")

	// precondition: z lands between a and b in the ordered layer.
	idx := g.Index()
	av, zv, bv := vertexOf(t, g, "a"), vertexOf(t, g, "z"), vertexOf(t, g, "b")
	require.Less(t, idx[av], idx[zv], "z should order after a")
	require.Less(t, idx[zv], idx[bv], "z should order before b")

	var forced, dropped, placed int
	droppedEdges := map[string]bool{}
	for _, e := range got {
		switch e["decision"] {
		case "side_exit_forced":
			forced++
		case "side_exit_dropped":
			dropped++
			droppedEdges[e["edge"].(string)] = true
			assert.Equal(t, "S7", e["spec_ref"])
			assert.Equal(t, "diago", e["module"])
		case "placed":
			placed++
			assert.Equal(t, float64(0), e["forced"])
			assert.Equal(t, float64(2), e["dropped"])
		}
	}
	assert.Equal(t, 0, forced)
	assert.Equal(t, 2, dropped)
	assert.Equal(t, 1, placed)
	assert.Equal(t, map[string]bool{"m->a#0": true, "m->b#0": true}, droppedEdges)
}

// laid runs S3 to S7 over lv as it is.
func laid(t *testing.T, lv *lgraph.Level, o Options) (*lgraph.Graph, func(id string) float64) {
	t.Helper()
	ctx := context.Background()
	reversed := cycle.Break(ctx, lv, nil)
	g, err := lgraph.Build(lv, rank.Assign(ctx, lv, reversed, nil), reversed, 8)
	require.NoError(t, err)
	g.Layers = order.Minimize(ctx, g, false)
	x := Place(ctx, g, o)
	return g, func(id string) float64 {
		for i, n := range lv.Nodes {
			if n.ID == id {
				return x[i]
			}
		}
		require.Failf(t, "no node", "%s", id)
		return math.NaN()
	}
}

// TestPlace_TerminalsKeepTheTerminalGap pins S7 (S9): two terminals, wire
// columns only, keep the terminal gap between their centers, not the node
// gap, so both align on ports about 26.7 px apart.
func TestPlace_TerminalsKeepTheTerminalGap(t *testing.T) {
	lv := lgraphtest.Level(t, "ta", "tb", "n", "ta->n", "tb->n")
	for _, i := range []int{0, 1} {
		lv.Nodes[i].Terminal, lv.Nodes[i].W, lv.Nodes[i].H = true, 8, 0
	}
	o := screen
	o.TerminalGap = 16
	_, x := laid(t, lv, o)
	assert.InDelta(t, x("n")-40.0/3, x("ta"), 1e-6, "on n's first in-port")
	assert.InDelta(t, x("n")+40.0/3, x("tb"), 1e-6, "on n's second in-port")
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

// TestPlace_APinnedExitTowardAnAnchoredEndAddsTheAnchor pins S7 (S9): a
// pinned node's chain of length 2 into an end anchored on a group's face
// adds the anchor's Fixed offset to the exit's Toward, so forcing judges
// where the wire actually lands on the group's face, not the group's bare
// center. m's second exit (to b) keeps grp's exit from being exempt as a
// lone exit, so the forced column (60 off m, m's out-face plus Reach) is
// what pins grp: grp's center sits 150 further left, at the anchor.
func TestPlace_APinnedExitTowardAnAnchoredEndAddsTheAnchor(t *testing.T) {
	lv := lgraphtest.Level(t, "m:diamond->grp", "m->b")
	lv.Nodes[1].Group, lv.Nodes[1].W, lv.Nodes[1].H = true, 200, 100
	lv.Edges[0].Anchor[1] = lgraph.Anchor{On: true, At: 150}
	lv.Nodes[2].W = 40
	_, x := laid(t, lv, screen)
	assert.InDelta(t, x("m")-210, x("grp"), 1e-6)
}

// TestPlace_ASideColumnKeepsItsRoom pins S7's separation in the second
// pass (S8): a node whose end the router moved to a side face keeps its
// side column's room on that side, the column's reach plus the clearance,
// in the text profile whole cells: 3.5 + 2 rounds up to 6, past the gap 5.
func TestPlace_ASideColumnKeepsItsRoom(t *testing.T) {
	text := Options{Gap: 5, TerminalGap: 2, Span: 1, Reach: 3, Clearance: 2, Dir: model.Down, Text: true}
	widths := map[string]float64{"n": 8, "m": 8, "b": 8}
	specs := []string{"n->m", "m->n", "b"}
	_, _, x := placed(t, text, widths, specs...)
	assert.InDelta(t, 13.0, math.Abs(x("b")-x("n")), 1e-9, "first pass: the gap")
	for _, tc := range []struct {
		name string
		side ports.Vertex
	}{{"right", ports.Right}, {"left", ports.Left}} {
		second := text
		second.Side = [][2]ports.Vertex{{}, {tc.side, ports.Bottom}}
		_, _, x = placed(t, second, widths, specs...)
		if (tc.side == ports.Right) == (x("b") > x("n")) {
			assert.InDelta(t, 14.0, math.Abs(x("b")-x("n")), 1e-9, "%s: b beside the column keeps its room", tc.name)
		} else {
			assert.InDelta(t, 13.0, math.Abs(x("b")-x("n")), 1e-9, "%s: b on the other side keeps the gap", tc.name)
		}
	}
}

// TestPlace_FacingSideColumnsKeepRoomForBoth pins S7's separation in the
// second pass: when a's right side and b's left side both hold a side
// column or a loop leg, the gap between them is at least the sum of their
// distances out plus InLaneGap, room for both and the in-lane gap between
// them, in the text profile rounded up to whole cells. A lone column keeps
// only its own room: its distance out plus the clearance.
func TestPlace_FacingSideColumnsKeepRoomForBoth(t *testing.T) {
	scr := screen
	scr.InLaneGap = 8
	text := Options{Gap: 5, TerminalGap: 2, Span: 1, Reach: 3, Clearance: 2, InLaneGap: 2, Dir: model.Down, Text: true}
	narrow := map[string]float64{"a": 8, "b": 8, "m": 8, "n": 8, "x": 8, "y": 8}
	// a and b share layer 0, a left of b: m->a leaves a, n->b leaves b,
	// each reversed; the router attached their ends where Side says
	columns := []string{"a->m", "m->a", "b->n", "n->b"}
	for _, tc := range []struct {
		name   string
		o      Options
		widths map[string]float64
		specs  []string
		side   [][2]ports.Vertex
		want   float64
	}{
		{"screen, facing columns", scr, nil, columns,
			[][2]ports.Vertex{{}, {ports.Right, ports.Bottom}, {}, {ports.Left, ports.Bottom}}, 80 + 20 + 20 + 8},
		{"screen, a column on b's far side", scr, nil, columns,
			[][2]ports.Vertex{{}, {ports.Right, ports.Bottom}, {}, {ports.Right, ports.Bottom}}, 80 + 40},
		{"text, facing columns: 3.5 + 3.5 + 2 cells", text, narrow, columns,
			[][2]ports.Vertex{{}, {ports.Right, ports.Bottom}, {}, {ports.Left, ports.Bottom}}, 8 + 9},
		{"text, a lone column: 3.5 + 2 rounds up to 6", text, narrow, columns,
			[][2]ports.Vertex{{}, {ports.Right, ports.Bottom}, {}, {}}, 8 + 6},
		{"screen, a loop leg facing a column", scr, nil, []string{"a->a", "a->x", "b->y", "y->b"},
			[][2]ports.Vertex{{}, {}, {}, {ports.Left, ports.Bottom}}, 80 + 20 + 20 + 8},
		{"screen, two nested loops facing a column", scr, nil, []string{"a->a", "a->a", "a->x", "b->y", "y->b"},
			[][2]ports.Vertex{{}, {}, {}, {}, {ports.Left, ports.Bottom}}, 80 + 28 + 20 + 8},
		{"text, a loop leg facing a column: 3 + 3.5 + 2 rounds up to 9", text, narrow, []string{"a->a", "a->x", "b->y", "y->b"},
			[][2]ports.Vertex{{}, {}, {}, {ports.Left, ports.Bottom}}, 8 + 9},
	} {
		o := tc.o
		o.Side = tc.side
		g, _, x := placed(t, o, tc.widths, tc.specs...)
		require.Less(t, x("a"), x("b"), "%s: a left of b", tc.name)
		assert.InDelta(t, tc.want, newPlacer(g, o).sep(vertexOf(t, g, "a"), vertexOf(t, g, "b")), 1e-9, tc.name)
		assert.GreaterOrEqual(t, x("b")-x("a"), tc.want-1e-9, "%s: placed at least that far apart", tc.name)
	}
}

// TestPlace_APinnedLoopKeepsItsRoomOnItsSide pins S7's separation for a
// pinned node's self-loops: each keeps its room beside the side vertex it
// leaves, picked as S8 picks it from the side vertices the recorded
// attachments hold (none in the first pass): the right one, unless another
// end holds it and the left one is free.
func TestPlace_APinnedLoopKeepsItsRoomOnItsSide(t *testing.T) {
	// first pass: b's first loop takes its right vertex, the second its
	// left one, where it keeps Reach + Clearance from a
	narrow := Options{Gap: 10, Span: 16, Reach: 20, Clearance: 12, InLaneGap: 8, Dir: model.Down}
	_, _, x := placed(t, narrow, nil, "a", "b:diamond->b", "b->b", "a->x", "b->x")
	assert.InDelta(t, 80+20+12, x("b")-x("a"), 1e-6, "b's second loop on its left")

	// second pass: n->b holds b's right vertex, so b's loop leaves the left
	// one and faces a's right column: room for both, InLaneGap apart. Each
	// back edge has both its ends at a side, as the router attaches a
	// one-hop edge's two ends (S8). With n's end left on n's in-face, b->n
	// took the other in-port, off n's center, and n, which the back edge no
	// longer pulls (act 1), drifted after it, carrying b away from a.
	o := screen
	o.InLaneGap = 8
	o.Side = [][2]ports.Vertex{{}, {ports.Right, ports.Right}, {}, {ports.Right, ports.Right}, {}}
	_, _, x = placed(t, o, nil, "a->m", "m->a", "b:diamond->n", "n->b", "b->b")
	require.Less(t, x("a"), x("b"), "a left of b")
	assert.InDelta(t, 80+20+20+8, x("b")-x("a"), 1e-6, "a's column and b's loop, facing")
}

// beforePacking is act 2's result on g, the baseline without forcing: what
// Place returns before it packs the level's parts, on a level without
// pinned nodes.
func beforePacking(g *lgraph.Graph, o Options) []float64 {
	p := newPlacer(g, o)
	return align(alignInput{g: g, space: p.space, span: o.Span, ref: p.reference(), links: p.links, weight: p.weight})
}

// parts numbers g's hop-connected parts, per vertex.
func parts(g *lgraph.Graph) []int {
	part := make([]int, len(g.Vertices))
	for v := range part {
		part[v] = v
	}
	var find func(int) int
	find = func(v int) int {
		if part[v] != v {
			part[v] = find(part[v])
		}
		return part[v]
	}
	for _, h := range g.Hops() {
		part[find(h.Upper)] = find(h.Lower)
	}
	for v := range part {
		part[v] = find(v)
	}
	return part
}

// TestPlace_UnconnectedPartsPackTowardTheCenter pins S7's *Packing*: o's
// out-face holds two ports, p's and the long edge o->q's column. In a
// downward pass p and the column, squeezed, pool at their weighted mean
// (the column weighs 8, p 3), while in an upward pass o's target is their
// unweighted mean, so every upward pass moves o's part left, and c->d,
// which shares no edge with it, does not follow. After act 2 the two parts
// sit at their minimum separation, each moved rigidly, the level's
// weighted center where it was.
func TestPlace_UnconnectedPartsPackTowardTheCenter(t *testing.T) {
	g, x, _ := placed(t, screen, nil, "o->p", "p->q", "o->q", "c->d")
	before := beforePacking(g, screen)
	part := parts(g)
	p := newPlacer(g, screen)
	slack := func(x []float64) float64 {
		least := math.Inf(1)
		for _, layer := range g.Layers {
			for i := 1; i < len(layer); i++ {
				a, b := layer[i-1], layer[i]
				if part[a] != part[b] {
					least = min(least, x[b]-x[a]-p.sep(a, b))
				}
			}
		}
		return least
	}
	require.Greater(t, slack(before), 10.0, "act 2 leaves the parts apart")
	assert.InDelta(t, 0, slack(x), 1e-6, "packed at the minimum separation")
	shift := map[int]float64{}
	for v := range x {
		if s, ok := shift[part[v]]; ok {
			assert.InDelta(t, s, x[v]-before[v], 1e-6, "%s moves with its part", g.Vertices[v].ID)
			continue
		}
		shift[part[v]] = x[v] - before[v]
	}
	center := func(x []float64) float64 {
		sum, weight := 0.0, 0.0
		for v := range x {
			sum += p.weight[v] * x[v]
			weight += p.weight[v]
		}
		return sum / weight
	}
	assert.InDelta(t, center(before), center(x), 1e-6, "the weighted center stays")
}

// TestPlace_PackingLeavesInterleavedPartsAndASinglePartAlone pins S7's
// *Packing*: a level whose parts no one left-to-right order of them agrees
// with keeps act 2's result, and so does a level with a single part. r's
// two fans hold a1 and a2 apart with room between them, where a part of
// its own, an isolated node or b with its long edge to z, would slide
// toward the center; o->p and c->d cross, so their parts swap sides
// between the layers, each with room to close up.
func TestPlace_PackingLeavesInterleavedPartsAndASinglePartAlone(t *testing.T) {
	// built lays specs out on the given layers, each layer ordered by ids
	// (nodes and dummies) where order gives it
	built := func(layers map[string]int, order [][]string, specs ...string) *lgraph.Graph {
		lv := lgraphtest.Level(t, specs...)
		ls := make([]int, len(lv.Nodes))
		for i, n := range lv.Nodes {
			ls[i] = layers[n.ID]
		}
		g, err := lgraph.Build(lv, ls, make([]bool, len(lv.Edges)), 8)
		require.NoError(t, err)
		for l, ids := range order {
			if ids == nil {
				continue
			}
			g.Layers[l] = g.Layers[l][:0]
			for _, id := range ids {
				v := slices.IndexFunc(g.Vertices, func(vx lgraph.Vertex) bool { return vx.ID == id })
				require.GreaterOrEqual(t, v, 0, "no vertex %s", id)
				g.Layers[l] = append(g.Layers[l], v)
			}
		}
		return g
	}
	fans := []string{"r->a1", "r->a2", "a1->x1", "a1->x2", "a1->x3", "a2->y1", "a2->y2", "a2->y3"}
	layers := map[string]int{"r": 0, "a1": 1, "a2": 1, "iso": 1, "b": 1, "x1": 2, "x2": 2, "x3": 2,
		"y1": 2, "y2": 2, "y3": 2, "z": 3, "o": 0, "c": 1, "q": 2}
	for _, tc := range []struct {
		name string
		g    *lgraph.Graph
	}{
		{"an isolated node inside r's part", built(layers, [][]string{nil, {"a1", "iso", "a2"}}, append(fans, "iso")...)},
		{"b's part inside r's", built(layers, [][]string{nil, {"a1", "b", "a2"}}, append(fans, "b->z")...)},
		{"a single part", built(layers, nil, "o->c", "c->q", "o->q")},
	} {
		assert.Equal(t, beforePacking(tc.g, screen), Place(context.Background(), tc.g, screen), tc.name)
	}

	g := built(map[string]int{"o": 0, "c": 0, "p": 1, "d": 1}, [][]string{{"o", "c"}, {"d", "p"}}, "o->p", "c->d")
	x := []float64{0, 0, 400, -300} // o, p, c, d
	newPlacer(g, screen).pack(context.Background(), x, nil)
	assert.Equal(t, []float64{0, 0, 400, -300}, x, "two parts that swap sides")
}

// TestPlace_TextTerminalsLineUpWithPortsOneCellApart pins S7 (S9, S14):
// under RIGHT a text row's terminals keep only the terminal gap, one cell,
// between their centers, so three of them sit on the three in-ports of a
// node five cells across, one cell apart: each wire runs straight. With
// the lane gap between them, two cells, two would miss their ports.
func TestPlace_TextTerminalsLineUpWithPortsOneCellApart(t *testing.T) {
	lv := lgraphtest.Level(t, "ta", "tb", "tc", "n", "ta->n", "tb->n", "tc->n")
	for _, i := range []int{0, 1, 2} {
		lv.Nodes[i].Terminal, lv.Nodes[i].W, lv.Nodes[i].H = true, 1, 0
	}
	lv.Nodes[3].W, lv.Nodes[3].H = 5, 16
	o := Options{Gap: 2, TerminalGap: 1, Span: 1, Reach: 2, Clearance: 1, InLaneGap: 1, Dir: model.Right, Text: true}
	_, x := laid(t, lv, o)
	assert.InDelta(t, x("n")-1, x("ta"), 1e-9, "on n's first in-port")
	assert.InDelta(t, x("n"), x("tb"), 1e-9, "on n's second in-port")
	assert.InDelta(t, x("n")+1, x("tc"), 1e-9, "on n's third in-port")
}
