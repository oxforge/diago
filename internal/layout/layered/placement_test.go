package layered

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"slices"
	"strings"
	"testing"

	"github.com/oxforge/diago/internal/layout/contract"
	"github.com/oxforge/diago/internal/layoutdbg/layoutdbgtest"
	"github.com/oxforge/diago/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lrPipeline is the lr-pipeline example in direction dir.
func lrPipeline(dir model.Direction) model.Graph {
	return model.Graph{
		Direction: dir,
		Nodes: []model.Node{
			{ID: "src", Label: "Source", Shape: model.ShapeRounded}, {ID: "extract", Label: "Extract", Shape: model.ShapeRect},
			{ID: "valid", Label: "Valid?", Shape: model.ShapeDiamond}, {ID: "transform", Label: "Transform", Shape: model.ShapeRect},
			{ID: "load", Label: "Warehouse", Shape: model.ShapeCylinder}, {ID: "errors", Label: "Error log", Shape: model.ShapeRect},
		},
		Edges: []model.Edge{
			{ID: "src->extract#0", From: "src", To: "extract"}, {ID: "extract->valid#0", From: "extract", To: "valid"},
			{ID: "valid->transform#0", From: "valid", To: "transform", Label: "yes"},
			{ID: "valid->errors#0", From: "valid", To: "errors", Label: "no"},
			{ID: "transform->transform#0", From: "transform", To: "transform", Label: "retry"},
			{ID: "transform->load#0", From: "transform", To: "load"},
			{ID: "errors->extract#0", From: "errors", To: "extract"},
		},
	}
}

// TestLayout_LRPipelineKeepsItsMainLineStraight pins S7's act 1 and
// side-exit forcing on lr-pipeline, in every direction: the back
// edge errors->extract does not pull Extract (a node with faces), and
// Valid?'s "yes" exit is its primary (Transform leads on to Warehouse,
// Error log only back), so the main line Source -> Extract -> Valid? ->
// Transform runs straight through the diamond; "no" leaves a side vertex
// as an L into Error log, whose in-face it shares with the back edge. The
// back edge's pull would put Extract 30 px off Valid?'s axis, and side-exit
// forcing judged by Error log's center would pack both ends 10 px apart at
// one end of the face. Under RIGHT and LEFT the two ends sit at a third and
// two thirds of that face and the back edge runs as an L into Extract;
// under DOWN and UP "no"'s port gives way from the back edge's dummy
// column, which S7 leaves where act 1 puts it (both of that chain's hops
// end at counter-flow ends, so it has no link), and the back edge jogs to
// that column.
func TestLayout_LRPipelineKeepsItsMainLineStraight(t *testing.T) {
	for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
		t.Run(dir.String(), func(t *testing.T) {
			pg := lay(t, lrPipeline(dir), screen(), dir)
			for _, id := range []string{"src->extract#0", "extract->valid#0", "valid->transform#0"} {
				assert.Len(t, edge(t, pg, id).Points, 2, "%s runs straight", id)
			}
			valid := positioned(t, pg, "valid")
			near(t, outVertex(valid, dir), edge(t, pg, "valid->transform#0").Points[0], "yes leaves the flow-axis vertex")
			errors := positioned(t, pg, "errors")
			// across the flow: x under DOWN and UP, y under RIGHT and LEFT
			across, along := func(p model.Point) float64 { return p.X }, func(p model.Point) float64 { return p.Y }
			lo, size := errors.X-errors.Width/2, errors.Width
			sideways := dir == model.Right || dir == model.Left
			if sideways {
				across, along = along, across
				lo, size = errors.Y-errors.Height/2, errors.Height
			}
			no := edge(t, pg, "valid->errors#0").Points
			require.Len(t, no, 3, "no runs as an L: %v", no)
			assert.InDelta(t, along(model.Point{X: valid.X, Y: valid.Y}), along(no[0]), 1e-6, "no leaves a side vertex")
			back := edge(t, pg, "errors->extract#0").Points
			assert.InDelta(t, along(no[len(no)-1]), along(back[0]), 1e-6, "both ends on Error log's in-face")
			if !sideways {
				return
			}
			assert.InDelta(t, lo+size/3, across(no[len(no)-1]), 1e-6, "no enters at a third of Error log's face")
			assert.InDelta(t, lo+2*size/3, across(back[0]), 1e-6, "the back edge leaves at two thirds")
			assert.Len(t, back, 3, "the back edge runs as an L: %v", back)
		})
	}
}

// TestLayout_ADiamondsMainLineRunsStraight pins S7's primary clause on the
// finished layout, in every direction: of decision d's two exits, only
// the one into main leads on (to next), so it is the main line, a single
// straight segment from d's flow-axis vertex, and the one into end leaves
// a side vertex as an L. When both exits lead on, or neither, no branch
// is the main line: both leave the side vertices as Ls, symmetric about
// d's axis (Q4).
func TestLayout_ADiamondsMainLineRunsStraight(t *testing.T) {
	for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
		t.Run(dir.String(), func(t *testing.T) {
			g := graph(t, nil, "s->d:diamond", "d->main", "d->end", "main->next")
			g.Direction = dir
			pg := lay(t, g, screen(), dir)
			d := positioned(t, pg, "d")
			main := edge(t, pg, "d->main#0").Points
			require.Len(t, main, 2, "the main line runs straight: %v", main)
			near(t, outVertex(d, dir), main[0], "from d's flow-axis vertex")
			assert.Len(t, edge(t, pg, "main->next#0").Points, 2, "and on through main")
			other := edge(t, pg, "d->end#0").Points
			require.Len(t, other, 3, "the other branch runs as an L: %v", other)
			assert.True(t, sideVertex(d, other[0], dir), "from a side vertex: %v", other)

			for _, specs := range [][]string{
				{"s->d:diamond", "d->yes", "d->no"},
				{"s->d:diamond", "d->yes", "d->no", "yes->a", "no->b"},
			} {
				g := graph(t, nil, specs...)
				g.Direction = dir
				pg := lay(t, g, screen(), dir)
				d := positioned(t, pg, "d")
				yes, no := edge(t, pg, "d->yes#0").Points, edge(t, pg, "d->no#0").Points
				require.Len(t, yes, 3, "%v: an L: %v", specs, yes)
				require.Len(t, no, 3, "%v: an L: %v", specs, no)
				assert.True(t, sideVertex(d, yes[0], dir), "%v: from a side vertex: %v", specs, yes)
				assert.True(t, sideVertex(d, no[0], dir), "%v: from a side vertex: %v", specs, no)
				across := func(n model.PositionedNode) float64 {
					if dir == model.Right || dir == model.Left {
						return n.Y
					}
					return n.X
				}
				ya, na := across(positioned(t, pg, "yes")), across(positioned(t, pg, "no"))
				assert.InDelta(t, across(d), (ya+na)/2, 1e-6, "%v: symmetric about d's axis", specs)
			}
		})
	}
}

// TestLayout_ThePrimaryClausesEdgeCases pins S7's primary clause on the
// finished layout, in every direction, in the cases its placement tests
// left out: an exit into a group leads on by the group's own edges, so
// d->a, into G, which feeds x, is d's main line, straight from its
// flow-axis vertex, and d->end a side exit; an exit that leaves d's group
// ends at a terminal there, which never leads on, so d->a, which leads on
// inside G, is the main line though o, outside, leads on too, and with
// nothing inside leading on, d->a and d->o both leave side vertices, no
// main line; a diamond whose own exit runs against the flow (d->s) has no
// main line, its back edge claiming a side vertex, so d->main, which
// alone leads on, leaves a side vertex too; and a diamond with three exits
// has none either, its exits spread over its three vertices, d->main on a
// side. The layouts meet the whole contract.
func TestLayout_ThePrimaryClausesEdgeCases(t *testing.T) {
	inG := func(g model.Graph, ids ...string) model.Graph {
		return grouped(g, model.Group{ID: "G", Label: "G", Contains: ids})
	}
	for _, tc := range []struct {
		name  string
		g     model.Graph
		main  string   // d's main line, "" for none
		sides []string // d's exits from a side vertex
	}{
		{"a group leads on by its own edges", inG(graph(t, nil, "s->d:diamond", "d->a", "d->end", "a->x"), "a"), "d->a#0", []string{"d->end#0"}},
		{"a terminal never leads on", inG(graph(t, nil, "s->d:diamond", "d->a", "d->o", "a->x", "o->y"), "s", "d", "a", "x"), "d->a#0", []string{"d->o#0"}},
		{"nothing leads on inside the group", inG(graph(t, nil, "s->d:diamond", "d->a", "d->o", "o->y"), "s", "d", "a"), "", []string{"d->a#0", "d->o#0"}},
		{"a back edge of its own", graph(t, nil, "s->d:diamond", "d->main", "d->end", "main->next", "d->s"), "", []string{"d->main#0", "d->s#0"}},
		{"three exits", graph(t, nil, "s->d:diamond", "d->main", "d->b", "d->c", "main->next"), "", []string{"d->main#0", "d->c#0"}},
	} {
		for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
			t.Run(tc.name+"/"+dir.String(), func(t *testing.T) {
				pg, _ := layLogged(t, tc.g, dir, false)
				d := positioned(t, pg, "d")
				if tc.main != "" {
					pts := edge(t, pg, tc.main).Points
					require.Len(t, pts, 2, "the main line runs straight: %v", pts)
					near(t, outVertex(d, dir), pts[0], "from d's flow-axis vertex")
				}
				for _, id := range tc.sides {
					pts := edge(t, pg, id).Points
					assert.True(t, sideVertex(d, pts[0], dir), "%s from a side vertex of d: %v", id, pts)
				}
				if tc.name == "three exits" {
					b := edge(t, pg, "d->b#0").Points
					near(t, outVertex(d, dir), b[0], "d->b from d's flow-axis vertex")
					assert.NotEqual(t, edge(t, pg, "d->main#0").Points[0], edge(t, pg, "d->c#0").Points[0], "two side vertices")
				}
			})
		}
	}
}

// TestLayout_ACascadesSecondariesRouteAsLs pins S7's room on the finished
// layout, in every direction: in a cascade of decisions, every decision's
// main line runs straight on from its flow-axis vertex, every edge into a
// decision runs straight, and every other branch leaves a side vertex as
// an L. Each decision's other branch lands in the row of the decision
// below it, where it would keep that decision's side column from moving
// out over its port; it keeps the node clearance beyond the port instead
// (the room): 2 and 3 bends in all, where denying the lower decisions
// their primaries would fan them out symmetrically and bend their main
// lines, 3 and 5. In the shape where z, the other source into end, sits
// in m's row, m keeps its primary too (3 bends under RIGHT and LEFT, not
// 4).
func TestLayout_ACascadesSecondariesRouteAsLs(t *testing.T) {
	for _, tc := range []struct {
		specs    []string
		straight []string // the exits that run straight on, the rest leave a side vertex as Ls
	}{
		{specs: []string{"s->a:diamond", "a->b:diamond", "a->ea", "b->ok", "b->eb", "ok->done"}, straight: []string{"a->b#0", "b->ok#0"}},
		{specs: []string{"s->a:diamond", "a->b:diamond", "a->ea", "b->c:diamond", "b->eb", "c->ok", "c->ec", "ok->done"}, straight: []string{"a->b#0", "b->c#0", "c->ok#0"}},
		{specs: []string{"s->m:diamond", "m->main", "m->end", "z->end", "main->next"}, straight: []string{"m->main#0"}},
	} {
		for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
			g := graph(t, nil, tc.specs...)
			g.Direction = dir
			assertCascade(t, lay(t, g, screen(), dir), g, dir, tc.straight)
		}
	}
}

// TestLayout_PackingKeepsARoom pins S7's packing on a room: d's other
// branch, c, lands beyond d's side column, where o, in d's row but in
// another part of the level (x->o shares no edge with the rest), blocks
// the column's way out, so c gets room. Packing then moves the two parts
// toward the level's center; kept apart only by in-layer separations,
// they closed up over the room and d->c jogged from the column Reach out
// to its port, with the rounds already settled. The room is a separation
// between the parts too, so d's main line runs straight on and d->c is an
// L. Under RIGHT and LEFT o leaves the row clear when the rounds judge it,
// so c gets no room, and packing still closes up over the column the
// router moves out.
func TestLayout_PackingKeepsARoom(t *testing.T) {
	specs := []string{"s->d:diamond", "d->a", "a->b", "d->c", "x->o"}
	labels := map[string]string{"c": "Other branch", "o": "Other node"}
	for _, dir := range []model.Direction{model.Down, model.Up} {
		g := graph(t, labels, specs...)
		g.Direction = dir
		assertCascade(t, lay(t, g, screen(), dir), g, dir, []string{"d->a#0"})
	}
}

// TestLayout_ARoomGivesWayToAForcing pins S7's precedence on two
// decisions in one row that share their "no" target: n2's secondary into
// n4 gets room past n1, which stands in its way, but that room would put
// n4 right of n3, under n1, against the order n1's forced primary needs.
// The forcing wins and the room gives way: n2's secondary stays blocked,
// so n2 is denied its primary (and given it back, its denial clearing no
// row), and no forcing drops, in every direction. Kept over the forcing,
// the room would drop n1's primary and the layout take 12 bends; it takes
// 8, where denying both diamonds would take 7.
func TestLayout_ARoomGivesWayToAForcing(t *testing.T) {
	specs := []string{"n0->n4", "n1:diamond", "n2:diamond", "n1->n4", "n1->n3", "n2->n4", "n2->n6", "n3->n7", "n6->n8"}
	for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
		g := graph(t, nil, specs...)
		g.Direction = dir
		_, recs := layLogged(t, g, dir, false) // the screen contract holds too
		room, denial := -1, -1
		for i, rec := range recs {
			assert.NotEqual(t, "side_exit_dropped", rec["decision"], "%s: no forcing drops", dir)
			switch {
			case rec["decision"] == "secondary_room" && rec["node"] == "n2" && room < 0:
				room = i
			case rec["decision"] == "primary_denied" && rec["node"] == "n2" && denial < 0:
				denial = i
			}
		}
		require.GreaterOrEqual(t, room, 0, "%s: n2's secondary gets room", dir)
		assert.Greater(t, denial, room, "%s: the room gives way, and n2 is denied its primary after it", dir)
	}
}

// TestLayout_ADenialThatBuysNoLIsGivenBack pins the undo of S7's denial
// on the finished layout, where the placement test stops at synthetic
// widths: under DOWN and UP, z, as wide as its label makes it, also feeds
// endendee and blocks m's right column; m's secondary gets room, the room
// does not hold, and m is denied its primary; denied, m->endendee still
// cannot leave as an L, so m gets its primary back, and its main line runs
// straight on from its out-vertex into mainmainmainmain.
func TestLayout_ADenialThatBuysNoLIsGivenBack(t *testing.T) {
	for _, dir := range []model.Direction{model.Down, model.Up} {
		g := graph(t, nil, "s->m:diamond", "m->mainmainmainmain", "m->endendee", "z->endendee", "mainmainmainmain->next")
		pg, recs := layLogged(t, g, dir, false)
		var changes []string
		for _, rec := range recs {
			if d := rec["decision"]; d == "primary_denied" || d == "primary_restored" {
				changes = append(changes, fmt.Sprint(d, " ", rec["node"], " ", rec["edge"]))
			}
		}
		assert.Equal(t, []string{"primary_denied m m->endendee#0", "primary_restored m m->endendee#0"}, changes, "%s", dir)
		pts := edge(t, pg, "m->mainmainmainmain#0").Points
		require.Len(t, pts, 2, "%s: the main line runs straight on: %v", dir, pts)
		near(t, outVertex(positioned(t, pg, "m"), dir), pts[0], dir.String()+" m's main line from its out-vertex")
	}
}

// decisionFixture builds a flow graph of decisions from node specs ("id" or
// "id:diamond", labeled by id) and edges (from, to, label): a frozen copy of
// a random graph of the probe's, so the test needs no seed.
func decisionFixture(nodes []string, edges [][3]string) model.Graph {
	var g model.Graph
	for _, spec := range nodes {
		id, shape, _ := strings.Cut(spec, ":")
		n := model.Node{ID: id, Label: id, Shape: model.ShapeRect}
		if shape == "diamond" {
			n.Shape = model.ShapeDiamond
		}
		g.Nodes = append(g.Nodes, n)
	}
	for _, e := range edges {
		g.Edges = append(g.Edges, model.Edge{ID: e[0] + "->" + e[1] + "#0", From: e[0], To: e[1], Label: e[2]})
	}
	return g
}

// decisionSeed387 is the probe's decision graph of seed 387.
func decisionSeed387() model.Graph {
	return decisionFixture(
		[]string{"n0", "n1:diamond", "n2:diamond", "n3", "n4:diamond", "n5", "n6:diamond", "n7:diamond", "n8:diamond", "n9", "n10:diamond", "n11:diamond", "n12:diamond", "n13", "n14", "n15", "n16"},
		[][3]string{{"n0", "n8", ""}, {"n1", "n12", "yes"}, {"n1", "n6", "no"}, {"n2", "n6", "yes"}, {"n2", "n4", "no"}, {"n3", "n14", ""}, {"n4", "n16", "yes"}, {"n4", "n14", "no"}, {"n5", "n12", ""}, {"n6", "n14", "yes"}, {"n6", "n16", "no"}, {"n7", "n16", "yes"}, {"n7", "n15", "no"}, {"n8", "n13", "yes"}, {"n8", "n11", "no"}, {"n9", "n12", ""}, {"n10", "n15", "yes"}, {"n10", "n16", "no"}, {"n11", "n16", "yes"}, {"n11", "n15", "no"}, {"n12", "n16", "yes"}, {"n12", "n14", "no"}, {"n14", "n15", ""}, {"n15", "n16", ""}})
}

// decisionSeed27 is the probe's decision graph of seed 27.
func decisionSeed27() model.Graph {
	return decisionFixture(
		[]string{"n0", "n1", "n2:diamond", "n3:diamond", "n4", "n5", "n6:diamond", "n7", "n8", "n9"},
		[][3]string{{"n0", "n6", ""}, {"n1", "n9", ""}, {"n2", "n6", "yes"}, {"n2", "n9", "no"}, {"n3", "n6", "yes"}, {"n3", "n7", "no"}, {"n4", "n7", ""}, {"n5", "n9", ""}, {"n6", "n7", "yes"}, {"n6", "n8", "no"}, {"n7", "n9", ""}, {"n8", "n9", ""}})
}

// TestLayout_ARoomWaitsForItsPrimary pins S7's room on a diamond whose
// primary's forcing alignment dropped: the room's premise, the secondary's
// next vertex kept the node gap from the primary's under the diamond, does
// not hold, so the diamond gets no room and is denied its primary, which
// takes its primary out of the forced batch that could not be met. Given
// rooms anyway, d1 (and on the probe's decision seed 387 DOWN, n6 and n12)
// would keep a batch no placement meets, every forcing in it dropped, and
// seed 387 would take 59 bends where the denials take 50.
func TestLayout_ARoomWaitsForItsPrimary(t *testing.T) {
	small := graph(t, nil, "d1:diamond->t", "d1->s1", "d2:diamond->t", "d2->s1", "t->u")
	small.Direction = model.Down
	seed := decisionSeed387()
	seed.Direction = model.Down
	for name, g := range map[string]model.Graph{"two decisions": small, "decision seed 387": seed} {
		ctx, buf := layoutdbgtest.Capture()
		_, err := Layout(ctx, g, screen(), nil)
		require.NoError(t, err)
		// per pass (a placed record closes one): the diamonds whose primary's
		// forcing dropped, and the diamonds that got room
		dropped, roomed := map[string]bool{}, map[string]bool{}
		passes := 0
		for _, rec := range layoutdbgtest.Records(t, buf) {
			switch rec["decision"] {
			case "side_exit_dropped":
				if rec["primary"] == true {
					from, _, _ := strings.Cut(rec["edge"].(string), "->")
					dropped[from] = true
				}
			case "secondary_room":
				roomed[rec["node"].(string)] = true
			case "placed":
				passes++
				for d := range roomed {
					assert.False(t, dropped[d], "%s: %s got room while its primary's forcing dropped", name, d)
				}
				dropped, roomed = map[string]bool{}, map[string]bool{}
			}
		}
		require.Positive(t, passes, "%s: a placed record per pass", name)
	}
}

// TestLayout_AColumnGivesWayOnlyWhereTheRowIsClear pins the scope of S8's
// float-noise allowance: it lets a column move out over the port it aims
// at, where a room keeps a node exactly at the node clearance, and nowhere
// else. On the probe's decision seed 27 DOWN, n0 sits at the node gap from
// n3, which puts the lane n3->n6's column would give way to exactly on
// n0's clearance (40 - 12 = Reach + InLaneGap); taken, as the allowance
// let it, the wire jogged 8 px back to its port. It stays an L.
func TestLayout_AColumnGivesWayOnlyWhereTheRowIsClear(t *testing.T) {
	g := decisionSeed27()
	g.Direction = model.Down
	pg := lay(t, g, screen(), model.Down)
	assert.Len(t, edge(t, pg, "n3->n6#0").Points, 3, "n3->n6 is an L")
}

// assertCascade checks pg, laid out from g in dir, for TestLayout_ACascades*:
// every edge into a diamond runs straight, every exit of a diamond in
// straight runs straight on from its flow-axis vertex, and every other
// exit of a diamond leaves a side vertex as an L.
func assertCascade(t *testing.T, pg *model.PositionedGraph, g model.Graph, dir model.Direction, straight []string) {
	t.Helper()
	for _, e := range g.Edges {
		pts := edge(t, pg, e.ID).Points
		if positioned(t, pg, e.To).Shape == model.ShapeDiamond {
			assert.Len(t, pts, 2, "%s %s runs straight into its diamond: %v", dir, e.ID, pts)
		}
		n := positioned(t, pg, e.From)
		if n.Shape != model.ShapeDiamond {
			continue
		}
		if slices.Contains(straight, e.ID) {
			require.Len(t, pts, 2, "%s %s runs straight on: %v", dir, e.ID, pts)
			near(t, outVertex(n, dir), pts[0], dir.String()+" "+e.ID+" from the flow-axis vertex")
			continue
		}
		require.Len(t, pts, 3, "%s %s is an L: %v", dir, e.ID, pts)
		assert.True(t, sideVertex(n, pts[0], dir), "%s %s leaves a side vertex: %v", dir, e.ID, pts)
	}
}

// TestLayout_ACascadesMainLineHoldsWhateverItsLabels pins S7's room on a
// cascade where z, the other source into end, sits in m's row: with every
// subset of the five edges labelled, in every direction, m keeps its
// primary straight on into main, s->m runs straight into m, and m->end is
// an L.
func TestLayout_ACascadesMainLineHoldsWhateverItsLabels(t *testing.T) {
	specs := []string{"s->m:diamond", "m->main", "m->end", "z->end", "main->next"}
	for mask := range 32 {
		for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
			g := graph(t, nil, specs...)
			g.Direction = dir
			for i := range g.Edges {
				if mask&(1<<i) != 0 {
					g.Edges[i].Label = "label"
				}
			}
			t.Run(fmt.Sprintf("%05b/%s", mask, dir), func(t *testing.T) {
				assertCascade(t, lay(t, g, screen(), dir), g, dir, []string{"m->main#0"})
			})
		}
	}
}

// TestLayout_FlowLoopRunsItsMainLineStraight pins flow-loop's decision in
// every direction: "true" runs straight from its flow-axis vertex into
// Process, the loop's main line, and "false" leaves a side vertex as an L
// into End. Its back edge from i++ holds the other side vertex, so no
// forcing applies (S7): the exit rule gives the exit heading farthest
// toward the free side, End's, that side and Process the bottom (S8).
func TestLayout_FlowLoopRunsItsMainLineStraight(t *testing.T) {
	for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
		t.Run(dir.String(), func(t *testing.T) {
			pg := lay(t, flowLoop(dir), screen(), dir)
			check := positioned(t, pg, "check")
			body := edge(t, pg, "check->process#0").Points
			require.Len(t, body, 2, "true runs straight: %v", body)
			near(t, outVertex(check, dir), body[0], "from the flow-axis vertex")
			end := edge(t, pg, "check->end#0").Points
			require.Len(t, end, 3, "false runs as an L: %v", end)
			assert.True(t, sideVertex(check, end[0], dir), "from a side vertex: %v", end)
		})
	}
}

// interleavedParts is the random-graph probe's graph of seed 236 without
// groups (-probe.flat): a part n2, n4, n5 and n1, and two isolated
// diamonds and parallelograms, n0 and n3, n3 on the sources' row between
// n2 and n4, so that no one left-to-right order of the level's three parts
// agrees with every layer.
var interleavedParts = []string{"n0:parallelogram", "n1:cylinder", "n2:diamond", "n3:diamond", "n4:parallelogram", "n5",
	"n4->n1", "n2->n1", "n4->n5", "n5->n4"}

// TestLayout_InterleavedPartsAreNotPacked pins S7's *Packing* on the
// finished layout, where the placement test stops at synthetic levels: a
// level whose parts interleave is left as act 2 placed it, recorded as
// parts_packed with interleaved set and nothing moved, n3 still between n2
// and n4 across the flow. On screen under DOWN and RIGHT, and in text
// under RIGHT (under DOWN the text layout's order does not interleave);
// the layouts meet the whole contract.
func TestLayout_InterleavedPartsAreNotPacked(t *testing.T) {
	for _, tc := range []struct {
		dir  model.Direction
		text bool
	}{{model.Down, false}, {model.Right, false}, {model.Right, true}} {
		dir, text := tc.dir, tc.text
		pg, recs := layLogged(t, graph(t, nil, interleavedParts...), dir, text)
		packed := 0
		for _, rec := range recs {
			if rec["decision"] == "parts_packed" {
				packed++
				assert.Equal(t, true, rec["interleaved"], "%s text=%v", dir, text)
				assert.InDelta(t, 0, rec["moved"], 0, "%s text=%v", dir, text)
			}
		}
		assert.Positive(t, packed, "%s text=%v: the level has parts", dir, text)
		_, n2 := flowAxis(center(positioned(t, pg, "n2")), dir)
		_, n3 := flowAxis(center(positioned(t, pg, "n3")), dir)
		_, n4 := flowAxis(center(positioned(t, pg, "n4")), dir)
		assert.True(t, (n2 < n3 && n3 < n4) || (n4 < n3 && n3 < n2), "%s text=%v: n3 between n2 and n4: %v %v %v", dir, text, n2, n3, n4)
	}
}

// TestLayout_WiresWithinTheSpanRunStraight pins S8's stops on the finished
// layout, in every direction and both profiles: a port slides along its
// face to straighten a wire placement leaves within Span. Two parallel
// edges leave a wide node's face at a wider pitch than they enter a narrow
// one's; two wires into a cylinder in a group cross its border at
// terminals a lane gap apart, wider than the ports on the cylinder's side
// under RIGHT and LEFT. None leaves a micro-jog (Q2): an interior segment
// under 12 px.
func TestLayout_WiresWithinTheSpanRunStraight(t *testing.T) {
	for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
		for _, text := range []bool{false, true} {
			cfg, lim := screen(), contract.ScreenLimits()
			if text {
				cfg, lim = TextConfig(dir), contract.TextLimits(dir)
			}
			layout := func(g model.Graph) *model.PositionedGraph {
				g.Direction = dir
				pg, err := Layout(context.Background(), g, cfg, nil)
				require.NoError(t, err)
				for _, v := range contract.Check(pg, contract.Options{Limits: lim, Direction: dir, EdgeIDs: ids(g), Text: text}) {
					t.Errorf("%s text=%v: %s %s: %s", dir, text, v.Rule, v.Subject, v.Detail)
				}
				return pg
			}
			pg := layout(graph(t, map[string]string{"client": "Client", "api": "API"}, "client->api", "client->api"))
			for _, e := range pg.Edges {
				assert.Len(t, e.Points, 2, "%s text=%v: %s straight: %v", dir, text, e.ID, e.Points)
			}
			g := graph(t, map[string]string{"p": "Orders", "q": "Catalog", "db": "PostgreSQL"}, "p->db:cylinder", "q->db")
			pg = layout(grouped(g, model.Group{ID: "data", Label: "Data", Contains: []string{"db"}}))
			for _, e := range pg.Edges {
				for i := 1; i+2 < len(e.Points); i++ {
					p, q := e.Points[i], e.Points[i+1]
					assert.GreaterOrEqual(t, math.Hypot(q.X-p.X, q.Y-p.Y), 12.0, "%s text=%v: %s segment %d: %v", dir, text, e.ID, i, e.Points)
				}
			}
		}
	}
}

// xAt returns the x of route's first vertical segment whose extent holds
// y: where a wire of a DOWN layout crosses that height.
func xAt(t *testing.T, route []model.Point, y float64) float64 {
	t.Helper()
	for i := 0; i+1 < len(route); i++ {
		p, q := route[i], route[i+1]
		if p.X == q.X && min(p.Y, q.Y) <= y && y <= max(p.Y, q.Y) {
			return p.X
		}
	}
	require.Failf(t, "no vertical run", "at y %v: %v", y, route)
	return math.NaN()
}

// TestLayout_FlowNetworksSpineRunsMidCanvas pins S5's Corridor seats on
// flow-network's final layout, DOWN: api -> db's column starts between
// Redis Cache and Message Queue, so S7's straight chain lines Web, API and
// Database up in the middle of the canvas instead of at its right edge.
// Without the rule the column would come last in the middle row, x 611 on
// a 681 px canvas, API 214.9 px off the midpoint of its fan (its four
// wires where they reach the middle row) and Database 144.2 px off the
// midpoint of its parents' wires. With it, 29.7 and 31.5 px: API's four
// ports spread over its face put the query wire's port 14 px right of its
// center, and Message Queue, a 218 px parallelogram, keeps its port 155 px
// right of the column, Redis Cache's 92 px left.
func TestLayout_FlowNetworksSpineRunsMidCanvas(t *testing.T) {
	pg := lay(t, flowNetwork(), screen(), model.Down)
	web, api, db := positioned(t, pg, "web"), positioned(t, pg, "api"), positioned(t, pg, "db")
	auth, cache, queue := positioned(t, pg, "auth"), positioned(t, pg, "cache"), positioned(t, pg, "queue")
	query := edge(t, pg, "api->db#0").Points
	require.Len(t, query, 2, "the query wire runs straight: %v", query)
	column := query[0].X
	assert.InDelta(t, web.X, api.X, 1e-6, "Web straight above API")
	assert.InDelta(t, column, db.X, 1e-6, "Database straight under the query wire")
	assert.Less(t, cache.X, column, "the column right of Redis Cache")
	assert.Less(t, column, queue.X, "and left of Message Queue")
	assert.Greater(t, column, pg.Width/3, "in the middle third of the canvas")
	assert.Less(t, column, 2*pg.Width/3, "in the middle third of the canvas")

	mid := func(xs ...float64) float64 { return (slices.Min(xs) + slices.Max(xs)) / 2 }
	row := auth.Y - auth.Height/2 // the middle row's leading edge, which its nodes share (S4)
	var fan []float64
	for _, id := range []string{"api->auth#0", "api->cache#0", "api->queue#0", "api->db#0"} {
		fan = append(fan, xAt(t, edge(t, pg, id).Points, row))
	}
	// Measured 33.0 and 35.2 px since a parallelogram's ports take the
	// stretch its top and bottom edges share (S8, Shape ports).
	assert.LessOrEqual(t, math.Abs(api.X-mid(fan...)), 34.0, "API over its fan: %v", fan)
	parents := mid(edge(t, pg, "cache->db#0").Points[0].X, edge(t, pg, "queue->db#0").Points[0].X, column)
	assert.LessOrEqual(t, math.Abs(db.X-parents), 36.0, "Database under its parents")
}

// ecommerceCheckout is the ecommerce-checkout spec: Clients
// feed an API Gateway, which fans out into the Services group, whose
// nodes reach PostgreSQL and Redis in the Data group and the Payment
// Provider.
func ecommerceCheckout(dir model.Direction) model.Graph {
	return model.Graph{
		Direction: dir,
		Nodes: []model.Node{
			{ID: "web", Label: "Web Storefront", Shape: model.ShapeRounded}, {ID: "mobile", Label: "Mobile App", Shape: model.ShapeRounded},
			{ID: "gateway", Label: "API Gateway", Shape: model.ShapeHexagon},
			{ID: "catalog", Label: "Catalog Service", Shape: model.ShapeRect}, {ID: "cart", Label: "Cart Service", Shape: model.ShapeRect},
			{ID: "orders", Label: "Order Service", Shape: model.ShapeRect}, {ID: "payments", Label: "Payment Service", Shape: model.ShapeRect},
			{ID: "provider", Label: "Payment Provider", Shape: model.ShapeRounded},
			{ID: "postgres", Label: "PostgreSQL", Shape: model.ShapeCylinder}, {ID: "redis", Label: "Redis", Shape: model.ShapeCylinder},
		},
		Edges: []model.Edge{
			{ID: "web->gateway#0", From: "web", To: "gateway", Label: "HTTPS"},
			{ID: "mobile->gateway#0", From: "mobile", To: "gateway", Label: "HTTPS"},
			{ID: "gateway->catalog#0", From: "gateway", To: "catalog"},
			{ID: "gateway->cart#0", From: "gateway", To: "cart"},
			{ID: "gateway->orders#0", From: "gateway", To: "orders"},
			{ID: "orders->payments#0", From: "orders", To: "payments", Label: "charge"},
			{ID: "payments->provider#0", From: "payments", To: "provider", Label: "API"},
			{ID: "catalog->postgres#0", From: "catalog", To: "postgres"},
			{ID: "orders->postgres#0", From: "orders", To: "postgres"},
			{ID: "cart->redis#0", From: "cart", To: "redis"},
		},
		Groups: []model.Group{
			{ID: "clients", Label: "Clients", Contains: []string{"web", "mobile"}},
			{ID: "services", Label: "Services", Contains: []string{"catalog", "cart", "orders", "payments"}},
			{ID: "data", Label: "Data", Contains: []string{"postgres", "redis"}},
		},
	}
}

// TestLayout_EcommerceServicesPackTheirParts pins S7's *Packing* on the
// final layout of ecommerce-checkout, DOWN: in the Services level, Order
// Service's part (its entry, Payment Service, the Payment Provider's and
// PostgreSQL's wires) shares no edge with Catalog Service's or Cart
// Service's, and act 1's reference alone drifts it 60.6 px away from
// Catalog Service, a 780.3 px box. Packed, the box is at most 720 px wide,
// and the row keeps its order.
func TestLayout_EcommerceServicesPackTheirParts(t *testing.T) {
	pg := lay(t, ecommerceCheckout(model.Down), screen(), model.Down)
	services := group(t, pg, "services")
	assert.LessOrEqual(t, services.Width, 720.0, "the Services box")
	orders, catalog, cart := positioned(t, pg, "orders"), positioned(t, pg, "catalog"), positioned(t, pg, "cart")
	payments := positioned(t, pg, "payments")
	assert.Less(t, orders.X, catalog.X, "Order Service left of Catalog Service")
	assert.Less(t, catalog.X, cart.X, "Catalog Service left of Cart Service")
	assert.Less(t, payments.X, orders.X, "Payment Service below and left of Order Service")
	assert.Greater(t, payments.Y, orders.Y, "Payment Service below Order Service")
	gap := (catalog.X - catalog.Width/2) - (orders.X + orders.Width/2)
	assert.InDelta(t, screen().NodeGap, gap, 1e-6, "Order Service and Catalog Service a node gap apart")
}

// Edge kinds of denseLabelled's graphs.
const (
	denseBipartite = iota // from the first half of the nodes to the second
	denseForward          // from each node to one of the six after it
	denseCyclic           // from any node to any other
)

// denseLabelled builds a random flow graph from seed: n nodes, m edges
// of the kind edges between random pairs, parallel ones included, every
// edge labelled, laid out RIGHT, where a label's length lies along the
// flow. It is S10's hardest case: most labels find no clear spot and score
// their whole ladder. A cyclic graph has edges that S2 reverses, so S6's
// loop transposition runs. pinned makes every fourth node a diamond and
// every seventh a circle (a diamond first), so S7 forces side exits and,
// on a forward graph, whose diamonds' exits spread over the layers below,
// the recheck forces more after the rerun.
func denseLabelled(seed int64, n, m, edges int, pinned bool) model.Graph {
	rng := rand.New(rand.NewSource(seed))
	words := []string{"read", "write", "query", "HTTPS", "result", "publish", "subscribe", "sync state", "ack", "retry later"}
	g := model.Graph{Direction: model.Right}
	for i := range n {
		id, shape := fmt.Sprintf("n%d", i), model.ShapeRect
		switch {
		case pinned && i%4 == 3:
			shape = model.ShapeDiamond
		case pinned && i%7 == 6:
			shape = model.ShapeCircle
		}
		g.Nodes = append(g.Nodes, model.Node{ID: id, Label: id, Shape: shape})
	}
	count := map[string]int{}
	for range m {
		var from, to int
		switch edges {
		case denseBipartite:
			from, to = rng.Intn(n/2), n/2+rng.Intn(n-n/2)
		case denseForward:
			from = rng.Intn(n - 1)
			to = from + 1 + rng.Intn(min(6, n-1-from))
		case denseCyclic:
			from, to = rng.Intn(n), rng.Intn(n-1)
			if to >= from {
				to++
			}
		}
		a, b := fmt.Sprintf("n%d", from), fmt.Sprintf("n%d", to)
		key := a + "->" + b
		g.Edges = append(g.Edges, model.Edge{ID: fmt.Sprintf("%s#%d", key, count[key]), From: a, To: b, Label: words[rng.Intn(len(words))]})
		count[key]++
	}
	return g
}

// cascades builds k cascades of l decisions side by side below one start
// node, laid out DOWN: each decision's "yes" leads on to the next, its "no"
// to a node that leads nowhere, and the last decision's "yes" to an end
// that leads on. Each decision's "no" branch lands in the row of the
// decision below it and blocks its secondary's column, so S7 gives the
// lower decisions' secondaries room (TestLayout_ACascadesSecondariesRouteAsLs),
// many of them in one round.
func cascades(k, l int) model.Graph {
	g := model.Graph{Direction: model.Down, Nodes: []model.Node{{ID: "s", Label: "Start", Shape: model.ShapeRounded}}}
	link := func(from, to, label string) {
		g.Edges = append(g.Edges, model.Edge{ID: from + "->" + to + "#0", From: from, To: to, Label: label})
	}
	for c := range k {
		prev, yes := "s", ""
		for i := range l {
			d, x := fmt.Sprintf("d%d_%d", c, i), fmt.Sprintf("x%d_%d", c, i)
			g.Nodes = append(g.Nodes,
				model.Node{ID: d, Label: fmt.Sprintf("Check %d.%d?", c, i), Shape: model.ShapeDiamond},
				model.Node{ID: x, Label: fmt.Sprintf("Fail %d.%d", c, i)})
			link(prev, d, yes)
			link(d, x, "no")
			prev, yes = d, "yes"
		}
		ok, done := fmt.Sprintf("ok%d", c), fmt.Sprintf("done%d", c)
		g.Nodes = append(g.Nodes, model.Node{ID: ok, Label: fmt.Sprintf("OK %d", c)}, model.Node{ID: done, Label: fmt.Sprintf("Done %d", c)})
		link(prev, ok, yes)
		link(ok, done, "")
	}
	return g
}

// TestLayout_CascadesSideBySideGetRoomTogether pins S7's batched rooms on
// the benchmark's cascade graph, smaller: every cascade's lower decisions
// keep their primaries, their secondaries get room, and nothing restarts
// the rounds. One restart per diamond made S7's cost grow with the number
// of diamonds: the benchmark's 30 cascades of 8 took 1.09 s with one
// restart per denial, 35 ms with every round's denials in one restart,
// and take 52 ms with rooms instead.
func TestLayout_CascadesSideBySideGetRoomTogether(t *testing.T) {
	g := cascades(4, 4)
	ctx, buf := layoutdbgtest.Capture()
	pg, err := Layout(ctx, g, screen(), nil)
	require.NoError(t, err)
	rooms, denied, restarts := 0, 0, 0.0
	for _, rec := range layoutdbgtest.Records(t, buf) {
		switch rec["decision"] {
		case "secondary_room":
			rooms++
		case "primary_denied":
			denied++
		case "placed":
			restarts += rec["restarts"].(float64)
		}
	}
	assert.Equal(t, 2*4*3, rooms, "every cascade's lower decisions get room, in each pass")
	assert.Zero(t, denied, "no decision is denied its primary")
	assert.Zero(t, restarts, "rooms take no restart")
	for _, e := range g.Edges {
		if strings.HasPrefix(e.From, "d") && e.Label == "yes" {
			assert.Len(t, edge(t, pg, e.ID).Points, 2, "%s runs straight on", e.ID)
		}
	}
}

// BenchmarkLayout_DenseLabelledGraph measures the layout's time on dense
// labelled graphs, up to 50 nodes and 300 edges. The bipartite graphs of
// rectangles run neither S6's loop transposition nor S7's side-exit
// forcing: the pinned case, forward edges between diamonds, circles and
// rectangles, runs S7's forcing and its recheck, and the cyclic case S6's
// loop transposition. The cascade case, 30 cascades of 8 decisions side by
// side, reaches S7's rooms for blocked secondaries, which take one more
// round and no restart. It is not a gate.
//
//	go test ./internal/layout/layered/ -run '^$' -bench DenseLabelled -benchtime 1x
func BenchmarkLayout_DenseLabelledGraph(b *testing.B) {
	for _, tc := range []struct {
		n, m, edges  int
		text, pinned bool
		cascades     int // cascades of 8 decisions side by side, instead of a random graph
	}{
		{n: 30, m: 100}, {n: 40, m: 200}, {n: 50, m: 300}, {n: 40, m: 200, text: true},
		{n: 40, m: 200, edges: denseForward, pinned: true}, {n: 40, m: 200, edges: denseCyclic},
		{cascades: 30},
	} {
		g := denseLabelled(1, tc.n, tc.m, tc.edges, tc.pinned)
		cfg, name := screen(), fmt.Sprintf("%d_nodes_%d_edges", tc.n, tc.m)
		if tc.cascades > 0 {
			g, name = cascades(tc.cascades, 8), fmt.Sprintf("%d_cascades_of_8", tc.cascades)
		}
		if tc.text {
			cfg, name = TextConfig(g.Direction), name+"_text"
		}
		if tc.pinned {
			name += "_pinned"
		}
		if tc.edges == denseCyclic {
			name += "_cyclic"
		}
		b.Run(name, func(b *testing.B) {
			for b.Loop() {
				_, err := Layout(context.Background(), g, cfg, nil)
				require.NoError(b, err)
			}
		})
	}
}
