package layered

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/layout/contract"
	"github.com/oxforge/diago/internal/layout/layered/flat"
	"github.com/oxforge/diago/internal/layoutdbg"
	"github.com/oxforge/diago/internal/model"
)

// flatten is g with the edges ids flat.
func flatten(g model.Graph, ids ...string) model.Graph {
	g.Edges = slices.Clone(g.Edges)
	for i := range g.Edges {
		if slices.Contains(ids, g.Edges[i].ID) {
			g.Edges[i].Flat = true
		}
	}
	return g
}

// layFlat lays g out under dir in the screen profile, or in the text
// profile when text, with the decision log armed, checks the result
// against the output contract (C2-C17) under that profile's values, and
// returns it with its records of phase flat, each as its decision and
// its edge.
func layFlat(t *testing.T, g model.Graph, dir model.Direction, text bool) (*model.PositionedGraph, []string) {
	t.Helper()
	pg, recs := layLogged(t, g, dir, text)
	var flat []string
	for _, rec := range recs {
		if rec["phase"] == "flat" {
			flat = append(flat, fmt.Sprint(rec["decision"], " ", rec["edge"]))
		}
	}
	return pg, flat
}

// layLogged is layFlat with every record, each decoded.
func layLogged(t *testing.T, g model.Graph, dir model.Direction, text bool) (*model.PositionedGraph, []map[string]any) {
	t.Helper()
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
		t.Errorf("%s text=%v: %s %s: %s", dir, text, v.Rule, v.Subject, v.Detail)
	}
	var recs []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var rec map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &rec))
		recs = append(recs, rec)
	}
	return pg, recs
}

// along is a point's coordinate along the flow under dir, in the output
// frame, downstream larger.
func along(p model.Point, dir model.Direction) float64 {
	a, _ := flowAxis(p, dir)
	if dir == model.Up || dir == model.Left {
		return -a
	}
	return a
}

func center(n model.PositionedNode) model.Point { return model.Point{X: n.X, Y: n.Y} }

// sideAt reports whether p lies on one of n's two side faces under dir,
// the faces a straight run across the flow joins.
func sideAt(n model.PositionedNode, p model.Point, dir model.Direction) bool {
	b := box(n)
	if dir == model.Right || dir == model.Left {
		return (math.Abs(p.Y-b[1]) < 1e-6 || math.Abs(p.Y-b[3]) < 1e-6) && p.X > b[0] && p.X < b[2]
	}
	return (math.Abs(p.X-b[0]) < 1e-6 || math.Abs(p.X-b[2]) < 1e-6) && p.Y > b[1] && p.Y < b[3]
}

// twoChains is two chains, a1 -> a2 and b1 -> b2, whose heads a1 -> b1
// joins, labelled sync.
func twoChains(t *testing.T) model.Graph {
	g := graph(t, nil, "a1->a2", "b1->b2", "a1->b1")
	g.Edges[2].Label = "sync"
	return g
}

// TestLayout_AFlatEdgeRanksNothing pins S9's Flat edges: a flat edge
// takes no part in the layering, so the two chains' heads it joins lie
// side by side on one row, where it runs straight across the flow between
// their facing sides, its label placed (S10); the same edge not flat
// puts b1 a layer below a1. Every direction, both profiles.
func TestLayout_AFlatEdgeRanksNothing(t *testing.T) {
	for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
		for _, text := range []bool{false, true} {
			pg, recs := layFlat(t, flatten(twoChains(t), "a1->b1#0"), dir, text)
			a1, b1 := positioned(t, pg, "a1"), positioned(t, pg, "b1")
			assert.InDelta(t, along(center(a1), dir), along(center(b1), dir), 1e-6, "%s text=%v: a1 and b1 on one row", dir, text)
			e := edge(t, pg, "a1->b1#0")
			require.Len(t, e.Points, 2, "%s text=%v: one straight run: %v", dir, text, e.Points)
			assert.InDelta(t, along(e.Points[0], dir), along(e.Points[1], dir), 1e-6, "%s text=%v: across the flow", dir, text)
			assert.True(t, sideAt(a1, e.Points[0], dir), "%s text=%v: from a1's side %v", dir, text, e.Points)
			assert.True(t, sideAt(b1, e.Points[1], dir), "%s text=%v: into b1's side %v", dir, text, e.Points)
			assert.False(t, e.FlatRanked, "%s text=%v", dir, text)
			assert.Equal(t, []string{"flat_edge_routed a1->b1#0"}, recs, "%s text=%v", dir, text)
			require.NotNil(t, e.LabelPos, "%s text=%v: the flat edge's label is placed", dir, text)
			assert.False(t, e.LabelUnresolved, "%s text=%v", dir, text)

			pg, _ = layFlat(t, twoChains(t), dir, text)
			a1, b1 = positioned(t, pg, "a1"), positioned(t, pg, "b1")
			assert.Greater(t, along(center(b1), dir), along(center(a1), dir)+a1.Height/2, "%s text=%v: not flat, b1 a layer below", dir, text)
		}
	}
}

// TestLayout_ANodeWithOnlyFlatEdgesIsIsolated pins S9's Flat edges with
// S3's isolated nodes: n, whose only edge is flat, is placed as an
// isolated node, exactly where it lies with no edge at all, and the flat
// edge is routed on that layout.
func TestLayout_ANodeWithOnlyFlatEdgesIsIsolated(t *testing.T) {
	for _, text := range []bool{false, true} {
		pg, recs := layLogged(t, flatten(graph(t, nil, "a->b", "a->c", "b->d", "n->d"), "n->d#0"), model.Down, text)
		alone, _ := layFlat(t, graph(t, nil, "a->b", "a->c", "b->d", "n"), model.Down, text)
		assert.Equal(t, alone.Nodes, pg.Nodes, "text=%v: every node where it lies without the flat edge", text)
		assert.GreaterOrEqual(t, len(edge(t, pg, "n->d#0").Points), 2)
		isolated, routed := 0, 0
		for _, rec := range recs {
			switch {
			case rec["decision"] == "isolated_placed" && rec["node"] == "n":
				isolated++
			case rec["decision"] == "flat_edge_routed" && rec["edge"] == "n->d#0":
				routed++
			}
		}
		assert.Equal(t, [2]int{1, 1}, [2]int{isolated, routed}, "text=%v: n placed as an isolated node, then n -> d routed", text)
	}
}

// twoSites is twoDCs with a flat edge, a -> b's replication between the
// two databases, declared among the others, so that every edge index
// after it differs between the spec and the ranked graph.
func twoSites(t *testing.T) model.Graph {
	g := twoDCs(t)
	g.Edges = slices.Insert(slices.Clone(g.Edges), 3, model.Edge{ID: "a_db->b_db#0", From: "a_db", To: "b_db", Label: "replication", Flat: true})
	return g
}

// borders counts the times route crosses the border of box b.
func borders(route []model.Point, b model.PositionedGroup) int {
	in := func(p model.Point) bool {
		return p.X > b.X && p.X < b.X+b.Width && p.Y > b.Y && p.Y < b.Y+b.Height
	}
	n := 0
	for i := 1; i < len(route); i++ {
		if in(route[i-1]) != in(route[i]) {
			n++
		}
	}
	return n
}

// TestLayout_AFlatEdgeBetweenSiblingGroups pins S9's Flat edges on the
// platform-topology shape in miniature: two sibling groups side by side,
// congruent (S12, which matches them without the flat edge), and a flat
// edge between a node in each, straight or a Z, crossing only its own
// ends' group borders, once each, its label placed (S10). The
// congruence's edge indices are the spec's (C17), the flat edge's among
// them.
func TestLayout_AFlatEdgeBetweenSiblingGroups(t *testing.T) {
	for _, dir := range []model.Direction{model.Down, model.Right} {
		for _, text := range []bool{false, true} {
			pg, recs := layFlat(t, twoSites(t), dir, text)
			assert.Equal(t, []string{"flat_edge_routed a_db->b_db#0"}, recs, "%s text=%v", dir, text)
			a, b := positioned(t, pg, "a_db"), positioned(t, pg, "b_db")
			assert.InDelta(t, along(center(a), dir), along(center(b), dir), 1e-6, "%s text=%v: the databases on one row", dir, text)
			e := edge(t, pg, "a_db->b_db#0")
			assert.Contains(t, []int{2, 4}, len(e.Points), "%s text=%v: straight or a Z: %v", dir, text, e.Points)
			assert.Equal(t, 1, borders(e.Points, group(t, pg, "a")), "%s text=%v: out of a once", dir, text)
			assert.Equal(t, 1, borders(e.Points, group(t, pg, "b")), "%s text=%v: into b once", dir, text)
			require.NotNil(t, e.LabelPos, "%s text=%v: replication is placed", dir, text)
			assert.False(t, e.LabelUnresolved, "%s text=%v", dir, text)
			require.Len(t, pg.AppliedCongruences, 1, "%s text=%v: a and b stay congruent", dir, text)
			ac := pg.AppliedCongruences[0]
			assert.Equal(t, []int{1, 2}, ac.RepEdges, "%s text=%v", dir, text)
			require.Len(t, ac.Members, 1)
			assert.Equal(t, []int{5, 6}, ac.Members[0].Edges, "%s text=%v: the spec's indices", dir, text)
		}
	}
}

// blocked is a chain a -> m -> b, and a flat a -> b that no candidate
// routes: a, m and b stack in one column, so a straight run along the
// flow passes through m, an L finds no column beside a or b, and the two
// are neither side by side nor apart across the flow.
func blocked(t *testing.T) model.Graph {
	return flatten(graph(t, nil, "a->m", "m->b", "a->b"), "a->b#0")
}

// unflagged is pg's JSON with every edge's FlatRanked cleared and no
// fallback in its carrier.
func unflagged(t *testing.T, pg *model.PositionedGraph) string {
	t.Helper()
	c := *pg
	c.Edges = slices.Clone(pg.Edges)
	for i := range c.Edges {
		c.Edges[i].FlatRanked = false
	}
	h := *pg.LayoutHints
	h.Ranked = nil
	c.LayoutHints = &h
	return layoutJSON(t, &c)
}

// TestLayout_AFlatEdgeWithNoRouteFallsBack pins S9's fallback: a flat
// edge the router keeps no route for is made ordinary and the layout
// redone with it ranked, exactly the layout of the same edge not flat,
// marked FlatRanked, with a flat_edge_ranked record, and listed in the
// carrier's ranked (C18). Both profiles.
func TestLayout_AFlatEdgeWithNoRouteFallsBack(t *testing.T) {
	for _, dir := range []model.Direction{model.Down, model.Right} {
		for _, text := range []bool{false, true} {
			pg, recs := layFlat(t, blocked(t), dir, text)
			assert.Equal(t, []string{"flat_edge_unrouted a->b#0", "flat_edge_ranked a->b#0"}, recs, "%s text=%v", dir, text)
			assert.True(t, edge(t, pg, "a->b#0").FlatRanked, "%s text=%v", dir, text)
			assert.False(t, edge(t, pg, "a->m#0").FlatRanked, "%s text=%v", dir, text)
			ordinary, _ := layFlat(t, graph(t, nil, "a->m", "m->b", "a->b"), dir, text)
			assert.Equal(t, layoutJSON(t, ordinary), unflagged(t, pg), "%s text=%v: laid out as the ordinary edge", dir, text)
			assert.Equal(t, []string{"a->b#0"}, pg.LayoutHints.Ranked, "%s text=%v", dir, text)
		}
	}
}

// TestLayout_FlatEdgesFallBackOneAtATime pins the fallback's order: of
// two flat edges no candidate routes, the first in edge order is ranked
// first, on a layout the second is not routed on; the layout is redone
// and every flat edge still flat is routed again on it, from the first,
// so the flat edge that routes (x -> y) is routed on each of the three
// layouts, and the second blocked one fails on the second layout.
func TestLayout_FlatEdgesFallBackOneAtATime(t *testing.T) {
	g := flatten(graph(t, nil, "x->x2", "y->y2", "x->y", "a->m", "m->b", "a->b", "c->n", "n->d", "c->d"), "x->y#0", "a->b#0", "c->d#0")
	for _, text := range []bool{false, true} {
		pg, recs := layFlat(t, g, model.Down, text)
		assert.Equal(t, []string{
			"flat_edge_routed x->y#0", "flat_edge_unrouted a->b#0", "flat_edge_ranked a->b#0",
			"flat_edge_routed x->y#0", "flat_edge_unrouted c->d#0", "flat_edge_ranked c->d#0",
			"flat_edge_routed x->y#0",
		}, recs, "text=%v", text)
		var ranked []string
		for _, e := range pg.Edges {
			if e.FlatRanked {
				ranked = append(ranked, e.ID)
			}
		}
		assert.Equal(t, []string{"a->b#0", "c->d#0"}, ranked, "text=%v", text)
		assert.Len(t, edge(t, pg, "x->y#0").Points, 2, "text=%v: x -> y straight", text)
	}
}

func TestArrange_RejectsAFlatEdge(t *testing.T) {
	_, err := Arrange(context.Background(), flatten(graph(t, nil, "a->b"), "a->b#0"), screen())
	assert.ErrorContains(t, err, "a->b#0 is flat")
}

// TestFlatOptions pins the distances a flat route keeps, from the Config
// (S9, Flat edges; S14): on screen each is one px value on both axes; in
// the text profile the engine's x counts columns and its y rows under
// DOWN (the other way round under RIGHT), a row two columns tall, so the
// node clearance along the flow is as many px as across it (half as many
// rows under DOWN, twice as many columns under RIGHT) and the port gap is
// one cell across the flow and as many px along it, while the stub and
// the track gap are the Config's own counts on each axis. These are the
// values the flat package's own tests fill in by hand.
func TestFlatOptions(t *testing.T) {
	for _, tt := range []struct {
		name string
		cfg  Config
		dir  model.Direction
		want flat.Options
	}{
		{"screen", screen(), model.Down, flat.Options{Clearance: flat.XY{X: 12, Y: 12}, Stub: flat.XY{X: 20, Y: 24}, TrackGap: flat.XY{X: 8, Y: 8}, PortGap: flat.XY{X: 4, Y: 4}, Dir: model.Down}},
		{"text DOWN", TextConfig(model.Down), model.Down, flat.Options{Clearance: flat.XY{X: 2, Y: 1}, Stub: flat.XY{X: 3, Y: 2}, TrackGap: flat.XY{X: 2, Y: 1}, PortGap: flat.XY{X: 1, Y: 0.5}, Dir: model.Down, Text: true}},
		{"text UP", TextConfig(model.Up), model.Up, flat.Options{Clearance: flat.XY{X: 2, Y: 1}, Stub: flat.XY{X: 3, Y: 2}, TrackGap: flat.XY{X: 2, Y: 1}, PortGap: flat.XY{X: 1, Y: 0.5}, Dir: model.Up, Text: true}},
		{"text RIGHT", TextConfig(model.Right), model.Right, flat.Options{Clearance: flat.XY{X: 1, Y: 2}, Stub: flat.XY{X: 2, Y: 3}, TrackGap: flat.XY{X: 1, Y: 2}, PortGap: flat.XY{X: 1, Y: 2}, Dir: model.Right, Text: true}},
		{"text LEFT", TextConfig(model.Left), model.Left, flat.Options{Clearance: flat.XY{X: 1, Y: 2}, Stub: flat.XY{X: 2, Y: 3}, TrackGap: flat.XY{X: 1, Y: 2}, PortGap: flat.XY{X: 1, Y: 2}, Dir: model.Left, Text: true}},
	} {
		assert.Equal(t, tt.want, flatOptions(tt.cfg, tt.dir), tt.name)
	}
}
