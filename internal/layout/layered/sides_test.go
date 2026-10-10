package layered

import (
	"context"
	"math"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/oxforge/diago/internal/layout/contract"
	"github.com/oxforge/diago/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLayout_TheSecondPassFreesTheFaces(t *testing.T) {
	g := graph(t, nil, "top->bot", "bot->top")
	pg, _, replays := routes(t, g)
	top, bot := pg.Nodes[0], pg.Nodes[1]
	// bot->top hugs one side of both nodes, so top->bot has both faces to
	// itself: one port in the middle of each, straight down
	assert.Equal(t, []model.Point{{X: top.X, Y: top.Y + top.Height/2}, {X: bot.X, Y: bot.Y - bot.Height/2}}, edge(t, pg, "top->bot#0").Points)
	back := edge(t, pg, "bot->top#0").Points
	assert.Equal(t, bot.Y, back[0].Y, "leaves bot at mid-height")
	assert.Equal(t, top.Y, back[len(back)-1].Y, "enters top at mid-height")
	assert.Equal(t, 1, replays, "one replayed route")
}

func TestLayout_WithoutASideAttachmentRouteRunsOnce(t *testing.T) {
	// a DAG has no counter-flow end, so no end attaches at a side face and
	// place and route run once (S8, Second pass)
	g := graph(t, nil, "a->b", "a->c", "b->d", "c->d")
	_, fresh, replays := routes(t, g)
	assert.Equal(t, 1, fresh, "one route")
	assert.Zero(t, replays, "no replayed route")
}

// TestLayout_FlowLoopTakesTwoAdjacentVertices pins S6's Loops kept
// together on the finished flow-loop, in every direction: the loop enters
// and leaves the diamond at two adjacent vertices, Process's exit and the
// back edge's return, and End's exit takes the vertex left over, the one
// opposite the loop's side vertex, outside the loop; nothing crosses.
// With End inside the loop the two loop ends would take opposite
// vertices.
func TestLayout_FlowLoopTakesTwoAdjacentVertices(t *testing.T) {
	for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
		t.Run(dir.String(), func(t *testing.T) {
			pg := lay(t, flowLoop(dir), screen(), dir)
			c := positioned(t, pg, "check")
			w, h := c.Width/2, c.Height/2
			vertices := map[string]model.Point{
				"left": {X: c.X - w, Y: c.Y}, "right": {X: c.X + w, Y: c.Y},
				"top": {X: c.X, Y: c.Y - h}, "bottom": {X: c.X, Y: c.Y + h},
			}
			at := func(p model.Point) string {
				for name, v := range vertices {
					if math.Abs(p.X-v.X) < 1e-6 && math.Abs(p.Y-v.Y) < 1e-6 {
						return name
					}
				}
				return "none"
			}
			first := func(id string) string { return at(edge(t, pg, id).Points[0]) }
			last := func(id string) string { pts := edge(t, pg, id).Points; return at(pts[len(pts)-1]) }
			in, body, back, end := last("init->check#0"), first("check->process#0"), last("increment->check#0"), first("check->end#0")
			used := []string{in, body, back, end}
			slices.Sort(used)
			require.Equal(t, []string{"bottom", "left", "right", "top"}, used, "one vertex each: in %s, Process %s, back edge %s, End %s", in, body, back, end)
			opposite := map[string]string{"left": "right", "right": "left", "top": "bottom", "bottom": "top"}
			assert.NotEqual(t, opposite[body], back, "the loop on two adjacent vertices: Process %s, back edge %s", body, back)
			for _, e := range pg.Edges {
				assert.Empty(t, e.Crossings, "%s crosses nothing", e.ID)
			}
		})
	}
}

// TestLayout_AReplayedSideExitRoutesAsAnL pins S7's second-pass
// prediction on the finished layout: the router replays diamond n1's two
// exits to n0 on its left vertex, a side rail, and n1->n5 alone on its
// right (S8, Second pass). S7 predicts that replay and forces the rail's
// exit, so n0 lands under the rail's column and both wires are Ls, in
// every direction. Predicted by the exit rule instead, as in the first
// pass, the forced rerun puts n2 beyond the band on n1's right, so
// n1->n2 would join n1->n5 on the right vertex; the forcing that adds,
// n5 onto that rail's column, contradicts the left one, both are dropped,
// and the replayed wires leave the left vertex and hook back to n0's
// in-vertex (four bends). (A random graph, found by a search with the
// replay prediction undone: the probe's seed 4048, flat, trimmed to what
// the case needs.)
func TestLayout_AReplayedSideExitRoutesAsAnL(t *testing.T) {
	g := graph(t, nil, "n0:diamond", "n1:diamond", "n2:circle", "n4:rounded", "n5",
		"n1->n5", "n0->n2", "n1->n0", "n1->n0", "n1->n2", "n4->n2")
	for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
		g.Direction = dir
		pg := lay(t, g, screen(), dir)
		assert.Len(t, edge(t, pg, "n1->n0#0").Points, 3, "%s: n1 -> n0 is an L", dir)
		assert.Len(t, edge(t, pg, "n1->n0#1").Points, 3, "%s: the second n1 -> n0 is an L", dir)
	}
}

// TestLayout_ABackEdgeIntoASideVertexRunsAsAC pins S7's column link on
// the finished layout: flow-loop's i++ -> check is reversed, its end on
// diamond check attaches at a side vertex, and its dummy beside Process
// lines up under that vertex's column, so the back edge leaves i++'s side,
// runs along the column and turns into the vertex: a C of two bends, in
// every direction. Under RIGHT and LEFT the diamond's column lay 18.5 px
// beyond the dummy, which the router never moves the column in onto, and
// the back edge stepped on its way (four bends).
func TestLayout_ABackEdgeIntoASideVertexRunsAsAC(t *testing.T) {
	labels := map[string]string{"start": "Start", "init": "i = 0", "check": "i < array.length?",
		"process": "Process array[i]", "increment": "i++", "end": "End"}
	g := graph(t, labels, "start:circle->init", "init->check:diamond", "check->process", "process->increment",
		"increment->check", "check->end:circle")
	for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
		g.Direction = dir
		pg := lay(t, g, screen(), dir)
		back := edge(t, pg, "increment->check#0").Points
		assert.Len(t, back, 4, "%s: i++ -> check is a C: %v", dir, back)
	}
}

// TestLayout_AColumnLinkComesLast pins the order of S7's column link: the
// flat random probe's seed 243, where diamond n6's back edge to n4 leaves
// its right vertex and runs through a dummy beside circle n7. Lined up
// under that column first, the dummy held n6 off n7's line, so n7 -> n6,
// from n7's bottom vertex into n6's top one, ran 3.47 px from the rail into
// n6 (C9.2), neither pinned vertex able to give way. Tried last, the column
// link gives way, and the layout meets the contract under DOWN and UP.
func TestLayout_AColumnLinkComesLast(t *testing.T) {
	g := graph(t, nil, "n0:circle", "n1:circle", "n2", "n3", "n4:hexagon", "n5:circle", "n6:diamond", "n7:circle",
		"n8:parallelogram", "n1->n7", "n0->n4", "n8->n5", "n8->n5", "n2->n3", "n8->n3", "n5->n2", "n6->n4", "n4->n7",
		"n8->n5", "n1->n4", "n8->n6", "n7->n6", "n8->n6")
	for _, dir := range []model.Direction{model.Down, model.Up} {
		g.Direction = dir
		pg := lay(t, g, screen(), dir)
		for _, v := range contract.Check(pg, contract.Options{Limits: contract.ScreenLimits(), Direction: dir, EdgeIDs: ids(g)}) {
			t.Errorf("%s: %s %s: %s", dir, v.Rule, v.Subject, v.Detail)
		}
	}
}

// mergedFanOut checks that the exits of node id all leave its out-vertex
// under dir, and that those that do not run straight share one trunk at
// least C7's stub long (20 px), straight on from the vertex, and then
// split, each turning off it where the trunk ends (S8, C8.5); it returns
// how many split off.
func mergedFanOut(t *testing.T, pg *model.PositionedGraph, dir model.Direction, id string, exits ...string) int {
	t.Helper()
	out := outVertex(positioned(t, pg, id), dir)
	var trunk []model.Point
	var split []float64
	for _, e := range exits {
		pts := edge(t, pg, e).Points
		assert.InDelta(t, out.X, pts[0].X, 1e-6, "%s: %s leaves %s's out-vertex: %v", dir, e, id, pts)
		assert.InDelta(t, out.Y, pts[0].Y, 1e-6, "%s: %s leaves %s's out-vertex: %v", dir, e, id, pts)
		if len(pts) == 2 {
			continue // straight on, through the trunk
		}
		require.GreaterOrEqual(t, len(pts), 3, "%s: %s: %v", dir, e, pts)
		if trunk == nil {
			trunk = pts[:2]
		}
		assert.Equal(t, trunk, pts[:2], "%s: %s runs down the trunk: %v", dir, e, pts)
		along0, across0 := flowAxis(pts[0], dir)
		along1, across1 := flowAxis(pts[1], dir)
		along2, across2 := flowAxis(pts[2], dir)
		assert.InDelta(t, across0, across1, 1e-6, "%s: %s's trunk runs straight on: %v", dir, e, pts)
		assert.GreaterOrEqual(t, math.Abs(along1-along0), 20.0, "%s: %s's trunk keeps C7's stub: %v", dir, e, pts)
		assert.InDelta(t, along1, along2, 1e-6, "%s: %s turns off where the trunk ends: %v", dir, e, pts)
		assert.NotContains(t, split, across2, "%s: %s splits off on its own run: %v", dir, e, pts)
		split = append(split, across2)
	}
	return len(split)
}

// TestLayout_ACirclesExitsShareOneTrunk pins S8's diamonds and circles on
// the finished layout, in every direction: a circle's two or three forward
// exits all leave its out-vertex and run on together, at least C7's stub,
// before they split toward their children (a bracket, C8.5's merged
// fan-out), rather than leaving its side vertices as Ls. A diamond with
// the same exits still spreads them over its vertices, which C8.5 forbids
// it to share. The layouts meet the whole contract.
func TestLayout_ACirclesExitsShareOneTrunk(t *testing.T) {
	for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
		t.Run(dir.String(), func(t *testing.T) {
			g := graph(t, nil, "m:circle->a", "m->b")
			g.Direction = dir
			pg := lay(t, g, screen(), dir)
			assert.Equal(t, 2, mergedFanOut(t, pg, dir, "m", "m->a#0", "m->b#0"), "both exits split off the trunk")

			g = graph(t, nil, "m:circle->a", "m->b", "m->c")
			g.Direction = dir
			pg = lay(t, g, screen(), dir)
			assert.GreaterOrEqual(t, mergedFanOut(t, pg, dir, "m", "m->a#0", "m->b#0", "m->c#0"), 2, "the outer exits split off the trunk")

			g = graph(t, nil, "m:diamond->a", "m->b")
			g.Direction = dir
			pg = lay(t, g, screen(), dir)
			assert.NotEqual(t, edge(t, pg, "m->a#0").Points[0], edge(t, pg, "m->b#0").Points[0], "a diamond's exits leave two vertices")
		})
	}
}

// TestLayout_ACirclesMergedFanOutBesideACounterFlowEnd pins S8's diamonds
// and circles on the finished layout, in every direction, where the route
// tests stop at the route level: a circle's four forward exits all leave
// its out-vertex on one trunk (C8.5's merged fan-out), and so do its two
// beside a counter-flow end, which keeps to a side vertex (C8.4): neither
// gives way to the other. The layouts meet the whole contract.
func TestLayout_ACirclesMergedFanOutBesideACounterFlowEnd(t *testing.T) {
	for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
		t.Run(dir.String(), func(t *testing.T) {
			g := graph(t, nil, "m:circle->a", "m->b", "m->c", "m->d")
			g.Direction = dir
			pg := lay(t, g, screen(), dir)
			assert.GreaterOrEqual(t, mergedFanOut(t, pg, dir, "m", "m->a#0", "m->b#0", "m->c#0", "m->d#0"), 2, "the outer exits split off the trunk")

			g = graph(t, nil, "m:circle->a", "m->b", "b->m")
			g.Direction = dir
			pg = lay(t, g, screen(), dir)
			assert.Equal(t, 2, mergedFanOut(t, pg, dir, "m", "m->a#0", "m->b#0"), "both exits split off the trunk")
			m := positioned(t, pg, "m")
			sides := []model.Point{{X: m.X - m.Width/2, Y: m.Y}, {X: m.X + m.Width/2, Y: m.Y}}
			if dir == model.Right || dir == model.Left {
				sides = []model.Point{{X: m.X, Y: m.Y - m.Height/2}, {X: m.X, Y: m.Y + m.Height/2}}
			}
			back := edge(t, pg, "b->m#0").Points
			end := back[len(back)-1]
			assert.True(t, slices.ContainsFunc(sides, func(p model.Point) bool { return math.Abs(p.X-end.X) < 1e-6 && math.Abs(p.Y-end.Y) < 1e-6 }),
				"b->m enters a side vertex of m %v: %v", sides, back)
		})
	}
}

// cicdPipeline is the cicd-pipeline spec: "git push", a
// circle, feeds Lint and Unit Tests in the CI group, which build an image
// that CD deploys.
func cicdPipeline(dir model.Direction) model.Graph {
	return model.Graph{
		Direction: dir,
		Nodes: []model.Node{
			{ID: "push", Label: "git push", Shape: model.ShapeCircle}, {ID: "lint", Label: "Lint", Shape: model.ShapeRect},
			{ID: "test", Label: "Unit Tests", Shape: model.ShapeRect}, {ID: "build", Label: "Build Image", Shape: model.ShapeRect},
			{ID: "registry", Label: "Registry", Shape: model.ShapeCylinder}, {ID: "staging", Label: "Deploy Staging", Shape: model.ShapeRect},
			{ID: "e2e", Label: "E2E Tests", Shape: model.ShapeRect}, {ID: "gate", Label: "Approved?", Shape: model.ShapeDiamond},
			{ID: "prod", Label: "Deploy Prod", Shape: model.ShapeRect}, {ID: "rollback", Label: "Rollback", Shape: model.ShapeRect},
		},
		Edges: []model.Edge{
			{ID: "push->lint#0", From: "push", To: "lint"}, {ID: "push->test#0", From: "push", To: "test"},
			{ID: "lint->build#0", From: "lint", To: "build"}, {ID: "test->build#0", From: "test", To: "build"},
			{ID: "build->registry#0", From: "build", To: "registry", Label: "push"},
			{ID: "registry->staging#0", From: "registry", To: "staging", Label: "pull"},
			{ID: "staging->e2e#0", From: "staging", To: "e2e"}, {ID: "e2e->gate#0", From: "e2e", To: "gate"},
			{ID: "gate->prod#0", From: "gate", To: "prod", Label: "yes"},
			{ID: "prod->rollback#0", From: "prod", To: "rollback", Label: "on failure", Style: model.EdgeDashed},
		},
		Groups: []model.Group{
			{ID: "ci", Label: "CI", Contains: []string{"lint", "test", "build"}},
			{ID: "cd", Label: "CD", Contains: []string{"staging", "e2e", "gate", "prod", "rollback"}},
		},
	}
}

// TestLayout_GitPushsExitsLeaveOnePoint pins S8's merged fan-out on
// cicd-pipeline, in every direction: "git push"'s two exits leave its
// out-vertex (its right vertex under RIGHT) on one trunk and split into a
// bracket that enters Lint and Unit Tests, two bends each. From its side
// vertices, under RIGHT and LEFT, each would hook back toward the CI
// group's anchored ports, which sit 86.6 px apart against side columns
// 150.7 px apart, three bends each.
func TestLayout_GitPushsExitsLeaveOnePoint(t *testing.T) {
	for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
		t.Run(dir.String(), func(t *testing.T) {
			pg := lay(t, cicdPipeline(dir), screen(), dir)
			assert.Equal(t, 2, mergedFanOut(t, pg, dir, "push", "push->lint#0", "push->test#0"), "both exits split off the trunk")
			for _, id := range []string{"push->lint#0", "push->test#0"} {
				assert.Len(t, edge(t, pg, id).Points, 4, "%s: two bends: %v", id, edge(t, pg, id).Points)
			}
		})
	}
}

// TestLayout_InterleavedBackEdgesRunOnOppositeSides pins S6's counter-flow
// sides on the finished layout, in every direction and both profiles: the
// back edges d->a and e->b, whose layer spans interleave (0 to 3 and 1 to
// 4), run on opposite sides of c, the node both pass, and cross nothing,
// where on one side they would cross. x->a, whose span nests in e->a's,
// keeps e->a's side, although S6 leaves the two a crossing a side move of
// x->a would take away (order's TestMinimize_NestedChainsKeepTheirSides).
func TestLayout_InterleavedBackEdgesRunOnOppositeSides(t *testing.T) {
	for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
		for _, text := range []bool{false, true} {
			cfg, lim := screen(), contract.ScreenLimits()
			if text {
				cfg, lim = TextConfig(dir), contract.TextLimits(dir)
			}
			layout := func(specs ...string) *model.PositionedGraph {
				g := graph(t, nil, specs...)
				g.Direction = dir
				pg, err := Layout(context.Background(), g, cfg, nil)
				require.NoError(t, err)
				for _, v := range contract.Check(pg, contract.Options{Limits: lim, Direction: dir, EdgeIDs: ids(g), Text: text}) {
					t.Errorf("%s text=%v: %s %s: %s", dir, text, v.Rule, v.Subject, v.Detail)
				}
				return pg
			}
			pg := layout("a->b", "b->c", "c->d", "d->e", "d->a", "e->b")
			for _, e := range pg.Edges {
				assert.Empty(t, e.Crossings, "%s text=%v: %s", dir, text, e.ID)
			}
			assert.Equal(t, -side(t, pg, dir, "d->a#0"), side(t, pg, dir, "e->b#0"), "%s text=%v: interleaved", dir, text)
			pg = layout("a->b", "b->c", "c->d", "d->e", "c->x", "e->a", "x->a")
			assert.Equal(t, side(t, pg, dir, "e->a#0"), side(t, pg, dir, "x->a#0"), "%s text=%v: nested", dir, text)
		}
	}
}

// TestLayout_ASideColumnKeepsReachWhileAWireStraightens pins S8's stops on
// the finished layout, in every direction and both profiles: a side column
// moves to straighten a wire only where it stays Reach outside its side,
// however near that x. Under DOWN and UP, diamond n5's exit to n6 leaves
// its left vertex, and placement leaves n6's in-port 2.67 px inside the
// column Reach out: the wire runs as an L on the column, n6's port coming
// onto it, and its stub out of the vertex keeps C7's floor, where the
// column taking the port's x would leave 17.33 px. (The random-graph
// probe's seed 76 of its decisions mode, found by a search with that
// check undone.)
func TestLayout_ASideColumnKeepsReachWhileAWireStraightens(t *testing.T) {
	specs := []string{"n0", "n1:diamond", "n2:diamond", "n3:diamond", "n4:diamond", "n5:diamond", "n6", "n7", "n8",
		"n0->n7", "n1->n8", "n1->n2", "n2->n3", "n2->n7", "n3->n5", "n3->n8", "n4->n5", "n4->n6", "n5->n8", "n5->n6"}
	for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
		for _, text := range []bool{false, true} {
			cfg, lim := screen(), contract.ScreenLimits()
			if text {
				cfg, lim = TextConfig(dir), contract.TextLimits(dir)
			}
			g := graph(t, nil, specs...)
			g.Direction = dir
			pg, err := Layout(context.Background(), g, cfg, nil)
			require.NoError(t, err)
			for _, v := range contract.Check(pg, contract.Options{Limits: lim, Direction: dir, EdgeIDs: ids(g), Text: text}) {
				t.Errorf("%s text=%v: %s %s: %s", dir, text, v.Rule, v.Subject, v.Detail)
			}
			if text || dir == model.Right || dir == model.Left {
				continue
			}
			got := edge(t, pg, "n5->n6#0").Points
			require.Len(t, got, 3, "%s: an L from n5's side vertex down its column into n6: %v", dir, got)
			assert.InDelta(t, screen().Reach, math.Abs(got[1].X-got[0].X), 1e-9, "%s: the column Reach out: %v", dir, got)
		}
	}
}

// side is the side of node c's center, across the flow, where edge id's
// route runs farthest out: -1 or 1.
func side(t *testing.T, pg *model.PositionedGraph, dir model.Direction, id string) int {
	t.Helper()
	c := positioned(t, pg, "c")
	far := 0.0
	for _, p := range edge(t, pg, id).Points {
		off := p.X - c.X
		if dir == model.Right || dir == model.Left {
			off = p.Y - c.Y
		}
		if math.Abs(off) > math.Abs(far) {
			far = off
		}
	}
	require.NotZero(t, far, "%s leaves c's axis", id)
	if far < 0 {
		return -1
	}
	return 1
}

func TestLayout_ASideColumnKeepsItsReach(t *testing.T) {
	// n1->n0#1 is reversed and leaves cylinder n1 at a side face. Under
	// DOWN and UP the dummy column it runs along lies 19.80 px out from
	// that side, 0.20 px inside Reach; a snap of its column toward the
	// dummy would leave a 19.80 px stub (C7), so the dummy moves to the
	// column instead (S8, Stops). (A random graph, found by a search with
	// the column's give-way undone; it needs every node and edge.)
	g := graph(t, map[string]string{"n0": "staging Deploy", "n1": "Rollback", "n2": "Typecheck", "n3": "Fix Deploy"},
		"n0:parallelogram", "n1:cylinder", "n2:diamond", "n3:diamond", "n1->n2", "n0->n2", "n1->n0", "n1->n0", "n3->n1", "n2->n3")
	for _, dir := range []model.Direction{model.Down, model.Up} {
		g.Direction = dir
		pg := lay(t, g, screen(), dir)
		back := edge(t, pg, "n1->n0#1").Points
		n1 := positioned(t, pg, "n1")
		assert.Equal(t, n1.Y, back[0].Y, "%s: leaves n1's side at its middle", dir)
		assert.InDelta(t, 20.0, math.Abs(back[1].X-back[0].X), 1e-9, "%s: the column Reach outside the side", dir)
	}
}

// TestLayout_PinnedSideColumnsJoinTheSecondPass pins S8's second pass on
// pinned nodes: the counter-flow ends of l->top and r->top attach at the
// facing side vertices of diamonds l and r, the only side attachments of
// the level, so place runs again and keeps room for both columns (S7),
// Reach out of each and InLaneGap between them, and route replays once.
func TestLayout_PinnedSideColumnsJoinTheSecondPass(t *testing.T) {
	g := graph(t, nil, "top:diamond->l:diamond", "top->r:diamond", "l->top", "r->top")
	pg, _, replays := routes(t, g)
	assert.Equal(t, 1, replays, "one replayed route")
	var l, r model.PositionedNode
	for _, n := range pg.Nodes {
		switch n.ID {
		case "l":
			l = n
		case "r":
			r = n
		}
	}
	cfg := screen()
	assert.GreaterOrEqual(t, (r.X-r.Width/2)-(l.X+l.Width/2), 2*cfg.Reach+cfg.InLaneGap-1e-9, "l's right side and r's left side")
}

// TestLayout_FacingPinnedColumnsRunApart pins S7's facing room with S8's
// column rule, end to end: l->top's and r->top's columns face each other
// between diamonds l and r and both aim over top's center. The second pass
// keeps room for both (TestLayout_PinnedSideColumnsJoinTheSecondPass), and
// since each column moves out only where it keeps InLaneGap from the
// other, they run InLaneGap apart, not on one x.
func TestLayout_FacingPinnedColumnsRunApart(t *testing.T) {
	g := graph(t, nil, "top:diamond->l:diamond", "top->r:diamond", "l->top", "r->top")
	pg, _, _ := routes(t, g)
	lt, rt := edge(t, pg, "l->top#0").Points, edge(t, pg, "r->top#0").Points
	require.Greater(t, len(lt), 1)
	require.Greater(t, len(rt), 1)
	assert.GreaterOrEqual(t, rt[1].X-lt[1].X, screen().InLaneGap-1e-9, "l's right column and r's left column: %v, %v", lt, rt)
}

// TestLayout_ArrivalsGiveWayToAPinnedNodesExits pins S8's diamonds and
// circles end to end, through the second pass: diamond n0 forms a
// two-cycle one hop apart with each of diamonds n1 and n2 (the random
// probe's seed 145), so the reversed n1->n0 and n2->n0 arrive at n0 and
// claim both of its side vertices. The one on the side the farther-heading
// exit heads for gives way and shares the other side, the replay keeps
// those sides, and n0's two exits leave from their own vertices (C8.5):
// one from the side vertex the arrivals left, the other from the
// out-vertex. The layout meets the whole contract, in every direction.
func TestLayout_ArrivalsGiveWayToAPinnedNodesExits(t *testing.T) {
	g := graph(t, nil, "n0:diamond->n1:diamond", "n1->n0", "n2:diamond->n0", "n0->n2")
	for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
		t.Run(dir.String(), func(t *testing.T) {
			g.Direction = dir
			pg := lay(t, g, screen(), dir)
			i := slices.IndexFunc(pg.Nodes, func(n model.PositionedNode) bool { return n.ID == "n0" })
			require.GreaterOrEqual(t, i, 0)
			n := pg.Nodes[i]
			// n0's vertices in the output frame: its two side vertices on
			// the cross axis, and its out-vertex downstream
			w, h := n.Width/2, n.Height/2
			vertices := map[string]model.Point{"side1": {X: n.X - w, Y: n.Y}, "side2": {X: n.X + w, Y: n.Y}, "out": {X: n.X, Y: n.Y + h}}
			switch dir {
			case model.Up:
				vertices["out"] = model.Point{X: n.X, Y: n.Y - h}
			case model.Right, model.Left:
				vertices = map[string]model.Point{"side1": {X: n.X, Y: n.Y - h}, "side2": {X: n.X, Y: n.Y + h}, "out": {X: n.X + w, Y: n.Y}}
				if dir == model.Left {
					vertices["out"] = model.Point{X: n.X - w, Y: n.Y}
				}
			}
			at := func(p model.Point) string {
				for _, name := range []string{"side1", "side2", "out"} {
					if v := vertices[name]; math.Abs(p.X-v.X) < 1e-6 && math.Abs(p.Y-v.Y) < 1e-6 {
						return name
					}
				}
				return "none"
			}
			first := func(id string) model.Point { return edge(t, pg, id).Points[0] }
			last := func(id string) model.Point { pts := edge(t, pg, id).Points; return pts[len(pts)-1] }
			arrivals := at(last("n1->n0#0"))
			require.Contains(t, []string{"side1", "side2"}, arrivals, "n1->n0 enters a side vertex of n0")
			assert.Equal(t, arrivals, at(last("n2->n0#0")), "n2->n0 enters the same side vertex")
			want := []string{map[string]string{"side1": "side2", "side2": "side1"}[arrivals], "out"}
			got := []string{at(first("n0->n1#0")), at(first("n0->n2#0"))}
			slices.Sort(want)
			slices.Sort(got)
			assert.Equal(t, want, got, "n0's exits leave the other side vertex and the out-vertex")
		})
	}
}

// TestLayout_AFartherExitSharesALeavingEndsSide pins S8's diamonds and
// circles: diamond m forms a two-cycle with u, where m->u leaves and holds
// a side vertex, and a second two-cycle with b, where b->m arrives and
// holds the other; m's third exit, m->a, heads farther than m->b and
// toward u's side. The farther-heading exit takes that side (C8.5), even
// though a leaving end already holds it: m->a shares it with m->u, m->b
// takes the bottom, and b->m stays on its own side. Dropping S8's clause
// (the one no leaving end holds) means this is the exit's own side
// regardless of what kind of counter-flow end already holds it: the
// arriving end b->m never moves. The layout meets the whole contract, in
// every direction.
func TestLayout_AFartherExitSharesALeavingEndsSide(t *testing.T) {
	g := graph(t, nil, "u->m:diamond", "m->u", "m->b", "b->m", "m->a")
	for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
		t.Run(dir.String(), func(t *testing.T) {
			g.Direction = dir
			pg := lay(t, g, screen(), dir)
			i := slices.IndexFunc(pg.Nodes, func(n model.PositionedNode) bool { return n.ID == "m" })
			require.GreaterOrEqual(t, i, 0)
			n := pg.Nodes[i]
			w, h := n.Width/2, n.Height/2
			vertices := map[string]model.Point{"side1": {X: n.X - w, Y: n.Y}, "side2": {X: n.X + w, Y: n.Y}, "out": {X: n.X, Y: n.Y + h}}
			switch dir {
			case model.Up:
				vertices["out"] = model.Point{X: n.X, Y: n.Y - h}
			case model.Right, model.Left:
				vertices = map[string]model.Point{"side1": {X: n.X, Y: n.Y - h}, "side2": {X: n.X, Y: n.Y + h}, "out": {X: n.X + w, Y: n.Y}}
				if dir == model.Left {
					vertices["out"] = model.Point{X: n.X - w, Y: n.Y}
				}
			}
			at := func(p model.Point) string {
				for _, name := range []string{"side1", "side2", "out"} {
					if v := vertices[name]; math.Abs(p.X-v.X) < 1e-6 && math.Abs(p.Y-v.Y) < 1e-6 {
						return name
					}
				}
				return "none"
			}
			first := func(id string) model.Point { return edge(t, pg, id).Points[0] }
			last := func(id string) model.Point { pts := edge(t, pg, id).Points; return pts[len(pts)-1] }
			leaves := at(first("m->u#0"))
			require.Contains(t, []string{"side1", "side2"}, leaves, "m->u leaves a side vertex of m")
			arrives := at(last("b->m#0"))
			require.NotEqual(t, leaves, arrives, "b->m arrives at the side m->u does not hold")
			assert.Equal(t, leaves, at(first("m->a#0")), "m->a, heading farther, shares m->u's side")
			assert.Equal(t, "out", at(first("m->b#0")), "m->b, heading nearer, takes the bottom")
		})
	}
}

// TestLayout_TwoCyclesKeepTheirVerticalsApart pins S8's ports end to end:
// diamond n0 forms a two-cycle one hop apart with each of diamonds n1 and
// n2 (the random probe's seed 145). The back edges' side columns land
// within InLaneGap of a forward edge's port in their channel and give way
// by InLaneGap, a side rail as one: no two edges share a run or run side
// by side (C9), in any direction. (TestLayout_ArrivalsGiveWayToAPinnedNodesExits
// reads the whole contract on this graph.)
func TestLayout_TwoCyclesKeepTheirVerticalsApart(t *testing.T) {
	g := graph(t, nil, "n0:diamond->n1:diamond", "n1->n0", "n2:diamond->n0", "n0->n2")
	for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
		t.Run(dir.String(), func(t *testing.T) {
			g.Direction = dir
			pg, err := Layout(context.Background(), g, screen(), nil)
			require.NoError(t, err)
			for _, v := range contract.Check(pg, contract.Options{Limits: contract.ScreenLimits(), Direction: dir, EdgeIDs: ids(g)}) {
				if strings.HasPrefix(v.Rule, "C9") {
					t.Errorf("%s %s: %s", v.Rule, v.Subject, v.Detail)
				}
			}
		})
	}
}

// TestLayout_AParallelogramsTopAndBottomPortsLineUp pins S8's Shape ports
// for a parallelogram under DOWN and UP: both faces take the stretch its
// top and bottom edges share, so a lone port on each lies on the node's
// center line, and two ports on each line up across the node.
func TestLayout_AParallelogramsTopAndBottomPortsLineUp(t *testing.T) {
	positioned := func(pg *model.PositionedGraph, id string) model.PositionedNode {
		for _, n := range pg.Nodes {
			if n.ID == id {
				return n
			}
		}
		t.Fatalf("no node %s", id)
		return model.PositionedNode{}
	}
	for _, dir := range []model.Direction{model.Down, model.Up} {
		t.Run("one in, one out, "+dir.String(), func(t *testing.T) {
			g := model.Graph{
				Direction: dir,
				Nodes: []model.Node{
					{ID: "start", Label: "Start", Shape: model.ShapeRounded},
					{ID: "input", Label: "Read input", Shape: model.ShapeParallelogram},
					{ID: "check", Label: "Check", Shape: model.ShapeRect},
				},
				Edges: []model.Edge{{ID: "in", From: "start", To: "input"}, {ID: "out", From: "input", To: "check"}},
			}
			pg := lay(t, g, screen(), dir)
			p := positioned(pg, "input")
			in, out := edge(t, pg, "in").Points, edge(t, pg, "out").Points
			assert.InDelta(t, p.X, in[len(in)-1].X, 1e-6, "the in-port is on the center line")
			assert.InDelta(t, p.X, out[0].X, 1e-6, "the out-port is on the center line")
		})
		t.Run("two in, two out, "+dir.String(), func(t *testing.T) {
			g := model.Graph{
				Direction: dir,
				Nodes: []model.Node{
					{ID: "a", Label: "Alpha", Shape: model.ShapeRect}, {ID: "b", Label: "Beta", Shape: model.ShapeRect},
					{ID: "p", Label: "A wide parallelogram", Shape: model.ShapeParallelogram},
					{ID: "c", Label: "Gamma", Shape: model.ShapeRect}, {ID: "d", Label: "Delta", Shape: model.ShapeRect},
				},
				Edges: []model.Edge{
					{ID: "ap", From: "a", To: "p"}, {ID: "bp", From: "b", To: "p"},
					{ID: "pc", From: "p", To: "c"}, {ID: "pd", From: "p", To: "d"},
				},
			}
			pg := lay(t, g, screen(), dir)
			ins := []float64{last(edge(t, pg, "ap").Points).X, last(edge(t, pg, "bp").Points).X}
			outs := []float64{edge(t, pg, "pc").Points[0].X, edge(t, pg, "pd").Points[0].X}
			sort.Float64s(ins)
			sort.Float64s(outs)
			assert.InDeltaSlice(t, ins, outs, 1e-6, "the in-ports and the out-ports line up")
		})
	}
}
