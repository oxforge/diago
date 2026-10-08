package layered

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/layout/contract"
	"github.com/oxforge/diago/internal/layout/metrics"
	"github.com/oxforge/diago/internal/layoutdbg"
	"github.com/oxforge/diago/internal/model"
)

// anchored lays g out under cfg anchored on prev (nil for a fresh layout)
// and returns the layout and its anchoring records (S13).
func anchored(t *testing.T, g model.Graph, cfg Config, prev *model.LayoutHints) (*model.PositionedGraph, []map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	ctx := layoutdbg.NewContext(context.Background(), slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	pg, err := Layout(ctx, g, cfg, prev)
	require.NoError(t, err)
	var recs []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var rec map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &rec))
		if rec["phase"] == "anchoring" {
			recs = append(recs, rec)
		}
	}
	return pg, recs
}

// record returns the first anchoring record named decision whose scope is
// scope, or nil.
func record(recs []map[string]any, decision, scope string) map[string]any {
	for _, r := range recs {
		if r["decision"] == decision && (r["scope"] == nil || r["scope"] == scope) {
			return r
		}
	}
	return nil
}

// strs reads a record's list attribute.
func strs(v any) []string {
	var out []string
	list, _ := v.([]any)
	for _, s := range list {
		out = append(out, s.(string))
	}
	return out
}

func layoutJSON(t *testing.T, pg *model.PositionedGraph) string {
	t.Helper()
	b, err := json.Marshal(pg)
	require.NoError(t, err)
	return string(b)
}

// base215 is golayout's anchoring fixture (C18): a fan, a join, and a
// chain that spans three layers.
func base215(t *testing.T) model.Graph {
	return graph(t, map[string]string{"in": "Ingress", "a": "Auth", "b": "Billing", "c": "Catalog", "d": "DB", "q": "Queue", "w": "Worker"},
		"in->a", "in->b", "in->c", "a->d", "b->d", "c->d", "b->q", "q->w", "w->d")
}

func TestCarrier_AFlatLevel(t *testing.T) {
	// C18: layers and order per node, a slot per interior layer of a long
	// edge, the reversed edges; the same whatever the direction and the
	// profile.
	g := graph(t, nil, "a->b", "b->c", "a->c", "c->a")
	pg := lay(t, g, screen(), model.Down)
	h := pg.LayoutHints
	require.NotNil(t, h)
	assert.Equal(t, model.LayoutHintsVersion, h.Version)
	assert.Equal(t, map[string]int{"a": 0, "b": 1, "c": 2}, h.Scopes[""].Layers)
	assert.ElementsMatch(t, []string{"a", "b", "c", "a->c#0@1", "c->a#0@1"}, slices.Collect(maps.Keys(h.Scopes[""].Order)))
	assert.Equal(t, []string{"c->a#0"}, h.Reversed)
	for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
		g.Direction = dir
		assert.Equal(t, h, lay(t, g, screen(), dir).LayoutHints, "%s", dir)
		pt, err := Layout(context.Background(), g, TextConfig(dir), nil)
		require.NoError(t, err)
		assert.Equal(t, h, pt.LayoutHints, "%s text", dir)
	}
}

func TestCarrier_GroupedLevels(t *testing.T) {
	// A scope per container; a child group is a node of its parent's
	// scope, and a terminal is its edge's slot on its border row (C18,
	// S13), never its vertex id.
	g := grouped(graph(t, nil, "x->a", "a->b", "b->y"), model.Group{ID: "g", Label: "G", Contains: []string{"a", "b"}})
	h := lay(t, g, screen(), model.Down).LayoutHints
	assert.Equal(t, map[string]int{"x": 0, "g": 1, "y": 2}, h.Scopes[""].Layers)
	assert.Equal(t, map[string]int{"a": 1, "b": 2}, h.Scopes["g"].Layers)
	assert.Equal(t, map[string]int{"a": 0, "b": 0, "x->a#0@0": 0, "b->y#0@3": 0}, h.Scopes["g"].Order)
	assert.Empty(t, h.Reversed)
}

func TestCarrier_ANumericGroupIDIsNoSlot(t *testing.T) {
	// In group "1", x->c's terminal sits on row 0 and its chain runs
	// through rows 1 and 2 to c: the terminal is the edge's slot on its
	// row, never its vertex id "x->c#0@1", which would take row 1's slot.
	g := grouped(graph(t, nil, "x->c", "a->b", "b->c"), model.Group{ID: "1", Label: "One", Contains: []string{"a", "b", "c"}})
	order := lay(t, g, screen(), model.Down).LayoutHints.Scopes["1"].Order
	assert.ElementsMatch(t, []string{"a", "b", "c", "x->c#0@0", "x->c#0@1", "x->c#0@2"}, slices.Collect(maps.Keys(order)))
}

func TestCarrier_ReversedListsAnEdgeOnceAtItsHome(t *testing.T) {
	// b->x runs back up into x at the root; inside g its terminal's edge
	// is flipped too (S2, S9), but the carrier names the edge once.
	g := grouped(graph(t, nil, "x->a", "a->b", "b->x"), model.Group{ID: "g", Contains: []string{"a", "b"}})
	h := lay(t, g, screen(), model.Down).LayoutHints
	assert.Equal(t, []string{"b->x#0"}, h.Reversed)
}

// congruentCycles is a load balancer feeding two congruent groups, a and
// b, each a node p above a two-cycle between q and s. b declares p's
// edges in the other order, so its own search (S2) would enter the cycle
// at s and reverse q->s, where a's enters at q and reverses s->q.
func congruentCycles(t *testing.T) model.Graph {
	t.Helper()
	g := graph(t, nil, "lb", "a_p", "a_q", "a_s", "b_p", "b_q", "b_s",
		"lb->a_p", "a_p->a_q", "a_p->a_s", "a_q->a_s", "a_s->a_q",
		"lb->b_p", "b_p->b_s", "b_p->b_q", "b_q->b_s", "b_s->b_q")
	return grouped(g,
		model.Group{ID: "a", Label: "A", Contains: []string{"a_p", "a_q", "a_s"}},
		model.Group{ID: "b", Label: "B", Contains: []string{"b_p", "b_q", "b_s"}})
}

// agrees checks that h's reversed edges are the ones its layers draw
// upward: in every scope, an edge between two ids it gives a layer runs
// down unless h lists it, and up when it does (C18, S13).
func agrees(t *testing.T, g model.Graph, h *model.LayoutHints) {
	t.Helper()
	for name, sh := range h.Scopes {
		for _, e := range g.Edges {
			from, okFrom := sh.Layers[e.From]
			to, okTo := sh.Layers[e.To]
			if !okFrom || !okTo || e.From == e.To {
				continue
			}
			if slices.Contains(h.Reversed, e.ID) {
				assert.Greater(t, from, to, "%s is reversed but runs down in %q", e.ID, name)
			} else {
				assert.Less(t, from, to, "%s is not reversed but runs up in %q", e.ID, name)
			}
		}
	}
}

func TestCarrier_ACopyingLevelListsTheReversalsItDraws(t *testing.T) {
	// b copies a's level (S12, Rule A), a's reversal included: the carrier
	// lists b_s->b_q, the edge b's layers draw upward, never q->s, the one
	// b's own search would have reversed (S13, Out).
	g := congruentCycles(t)
	pg := lay(t, g, screen(), model.Down)
	require.Len(t, pg.AppliedCongruences, 1)
	h := pg.LayoutHints
	assert.Equal(t, []string{"a_s->a_q#0", "b_s->b_q#0"}, h.Reversed)
	agrees(t, g, h)
}

func TestCarrier_ACopyingLevelHasItsOwnScope(t *testing.T) {
	// b copies a's level (S12, Rule A); its scope holds b's ids where a's
	// scope holds their counterparts.
	h := lay(t, twoDCs(t), screen(), model.Down).LayoutHints
	a, b := h.Scopes["a"], h.Scopes["b"]
	for _, id := range []string{"in", "app", "db"} {
		assert.Equal(t, a.Layers["a_"+id], b.Layers["b_"+id], id)
		assert.Equal(t, a.Order["a_"+id], b.Order["b_"+id], id)
	}
	assert.Len(t, b.Order, len(a.Order))
}

func TestAnchoring_ExactReproduction(t *testing.T) {
	// C18.1: an unchanged spec anchored on its own carrier gives the same
	// layout, carrier included. The cases: golayout's fixture; a K3,3,
	// which ends with crossings; the layer-skipping back edge golayout
	// does not reproduce; nested groups and a congruence.
	backEdge := graph(t, map[string]string{"client": "Web Client", "lb": "Load Balancer", "api": "API Gateway", "auth": "Auth Service",
		"db": "Postgres", "cache": "Redis Cache", "queue": "Job Queue", "worker": "Email Worker", "smtp": "SMTP Relay"},
		"client->lb", "lb->api", "api->auth", "api->db:cylinder", "auth->db", "api->cache:cylinder", "api->queue", "queue->worker", "worker->db", "smtp")
	cases := []struct {
		name string
		g    model.Graph
	}{
		{"fixture", base215(t)},
		// S6's first run ends with a crossing that a run of 2 sweeps from
		// it removes: without the restarts, anchoring would reorder it.
		{"a first run short of the best", graph(t, nil, "n0", "n1", "n2", "n3", "n4", "n5", "n6", "n7", "n8",
			"n6->n0", "n2->n7", "n6->n7", "n3->n3", "n2->n4", "n8->n0", "n7->n3", "n1->n3", "n6->n7", "n3->n8", "n6->n2", "n0->n7")},
		{"k33", graph(t, nil, "a->d", "a->e", "a->f", "b->d", "b->e", "b->f", "c->d", "c->e", "c->f")},
		{"back edge", backEdge},
		{"nested groups", datacenters(t)},
		{"congruence", twoDCs(t)},
	}
	for _, tc := range cases {
		for _, dir := range []model.Direction{model.Down, model.Right} {
			for _, text := range []bool{false, true} {
				g := tc.g
				g.Direction = dir
				cfg := screen()
				if text {
					cfg = TextConfig(dir)
				}
				fresh, _ := anchored(t, g, cfg, nil)
				again, _ := anchored(t, g, cfg, fresh.LayoutHints)
				assert.Equal(t, layoutJSON(t, fresh), layoutJSON(t, again), "%s %s text=%v", tc.name, dir, text)
			}
		}
	}
}

func TestAnchoring_TheK33EndsWithCrossings(t *testing.T) {
	// The K3,3 of TestAnchoring_ExactReproduction pins the case a previous
	// layout ended with crossings, which 2 sweeps from it could reduce
	// were S6's ordering not its own anchored ordering.
	pg := lay(t, graph(t, nil, "a->d", "a->e", "a->f", "b->d", "b->e", "b->f", "c->d", "c->e", "c->f"), screen(), model.Down)
	n := 0
	for _, e := range pg.Edges {
		n += len(e.Crossings)
	}
	assert.Positive(t, n)
}

func TestAnchoring_StaleEntriesNeverBreakALayout(t *testing.T) {
	// C18.2: unknown ids and slots, negative values, a vanished scope and
	// an unknown reversed edge are ignored, each reported.
	g := base215(t)
	prev := model.NewLayoutHints()
	s := prev.Scope("")
	s.Layers["ghost"], s.Order["ghost"] = 9, 9
	s.Layers["a"] = -3
	s.Order["b"] = -1
	s.Order["zz@1"] = 0
	prev.Scope("gone").Layers["in"] = 2
	prev.Reversed = []string{"nope->nope#0"}
	pg, recs := anchored(t, g, screen(), prev)
	require.Len(t, pg.Nodes, 7)
	assert.Empty(t, contract.Check(pg, contract.Options{Limits: contract.ScreenLimits(), Direction: model.Down, EdgeIDs: ids(g)}))
	read := record(recs, "previous_read", "")
	require.NotNil(t, read)
	assert.Equal(t, []string{"gone"}, strs(read["vanished"]))
	assert.Equal(t, []string{"nope->nope#0"}, strs(read["unknown_reversed"]))
	applied := record(recs, "previous_applied", "")
	require.NotNil(t, applied)
	assert.Equal(t, []string{"ghost", "zz@1"}, strs(applied["missing"]))
	assert.Equal(t, []string{"a", "b"}, strs(applied["rejected"]))
}

func TestAnchoring_AMovedNodeLaysOutFresh(t *testing.T) {
	// C18, Locality: the carrier holds x at the root; the spec puts it in
	// g, whose scope the carrier lacks.
	g := grouped(graph(t, nil, "s->x", "s->y"), model.Group{ID: "g", Label: "G", Contains: []string{"x"}})
	prev := model.NewLayoutHints()
	prev.Scope("").Layers["x"], prev.Scope("").Order["x"] = 5, 0
	_, recs := anchored(t, g, screen(), prev)
	inG := record(recs, "previous_applied", "g")
	require.NotNil(t, inG)
	assert.Equal(t, false, inG["scope_known"])
	assert.Equal(t, []string{"x"}, strs(inG["moved"]))
	assert.Contains(t, strs(record(recs, "previous_applied", "")["missing"]), "x")
	for _, r := range recs {
		assert.False(t, r["decision"] == "layer_adopted" && r["node"] == "x", "x takes no previous layer")
	}
}

func TestAnchoring_ReversalsStick(t *testing.T) {
	// C18.3: declared in the other order, the nodes make a fresh layout
	// enter the cycles elsewhere (S2); anchored, it keeps the previous
	// back edges.
	edges := []string{"a->b", "b->c", "c->a", "c->d", "d->b"}
	cyc := graph(t, nil, edges...)
	turned := graph(t, nil, append([]string{"d", "c", "b", "a"}, edges...)...)
	prev := lay(t, cyc, screen(), model.Down).LayoutHints
	require.NotEmpty(t, prev.Reversed)
	require.NotEqual(t, prev.Reversed, lay(t, turned, screen(), model.Down).LayoutHints.Reversed, "the order must change the fresh choice")
	again, _ := anchored(t, turned, screen(), prev)
	assert.Equal(t, prev.Reversed, again.LayoutHints.Reversed)
}

func TestAnchoring_ALeafKeepsTheSurvivorsLayersAndOrder(t *testing.T) {
	// C18.4: a leaf, under a survivor or above one, moves no survivor to
	// another layer against the others, nor out of its place in its layer.
	// A leaf above the top node adds a layer above them all, so every
	// survivor adopts its previous layer and every slot finds its previous
	// one (S13's alignment). g2 is g1 less q, anchored on it, which keeps x
	// a layer below p; a leaf above r must not lift x onto p's layer.
	fixture := lay(t, base215(t), screen(), model.Down).LayoutHints
	g1 := graph(t, nil, "r->p", "p->q", "q->x", "r->x")
	g2, _ := anchored(t, without(g1, "q"), screen(), lay(t, g1, screen(), model.Down).LayoutHints)
	require.Equal(t, map[string]int{"r": 0, "p": 1, "x": 2}, g2.LayoutHints.Scopes[""].Layers)
	grown := func(g model.Graph, from, to string) model.Graph {
		g.Nodes = append(slices.Clone(g.Nodes), model.Node{ID: "m", Label: "Metrics"})
		g.Edges = append(slices.Clone(g.Edges), model.Edge{ID: from + "->" + to + "#0", From: from, To: to})
		return g
	}
	cases := []struct {
		name   string
		before *model.LayoutHints
		g      model.Graph
	}{
		{"under c", fixture, grown(base215(t), "c", "m")},
		{"above in", fixture, grown(base215(t), "m", "in")},
		{"above r", g2.LayoutHints, grown(without(g1, "q"), "m", "r")},
	}
	for _, tc := range cases {
		after, recs := anchored(t, tc.g, screen(), tc.before)
		h := after.LayoutHints
		assert.Empty(t, metrics.Relayered(tc.before, h), tc.name)
		assert.Nil(t, record(recs, "layer_rejected", ""), tc.name)
		assert.Empty(t, strs(record(recs, "previous_applied", "")["missing"]), tc.name)
		layers := tc.before.Scopes[""].Layers
		for id, l := range layers {
			for id2, l2 := range layers {
				if l == l2 && tc.before.Scopes[""].Order[id] < tc.before.Scopes[""].Order[id2] {
					assert.Less(t, h.Scopes[""].Order[id], h.Scopes[""].Order[id2], "%s: %s stays before %s", tc.name, id, id2)
				}
			}
		}
		if tc.name == "above r" {
			assert.Equal(t, 1, h.Scopes[""].Layers["x"]-h.Scopes[""].Layers["p"], "x stays a layer below p")
		}
	}
}

// slotLayer splits a carrier's slot key into its edge id and its layer;
// ok is false for a node's or a group's id.
func slotLayer(key string) (string, int, bool) {
	at := strings.LastIndex(key, "@")
	if at < 0 {
		return "", 0, false
	}
	l, err := strconv.Atoi(key[at+1:])
	return key[:at], l, err == nil
}

func TestAnchoring_DeletingTheTopNodeKeepsTheSlots(t *testing.T) {
	// S13, Scopes: a dummy is looked up on its row's previous layer, where
	// the nodes that kept theirs sat, and a terminal on the previous
	// level's row on its side. Without in, alone on the fixture's top
	// layer, every row moves up one, and the dummies of a->d, b->d and
	// c->d find their slots a layer down. Without t, alone on g's top row,
	// the terminals of a->y and b->y find theirs on g's previous last row.
	// Each slot keeps its place among those of its row.
	cases := []struct {
		name, gone, scope string
		g                 model.Graph
	}{
		{"dummies", "in", "", base215(t)},
		{"terminals", "t", "g", grouped(graph(t, nil, "t->a", "t->b", "a->y", "b->y"), model.Group{ID: "g", Label: "G", Contains: []string{"t", "a", "b"}})},
	}
	for _, tc := range cases {
		prev := lay(t, tc.g, screen(), model.Down).LayoutHints
		before := prev.Scopes[tc.scope]
		pg, recs := anchored(t, without(tc.g, tc.gone), screen(), prev)
		applied := record(recs, "previous_applied", tc.scope)
		require.NotNil(t, applied, tc.name)
		assert.Equal(t, []string{tc.gone}, strs(applied["missing"]), tc.name)
		after := pg.LayoutHints.Scopes[tc.scope]
		slots := 0
		for key, i := range before.Order {
			e, l, ok := slotLayer(key)
			if !ok {
				continue
			}
			slots++
			require.Contains(t, after.Order, slot(e, l-1), tc.name)
			for key2, i2 := range before.Order {
				if e2, l2, ok := slotLayer(key2); ok && l2 == l && i < i2 {
					assert.Less(t, after.Order[slot(e, l-1)], after.Order[slot(e2, l2-1)], "%s: %s stays before %s", tc.name, key, key2)
				}
			}
		}
		require.Positive(t, slots, tc.name)
	}
}

func TestAnchoring_AStaleRowAboveMovesNothing(t *testing.T) {
	// S13: the alignment and the slots rest on the nodes the level knows,
	// never on the lowest entry the scope holds. The fixture's carrier,
	// every row moved one down below a row that only ghost holds, an id
	// the graph lacks, anchors the fixture as its own carrier does.
	g := base215(t)
	fresh := lay(t, g, screen(), model.Down)
	prev := model.NewLayoutHints()
	s := prev.Scope("")
	for id, l := range fresh.LayoutHints.Scopes[""].Layers {
		s.Layers[id] = l + 1
	}
	for key, i := range fresh.LayoutHints.Scopes[""].Order {
		if e, l, ok := slotLayer(key); ok {
			key = slot(e, l+1)
		}
		s.Order[key] = i
	}
	s.Layers["ghost"], s.Order["ghost"] = 0, 0
	pg, recs := anchored(t, g, screen(), prev)
	assert.Equal(t, layoutJSON(t, fresh), layoutJSON(t, pg))
	assert.Equal(t, []string{"ghost"}, strs(record(recs, "previous_applied", "")["missing"]))
}

// without is g less node id, its edges and its place in any group.
func without(g model.Graph, id string) model.Graph {
	out := g
	out.Nodes = slices.DeleteFunc(slices.Clone(g.Nodes), func(n model.Node) bool { return n.ID == id })
	out.Edges = slices.DeleteFunc(slices.Clone(g.Edges), func(e model.Edge) bool { return e.From == id || e.To == id })
	out.Groups = slices.Clone(g.Groups)
	for i := range out.Groups {
		out.Groups[i].Contains = slices.DeleteFunc(slices.Clone(out.Groups[i].Contains), func(s string) bool { return s == id })
	}
	return out
}

func TestAnchoring_DeletionConvergesInOneStep(t *testing.T) {
	// C18.5. In the grouped case the deleted node is the only one its
	// group's entering wires reach: the group loses its first terminal
	// row (S3, S13).
	cases := []struct {
		name string
		g    model.Graph
		gone string
	}{
		{"fixture", base215(t), "q"},
		{"a group loses its entry", grouped(graph(t, nil, "n0", "n1->n2", "n4->n3", "n2->n4", "n1->n2"),
			model.Group{ID: "g0", Label: "G0", Contains: []string{"n2", "n4"}}), "n2"},
	}
	for _, tc := range cases {
		for _, text := range []bool{false, true} {
			cfg := screen()
			if text {
				cfg = TextConfig(model.Down)
			}
			before, _ := anchored(t, tc.g, cfg, nil)
			once, _ := anchored(t, without(tc.g, tc.gone), cfg, before.LayoutHints)
			twice, _ := anchored(t, without(tc.g, tc.gone), cfg, once.LayoutHints)
			assert.Equal(t, layoutJSON(t, once), layoutJSON(t, twice), "%s text=%v", tc.name, text)
		}
	}
}

func TestAnchoring_ThePreviousOrderComesFirst(t *testing.T) {
	// S13, Precedence: the previous order over the declaration seed, and
	// over the whole-graph seed inside a group.
	flat := graph(t, nil, "a->b", "a->c")
	require.Less(t, positioned(t, lay(t, flat, screen(), model.Down), "b").X, positioned(t, lay(t, flat, screen(), model.Down), "c").X)
	prev := model.NewLayoutHints()
	s := prev.Scope("")
	s.Layers["a"], s.Layers["b"], s.Layers["c"] = 0, 1, 1
	s.Order["a"], s.Order["c"], s.Order["b"] = 0, 0, 1
	pg, _ := anchored(t, flat, screen(), prev)
	assert.Less(t, positioned(t, pg, "c").X, positioned(t, pg, "b").X, "flat")

	g := grouped(graph(t, nil, "s->b", "s->c"), model.Group{ID: "g", Label: "G", Contains: []string{"b", "c"}})
	fresh := lay(t, g, screen(), model.Down)
	require.Less(t, positioned(t, fresh, "b").X, positioned(t, fresh, "c").X)
	prev = model.NewLayoutHints()
	in := prev.Scope("g")
	in.Layers["b"], in.Layers["c"] = 1, 1
	in.Order["c"], in.Order["b"] = 0, 1
	in.Order["s->c#0@0"], in.Order["s->b#0@0"] = 0, 1 // their terminals
	pg, _ = anchored(t, g, screen(), prev)
	assert.Less(t, positioned(t, pg, "c").X, positioned(t, pg, "b").X, "grouped")
}

// TestAnchoring_ALoopKeepsThePreviousOrder pins S6's yield to S13 on the
// finished flow-loop: fresh, S6 keeps the loop together (End, Process,
// return dummy); anchored on a carrier whose order puts End between
// Process and the return dummy, inside the loop, the level keeps that
// order, since there the previous order wins the tie, and so does a
// render anchored on its own result (C18.1).
func TestAnchoring_ALoopKeepsThePreviousOrder(t *testing.T) {
	g := flowLoop(model.Down)
	slots := []string{"process", "end", "increment->check#0@3"}
	order := func(pg *model.PositionedGraph) map[string]int {
		out := map[string]int{}
		for _, id := range slots {
			out[id] = pg.LayoutHints.Scope("").Order[id]
		}
		return out
	}
	fresh, _ := anchored(t, g, screen(), nil)
	require.Equal(t, map[string]int{"end": 0, "process": 1, "increment->check#0@3": 2}, order(fresh))
	prev := fresh.LayoutHints
	for i, id := range slots {
		prev.Scope("").Order[id] = i
	}
	pg, recs := anchored(t, g, screen(), prev)
	require.NotNil(t, record(recs, "previous_applied", ""))
	assert.Equal(t, map[string]int{"process": 0, "end": 1, "increment->check#0@3": 2}, order(pg))
	again, _ := anchored(t, g, screen(), pg.LayoutHints)
	assert.Equal(t, layoutJSON(t, pg), layoutJSON(t, again))
}

func TestAnchoring_ACopyingLevelReadsNoCarrier(t *testing.T) {
	// S13: b copies a's level (S12, Rule A) whatever the carrier says of
	// b's scope.
	g := twoDCs(t)
	prev := lay(t, g, screen(), model.Down).LayoutHints
	b := prev.Scope("b")
	b.Layers["b_in"], b.Layers["b_db"] = b.Layers["b_db"], b.Layers["b_in"]
	pg, recs := anchored(t, g, screen(), prev)
	require.Len(t, pg.AppliedCongruences, 1)
	assert.Empty(t, contract.Check(pg, contract.Options{Limits: contract.ScreenLimits(), Direction: model.Down, EdgeIDs: ids(g)}))
	assert.NotNil(t, record(recs, "previous_applied", "a"))
	assert.Nil(t, record(recs, "previous_applied", "b"))

	// Nor the carrier's reversals. Fed at b2, b is no copy and reverses
	// b1->b2; fed at b1, b copies a, which reverses a2->a1, so b draws
	// b2->b1 upward whatever the carrier says. Anchored on the first
	// carrier, the layout is the fresh one, carrier included. Once b3
	// breaks the congruence, b copies nothing and reads its scope off the
	// carrier.
	cycles := func(fed string, more ...string) model.Graph {
		g := grouped(graph(t, nil, "lb", "a1", "a2", "b1", "b2", "lb->a1", "a1->a2", "a2->a1", "lb->"+fed, "b1->b2", "b2->b1"),
			model.Group{ID: "a", Label: "A", Contains: []string{"a1", "a2"}},
			model.Group{ID: "b", Label: "B", Contains: []string{"b1", "b2"}})
		for _, id := range more {
			g.Nodes = append(g.Nodes, model.Node{ID: id, Label: id})
			g.Groups[1].Contains = append(g.Groups[1].Contains, id)
		}
		return g
	}
	prev = lay(t, cycles("b2"), screen(), model.Down).LayoutHints
	require.Contains(t, prev.Reversed, "b1->b2#0")
	fresh := lay(t, cycles("b1"), screen(), model.Down)
	require.Len(t, fresh.AppliedCongruences, 1)
	require.Contains(t, fresh.LayoutHints.Reversed, "b2->b1#0")
	again, _ := anchored(t, cycles("b1"), screen(), prev)
	assert.Equal(t, layoutJSON(t, fresh), layoutJSON(t, again))
	agrees(t, cycles("b1"), again.LayoutHints)
	broken, recs := anchored(t, cycles("b1", "b3"), screen(), fresh.LayoutHints)
	assert.Empty(t, broken.AppliedCongruences)
	assert.NotNil(t, record(recs, "previous_applied", "b"))
}

func TestAnchoring_AFirstRowOneLayoutLacksShiftsNothing(t *testing.T) {
	// S13, Scopes and rank: the previous layout's group g had a first
	// terminal row, for x->a, so its nodes sat from layer 1 and a's long
	// edge had its dummy on layer 2. Without x, g has no first row: its
	// layers align one up, a new isolated node z still shares a's layer,
	// and the dummy finds its slot on the layer a's row held.
	with := grouped(graph(t, nil, "x->a", "a->b", "b->c", "a->c"), model.Group{ID: "g", Label: "G", Contains: []string{"a", "b", "c"}})
	prev := lay(t, with, screen(), model.Down).LayoutHints
	require.Equal(t, 1, prev.Scopes["g"].Layers["a"])
	require.Contains(t, prev.Scopes["g"].Order, "a->c#0@2")
	g := grouped(graph(t, nil, "a->b", "b->c", "a->c", "z"), model.Group{ID: "g", Label: "G", Contains: []string{"a", "b", "c", "z"}})
	pg, recs := anchored(t, g, screen(), prev)
	assert.Equal(t, positioned(t, pg, "a").Y, positioned(t, pg, "z").Y)
	inG := record(recs, "previous_applied", "g")
	require.NotNil(t, inG)
	assert.NotContains(t, strs(inG["missing"]), "a->c#0@2")
}

func TestAnchoring_AFlatEdgeInTheCarrier(t *testing.T) {
	// S13 with S9's Flat edges: a flat edge is on no level, so the carrier
	// is the one the graph without it writes (bare), with no slot and no
	// reversal of it, even against the flow, where on a level it would
	// close a cycle (S2); a flat edge that fell back is an ordinary edge
	// there, and ranked lists it (C18). An unchanged spec anchored on its
	// own carrier reproduces its fresh layout (C18.1): side by side, across
	// two groups' borders, against the flow, and after a fallback, even
	// where anchoring on the layers the fallback gave the ends would let the
	// router route the edge. Both profiles, DOWN and RIGHT.
	wrapped := map[string]string{"a": "a label long enough to wrap onto three lines of text", "b": "b label long enough to wrap onto three lines of text"}
	cases := []struct {
		name    string
		g, bare model.Graph
		flat    string   // the flat edge routed flat, "" for none
		ranked  []string // the flat edges that fall back
	}{
		{"side by side", flatten(twoChains(t), "a1->b1#0"), graph(t, nil, "a1->a2", "b1->b2"), "a1->b1#0", nil},
		{"across two groups", twoSites(t), twoDCs(t), "a_db->b_db#0", nil},
		{"a fallback", flatten(graph(t, nil, "x->x2", "y->y2", "x->y", "a->m", "m->b", "a->b"), "x->y#0", "a->b#0"),
			graph(t, nil, "x->x2", "y->y2", "a->m", "m->b", "a->b"), "x->y#0", []string{"a->b#0"}},
		// fresh, x stands between a and b on their row and blocks every
		// candidate, so a -> b falls back and b takes the layer below a;
		// anchored on that carrier without a -> b ranked, b would keep that
		// layer and the router would route a -> b flat
		{"a fallback anchoring alone would undo", flatten(graph(t, nil, "a->a2", "x->x2", "b->b2", "a->b"), "a->b#0"),
			graph(t, nil, "a->a2", "x->x2", "b->b2", "a->b"), "", []string{"a->b#0"}},
		// b -> a closes a cycle with a -> b: on a level S2 would reverse
		// it, and the carrier's reversed list would hold it; three-line
		// labels give the straight run along the flow a second column
		// beside a -> b's in the text profile under RIGHT
		{"against the flow", flatten(graph(t, wrapped, "a->b", "b->a"), "b->a#0"), graph(t, wrapped, "a->b"), "b->a#0", nil},
	}
	for _, tc := range cases {
		for _, dir := range []model.Direction{model.Down, model.Right} {
			for _, text := range []bool{false, true} {
				g, bare := tc.g, tc.bare
				g.Direction, bare.Direction = dir, dir
				cfg := screen()
				if text {
					cfg = TextConfig(dir)
				}
				fresh, _ := anchored(t, g, cfg, nil)
				again, _ := anchored(t, g, cfg, fresh.LayoutHints)
				assert.Equal(t, layoutJSON(t, fresh), layoutJSON(t, again), "%s %s text=%v", tc.name, dir, text)
				without, _ := anchored(t, bare, cfg, nil)
				want := *without.LayoutHints
				want.Ranked = tc.ranked
				assert.Equal(t, &want, fresh.LayoutHints, "%s %s text=%v: the carrier without the flat edge, and its fallbacks", tc.name, dir, text)
				if tc.flat == "" {
					continue
				}
				for _, sh := range fresh.LayoutHints.Scopes {
					for key := range sh.Order {
						assert.False(t, strings.HasPrefix(key, tc.flat+"@"), "%s %s text=%v: %s", tc.name, dir, text, key)
					}
				}
				assert.NotContains(t, fresh.LayoutHints.Reversed, tc.flat)
			}
		}
	}
}

// flatRecords lists the records of phase flat and the previous_ranked
// record in a layout's log, each as its decision and its edge (or, for
// previous_ranked, its lists).
func flatRecords(t *testing.T, g model.Graph, cfg Config, prev *model.LayoutHints) (*model.PositionedGraph, []string) {
	t.Helper()
	var buf bytes.Buffer
	ctx := layoutdbg.NewContext(context.Background(), slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	pg, err := Layout(ctx, g, cfg, prev)
	require.NoError(t, err)
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var rec map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &rec))
		switch {
		case rec["phase"] == "flat":
			out = append(out, fmt.Sprint(rec["decision"], " ", rec["edge"]))
		case rec["decision"] == "previous_ranked":
			out = append(out, fmt.Sprint(rec["decision"], " ", rec["ranked"], " ", rec["stale"]))
		}
	}
	return pg, out
}

func TestAnchoring_TheCarrierKeepsAFallback(t *testing.T) {
	// C18, S13: the carrier lists a flat edge that fell back in ranked, and
	// a layout anchored on it starts with that edge ranked: no route is
	// tried for it and nothing falls back again, and the layout reproduces
	// (C18.1). A carrier without fallbacks has no ranked key at all.
	for _, text := range []bool{false, true} {
		cfg := screen()
		if text {
			cfg = TextConfig(model.Down)
		}
		fresh, recs := flatRecords(t, blocked(t), cfg, nil)
		assert.Equal(t, []string{"flat_edge_unrouted a->b#0", "flat_edge_ranked a->b#0"}, recs, "text=%v", text)
		assert.Equal(t, []string{"a->b#0"}, fresh.LayoutHints.Ranked, "text=%v", text)
		again, recs := flatRecords(t, blocked(t), cfg, fresh.LayoutHints)
		assert.Equal(t, []string{"previous_ranked [a->b#0] []"}, recs, "text=%v: no route tried, no fallback", text)
		assert.Equal(t, layoutJSON(t, fresh), layoutJSON(t, again), "text=%v", text)
		assert.True(t, edge(t, again, "a->b#0").FlatRanked, "text=%v", text)

		routed, _ := flatRecords(t, flatten(twoChains(t), "a1->b1#0"), cfg, nil)
		b, err := json.Marshal(routed.LayoutHints)
		require.NoError(t, err)
		assert.NotContains(t, string(b), "ranked", "text=%v", text)
	}
}

// seed41 is the random-graph probe's graph of seed 41 under DOWN with
// its flat-edges mode (-probe.flatedges): a group g0 and a flat edge
// n6 -> n4 from a circle to a hexagon. Fresh, the router routes it; with
// n0 deleted and anchored on the fresh carrier, it falls back. n2, a
// diamond in the probe's graph, is a box here: since a parallelogram's
// ports take the stretch its edges share (S8, Shape ports), the fresh
// screen layout of the probe's graph ranks the edge from the start.
func seed41(t *testing.T) model.Graph {
	g := graph(t, nil, "n0:parallelogram", "n1", "n2", "n3:diamond", "n4:hexagon", "n5:rounded", "n6:circle",
		"n0->n5", "n2->n6", "n5->n0", "n1->n2", "n1->n2", "n0->n1", "n3->n0", "n4->n3", "n6->n4")
	return grouped(flatten(g, "n6->n4#0"), model.Group{ID: "g0", Contains: []string{"n0", "n2", "n6"}})
}

func TestAnchoring_AFallbackAfterADeletionConverges(t *testing.T) {
	// C18.5: n0 deleted, the layout anchored on the fresh carrier ranks
	// n6 -> n4, which the smaller graph's fresh layout routes; its carrier
	// keeps the fallback, so the same layout anchored on it comes out the
	// same, instead of routing the edge on the layers the fallback gave
	// its ends.
	for _, text := range []bool{false, true} {
		g := seed41(t)
		cfg := screen()
		if text {
			cfg = TextConfig(model.Down)
		}
		fresh, _ := anchored(t, g, cfg, nil)
		require.Empty(t, fresh.LayoutHints.Ranked, "text=%v", text)
		smaller := without(g, "n0")
		own, _ := anchored(t, smaller, cfg, nil)
		require.False(t, edge(t, own, "n6->n4#0").FlatRanked, "text=%v: the smaller graph's fresh layout routes it", text)
		once, _ := anchored(t, smaller, cfg, fresh.LayoutHints)
		require.True(t, edge(t, once, "n6->n4#0").FlatRanked, "text=%v: the fixture must fall back once anchored", text)
		assert.Equal(t, []string{"n6->n4#0"}, once.LayoutHints.Ranked, "text=%v", text)
		twice, _ := anchored(t, smaller, cfg, once.LayoutHints)
		assert.Equal(t, layoutJSON(t, once), layoutJSON(t, twice), "text=%v", text)
	}
}

// flatIDs lists g's flat edges.
func flatIDs(g model.Graph) []string {
	var out []string
	for _, e := range g.Edges {
		if e.Flat {
			out = append(out, e.ID)
		}
	}
	return out
}

func TestAnchoring_ALeafThatBlocksAFlatRoute(t *testing.T) {
	// C18.4's exemption: a leaf above a2 lands between a1 and b1 on their
	// row and blocks the straight run between them, so the anchored layout
	// ranks a1 -> b1, a flat edge the previous layout routed flat; b1 and
	// b2 take other layers against the other survivors, which C18.4 allows
	// only because the new carrier ranks the edge and the previous one did
	// not (metrics.NewlyRanked).
	for _, text := range []bool{false, true} {
		g := flatten(twoChains(t), "a1->b1#0")
		cfg := screen()
		if text {
			cfg = TextConfig(model.Down)
		}
		fresh, _ := anchored(t, g, cfg, nil)
		require.False(t, edge(t, fresh, "a1->b1#0").FlatRanked, "text=%v", text)
		grown := g
		grown.Nodes = append(slices.Clone(g.Nodes), model.Node{ID: "leaf", Label: "leaf"})
		grown.Edges = append(slices.Clone(g.Edges), model.Edge{ID: "leaf->a2#0", From: "leaf", To: "a2"})
		after, _ := anchored(t, grown, cfg, fresh.LayoutHints)
		assert.True(t, edge(t, after, "a1->b1#0").FlatRanked, "text=%v", text)
		assert.Equal(t, []string{"a1->b1#0"}, after.LayoutHints.Ranked, "text=%v", text)
		assert.NotEmpty(t, metrics.Relayered(fresh.LayoutHints, after.LayoutHints), "text=%v: survivors moved", text)
		assert.Equal(t, []string{"a1->b1#0"}, metrics.NewlyRanked(fresh.LayoutHints, after.LayoutHints, flatIDs(g)), "text=%v: exempt", text)
	}
}

func TestAnchoring_AKeptFallbackStillBindsC18_4(t *testing.T) {
	// C18.4 and S13: a flat edge that fell back before stays ranked, so it
	// moves nothing and the exemption for a flat edge that newly falls back
	// does not apply: with a leaf added below b and above m, the layout
	// anchored on the fallback's carrier keeps a -> b ranked, ranks nothing
	// newly (metrics.NewlyRanked) and keeps every survivor's layer
	// relative to the others (metrics.Relayered). Both profiles.
	for _, text := range []bool{false, true} {
		cfg := screen()
		if text {
			cfg = TextConfig(model.Down)
		}
		g := blocked(t)
		fresh, _ := anchored(t, g, cfg, nil)
		require.Equal(t, []string{"a->b#0"}, fresh.LayoutHints.Ranked, "text=%v", text)
		for _, leaf := range []model.Edge{{ID: "b->leaf#0", From: "b", To: "leaf"}, {ID: "leaf->m#0", From: "leaf", To: "m"}} {
			grown := g
			grown.Nodes = append(slices.Clone(g.Nodes), model.Node{ID: "leaf", Label: "leaf"})
			grown.Edges = append(slices.Clone(g.Edges), leaf)
			after, recs := flatRecords(t, grown, cfg, fresh.LayoutHints)
			assert.Equal(t, []string{"previous_ranked [a->b#0] []"}, recs, "text=%v %s: kept, no route tried", text, leaf.ID)
			assert.True(t, edge(t, after, "a->b#0").FlatRanked, "text=%v %s", text, leaf.ID)
			assert.Nil(t, metrics.NewlyRanked(fresh.LayoutHints, after.LayoutHints, flatIDs(g)), "text=%v %s: not exempt", text, leaf.ID)
			assert.Empty(t, metrics.Relayered(fresh.LayoutHints, after.LayoutHints), "text=%v %s: no survivor moved", text, leaf.ID)
		}
	}
}

func TestAnchoring_ATextLayoutOnAScreenCarrierKeepsItsFallbacks(t *testing.T) {
	// C18 and S13: a carrier reads the same in either profile, so a text
	// layout anchored on a screen carrier keeps the screen layout's
	// fallbacks. Under RIGHT n4 -> n2 falls back on screen and routes in a
	// fresh text layout; anchored on the screen carrier, the text layout
	// keeps it ranked without trying its route, and lists it in its own
	// carrier.
	g := flatten(graph(t, nil, "n0", "n1", "n2", "n3", "n4", "n2->n1", "n4->n1", "n4->n2"), "n4->n2#0")
	g.Direction = model.Right
	onScreen, _ := anchored(t, g, screen(), nil)
	require.Equal(t, []string{"n4->n2#0"}, onScreen.LayoutHints.Ranked, "the fixture falls back on screen")
	fresh, _ := anchored(t, g, TextConfig(model.Right), nil)
	require.False(t, edge(t, fresh, "n4->n2#0").FlatRanked, "the fixture routes in a fresh text layout")
	text, recs := flatRecords(t, g, TextConfig(model.Right), onScreen.LayoutHints)
	assert.Equal(t, []string{"previous_ranked [n4->n2#0] []"}, recs, "kept, no route tried")
	assert.True(t, edge(t, text, "n4->n2#0").FlatRanked)
	assert.Equal(t, []string{"n4->n2#0"}, text.LayoutHints.Ranked)
}

func TestAnchoring_StaleRankedEntries(t *testing.T) {
	// C18.2 and S13: a carrier's ranked entry for an edge the spec no
	// longer marks flat, or no longer has, is ignored and recorded as
	// stale: the layout is the one the carrier without it gives, and its
	// own carrier ranks nothing. An entry the spec still has flat stays
	// ranked, even where a fresh layout would route the edge (m deleted).
	for _, text := range []bool{false, true} {
		cfg := screen()
		if text {
			cfg = TextConfig(model.Down)
		}
		prev, _ := anchored(t, blocked(t), cfg, nil)
		require.Equal(t, []string{"a->b#0"}, prev.LayoutHints.Ranked)
		bare := *prev.LayoutHints
		bare.Ranked = nil
		for _, g := range []model.Graph{graph(t, nil, "a->m", "m->b", "a->b"), graph(t, nil, "a->m", "m->b")} {
			pg, recs := flatRecords(t, g, cfg, prev.LayoutHints)
			assert.Equal(t, []string{"previous_ranked [] [a->b#0]"}, recs, "text=%v %d edges", text, len(g.Edges))
			want, _ := anchored(t, g, cfg, &bare)
			assert.Equal(t, layoutJSON(t, want), layoutJSON(t, pg), "text=%v %d edges", text, len(g.Edges))
			assert.Nil(t, pg.LayoutHints.Ranked)
		}
		two := without(blocked(t), "m")
		free, _ := anchored(t, two, cfg, nil)
		require.False(t, edge(t, free, "a->b#0").FlatRanked, "text=%v: fresh, a -> b routes", text)
		kept, _ := anchored(t, two, cfg, prev.LayoutHints)
		assert.True(t, edge(t, kept, "a->b#0").FlatRanked, "text=%v: anchored, it stays ranked", text)
		assert.Equal(t, []string{"a->b#0"}, kept.LayoutHints.Ranked, "text=%v", text)
	}
}
