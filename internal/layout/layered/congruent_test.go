package layered

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/layout/contract"
	"github.com/oxforge/diago/internal/layout/layered/lgraph"
	"github.com/oxforge/diago/internal/layout/layered/lgraph/lgraphtest"
	"github.com/oxforge/diago/internal/layout/layered/size"
	"github.com/oxforge/diago/internal/layoutdbg"
	"github.com/oxforge/diago/internal/model"
)

// datacenters is the flow-datacenter-align example pared down: a load balancer feeds two data centers, each of a PCI and a
// non-PCI cluster (a control plane managing an ingress and the workers
// behind it) and a database cluster. The two data centers are congruent
// (S12), and inside each so are its two clusters.
func datacenters(t *testing.T) model.Graph {
	t.Helper()
	var specs []string
	labels := map[string]string{"lb": "Load Balancer"}
	var groups []model.Group
	for _, dc := range []string{"d1", "d2"} {
		for _, cl := range []string{"p", "q"} {
			c := dc + cl
			specs = append(specs, "lb:hexagon->"+c+"_ing:hexagon", c+"_cp->"+c+"_ing", c+"_cp->"+c+"_wrk", c+"_ing->"+c+"_wrk", c+"_wrk->"+dc+"_db:cylinder")
			labels[c+"_cp"], labels[c+"_ing"], labels[c+"_wrk"] = "Control Plane", "Ingress", "Workers"
			groups = append(groups, model.Group{ID: c, Label: "Cluster", Contains: []string{c + "_cp", c + "_ing", c + "_wrk"}})
		}
		labels[dc+"_db"] = "Primary"
		groups = append(groups, model.Group{ID: dc, Label: "Data Center", Contains: []string{dc + "_db"}, Children: []string{dc + "p", dc + "q"}})
	}
	g := grouped(graph(t, labels, specs...), groups...)
	for i := range g.Edges {
		if strings.HasPrefix(g.Edges[i].From, "lb") {
			g.Edges[i].Label = "traffic"
		}
	}
	return g
}

// twoDCs is a load balancer feeding two congruent data centers, a and b,
// each an ingress, an app server and a database in a chain.
func twoDCs(t *testing.T) model.Graph {
	t.Helper()
	labels := map[string]string{"a_in": "Ingress", "a_app": "App Server", "a_db": "Database", "b_in": "Ingress", "b_app": "App Server", "b_db": "Database"}
	g := graph(t, labels, "lb->a_in", "a_in->a_app", "a_app->a_db:cylinder", "lb->b_in", "b_in->b_app", "b_app->b_db:cylinder")
	return grouped(g,
		model.Group{ID: "a", Label: "A", Contains: []string{"a_in", "a_app", "a_db"}},
		model.Group{ID: "b", Label: "B", Contains: []string{"b_in", "b_app", "b_db"}})
}

// nestingOf builds g's levels and finds its congruences (S9, S12) under
// cfg, top to bottom.
func nestingOf(t *testing.T, ctx context.Context, g model.Graph, cfg Config) *nesting {
	t.Helper()
	n, err := arrangeNested(ctx, g, size.Measure(ctx, g.Nodes, cfg.Size), cfg, model.Down, nil)
	require.NoError(t, err)
	return n
}

func TestCongruences_FindEverySetFromTheRootDown(t *testing.T) {
	n := nestingOf(t, context.Background(), datacenters(t), screen())
	type set struct {
		source, parent, rep string
		members             []string
	}
	var got []set
	for _, cg := range n.cgs {
		s := set{source: n.g.Nodes[cg.source].ID, parent: n.name(cg.parent), rep: n.name(cg.rep)}
		for _, k := range cg.members {
			s.members = append(s.members, n.name(k))
		}
		got = append(got, s)
	}
	assert.Equal(t, []set{
		{"lb", "", "d1", []string{"d2"}},
		{"lb", "d1", "d1p", []string{"d1q"}},
		{"lb", "d2", "d2p", []string{"d2q"}},
	}, got)
	from := map[string]string{}
	for c, cp := range n.copies {
		from[n.name(c)] = n.name(cp.from)
	}
	assert.Equal(t, map[string]string{"d2": "d1", "d2p": "d1p", "d2q": "d1q", "d1q": "d1p"}, from,
		"a member's subtree copies the representative's, the outer set first")
}

func TestCongruences_AMemberThatDoesNotCorrespondDegradesTheSet(t *testing.T) {
	cases := []struct {
		name, reason string
		change       func(g *model.Graph)
	}{
		{"a node fewer", "node_count", func(g *model.Graph) {
			g.Nodes = g.Nodes[:len(g.Nodes)-1]
			g.Edges = g.Edges[:len(g.Edges)-1]
			g.Groups[1].Contains = []string{"b_in", "b_app"}
		}},
		{"another shape", "node_shape", func(g *model.Graph) { g.Nodes[6].Shape = model.ShapeRect }},
		{"another size", "node_size", func(g *model.Graph) { g.Nodes[5].Label = "App Server Two" }},
		{"another edge", "edges", func(g *model.Graph) { g.Edges[5].From = "b_in" }},
		{"a wire leaving on the other row", "edges", func(g *model.Graph) {
			// a's database feeds a sink below, b's feeds the load balancer
			// back: one wire leaves on the bottom row, the other on the top
			g.Nodes = append(g.Nodes, model.Node{ID: "sink", Label: "sink", Shape: model.ShapeRect})
			g.Edges = append(g.Edges,
				model.Edge{ID: "a_db->sink#0", From: "a_db", To: "sink"},
				model.Edge{ID: "b_db->lb#0", From: "b_db", To: "lb"})
		}},
		{"a label on one side", "edge_labels", func(g *model.Graph) { g.Edges[4].Label = "http" }},
		{"a cardinality on the member's side only", "edge_labels", func(g *model.Graph) {
			// the representative's edge would place no cardinality for its
			// copy to take (S10)
			g.Edges[4].ToCard = "*"
		}},
		{"a title on one side", "title", func(g *model.Graph) { g.Groups[1].Label = "" }},
		{"a group inside", "group_count", func(g *model.Graph) {
			g.Groups[1].Contains = []string{"b_in", "b_app"}
			g.Groups[1].Children = []string{"inner"}
			g.Groups = append(g.Groups, model.Group{ID: "inner", Contains: []string{"b_db"}})
		}},
		{"groups inside nested otherwise", "group_nesting", func(g *model.Graph) {
			// a holds two groups side by side, b as many, one inside the other
			g.Groups[0].Contains, g.Groups[0].Children = []string{"a_in"}, []string{"x1", "x2"}
			g.Groups[1].Contains, g.Groups[1].Children = []string{"b_in"}, []string{"y1"}
			g.Groups = append(g.Groups,
				model.Group{ID: "x1", Contains: []string{"a_app"}},
				model.Group{ID: "x2", Contains: []string{"a_db"}},
				model.Group{ID: "y1", Contains: []string{"b_app"}, Children: []string{"y2"}},
				model.Group{ID: "y2", Contains: []string{"b_db"}})
		}},
		{"a node in a group that does not correspond", "node_nesting", func(g *model.Graph) {
			// each holds one group, around its app server in a and around
			// its database in b
			g.Groups[0].Contains, g.Groups[0].Children = []string{"a_in", "a_db"}, []string{"x1"}
			g.Groups[1].Contains, g.Groups[1].Children = []string{"b_in", "b_app"}, []string{"y1"}
			g.Groups = append(g.Groups,
				model.Group{ID: "x1", Contains: []string{"a_app"}},
				model.Group{ID: "y1", Contains: []string{"b_db"}})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := twoDCs(t)
			tc.change(&g)
			var buf bytes.Buffer
			ctx := layoutdbg.NewContext(context.Background(), slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
			pg, err := Layout(ctx, g, screen(), nil)
			require.NoError(t, err)
			assert.Empty(t, pg.AppliedCongruences)
			assert.Equal(t, []string{"congruence_degraded " + tc.reason}, decisions(t, &buf, "congruence"))
		})
	}
}

// copiedApart is two congruent groups, r and m, fed by lb, each holding
// two groups whose three nodes form a V. Inside m, lb feeds both inner
// groups, a set of its own; inside r, different sources feed them, no
// set, so each is laid out on its own. m's inner groups copy r's, each
// its own counterpart, and need not come out alike (S12).
func copiedApart(t *testing.T) model.Graph {
	t.Helper()
	labels := map[string]string{}
	for _, id := range []string{"p1", "q1", "t1", "p2", "q2", "t2", "r1", "s1", "t3", "r2", "s2", "t4"} {
		labels[id] = "Node"
	}
	g := graph(t, labels,
		"lb->p1", "u->q1", "p1->t1", "q1->t1", "z->p2", "v->q2", "p2->t2", "q2->t2",
		"lb->r1", "y->s1", "r1->t3", "s1->t3", "lb->r2", "w->s2", "r2->t4", "s2->t4")
	return grouped(g,
		model.Group{ID: "r", Children: []string{"x1", "x2"}},
		model.Group{ID: "x1", Contains: []string{"p1", "q1", "t1"}},
		model.Group{ID: "x2", Contains: []string{"p2", "q2", "t2"}},
		model.Group{ID: "m", Children: []string{"y1", "y2"}},
		model.Group{ID: "y1", Contains: []string{"r1", "s1", "t3"}},
		model.Group{ID: "y2", Contains: []string{"r2", "s2", "t4"}})
}

func TestCongruences_ASetWhoseGroupsCopyApartDegrades(t *testing.T) {
	g := copiedApart(t)
	for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
		for _, text := range []bool{false, true} {
			g.Direction = dir
			cfg, lim := screen(), contract.ScreenLimits()
			if text {
				cfg, lim = TextConfig(dir), contract.TextLimits(dir)
			}
			var buf bytes.Buffer
			ctx := layoutdbg.NewContext(context.Background(), slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
			pg, err := Layout(ctx, g, cfg, nil)
			require.NoError(t, err)
			for _, v := range contract.Check(pg, contract.Options{Limits: lim, Direction: dir, EdgeIDs: ids(g), Text: text}) {
				if strings.HasPrefix(v.Rule, "C17") {
					t.Errorf("%s text=%v: %s %s: %s", dir, text, v.Rule, v.Subject, v.Detail)
				}
			}
			require.Len(t, pg.AppliedCongruences, 1, "%s text=%v: only r and m", dir, text)
			assert.Len(t, pg.AppliedCongruences[0].RepEdges, 4, "%s text=%v", dir, text)
			assert.Contains(t, decisions(t, &buf, "congruence"), "congruence_degraded copied_apart", "%s text=%v", dir, text)
		}
	}
}

// decisions lists the decision records of phase in buf, each as its
// decision and its reason when it has one.
func decisions(t *testing.T, buf *bytes.Buffer, phase string) []string {
	t.Helper()
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var rec struct{ Decision, Phase, Reason string }
		require.NoError(t, json.Unmarshal([]byte(line), &rec))
		if rec.Phase == phase {
			out = append(out, strings.TrimSpace(rec.Decision+" "+rec.Reason))
		}
	}
	return out
}

func TestCongruences_EdgesPairByTheirEndsWhateverTheirOrder(t *testing.T) {
	g := twoDCs(t)
	g.Edges[4], g.Edges[5] = g.Edges[5], g.Edges[4] // b's chain declared bottom up
	pg := lay(t, g, screen(), model.Down)
	require.Len(t, pg.AppliedCongruences, 1)
	assert.Equal(t, []int{1, 2}, pg.AppliedCongruences[0].RepEdges)
	assert.Equal(t, []int{5, 4}, pg.AppliedCongruences[0].Members[0].Edges)
}

// cyclesInside is lb feeding two congruent groups, o1 and o2, each a node
// s feeding two groups of one node, a and b, and a node beside each, x
// and y, that the node inside feeds and that feeds it back. o2 declares
// s's edges into a and x the other way round when turned is true, so its
// own search (S2) would reverse a2->x2, where o1's reverses x1->a1.
func cyclesInside(t *testing.T, turned bool) model.Graph {
	t.Helper()
	specs := []string{"lb", "s1", "a1", "x1", "b1", "y1", "s2", "a2", "x2", "b2", "y2", "lb->s1", "lb->s2"}
	for _, i := range []string{"1", "2"} {
		into := []string{"s" + i + "->a" + i, "s" + i + "->x" + i}
		if turned && i == "2" {
			into[0], into[1] = into[1], into[0]
		}
		specs = append(specs, into...)
		specs = append(specs, "a"+i+"->x"+i, "x"+i+"->a"+i, "s"+i+"->b"+i, "s"+i+"->y"+i, "b"+i+"->y"+i, "y"+i+"->b"+i)
	}
	labels := map[string]string{}
	for _, id := range []string{"s", "a", "x", "b", "y"} {
		labels[id+"1"], labels[id+"2"] = id, id
	}
	return grouped(graph(t, labels, specs...),
		model.Group{ID: "o1", Label: "O", Contains: []string{"s1", "x1", "y1"}, Children: []string{"a1g", "b1g"}},
		model.Group{ID: "a1g", Label: "A", Contains: []string{"a1"}},
		model.Group{ID: "b1g", Label: "A", Contains: []string{"b1"}},
		model.Group{ID: "o2", Label: "O", Contains: []string{"s2", "x2", "y2"}, Children: []string{"a2g", "b2g"}},
		model.Group{ID: "a2g", Label: "A", Contains: []string{"a2"}},
		model.Group{ID: "b2g", Label: "A", Contains: []string{"b2"}})
}

func TestCongruences_ACopyingLevelTakesItsOriginsReversals(t *testing.T) {
	// o2 copies o1 (S12, Rule A), and with it o1's reversals: the rows of
	// the wires into a2g and b2g are read off the copy, so they correspond
	// as a1g and b1g do, whatever o2's own search (turned) or a carrier's
	// flips (S13) would reverse.
	fresh := lay(t, cyclesInside(t, false), screen(), model.Down)
	require.Len(t, fresh.AppliedCongruences, 3)
	require.Equal(t, []string{"x1->a1#0", "x2->a2#0", "y1->b1#0", "y2->b2#0"}, fresh.LayoutHints.Reversed)
	var buf bytes.Buffer
	ctx := layoutdbg.NewContext(context.Background(), slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	turned, err := Layout(ctx, cyclesInside(t, true), screen(), nil)
	require.NoError(t, err)
	assert.Len(t, turned.AppliedCongruences, 3, "turned")
	assert.NotContains(t, decisions(t, &buf, "congruence"), "congruence_degraded edges")
	assert.Equal(t, fresh.LayoutHints.Reversed, turned.LayoutHints.Reversed, "turned")

	// A carrier that reversed a2->x2 flips it in o2's own search.
	prev := *fresh.LayoutHints
	prev.Reversed = []string{"a2->x2#0", "x1->a1#0", "y1->b1#0", "y2->b2#0"}
	again, _ := anchored(t, cyclesInside(t, false), screen(), &prev)
	assert.Equal(t, layoutJSON(t, fresh), layoutJSON(t, again), "anchored")
}

func TestCongruences_ParallelEdgesPairInDeclarationOrder(t *testing.T) {
	// a second wire from each ingress to its app server, the first ones
	// labelled: paired the other way round, a label would face none
	g := twoDCs(t)
	g.Edges = append(g.Edges,
		model.Edge{ID: "a_in->a_app#1", From: "a_in", To: "a_app"},
		model.Edge{ID: "b_in->b_app#1", From: "b_in", To: "b_app"})
	g.Edges[1].Label, g.Edges[4].Label = "first", "first"
	pg := lay(t, g, screen(), model.Down)
	require.Len(t, pg.AppliedCongruences, 1)
	assert.Equal(t, []int{1, 2, 6}, pg.AppliedCongruences[0].RepEdges)
	assert.Equal(t, []int{4, 5, 7}, pg.AppliedCongruences[0].Members[0].Edges, "a's first wire with b's first, its second with b's second")
}

// translation is the offset of pg's node or group b from a, which must be
// the same kind; its box as a group, its center as a node.
func translation(t *testing.T, pg *model.PositionedGraph, a, b string) model.Point {
	t.Helper()
	for _, gr := range pg.Groups {
		if gr.ID == a {
			o := group(t, pg, b)
			require.InDelta(t, gr.Width, o.Width, 0.01, "%s and %s", a, b)
			require.InDelta(t, gr.Height, o.Height, 0.01, "%s and %s", a, b)
			return model.Point{X: o.X - gr.X, Y: o.Y - gr.Y}
		}
	}
	na, nb := positioned(t, pg, a), positioned(t, pg, b)
	require.InDelta(t, na.Width, nb.Width, 0.01, "%s and %s", a, b)
	require.InDelta(t, na.Height, nb.Height, 0.01, "%s and %s", a, b)
	return model.Point{X: nb.X - na.X, Y: nb.Y - na.Y}
}

func TestLayout_MembersAreTranslatedCopies(t *testing.T) {
	g := datacenters(t)
	for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
		for _, text := range []bool{false, true} {
			g.Direction = dir
			cfg, lim := screen(), contract.ScreenLimits()
			if text {
				cfg, lim = TextConfig(dir), contract.TextLimits(dir)
			}
			pg, err := Layout(context.Background(), g, cfg, nil)
			require.NoError(t, err)
			for _, v := range contract.Check(pg, contract.Options{Limits: lim, Direction: dir, EdgeIDs: ids(g), Text: text}) {
				t.Errorf("%s text=%v: %s %s: %s", dir, text, v.Rule, v.Subject, v.Detail)
			}
			require.Len(t, pg.AppliedCongruences, 3, "%s text=%v", dir, text)
			// d2 is d1 moved, every node and group inside it by one offset,
			// and inside d1 the q cluster is the p cluster moved.
			moved := func(a, b string, pairs ...[2]string) {
				d := translation(t, pg, a, b)
				for _, p := range pairs {
					got := translation(t, pg, p[0], p[1])
					assert.InDelta(t, d.X, got.X, 0.01, "%s %s text=%v", p[1], dir, text)
					assert.InDelta(t, d.Y, got.Y, 0.01, "%s %s text=%v", p[1], dir, text)
				}
			}
			var outer [][2]string
			for _, id := range []string{"p", "q", "p_cp", "p_ing", "p_wrk", "q_cp", "q_ing", "q_wrk", "_db"} {
				outer = append(outer, [2]string{"d1" + id, "d2" + id})
			}
			moved("d1", "d2", outer...)
			moved("d1p", "d1q", [2]string{"d1p_cp", "d1q_cp"}, [2]string{"d1p_ing", "d1q_ing"}, [2]string{"d1p_wrk", "d1q_wrk"})
		}
	}
}

func TestLayout_CopiedBoxesMakeRoomForTheWidestTitle(t *testing.T) {
	g := twoDCs(t)
	g.Groups[1].Label = "A title much wider than its data center's content"
	pg := lay(t, g, screen(), model.Down)
	require.Len(t, pg.AppliedCongruences, 1)
	a, b := group(t, pg, "a"), group(t, pg, "b")
	w, _ := size.Title(g.Groups[1].Label, screen().Size)
	assert.InDelta(t, 2*screen().TitleInset+w, b.Width, 1e-6, "b widens for its title")
	assert.InDelta(t, b.Width, a.Width, 1e-6, "and a with it")
	d := translation(t, pg, "a_in", "b_in")
	assert.InDelta(t, b.X-a.X, d.X, 0.01, "the content sits alike in both")
}

// TestLayout_TheTranslationIsReported pins S12's Out: a member's
// translation is reported in the engine's frame, px in the text profile,
// whatever the direction: the output frame's offset mapped back (S11),
// under RIGHT and LEFT engine x along the output rows.
func TestLayout_TheTranslationIsReported(t *testing.T) {
	g := twoDCs(t)
	// engine is the engine-frame offset of an output-frame offset d.
	engine := map[model.Direction]func(d model.Point) model.Point{
		model.Down:  func(d model.Point) model.Point { return d },
		model.Up:    func(d model.Point) model.Point { return model.Point{X: d.X, Y: -d.Y} },
		model.Right: func(d model.Point) model.Point { return model.Point{X: d.Y, Y: d.X} },
		model.Left:  func(d model.Point) model.Point { return model.Point{X: d.Y, Y: -d.X} },
	}
	for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
		for _, text := range []bool{false, true} {
			g.Direction = dir
			cfg := screen()
			if text {
				cfg = TextConfig(dir)
			}
			pg, err := Layout(context.Background(), g, cfg, nil)
			require.NoError(t, err)
			require.Len(t, pg.AppliedCongruences, 1)
			m := pg.AppliedCongruences[0].Members[0]
			d := engine[dir](translation(t, pg, "a", "b"))
			require.NotZero(t, d.X, "%s text=%v: the members sit side by side", dir, text)
			assert.InDelta(t, d.X, m.Dx, 1e-6, "%s text=%v", dir, text)
			assert.InDelta(t, d.Y, m.Dy, 1e-6, "%s text=%v", dir, text)
			assert.Equal(t, []int{1, 2}, pg.AppliedCongruences[0].RepEdges)
			assert.Equal(t, []int{4, 5}, m.Edges)
		}
	}
}

// threeDCs is a load balancer, fed by a client, feeding three congruent
// data centers and a monitoring node beside them (S12, Rule C): the
// monitoring wire takes a lane on one side of the fan only.
func threeDCs(t *testing.T) model.Graph {
	t.Helper()
	labels := map[string]string{"client": "Client", "lb": "Load Balancer", "mon": "Monitoring"}
	specs := []string{"client->lb"}
	var groups []model.Group
	for _, dc := range []string{"a", "b", "c"} {
		labels[dc+"_in"], labels[dc+"_app"] = "Ingress", "App Server"
		specs = append(specs, "lb->"+dc+"_in", dc+"_in->"+dc+"_app")
		groups = append(groups, model.Group{ID: dc, Label: "DC", Contains: []string{dc + "_in", dc + "_app"}})
	}
	g := grouped(graph(t, labels, append(specs, "lb->mon")...), groups...)
	for i := range g.Edges {
		g.Edges[i].Label = "http"
	}
	return g
}

func TestLayout_AFansRanksShareTheirLanes(t *testing.T) {
	g := threeDCs(t)
	for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
		for _, text := range []bool{false, true} {
			g.Direction = dir
			cfg, lim := screen(), contract.ScreenLimits()
			if text {
				cfg, lim = TextConfig(dir), contract.TextLimits(dir)
			}
			pg, err := Layout(context.Background(), g, cfg, nil)
			require.NoError(t, err)
			for _, v := range contract.Check(pg, contract.Options{Limits: lim, Direction: dir, EdgeIDs: ids(g), Text: text}) {
				t.Errorf("%s text=%v: %s %s: %s", dir, text, v.Rule, v.Subject, v.Detail)
			}
			require.Len(t, pg.AppliedCongruences, 1)
			assert.Len(t, pg.AppliedCongruences[0].FanOut, 3, "%s text=%v: the fan-out is reported", dir, text)
		}
	}
}

// fourDCs is a load balancer feeding four congruent data centers, a fan
// of two ranks (S12, Rule C): a and d, then b and c.
func fourDCs(t *testing.T) model.Graph {
	t.Helper()
	labels := map[string]string{"lb": "Load Balancer"}
	var specs []string
	var groups []model.Group
	for _, dc := range []string{"a", "b", "c", "d"} {
		labels[dc+"_in"], labels[dc+"_app"] = "Ingress", "App Server"
		specs = append(specs, "lb->"+dc+"_in", dc+"_in->"+dc+"_app")
		groups = append(groups, model.Group{ID: dc, Label: "DC", Contains: []string{dc + "_in", dc + "_app"}})
	}
	return grouped(graph(t, labels, specs...), groups...)
}

// spreadAt is where edge e's spread run lies on the flow axis of pg, in
// the output frame: its first segment across the flow axis beyond its
// source's box on that axis (C17.3).
func spreadAt(t *testing.T, pg *model.PositionedGraph, dir model.Direction, e int) float64 {
	t.Helper()
	ed := pg.Edges[e]
	src := positioned(t, pg, ed.From)
	flow := func(p model.Point) float64 { return p.Y }
	lo, hi := src.Y-src.Height/2, src.Y+src.Height/2
	if sideways(dir) {
		flow = func(p model.Point) float64 { return p.X }
		lo, hi = src.X-src.Width/2, src.X+src.Width/2
	}
	for i := 0; i+1 < len(ed.Points); i++ {
		p, q := ed.Points[i], ed.Points[i+1]
		if flow(p) == flow(q) && p != q && (flow(p) < lo || flow(p) > hi) {
			return flow(p)
		}
	}
	require.Failf(t, "no spread run", "%s: %v", ed.ID, ed.Points)
	return 0
}

// TestLayout_AFansOuterPairRunsNearestTheSource pins S12's Rule C on a fan
// of two ranks: each rank's two spread runs share one lane, the outer
// pair's nearer the source than the inner pair's, so no two cross (S8,
// C17.3).
func TestLayout_AFansOuterPairRunsNearestTheSource(t *testing.T) {
	g := fourDCs(t)
	for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
		for _, text := range []bool{false, true} {
			g.Direction = dir
			cfg, lim := screen(), contract.ScreenLimits()
			if text {
				cfg, lim = TextConfig(dir), contract.TextLimits(dir)
			}
			pg, err := Layout(context.Background(), g, cfg, nil)
			require.NoError(t, err)
			for _, v := range contract.Check(pg, contract.Options{Limits: lim, Direction: dir, EdgeIDs: ids(g), Text: text}) {
				t.Errorf("%s text=%v: %s %s: %s", dir, text, v.Rule, v.Subject, v.Detail)
			}
			require.Len(t, pg.AppliedCongruences, 1, "%s text=%v", dir, text)
			fan := pg.AppliedCongruences[0].FanOut
			require.Len(t, fan, 4, "%s text=%v: the fan-out is reported", dir, text)
			outer := [2]float64{spreadAt(t, pg, dir, fan[0]), spreadAt(t, pg, dir, fan[3])}
			inner := [2]float64{spreadAt(t, pg, dir, fan[1]), spreadAt(t, pg, dir, fan[2])}
			assert.InDelta(t, outer[0], outer[1], 0.01, "%s text=%v: the outer pair on one lane", dir, text)
			assert.InDelta(t, inner[0], inner[1], 0.01, "%s text=%v: the inner pair on one lane", dir, text)
			lb := positioned(t, pg, "lb")
			at := lb.Y
			if sideways(dir) {
				at = lb.X
			}
			assert.Less(t, math.Abs(outer[0]-at), math.Abs(inner[0]-at), "%s text=%v: the outer pair nearest the source", dir, text)
		}
	}
}

func TestLayout_TheFanOutIsSkippedWhenItIsNotOneFan(t *testing.T) {
	cases := []struct {
		name, reason string
		g            func(t *testing.T) model.Graph
	}{
		{"the source in a group of its own", "source_outside", func(t *testing.T) model.Graph {
			g := twoDCs(t)
			g.Groups = append(g.Groups, model.Group{ID: "edge", Contains: []string{"lb"}})
			return g
		}},
		{"a member a layer lower", "layers", func(t *testing.T) model.Graph {
			g := twoDCs(t)
			more := graph(t, nil, "z->a_in", "y1->y2", "y2->b_in")
			g.Nodes = append(g.Nodes, more.Nodes[0], more.Nodes[2], more.Nodes[3])
			g.Edges = append(g.Edges, more.Edges...)
			return g
		}},
		{"a member fed from elsewhere where the representative is fed by the source", "fan_edges", func(t *testing.T) model.Graph {
			g := twoDCs(t)
			more := graph(t, nil, "z->a_in", "z->b_in")
			g.Nodes = append(g.Nodes, more.Nodes[0])
			g.Edges = append(g.Edges, more.Edges...)
			g.Edges[3], g.Edges[7] = g.Edges[7], g.Edges[3] // b's edge from z first
			return g
		}},
		{"a member fed twice by the source where the representative is fed once", "fan_edges", func(t *testing.T) model.Graph {
			// a's two edges in from above, lb's and z's, pair with b's two
			// from lb (S12), so the members correspond, but b holds more
			// edges from the source than a does
			g := twoDCs(t)
			more := graph(t, nil, "z->a_in", "lb->b_in", "lb->b_in")
			require.Equal(t, "lb->b_in#1", more.Edges[2].ID)
			g.Nodes = append(g.Nodes, more.Nodes[0])
			g.Edges = append(g.Edges, more.Edges[0], more.Edges[2])
			return g
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := tc.g(t)
			var buf bytes.Buffer
			ctx := layoutdbg.NewContext(context.Background(), slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
			pg, err := Layout(ctx, g, screen(), nil)
			require.NoError(t, err)
			require.Len(t, pg.AppliedCongruences, 1)
			assert.Empty(t, pg.AppliedCongruences[0].FanOut)
			assert.Contains(t, decisions(t, &buf, "congruence"), "congruence_fanout_skipped "+tc.reason)
		})
	}
}

// seed101 is the random-graph probe's congruent graph of seed 101
// (-probe.congruent): g0 and its copy g0_c0, each holding one
// parallelogram, fed by n2 and by the new node src, beside a chain of
// rounded nodes, a hexagon and a circle.
func seed101(t *testing.T) model.Graph {
	t.Helper()
	g := graph(t, map[string]string{"n0_c0": "n0"}, "n0:parallelogram", "n1:hexagon", "n2:rounded", "n3:rounded", "n4:circle", "src", "n0_c0:parallelogram",
		"n2->n4", "n1->n2", "n2->n0", "n3->n4", "n1->n3", "src->n0", "n2->n0_c0", "src->n0_c0")
	return grouped(g, model.Group{ID: "g0", Label: "G0", Contains: []string{"n0"}}, model.Group{ID: "g0_c0", Label: "G0", Contains: []string{"n0_c0"}})
}

// TestLayout_ProbeSeed101SpreadsItsFanOutInText pins S12's Rule C on the
// congruent probe graph of seed 101 under DOWN, in both profiles: the
// congruence is applied and its fan-out from src is spread, with no
// congruence_fanout_skipped record. The text layout once skipped it
// (reason lanes), and before that for a crossing.
func TestLayout_ProbeSeed101SpreadsItsFanOutInText(t *testing.T) {
	g := seed101(t)
	for _, text := range []bool{false, true} {
		cfg := screen()
		if text {
			cfg = TextConfig(model.Down)
		}
		var buf bytes.Buffer
		ctx := layoutdbg.NewContext(context.Background(), slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
		pg, err := Layout(ctx, g, cfg, nil)
		require.NoError(t, err)
		require.Len(t, pg.AppliedCongruences, 1, "text=%v", text)
		assert.Len(t, pg.AppliedCongruences[0].FanOut, 2, "text=%v: the fan-out from src is reported", text)
		for _, d := range decisions(t, &buf, "congruence") {
			assert.NotContains(t, d, "congruence_fanout_skipped", "text=%v", text)
		}
	}
}

func TestFanShape(t *testing.T) {
	// A source s over two members' nodes m1 and m2, in the engine's frame.
	pg := &model.PositionedGraph{
		Nodes: []model.PositionedNode{
			{ID: "s", X: 200, Y: 20, Width: 80, Height: 40},
			{ID: "m1", X: 100, Y: 200, Width: 80, Height: 40},
			{ID: "m2", X: 300, Y: 200, Width: 80, Height: 40},
		},
		Edges: []model.PositionedEdge{
			{From: "s", To: "m1", Points: []model.Point{{X: 190, Y: 40}, {X: 190, Y: 100}, {X: 100, Y: 100}, {X: 100, Y: 180}}},
			{From: "s", To: "m2", Points: []model.Point{{X: 210, Y: 40}, {X: 210, Y: 100}, {X: 300, Y: 100}, {X: 300, Y: 180}}},
		},
	}
	index := map[string]int{"s": 0, "m1": 1, "m2": 2}
	assert.Empty(t, fanShape(pg, index, []int{0, 1}), "a pair on one lane")

	pg.Edges[1].Points[1].Y, pg.Edges[1].Points[2].Y = 108, 108
	assert.Equal(t, "lanes", fanShape(pg, index, []int{0, 1}), "a pair on two lanes")

	// Side exits: the first run across the flow axis lies inside the
	// source's box, the spread run below it.
	pg.Edges[0].Points = []model.Point{{X: 160, Y: 20}, {X: 140, Y: 20}, {X: 140, Y: 100}, {X: 100, Y: 100}, {X: 100, Y: 180}}
	pg.Edges[1].Points = []model.Point{{X: 240, Y: 20}, {X: 260, Y: 20}, {X: 260, Y: 100}, {X: 300, Y: 100}, {X: 300, Y: 180}}
	assert.Empty(t, fanShape(pg, index, []int{0, 1}), "side exits spreading on one lane")

	pg.Edges[0].Points = []model.Point{{X: 190, Y: 40}, {X: 190, Y: 100}, {X: 320, Y: 100}, {X: 320, Y: 150}, {X: 100, Y: 150}, {X: 100, Y: 180}}
	pg.Edges[1].Points = []model.Point{{X: 210, Y: 40}, {X: 210, Y: 100}, {X: 300, Y: 100}, {X: 300, Y: 180}}
	assert.Equal(t, "crossing", fanShape(pg, index, []int{0, 1}), "a fan that crosses itself")
}

// TestCopyGraph turns a representative's layered graph onto a member's
// level (S12, Rule A): each vertex on its layer and in its place under the
// member's node and edge indices, a dummy on its edge's counterpart with
// that edge's slot (S13), each chain and reversal on the counterpart edge.
// The member declares its nodes and edges in another order, so every index
// moves. The copy takes no Bands: only S6's ordering reads them, and a
// copying level is laid out as a copy, never ordered.
func TestCopyGraph(t *testing.T) {
	rep := lgraphtest.Level(t, "a->b", "b->c", "c->a")
	reversed := []bool{false, false, true} // c->a runs down from a, past layer 1
	rg, err := lgraph.Build(rep, []int{0, 1, 2}, reversed, 10)
	require.NoError(t, err)
	require.Len(t, rg.Vertices, 4, "three nodes and c->a's dummy")

	member := lgraphtest.Level(t, "z", "x", "y", "y->z", "z->x", "x->y")
	nodeOf := []int{1, 2, 0} // a, b, c: x, y, z
	edgeOf := []int{2, 0, 1} // a->b, b->c, c->a: x->y, y->z, z->x
	g := copyGraph(rg, member, nodeOf, edgeOf)

	assert.Same(t, member, g.Level)
	assert.Equal(t, []lgraph.Vertex{
		{ID: "z", Node: 0, Edge: -1, Layer: 2, W: 80},
		{ID: "x", Node: 1, Edge: -1, Layer: 0, W: 80},
		{ID: "y", Node: 2, Edge: -1, Layer: 1, W: 80},
		{ID: "z->x#0@1", Node: -1, Edge: 1, Layer: 1, W: 10},
	}, g.Vertices)
	assert.Equal(t, [][]int{{1}, {2, 3}, {0}}, g.Layers)
	assert.Equal(t, [][]int{{2, 0}, {1, 3, 0}, {1, 2}}, g.Chains, "y->z, z->x from x down, x->y")
	assert.Equal(t, []bool{false, true, false}, g.Reversed)
	assert.Nil(t, g.Bands)
}

func TestCopyClasses_JoinCongruencesAcrossLevels(t *testing.T) {
	acs := []model.AppliedCongruence{
		{RepEdges: []int{1, 2}, Members: []model.AppliedCongruenceMember{{Edges: []int{5, 6}}}}, // the outer set
		{RepEdges: []int{1}, Members: []model.AppliedCongruenceMember{{Edges: []int{2}}}},       // a set inside its representative
		{RepEdges: []int{5}, Members: []model.AppliedCongruenceMember{{Edges: []int{6}}}},       // and inside its member
	}
	assert.Equal(t, map[int][]int{1: {2, 5, 6}}, copyClasses(8, acs), "one class, led by its first edge")
	assert.Empty(t, copyClasses(3, nil))
}

// TestLayout_CongruentLabelsThatOverlapAreAllMarkedUnresolved pins S10 on
// the labels of the edges a congruence pairs (S12): they place together,
// and when their boxes overlap at every spot all of them take the least
// overlap and are marked unresolved (C14.4). Under DOWN and UP a's middle
// label is far wider than the members' offset, so b's, moved as its wire
// is, lands on it at every spot: both keep a position, marked
// unresolved, on screen and in text. TestLayout_CongruenceLabelsPassC14
// checks the same layouts against the contract alone, which both labels
// dropped would pass too.
func TestLayout_CongruentLabelsThatOverlapAreAllMarkedUnresolved(t *testing.T) {
	g := wideAndNarrow(t)
	for _, dir := range []model.Direction{model.Down, model.Up} {
		for _, text := range []bool{false, true} {
			g.Direction = dir
			cfg := screen()
			if text {
				cfg = TextConfig(dir)
			}
			pg, err := Layout(context.Background(), g, cfg, nil)
			require.NoError(t, err)
			require.NotEmpty(t, pg.AppliedCongruences, "%s text=%v", dir, text)
			for _, id := range []string{g.Edges[1].ID, g.Edges[4].ID} {
				e := edge(t, pg, id)
				assert.NotNil(t, e.LabelPos, "%s text=%v: %s keeps a position", dir, text, id)
				assert.True(t, e.LabelUnresolved, "%s text=%v: %s is marked unresolved", dir, text, id)
			}
		}
	}
}

// wideAndNarrow is twoDCs with a's middle wire labelled far wider than
// the members' offset and b's counterpart labelled narrow: b's label,
// moved as its wire is from a's, lands on a's (S10, S12).
func wideAndNarrow(t *testing.T) model.Graph {
	t.Helper()
	g := twoDCs(t)
	g.Edges[1].Label = strings.Repeat("w", 74)
	g.Edges[4].Label = "x"
	return g
}

// TestLayout_CongruenceLabelsPassC14 lays out three congruence-heavy graphs
// (two and three data centers, and a pair whose representative's label is
// far wider than its members' offset) in every direction and profile and
// checks the result against the full C14, member and fan-out labels
// included: C17.4's exemption is gone, so nothing here nils out
// AppliedCongruences to defeat it any more, and a violation would now
// surface on its own. It still adds coverage the corpus's one congruent
// fixture (flow-congruent-fan.json, checked by the contract baseline) does
// not: two- and three-member congruences and the wide/narrow label case.
func TestLayout_CongruenceLabelsPassC14(t *testing.T) {
	for _, g := range []model.Graph{datacenters(t), threeDCs(t), wideAndNarrow(t)} {
		for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
			for _, text := range []bool{false, true} {
				g.Direction = dir
				cfg, lim := screen(), contract.ScreenLimits()
				if text {
					cfg, lim = TextConfig(dir), contract.TextLimits(dir)
				}
				pg, err := Layout(context.Background(), g, cfg, nil)
				require.NoError(t, err)
				require.NotEmpty(t, pg.AppliedCongruences)
				for _, v := range contract.Check(pg, contract.Options{Limits: lim, Direction: dir, EdgeIDs: ids(g), Text: text}) {
					t.Errorf("%s text=%v: %s %s: %s", dir, text, v.Rule, v.Subject, v.Detail)
				}
			}
		}
	}
}
