package layered

import (
	"context"
	"math"
	"testing"

	"github.com/oxforge/diago/internal/layout/contract"
	"github.com/oxforge/diago/internal/layoutdbg/layoutdbgtest"
	"github.com/oxforge/diago/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lay runs Layout and checks its result against the output contract
// (C2-C17) under the screen values, for the direction it resolves to.
func lay(t *testing.T, g model.Graph, cfg Config, dir model.Direction) *model.PositionedGraph {
	t.Helper()
	pg, err := Layout(context.Background(), g, cfg, nil)
	require.NoError(t, err)
	for _, v := range contract.Check(pg, contract.Options{Limits: contract.ScreenLimits(), Direction: dir, EdgeIDs: ids(g)}) {
		t.Errorf("%s %s: %s", v.Rule, v.Subject, v.Detail)
	}
	return pg
}

// ids lists g's edge ids, in edge order.
func ids(g model.Graph) []string {
	out := make([]string, len(g.Edges))
	for i, e := range g.Edges {
		out[i] = e.ID
	}
	return out
}

func edge(t *testing.T, pg *model.PositionedGraph, id string) model.PositionedEdge {
	t.Helper()
	for _, e := range pg.Edges {
		if e.ID == id {
			return e
		}
	}
	require.Failf(t, "no edge", "%s", id)
	return model.PositionedEdge{}
}

func TestLayout_MeetsTheContract(t *testing.T) {
	fixtures := []struct {
		name  string
		specs []string
	}{
		{"fan and merge", []string{"r->a", "r->b", "r->c", "a->m", "b->m", "c->m"}},
		{"decision loop", []string{"s->d:diamond", "d->a", "d->b", "a->s", "b->e"}},
		{"self-loops", []string{"p->p", "p->q:diamond", "q->q", "q->r", "r->q", "r->r:circle"}},
		{"long edges", []string{"a->b", "b->c", "c->d", "a->d", "a->d", "b->d"}},
		{"shaped ports", []string{"x->c:cylinder", "y->c", "c->h:hexagon", "c->k:parallelogram", "x->k", "h->k"}},
		{"three exits", []string{"m:diamond->a", "m->b", "m->c", "a->z", "b->z", "c->z"}},
		{"crossing lanes", []string{"a->d", "a->e", "b->d", "b->f", "c->e", "c->f"}},
	}
	for _, f := range fixtures {
		for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
			t.Run(f.name+"/"+dir.String(), func(t *testing.T) {
				g := graph(t, nil, f.specs...)
				g.Direction = dir
				g.Edges[0].Label = "first"
				lay(t, g, screen(), dir)
			})
		}
	}
}

// TestLayout_NestedSelfLoopsMeetTheContract pins S8's self-loops on the
// finished layout: several loops on one node of every shape with faces
// nest, so no two share a run (C9), and their ports stay on the drawn
// outline (C2.2) with outward stubs (C7), in both profiles and every
// direction.
func TestLayout_NestedSelfLoopsMeetTheContract(t *testing.T) {
	g := graph(t, nil, "p->p", "p->p", "p->p", "p->c:cylinder", "c->c", "c->c", "c->h:hexagon", "h->h", "h->h",
		"h->k:parallelogram", "k->k", "k->k")
	for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
		t.Run(dir.String(), func(t *testing.T) {
			g.Direction = dir
			lay(t, g, screen(), dir)
			pg, err := Layout(context.Background(), g, TextConfig(dir), nil)
			require.NoError(t, err)
			for _, v := range contract.Check(pg, contract.Options{Limits: contract.TextLimits(dir), Direction: dir, EdgeIDs: ids(g), Text: true}) {
				t.Errorf("text: %s %s: %s", v.Rule, v.Subject, v.Detail)
			}
		})
	}
}

func TestLayout_Auto(t *testing.T) {
	g := graph(t, nil, "a->b", "c", "d") // four nodes, one edge: RIGHT (S11)
	g.Direction = model.Auto
	pg := lay(t, g, screen(), model.Right)
	a, b := pg.Nodes[0], pg.Nodes[1]
	assert.Greater(t, b.X, a.X, "b lays out downstream: to the right")
}

// routes lays g out under the screen values with the decision log armed
// and counts its routed decisions (S8): the first pass's, and the replays.
func routes(t *testing.T, g model.Graph) (pg *model.PositionedGraph, fresh, replays int) {
	t.Helper()
	ctx, buf := layoutdbgtest.Capture()
	pg, err := Layout(ctx, g, screen(), nil)
	require.NoError(t, err)
	for _, rec := range layoutdbgtest.Records(t, buf) {
		if rec["decision"] != "routed" {
			continue
		}
		if rec["replay"] == true {
			replays++
		} else {
			fresh++
		}
	}
	return pg, fresh, replays
}

func TestLayout_RecordsCrossings(t *testing.T) {
	// K(3,3): no ordering avoids every crossing
	g := graph(t, nil, "a->d", "a->e", "a->f", "b->d", "b->e", "b->f", "c->d", "c->e", "c->f")
	pg := lay(t, g, screen(), model.Down)
	n := 0
	for _, e := range pg.Edges {
		n += len(e.Crossings)
	}
	assert.Positive(t, n)
}

// near asserts that got lies within a millionth of a unit of want.
func near(t *testing.T, want, got model.Point, msg string) {
	t.Helper()
	assert.InDelta(t, want.X, got.X, 1e-6, "%s: x of %v", msg, got)
	assert.InDelta(t, want.Y, got.Y, 1e-6, "%s: y of %v", msg, got)
}

// sideVertex reports whether p is a cross-axis vertex of pinned node n
// under dir (C8): left or right under DOWN and UP, top or bottom under
// RIGHT and LEFT.
func sideVertex(n model.PositionedNode, p model.Point, dir model.Direction) bool {
	if dir == model.Right || dir == model.Left {
		return math.Abs(p.X-n.X) < 1e-6 && math.Abs(math.Abs(p.Y-n.Y)-n.Height/2) < 1e-6
	}
	return math.Abs(p.Y-n.Y) < 1e-6 && math.Abs(math.Abs(p.X-n.X)-n.Width/2) < 1e-6
}

// outVertex is the vertex of pinned node n that faces downstream under dir
// (C8), in the output frame.
func outVertex(n model.PositionedNode, dir model.Direction) model.Point {
	switch dir {
	case model.Up:
		return model.Point{X: n.X, Y: n.Y - n.Height/2}
	case model.Right:
		return model.Point{X: n.X + n.Width/2, Y: n.Y}
	case model.Left:
		return model.Point{X: n.X - n.Width/2, Y: n.Y}
	}
	return model.Point{X: n.X, Y: n.Y + n.Height/2}
}

// flowAxis splits p, in the output frame, into its coordinate along the
// flow under dir and its coordinate across it.
func flowAxis(p model.Point, dir model.Direction) (along, across float64) {
	if dir == model.Right || dir == model.Left {
		return p.X, p.Y
	}
	return p.Y, p.X
}

func TestLayout_ClassDiagram(t *testing.T) {
	g := model.Graph{Direction: model.Down, Title: "Zoo", Legend: &model.Legend{},
		Nodes: []model.Node{
			{ID: "animal", Label: "Animal", Members: &model.Members{Methods: []model.Member{{Visibility: "+", Text: "speak(): string"}}}},
			{ID: "dog", Label: "Dog", Members: &model.Members{}},
			{ID: "owner", Label: "Owner", Members: &model.Members{}},
		},
		Edges: []model.Edge{
			{ID: "dog->animal#0", From: "animal", To: "dog", Relation: model.RelationInheritance},
			{ID: "owner->dog#0", From: "owner", To: "dog", Relation: model.RelationAggregation, Label: "walks", FromCard: "1", ToCard: "0..*"},
		},
	}
	pg := lay(t, g, screen(), model.Down)
	assert.Equal(t, "Zoo", pg.Title)
	assert.Equal(t, []model.LegendEntry{{Kind: "aggregation", Label: "aggregation"}, {Kind: "inheritance", Label: "inheritance"}}, pg.Legend)
	assert.NotNil(t, pg.Nodes[0].Members)
	walks := edge(t, pg, "owner->dog#0")
	require.NotNil(t, walks.LabelPos)
	require.NotNil(t, walks.FromCard.Pos)
	require.NotNil(t, walks.ToCard.Pos)
	assert.Positive(t, walks.FromCard.Width)
	assert.Equal(t, model.RelationAggregation, walks.Relation)
}

// TestLayout_LegendLabelsRenameAndHideRows pins the legend rows a class
// diagram's legend labels make: one row per relation kind among the
// edges, sorted by kind and labelled by the kind; a label renames its
// kind's row, an empty one hides it, and one for a kind no edge has adds
// no row.
func TestLayout_LegendLabelsRenameAndHideRows(t *testing.T) {
	g := model.Graph{Direction: model.Down,
		Nodes: []model.Node{
			{ID: "a", Label: "A", Members: &model.Members{}},
			{ID: "b", Label: "B", Members: &model.Members{}},
			{ID: "c", Label: "C", Members: &model.Members{}},
		},
		Edges: []model.Edge{
			{ID: "a->b#0", From: "a", To: "b", Relation: model.RelationInheritance},
			{ID: "a->c#0", From: "a", To: "c", Relation: model.RelationComposition},
			{ID: "b->c#0", From: "b", To: "c", Relation: model.RelationAggregation},
			{ID: "c->a#0", From: "c", To: "a"},
		},
	}
	for _, tc := range []struct {
		name   string
		labels map[string]string
		want   []model.LegendEntry
	}{
		{"no labels", nil, []model.LegendEntry{
			{Kind: "aggregation", Label: "aggregation"}, {Kind: "composition", Label: "composition"}, {Kind: "inheritance", Label: "inheritance"}}},
		{"a label renames its row", map[string]string{"inheritance": "is a"}, []model.LegendEntry{
			{Kind: "aggregation", Label: "aggregation"}, {Kind: "composition", Label: "composition"}, {Kind: "inheritance", Label: "is a"}}},
		{"an empty label hides its row", map[string]string{"composition": ""}, []model.LegendEntry{
			{Kind: "aggregation", Label: "aggregation"}, {Kind: "inheritance", Label: "inheritance"}}},
		{"a label for an absent kind adds no row", map[string]string{"dependency": "uses", "aggregation": "has"}, []model.LegendEntry{
			{Kind: "aggregation", Label: "has"}, {Kind: "composition", Label: "composition"}, {Kind: "inheritance", Label: "inheritance"}}},
		{"every row hidden", map[string]string{"aggregation": "", "composition": "", "inheritance": ""}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g.Legend = &model.Legend{Labels: tc.labels}
			assert.Equal(t, tc.want, lay(t, g, screen(), model.Down).Legend)
		})
	}
}

func TestLayout_CarriesTheEdgeStyle(t *testing.T) {
	g := graph(t, nil, "a->b")
	g.Nodes[0].Color = "red"
	g.Edges[0].Style, g.Edges[0].Direction, g.Edges[0].Color = model.EdgeDashed, model.EdgeBoth, "blue"
	pg := lay(t, g, screen(), model.Down)
	assert.Equal(t, "red", pg.Nodes[0].Color)
	e := pg.Edges[0]
	assert.Equal(t, [3]any{model.EdgeDashed, model.EdgeBoth, "blue"}, [3]any{e.Style, e.Direction, e.Color})
}

func TestLayout_Errors(t *testing.T) {
	g := graph(t, nil, "a->b")
	g.Groups = []model.Group{{ID: "one", Contains: []string{"a"}}, {ID: "two", Contains: []string{"a"}}}
	_, err := Layout(context.Background(), g, screen(), nil)
	assert.ErrorContains(t, err, "node a is in two groups")
}

func TestForText_ResolvesAuto(t *testing.T) {
	g := graph(t, nil, "a->b", "c", "d")
	g.Direction = model.Auto
	tg, cfg := ForText(context.Background(), g)
	assert.Equal(t, model.Right, tg.Direction)
	assert.Equal(t, TextConfig(model.Right), cfg)
	_, err := Layout(context.Background(), tg, cfg, nil)
	assert.NoError(t, err)
}

func last(ps []model.Point) model.Point { return ps[len(ps)-1] }

func TestLayout_StopsWhenTheContextIsDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	g := model.Graph{Direction: model.Down,
		Nodes: []model.Node{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}},
		Edges: []model.Edge{{ID: "a->b#0", From: "a", To: "b"}}}
	_, err := Layout(ctx, g, screen(), nil)
	assert.ErrorIs(t, err, context.Canceled)
}
