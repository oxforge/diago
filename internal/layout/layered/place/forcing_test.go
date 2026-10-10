package place

import (
	"context"
	"fmt"
	"math"
	"slices"
	"testing"

	"github.com/oxforge/diago/internal/layout/layered/lgraph"
	"github.com/oxforge/diago/internal/layout/layered/lgraph/lgraphtest"
	"github.com/oxforge/diago/internal/layout/layered/ports"
	"github.com/oxforge/diago/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPlace_ASideExitHeadsForItsPort pins S7's side-exit forcing on the
// port an exit ends on. b has two in-ports, m->b's the left one, 80/6 in
// from its center. On the baseline b's center lies 3.33 px beyond m's
// right column (50 + 20 out), but that port 10 px inside it, so the exit
// is forced and the port lands exactly under the column, where the router
// draws an L. Judged by b's center, it would not be forced, and the
// router would slide the port 10 px onto the column instead.
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
// (m is a diamond: a circle's exits merge instead,
// TestPlace_ACirclesExitsAreNotForced, and placement sees the shape only
// through m's width.)
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
// m's side column (40 + 20). Without the clause both children would sit
// symmetric, 60 either side of m. The forcing is logged as a primary's.
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
// its side column (40 + 20 out); so does a diamond with three exits, and
// one with a counter-flow end, which is not forced at all.
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
		g := ordered(t, context.Background(), lgraphtest.Level(t, specs...))
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
// in-port), and b keeps its primary, ok under it. Denied it, b would fan
// out symmetrically and the main line bend.
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
	// would take the other in-port, off n's center, and n, which the back
	// edge does not pull (act 1), would drift after it, carrying b away
	// from a.
	o := screen
	o.InLaneGap = 8
	o.Side = [][2]ports.Vertex{{}, {ports.Right, ports.Right}, {}, {ports.Right, ports.Right}, {}}
	_, _, x = placed(t, o, nil, "a->m", "m->a", "b:diamond->n", "n->b", "b->b")
	require.Less(t, x("a"), x("b"), "a left of b")
	assert.InDelta(t, 80+20+20+8, x("b")-x("a"), 1e-6, "a's column and b's loop, facing")
}
