package layered

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/layout/contract"
	"github.com/oxforge/diago/internal/layout/layered/frame"
	"github.com/oxforge/diago/internal/layoutdbg"
	"github.com/oxforge/diago/internal/model"
	textrender "github.com/oxforge/diago/internal/render/text"
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
	var buf bytes.Buffer
	ctx := layoutdbg.NewContext(context.Background(), slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	pg, err := Layout(ctx, g, screen(), nil)
	require.NoError(t, err)
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var rec map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &rec))
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

func TestLayout_TextAdapterScalesCellsToPixels(t *testing.T) {
	g := graph(t, nil, "start->check:diamond", "check->ok", "check->retry", "retry->start", "ok->ok")
	g.Edges[1].Label = "yes"
	for _, dir := range []model.Direction{model.Down, model.Right} {
		g.Direction = dir
		cfg := TextConfig(dir)
		pg, err := Layout(context.Background(), g, cfg, nil)
		require.NoError(t, err)
		for _, n := range pg.Nodes {
			for _, v := range []float64{n.X - n.Width/2, n.X + n.Width/2} {
				assert.Equal(t, 0.0, math.Mod(v, 8), "%s %s: box edges on column boundaries", dir, n.ID)
			}
			for _, v := range []float64{n.Y - n.Height/2, n.Y + n.Height/2} {
				assert.Equal(t, 0.0, math.Mod(v, 16), "%s %s: box edges on row boundaries", dir, n.ID)
			}
		}
		yes := edge(t, pg, "check->ok#0")
		assert.Equal(t, [2]float64{24, 16}, [2]float64{yes.LabelWidth, yes.LabelHeight}, "three runes in one row")
		assert.Equal(t, 0.0, math.Mod(pg.Width, 8))
		assert.Equal(t, 0.0, math.Mod(pg.Height, 16))

		// S8: a self-loop keeps at least LoopMinH rows on the flow axis.
		// Under DOWN "ok" has no other reason to be this tall (its label
		// alone needs 3 cells); under RIGHT the flow axis is "ok"'s own
		// label width, which already exceeds LoopMinH, so the rule does
		// not bind there and is not asserted (see the task-8 fix report).
		if dir == model.Down {
			var ok model.PositionedNode
			for _, n := range pg.Nodes {
				if n.ID == "ok" {
					ok = n
				}
			}
			assert.Equal(t, cfg.LoopMinH*cellH, ok.Height, "%s: self-loop room (S8), scaled to px (S14)", dir)
		}

		// S14: the label's own position scales along with everything
		// else, so its box (LabelPos +/- half its now-scaled extents)
		// still sits inside the now-scaled canvas.
		require.NotNil(t, yes.LabelPos, "%s", dir)
		lowX, highX := yes.LabelPos.X-yes.LabelWidth/2, yes.LabelPos.X+yes.LabelWidth/2
		lowY, highY := yes.LabelPos.Y-yes.LabelHeight/2, yes.LabelPos.Y+yes.LabelHeight/2
		assert.GreaterOrEqual(t, lowX, 0.0, "%s: label box left edge inside the canvas", dir)
		assert.LessOrEqual(t, highX, pg.Width, "%s: label box right edge inside the canvas", dir)
		assert.GreaterOrEqual(t, lowY, 0.0, "%s: label box top edge inside the canvas", dir)
		assert.LessOrEqual(t, highY, pg.Height, "%s: label box bottom edge inside the canvas", dir)
	}
}

// TestScale_TheTextAdapterScalesEveryField is a direct test of the text
// adapter (S14) on a synthetic positioned graph: every geometry field it
// owns (a node's box, a record box's line heights, an edge's points, its
// label's position and extents, and both cardinalities' positions and
// extents, plus the canvas) scales by sx and sy. Going through Layout on a real fixture cannot pin this
// precisely, because wire-derived positions (ports, labels placed off a
// jog) are not themselves grid-aligned, only quantities the placement
// stage owns are (S7); this test isolates the adapter from that noise.
func TestScale_TheTextAdapterScalesEveryField(t *testing.T) {
	pg := &model.PositionedGraph{
		Nodes: []model.PositionedNode{
			{X: 1, Y: 2, Width: 3, Height: 4},
			{Members: &model.PositionedMembers{HeaderLineHeight: 1, MemberLineHeight: 2}},
		},
		Edges: []model.PositionedEdge{{
			Points:      []model.Point{{X: 1, Y: 1}, {X: 2, Y: 3}},
			LabelPos:    &model.Point{X: 4, Y: 5},
			LabelWidth:  6,
			LabelHeight: 7,
			FromCard:    &model.EndLabel{Pos: &model.Point{X: 8, Y: 9}, Width: 10, Height: 11},
			ToCard:      &model.EndLabel{Pos: &model.Point{X: 12, Y: 13}, Width: 14, Height: 15},
		}},
		Width:  16,
		Height: 17,
	}
	scale(pg, model.Down, 8, 16)

	assert.Equal(t, model.PositionedNode{X: 8, Y: 32, Width: 24, Height: 64}, pg.Nodes[0], "a node's box scales")
	m := pg.Nodes[1].Members
	assert.Equal(t, [2]float64{16, 32}, [2]float64{m.HeaderLineHeight, m.MemberLineHeight}, "a record box's line heights scale by the row")
	e := pg.Edges[0]
	assert.Equal(t, []model.Point{{X: 8, Y: 16}, {X: 16, Y: 48}}, e.Points, "an edge's points scale")
	assert.Equal(t, &model.Point{X: 32, Y: 80}, e.LabelPos, "a label's position scales")
	assert.Equal(t, [2]float64{48, 112}, [2]float64{e.LabelWidth, e.LabelHeight}, "a label's own extents scale")
	assert.Equal(t, &model.Point{X: 64, Y: 144}, e.FromCard.Pos, "a cardinality's position scales too, not just its extents")
	assert.Equal(t, [2]float64{80, 176}, [2]float64{e.FromCard.Width, e.FromCard.Height}, "a cardinality's extents scale")
	assert.Equal(t, &model.Point{X: 96, Y: 208}, e.ToCard.Pos, "the other cardinality's position scales")
	assert.Equal(t, [2]float64{112, 240}, [2]float64{e.ToCard.Width, e.ToCard.Height}, "the other cardinality's extents scale")
	assert.Equal(t, [2]float64{128, 272}, [2]float64{pg.Width, pg.Height}, "the canvas scales")
}

// TestLayout_TextAdapterScalesCardinalities confirms the S14 adapter's
// cardinality scaling (TestScale_TheTextAdapterScalesEveryField's rule)
// through the real pipeline, on a fixture where every wire the
// cardinalities sit on is straight, so their positions land on the
// half-cell grid rather than the thirds a slanted jog can produce (see
// TestLayout_TextAdapterScalesCellsToPixels's label-position check for a
// fixture where that is not true).
func TestLayout_TextAdapterScalesCardinalities(t *testing.T) {
	g := model.Graph{Direction: model.Down, Legend: &model.Legend{},
		Nodes: []model.Node{
			{ID: "animal", Label: "Animal", Members: &model.Members{Methods: []model.Member{{Visibility: "+", Text: "speak(): string"}}}},
			{ID: "dog", Label: "Dog", Members: &model.Members{}},
			{ID: "owner", Label: "Owner", Members: &model.Members{}},
		},
		Edges: []model.Edge{
			{ID: "dog->animal#0", From: "animal", To: "dog", Relation: model.RelationInheritance},
			{ID: "owner->dog#0", From: "owner", To: "dog", Relation: model.RelationComposition, Label: "owns", FromCard: "1", ToCard: "*"},
			{ID: "dog->owner#0", From: "dog", To: "owner", Relation: model.RelationAssociation, FromCard: "0..*", ToCard: "1"},
		},
	}
	pg, err := Layout(context.Background(), g, TextConfig(model.Down), nil)
	require.NoError(t, err)
	for _, id := range []string{"owner->dog#0", "dog->owner#0"} {
		e := edge(t, pg, id)
		for _, c := range []*model.EndLabel{e.FromCard, e.ToCard} {
			require.NotNil(t, c, "%s", id)
			require.NotNil(t, c.Pos, "%s", id)
			assert.GreaterOrEqual(t, c.Pos.X, 0.0, "%s: inside the canvas", id)
			assert.LessOrEqual(t, c.Pos.X, pg.Width, "%s: inside the canvas", id)
			assert.GreaterOrEqual(t, c.Pos.Y, 0.0, "%s: inside the canvas", id)
			assert.LessOrEqual(t, c.Pos.Y, pg.Height, "%s: inside the canvas", id)
			assert.Equal(t, 0.0, math.Mod(c.Pos.X, cellW/2), "%s: cardinality position on the half-cell grid (S14)", id)
			assert.Equal(t, 0.0, math.Mod(c.Pos.Y, cellH/2), "%s: cardinality position on the half-cell grid (S14)", id)
		}
	}
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

// TestLayout_FlowLoopTakesTwoAdjacentVertices pins S6's Loops kept
// together on the finished flow-loop, in every direction: the loop enters
// and leaves the diamond at two adjacent vertices, Process's exit and the
// back edge's return, and End's exit takes the vertex left over, the one
// opposite the loop's side vertex, outside the loop; nothing crosses.
// With End inside the loop the two loop ends took opposite vertices.
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

// lrPipeline is neat's lr-pipeline in direction dir.
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
// side-exit forcing on neat's lr-pipeline, in every direction: the back
// edge errors->extract does not pull Extract (a node with faces), and
// Valid?'s "yes" exit is its primary (Transform leads on to Warehouse,
// Error log only back), so the main line Source -> Extract -> Valid? ->
// Transform runs straight through the diamond; "no" leaves a side vertex
// as an L into Error log, whose in-face it shares with the back edge. The
// back edge's pull once put Extract 30 px off Valid?'s axis, and side-exit
// forcing judged by Error log's center packed both ends 10 px apart at one
// end of the face. Under RIGHT and LEFT the two ends sit at a third and
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
// (the room). Denied their primaries, as before the room, the lower
// decisions fanned out symmetrically and their main lines bent: 3 and 5
// bends in all, now 2 and 3. In the shape where z, the other source into
// end, sits in m's row, m keeps its primary too (4 bends under RIGHT and
// LEFT before, 3 now).
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
// the room dropped n1's primary and the layout took 12 bends; it takes 8,
// where denying both diamonds, as before the room, took 7.
func TestLayout_ARoomGivesWayToAForcing(t *testing.T) {
	specs := []string{"n0->n4", "n1:diamond", "n2:diamond", "n1->n4", "n1->n3", "n2->n4", "n2->n6", "n3->n7", "n6->n8"}
	for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
		g := graph(t, nil, specs...)
		g.Direction = dir
		var buf bytes.Buffer
		ctx := layoutdbg.NewContext(context.Background(), slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
		pg, err := Layout(ctx, g, screen(), nil)
		require.NoError(t, err)
		for _, v := range contract.Check(pg, contract.Options{Limits: contract.ScreenLimits(), Direction: dir, EdgeIDs: ids(g)}) {
			t.Errorf("%s %s %s: %s", dir, v.Rule, v.Subject, v.Detail)
		}
		log := buf.String()
		assert.NotContains(t, log, `"decision":"side_exit_dropped"`, "%s: no forcing drops", dir)
		room := strings.Index(log, `"decision":"secondary_room","phase":"place","module":"diago","spec_ref":"S7","node":"n2"`)
		denial := strings.Index(log, `"decision":"primary_denied","phase":"place","module":"diago","spec_ref":"S7","node":"n2"`)
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

// TestLayout_ARoomWaitsForItsPrimary pins S7's room on a diamond whose
// primary's forcing alignment dropped: the room's premise, the secondary's
// next vertex kept the node gap from the primary's under the diamond, does
// not hold, so the diamond gets no room and is denied its primary as
// before the room, which takes its primary out of the forced batch that
// could not be met. Given rooms anyway, d1 (and on the probe's decision
// seed 387 DOWN, n6 and n12) kept a batch no placement meets, every forcing
// in it dropped, and seed 387 took 59 bends where the denials take 50.
func TestLayout_ARoomWaitsForItsPrimary(t *testing.T) {
	small := graph(t, nil, "d1:diamond->t", "d1->s1", "d2:diamond->t", "d2->s1", "t->u")
	small.Direction = model.Down
	seed := decisionGraph(387)
	seed.Direction = model.Down
	for name, g := range map[string]model.Graph{"two decisions": small, "decision seed 387": seed} {
		var buf bytes.Buffer
		ctx := layoutdbg.NewContext(context.Background(), slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
		_, err := Layout(ctx, g, screen(), nil)
		require.NoError(t, err)
		// per pass (a placed record closes one): the diamonds whose primary's
		// forcing dropped, and the diamonds that got room
		dropped, roomed := map[string]bool{}, map[string]bool{}
		for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
			var rec map[string]any
			require.NoError(t, json.Unmarshal([]byte(line), &rec))
			switch rec["decision"] {
			case "side_exit_dropped":
				if rec["primary"] == true {
					from, _, _ := strings.Cut(rec["edge"].(string), "->")
					dropped[from] = true
				}
			case "secondary_room":
				roomed[rec["node"].(string)] = true
			case "placed":
				for d := range roomed {
					assert.False(t, dropped[d], "%s: %s got room while its primary's forcing dropped", name, d)
				}
				dropped, roomed = map[string]bool{}, map[string]bool{}
			}
		}
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
	g := decisionGraph(27)
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

// TestLayout_ACascadesMainLineHoldsWhateverItsLabels pins S7's room on the
// shape that broke a room measured before it (plan 6a's review): z, the
// other source into end, sits in m's row, and with every subset of the
// five edges labelled, in every direction, m keeps its primary straight
// on into main, s->m runs straight into m, and m->end is an L.
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
// the case needs. The graph before it, seed 2079's, made its case with a
// circle's exits, which now merge on its bottom vertex.)
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
// fan-out), where S7 used to force them out of its side vertices as Ls. A
// diamond with the same exits still spreads them over its vertices, which
// C8.5 forbids it to share. The layouts meet the whole contract.
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

// TestLayout_GitPushsExitsLeaveOnePoint pins S8's merged fan-out on the
// gallery review's issue 5, cicd-pipeline, in every direction: "git
// push"'s two exits leave its out-vertex (its right vertex under RIGHT)
// on one trunk and split into a bracket that enters Lint and Unit Tests,
// two bends each, as golayout drew it. They left its side vertices, and
// under RIGHT and LEFT each hooked back toward the CI group's anchored
// ports, which sit 86.6 px apart against side columns 150.7 px apart,
// three bends each.
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
// sides on the finished layout, in every direction and both profiles:
// the back edges d->a and e->b, whose layer spans interleave (0 to 3 and
// 1 to 4), run on opposite sides of c, the node both pass, and cross
// nothing; once both ran on one side and crossed. x->a, whose span nests
// in e->a's, keeps e->a's side, although S6 leaves the two a crossing a
// side move of x->a would take away (order's
// TestMinimize_NestedChainsKeepTheirSides).
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
// flow-network's final layout, DOWN (the gallery review's issue 4): api
// -> db's column starts between Redis Cache and Message Queue, so S7's
// straight chain lines Web, API and Database up in the middle of the
// canvas instead of at its right edge. Without the rule the column came
// last in the middle row, x 611 on a 681 px canvas; API sat 214.9 px off
// the midpoint of its fan (its four wires where they reach the middle
// row) and Database 144.2 px off the midpoint of its parents' wires. With
// it, 29.7 and 31.5 px: API's four ports spread over its face put the
// query wire's port 14 px right of its center, and Message Queue, a
// 218 px parallelogram, keeps its port 155 px right of the column,
// Redis Cache's 92 px left.
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
	assert.LessOrEqual(t, math.Abs(api.X-mid(fan...)), 30.0, "API over its fan: %v", fan)
	parents := mid(edge(t, pg, "cache->db#0").Points[0].X, edge(t, pg, "queue->db#0").Points[0].X, column)
	assert.LessOrEqual(t, math.Abs(db.X-parents), 32.0, "Database under its parents")
}

// ecommerceCheckout is the ecommerce-checkout spec: Clients
// feed an API Gateway, which fans out into the Services group, whose
// nodes reach PostgreSQL and Redis in the Data group and Stripe.
func ecommerceCheckout(dir model.Direction) model.Graph {
	return model.Graph{
		Direction: dir,
		Nodes: []model.Node{
			{ID: "web", Label: "Web Storefront", Shape: model.ShapeRounded}, {ID: "mobile", Label: "Mobile App", Shape: model.ShapeRounded},
			{ID: "gateway", Label: "API Gateway", Shape: model.ShapeHexagon},
			{ID: "catalog", Label: "Catalog Service", Shape: model.ShapeRect}, {ID: "cart", Label: "Cart Service", Shape: model.ShapeRect},
			{ID: "orders", Label: "Order Service", Shape: model.ShapeRect}, {ID: "payments", Label: "Payment Service", Shape: model.ShapeRect},
			{ID: "stripe", Label: "Stripe", Shape: model.ShapeRounded},
			{ID: "postgres", Label: "PostgreSQL", Shape: model.ShapeCylinder}, {ID: "redis", Label: "Redis", Shape: model.ShapeCylinder},
		},
		Edges: []model.Edge{
			{ID: "web->gateway#0", From: "web", To: "gateway", Label: "HTTPS"},
			{ID: "mobile->gateway#0", From: "mobile", To: "gateway", Label: "HTTPS"},
			{ID: "gateway->catalog#0", From: "gateway", To: "catalog"},
			{ID: "gateway->cart#0", From: "gateway", To: "cart"},
			{ID: "gateway->orders#0", From: "gateway", To: "orders"},
			{ID: "orders->payments#0", From: "orders", To: "payments", Label: "charge"},
			{ID: "payments->stripe#0", From: "payments", To: "stripe", Label: "API"},
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
// final layout of ecommerce-checkout, DOWN (the gallery review's issue
// 6): in the Services level, Order Service's part (its entry, Payment
// Service, Stripe's and PostgreSQL's wires) shares no edge with Catalog
// Service's or Cart Service's, and act 1's reference drifted it 60.6 px
// away from Catalog Service, a 780.3 px box. Packed, the box is at most
// 720 px wide, and the row keeps its order.
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

// TestLayout_ASideColumnKeepsReachWhileAWireStraightens pins S8's stops on
// the finished layout, in every direction and both profiles: a side column
// moves to straighten a wire only where it stays Reach outside its side,
// however near that x. Under DOWN and UP, diamond n5's exit to n6 leaves
// its left vertex, and placement leaves n6's in-port 2.67 px inside the
// column Reach out: the wire runs as an L on the column, n6's port coming
// onto it, and its stub out of the vertex keeps C7's floor, where the
// column taking the port's x left 17.33 px. (The random-graph probe's
// seed 76 of its decisions mode, found by a search with that check
// undone. The graph before it lost the case when S7 forced the exits of a
// diamond with a back edge in the second pass, and the one before that
// when S7 stopped letting a back edge pull a node with faces.)
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

func TestLayout_ASideColumnKeepsItsReach(t *testing.T) {
	// n1->n0#1 is reversed and leaves cylinder n1 at a side face. Under
	// DOWN and UP the dummy column it runs along lies 19.80 px out from
	// that side, 0.20 px inside Reach; a snap of its column toward the
	// dummy would leave a 19.80 px stub (C7), so the dummy moves to the
	// column instead (S8, Stops). (A random graph, found by a search with
	// the column's give-way undone; it needs every node and edge. The
	// fixtures before it lost the case: neat's m-deploy when its diamond's
	// label went onto two lines (S1, Compact labels), and another random
	// graph when S7 stopped letting a back edge pull a node with faces.)
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

// TestLayout_ACrowdedFaceKeepsItsPortsApart pins S8's Shape ports on the
// finished screen layout, for every shape with faces and every direction:
// hub's twenty exits and the heads of ten back edges into it share its
// out-face (two of those may attach at its sides), and every two ports
// on one of its faces lie at least a rake's InLaneGap / 2 apart across
// the flow (C9.2). Spread over its size as S1 measures it, a face of 40
// px would put them 1.3 px apart. The layouts meet the whole contract.
func TestLayout_ACrowdedFaceKeepsItsPortsApart(t *testing.T) {
	var back []string
	for i := range 10 {
		back = append(back, fmt.Sprintf("l%d->hub", i))
	}
	for _, shape := range []string{"", "rounded", "hexagon", "parallelogram", "cylinder"} {
		for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
			g := crowded(t, shape, 20, back...)
			g.Direction = dir
			pg := lay(t, g, screen(), dir)
			hub := positioned(t, pg, "hub")
			center, _ := flowAxis(model.Point{X: hub.X, Y: hub.Y}, dir)
			faces := map[bool][]float64{} // by whether the face lies downstream of the center
			for _, e := range pg.Edges {
				for _, end := range []struct {
					node string
					p    model.Point
				}{{e.From, e.Points[0]}, {e.To, e.Points[len(e.Points)-1]}} {
					along, across := flowAxis(end.p, dir)
					if end.node != "hub" || math.Abs(along-center) < 1 {
						continue // another node's end, or one attached at a side, at mid-height
					}
					down := along > center
					faces[down] = append(faces[down], across)
				}
			}
			require.GreaterOrEqual(t, len(faces[true])+len(faces[false]), 28, "%s %q: hub's ports", dir, shape)
			for _, ports := range faces {
				slices.Sort(ports)
				for i := 1; i < len(ports); i++ {
					assert.GreaterOrEqual(t, ports[i]-ports[i-1], 4-1e-6, "%s %q: ports %d and %d of a face: %v", dir, shape, i-1, i, ports)
				}
			}
		}
	}
}

func TestLayout_TextPortsGetTheirOwnCells(t *testing.T) {
	g := fanOut(t, 10, "l0->m", "l1->m", "l2->m")
	for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
		t.Run(dir.String(), func(t *testing.T) {
			g.Direction = dir
			pg, err := Layout(context.Background(), g, TextConfig(dir), nil)
			require.NoError(t, err)
			for _, v := range contract.Check(pg, contract.Options{Limits: contract.TextLimits(dir), Direction: dir, EdgeIDs: ids(g), Text: true}) {
				t.Errorf("%s %s: %s", v.Rule, v.Subject, v.Detail)
			}
			// the cell a port lies in, across its face: a column under DOWN
			// and UP, a row under RIGHT and LEFT
			cell := func(p model.Point) int { return int(math.Floor(p.X / 8)) }
			lo := func(n model.PositionedNode) int { return int((n.X - n.Width/2) / 8) }
			hi := func(n model.PositionedNode) int { return int((n.X+n.Width/2)/8) - 1 }
			if dir == model.Right || dir == model.Left {
				cell = func(p model.Point) int { return int(math.Floor(p.Y / 16)) }
				lo = func(n model.PositionedNode) int { return int((n.Y - n.Height/2) / 16) }
				hi = func(n model.PositionedNode) int { return int((n.Y+n.Height/2)/16) - 1 }
			}
			for _, face := range []struct {
				node string
				end  func(model.PositionedEdge) (string, model.Point)
			}{
				{"hub", func(e model.PositionedEdge) (string, model.Point) { return e.From, e.Points[0] }},
				{"m", func(e model.PositionedEdge) (string, model.Point) { return e.To, e.Points[len(e.Points)-1] }},
			} {
				n := pg.Nodes[slices.IndexFunc(pg.Nodes, func(n model.PositionedNode) bool { return n.ID == face.node })]
				seen := map[int]bool{}
				for _, e := range pg.Edges {
					id, p := face.end(e)
					if id != face.node {
						continue
					}
					c := cell(p)
					assert.False(t, seen[c], "%s: two ports on cell %d", face.node, c)
					seen[c] = true
					assert.Greater(t, c, lo(n), "%s: a port inside the corner", face.node)
					assert.Less(t, c, hi(n), "%s: a port inside the corner", face.node)
					if dir == model.Down || dir == model.Up {
						assert.Equal(t, 4.0, math.Mod(p.X, 8), "%s: port not centered on cell %d", face.node, c)
					} else {
						assert.Equal(t, 8.0, math.Mod(p.Y, 16), "%s: port not centered on cell %d", face.node, c)
					}
				}
			}
		})
	}
}

func TestLayout_TextLanesSitOnTheCellGrid(t *testing.T) {
	// three jogs share the channel below r, one lane each (S8); a lane is
	// a run across the flow: vertical under RIGHT and LEFT, horizontal
	// under DOWN and UP. Every one lies on a cell boundary, where the text
	// renderer reads it.
	g := graph(t, nil, "r->a", "r->b", "r->c", "a->d", "c->d")
	for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
		g.Direction = dir
		pg, err := Layout(context.Background(), g, TextConfig(dir), nil)
		require.NoError(t, err)
		lanes := 0
		for _, e := range pg.Edges {
			for i := 1; i+2 < len(e.Points); i++ {
				p, q := e.Points[i], e.Points[i+1]
				if dir == model.Right || dir == model.Left {
					require.Equal(t, p.X, q.X, "%s %s: an interior run crosses the flow", dir, e.ID)
					assert.Equal(t, 0.0, math.Mod(p.X, 8), "%s %s: a lane on a column boundary", dir, e.ID)
				} else {
					require.Equal(t, p.Y, q.Y, "%s %s: an interior run crosses the flow", dir, e.ID)
					assert.Equal(t, 0.0, math.Mod(p.Y, 16), "%s %s: a lane on a row boundary", dir, e.ID)
				}
				lanes++
			}
		}
		assert.Positive(t, lanes, "%s: the fixture has lanes", dir)
	}
}

// drawnAt is the text a render's art shows on n cells from the cell where
// the text renderer starts a label centered on pos, w px wide: the cell
// holding its left side and its middle row.
func drawnAt(res textrender.Result, pos model.Point, w float64, n int) string {
	lines := strings.Split(res.Art, "\n")
	row, col := int(math.Floor(pos.Y/cellH))-res.TopRow, int(math.Floor((pos.X-w/2)/cellW))
	if row < 0 || row >= len(lines) || col < 0 {
		return ""
	}
	rs := []rune(lines[row])
	return string(rs[min(col, len(rs)):min(col+n, len(rs))])
}

func TestLayout_TextLabelsAreDrawnOnTheirBoxes(t *testing.T) {
	// web and cli, in a titled group, feed api above them under UP
	// (the flow-client-server example, pared down). web's label once sat
	// beside the middle of its wire's first run, a box straddling the
	// group's top border row and the row above it: the text renderer
	// started it on the border row, found the
	// frame there and drew it a row up. In the text profile a label's box
	// is whole cells, clear of every cell drawn there (S10), so the
	// renderer draws the label on it (C14.5); it now takes the row next to
	// its wire's lane, left of the wire's last run
	g := model.Graph{Direction: model.Up,
		Nodes: []model.Node{
			{ID: "web", Label: "Web", Shape: model.ShapeRect}, {ID: "cli", Label: "CLI", Shape: model.ShapeRect},
			{ID: "api", Label: "API", Shape: model.ShapeRect},
		},
		Edges:  []model.Edge{{ID: "web->api#0", From: "web", To: "api", Label: "HTTPS"}, {ID: "cli->api#0", From: "cli", To: "api"}},
		Groups: []model.Group{{ID: "clients", Label: "Clients", Contains: []string{"web", "cli"}}},
	}
	pg, err := Layout(context.Background(), g, TextConfig(model.Up), nil)
	require.NoError(t, err)
	web := edge(t, pg, "web->api#0")
	require.NotNil(t, web.LabelPos)
	assert.False(t, web.LabelUnresolved)
	left, top := web.LabelPos.X-web.LabelWidth/2, web.LabelPos.Y-web.LabelHeight/2
	assert.Equal(t, [2]float64{8 * cellW, 5 * cellH}, [2]float64{left, top}, "columns 8..12 of row 5, the row above the lane, a blank column left of the last run")
	res, err := textrender.Render(pg)
	require.NoError(t, err)
	assert.Equal(t, "HTTPS", drawnAt(res, *web.LabelPos, web.LabelWidth, 5), "drawn on its box")
}

func TestLayout_ClientServerTextLabelsSitByTheirWires(t *testing.T) {
	// under RIGHT and LEFT the gap between the two frames is narrower than
	// "HTTPS" and its blanks: web's and cli's labels once climbed out of
	// the frames, four rows from their wires, and "/assets/*" left
	// Services below its bottom side. A text label now takes the row next
	// to a wire along its row, covers a frame side that crosses its row
	// when its ring holds no clear spot, and stays in its edge's group
	// (S10)
	for _, dir := range []model.Direction{model.Right, model.Left} {
		t.Run(dir.String(), func(t *testing.T) {
			pg := layText(t, clientServer(dir))
			var services model.PositionedGroup
			for _, gr := range pg.Groups {
				if gr.ID == "services" {
					services = gr
				}
			}
			for _, id := range []string{"web->proxy#0", "cli->proxy#0", "proxy->assets#0"} {
				e := edge(t, pg, id)
				require.NotNil(t, e.LabelPos, id)
				assert.False(t, e.LabelUnresolved, id)
				b := [4]float64{e.LabelPos.X - e.LabelWidth/2, e.LabelPos.Y - e.LabelHeight/2, e.LabelPos.X + e.LabelWidth/2, e.LabelPos.Y + e.LabelHeight/2}
				assert.InDelta(t, cellH/2, gapTo(b, e.Points), 1e-6, "%s: on the row next to its wire", id)
			}
			e := edge(t, pg, "proxy->assets#0")
			assert.True(t, e.LabelPos.X-e.LabelWidth/2 > services.X && e.LabelPos.X+e.LabelWidth/2 < services.X+services.Width &&
				e.LabelPos.Y-e.LabelHeight/2 > services.Y && e.LabelPos.Y+e.LabelHeight/2 < services.Y+services.Height,
				"/assets/* inside Services")
		})
	}
}

func TestHomes_PicksTheInnermostGroupHoldingBothEnds(t *testing.T) {
	// depth as the groups carry it: 1 top-level, 2 nested. outer holds
	// inner (a, b) and the node c directly; other holds d (S10)
	outer := model.PositionedGroup{ID: "outer", Contains: []string{"c"}, Children: []string{"inner"}, Depth: 1}
	inner := model.PositionedGroup{ID: "inner", Contains: []string{"a", "b"}, Depth: 2}
	other := model.PositionedGroup{ID: "other", Contains: []string{"d"}, Depth: 1}
	for _, tc := range []struct {
		name   string
		groups []model.PositionedGroup
		a, b   string
		want   string // the group's id, "" for none
	}{
		{"both in inner, outer listed first", []model.PositionedGroup{outer, inner, other}, "a", "b", "inner"},
		{"both in inner, outer listed second", []model.PositionedGroup{inner, outer, other}, "a", "b", "inner"},
		{"one in inner, one in outer, outer first", []model.PositionedGroup{outer, inner, other}, "a", "c", "outer"},
		{"one in inner, one in outer, outer second", []model.PositionedGroup{inner, outer, other}, "a", "c", "outer"},
		{"ends in no common group", []model.PositionedGroup{outer, inner, other}, "a", "d", ""},
		{"an end in no group", []model.PositionedGroup{outer, inner, other}, "a", "z", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			k, ok := homes(tc.groups)(tc.a, tc.b)
			if tc.want == "" {
				assert.False(t, ok)
				return
			}
			require.True(t, ok)
			assert.Equal(t, tc.want, tc.groups[k].ID)
		})
	}
}

func TestLayout_TextANestedGroupsLabelStaysInsideTheInnerFrame(t *testing.T) {
	// a and b lie in inner, nested in outer beside the node c; the edge
	// a->b has both ends in inner, so in text its label stays inside
	// inner's frame, one cell clear of each side, resolved (S10)
	for _, dir := range []model.Direction{model.Down, model.Right} {
		t.Run(dir.String(), func(t *testing.T) {
			g := graph(t, nil, "a->b", "c->a", "c->b")
			g.Direction = dir
			g.Edges[0].Label = "long label"
			g.Groups = []model.Group{
				{ID: "outer", Label: "Outer", Contains: []string{"c"}, Children: []string{"inner"}, Depth: 1},
				{ID: "inner", Label: "Inner", Contains: []string{"a", "b"}, Depth: 2},
			}
			pg := layText(t, g)
			e := edge(t, pg, g.Edges[0].ID)
			require.NotNil(t, e.LabelPos)
			assert.False(t, e.LabelUnresolved)
			in := group(t, pg, "inner")
			assert.GreaterOrEqual(t, e.LabelPos.X-e.LabelWidth/2, in.X+cellW, "left")
			assert.LessOrEqual(t, e.LabelPos.X+e.LabelWidth/2, in.X+in.Width-cellW, "right")
			assert.GreaterOrEqual(t, e.LabelPos.Y-e.LabelHeight/2, in.Y+cellH, "top")
			assert.LessOrEqual(t, e.LabelPos.Y+e.LabelHeight/2, in.Y+in.Height-cellH, "bottom")
		})
	}
}

// gapTo is the distance from box b, left/top/right/bottom, to the nearest
// segment of route.
func gapTo(b [4]float64, route []model.Point) float64 {
	d := math.Inf(1)
	for i := 0; i+1 < len(route); i++ {
		p, q := route[i], route[i+1]
		dx := max(b[0]-max(p.X, q.X), min(p.X, q.X)-b[2], 0)
		dy := max(b[1]-max(p.Y, q.Y), min(p.Y, q.Y)-b[3], 0)
		d = min(d, math.Hypot(dx, dy))
	}
	return d
}

// nearerOther is how much nearer than its own wire the nearest wire of
// another edge lies to box b, a label of edge i in pg: positive when a
// reader would take the label for the other edge's (S10).
func nearerOther(pg *model.PositionedGraph, i int, b [4]float64) float64 {
	other := math.Inf(1)
	for j, e := range pg.Edges {
		if j != i {
			other = min(other, gapTo(b, e.Points))
		}
	}
	return gapTo(b, pg.Edges[i].Points) - other
}

func TestLayout_ASidewaysFanOutsLabelsSitBesideTheirWires(t *testing.T) {
	// gw fans out to four services under RIGHT and LEFT, each wire
	// labelled. A label's length lies along the flow there, longer than
	// any run between the two layers, so every label once fell back to
	// its least overlap. Each now slides beside its own wire, past a turn
	// or beside a jog, clear, within the eight rings of the slide, where
	// no other wire lies nearer: "/billing" once slid to 12 from its own
	// wire and 4 from search's, and now sits under billing's last run
	// (S10)
	for _, dir := range []model.Direction{model.Right, model.Left} {
		t.Run(dir.String(), func(t *testing.T) {
			g := graph(t, nil, "gw->users", "gw->orders", "gw->billing", "gw->search")
			g.Direction = dir
			for i := range g.Edges {
				g.Edges[i].Label = "/" + g.Edges[i].To
			}
			cfg := screen()
			pg := lay(t, g, cfg, dir)
			for i, e := range pg.Edges {
				require.NotNil(t, e.LabelPos, e.ID)
				assert.False(t, e.LabelUnresolved, "%s: unresolved", e.ID)
				b := [4]float64{e.LabelPos.X - e.LabelWidth/2, e.LabelPos.Y - e.LabelHeight/2, e.LabelPos.X + e.LabelWidth/2, e.LabelPos.Y + e.LabelHeight/2}
				assert.LessOrEqual(t, gapTo(b, e.Points), 8*cfg.LabelGap, "%s: beside its own wire", e.ID)
				assert.LessOrEqual(t, nearerOther(pg, i, b), cfg.OwnSlack, "%s: another wire lies nearer", e.ID)
			}
		})
	}
}

// clientServer is the flow-client-server example under dir: three clients in a group feed a reverse proxy in another,
// which fans out to three services, two of them backed by one database.
func clientServer(dir model.Direction) model.Graph {
	return model.Graph{
		Direction: dir,
		Nodes: []model.Node{
			{ID: "web", Label: "Web Client", Shape: model.ShapeRounded}, {ID: "mobile", Label: "Mobile Client", Shape: model.ShapeRounded},
			{ID: "cli", Label: "CLI Client", Shape: model.ShapeRounded}, {ID: "proxy", Label: "Reverse Proxy", Shape: model.ShapeHexagon},
			{ID: "server", Label: "App Server", Shape: model.ShapeRect}, {ID: "auth", Label: "Auth Service", Shape: model.ShapeRect},
			{ID: "assets", Label: "Assets Service", Shape: model.ShapeRect}, {ID: "db", Label: "Database", Shape: model.ShapeCylinder},
		},
		Edges: []model.Edge{
			{ID: "web->proxy#0", From: "web", To: "proxy", Label: "HTTPS"},
			{ID: "mobile->proxy#0", From: "mobile", To: "proxy", Label: "HTTPS"},
			{ID: "cli->proxy#0", From: "cli", To: "proxy", Label: "HTTPS"},
			{ID: "proxy->server#0", From: "proxy", To: "server", Label: "/api/*", Direction: model.EdgeBoth},
			{ID: "proxy->auth#0", From: "proxy", To: "auth", Label: "/auth/*", Direction: model.EdgeBoth},
			{ID: "proxy->assets#0", From: "proxy", To: "assets", Label: "/assets/*", Direction: model.EdgeBoth},
			{ID: "server->db#0", From: "server", To: "db", Label: "query/result", Direction: model.EdgeBoth},
			{ID: "auth->db#0", From: "auth", To: "db", Label: "query/result", Direction: model.EdgeBoth},
		},
		Groups: []model.Group{
			{ID: "clients", Label: "Clients", Contains: []string{"web", "mobile", "cli"}},
			{ID: "services", Label: "Services", Contains: []string{"proxy", "server", "auth", "assets", "db"}},
		},
	}
}

func TestLayout_ClientServerSidewaysLabelsNameTheirOwnWires(t *testing.T) {
	// flow-client-server under RIGHT and LEFT: "/auth/*" once slid 24 from
	// its own wire and 3.6 from proxy->assets', reading as that edge's. A
	// label resolved sits where no other wire lies nearer than its own,
	// beyond the slack, or falls back, marked unresolved (S10); only
	// proxy->auth's falls back, so the check covers every other label
	for _, dir := range []model.Direction{model.Right, model.Left} {
		t.Run(dir.String(), func(t *testing.T) {
			g := clientServer(dir)
			cfg := screen()
			pg := lay(t, g, cfg, dir)
			var unresolved []string
			for i, e := range pg.Edges {
				require.NotNil(t, e.LabelPos, e.ID)
				if e.LabelUnresolved {
					unresolved = append(unresolved, e.ID)
					continue
				}
				b := [4]float64{e.LabelPos.X - e.LabelWidth/2, e.LabelPos.Y - e.LabelHeight/2, e.LabelPos.X + e.LabelWidth/2, e.LabelPos.Y + e.LabelHeight/2}
				assert.LessOrEqual(t, nearerOther(pg, i, b), cfg.OwnSlack, "%s: another wire lies nearer", e.ID)
			}
			assert.Equal(t, []string{"proxy->auth#0"}, unresolved, "the labels that fall back")
		})
	}
}

// layText lays g out under the text profile and checks the result against
// the contract's text limits.
func layText(t *testing.T, g model.Graph) *model.PositionedGraph {
	t.Helper()
	pg, err := Layout(context.Background(), g, TextConfig(g.Direction), nil)
	require.NoError(t, err)
	for _, v := range contract.Check(pg, contract.Options{Limits: contract.TextLimits(g.Direction), Direction: g.Direction, EdgeIDs: ids(g), Text: true}) {
		t.Errorf("text: %s %s: %s", v.Rule, v.Subject, v.Detail)
	}
	return pg
}

func TestLayout_ClientServerTextQueryResultWiresMirror(t *testing.T) {
	// server and auth sit either side of db, and under RIGHT and LEFT
	// their wires into db jog exactly the in-lane gap apart in text: they
	// once took two lanes, auth's turning two columns past server's. They
	// share one lane, so both turn at one flow coordinate and mirror each
	// other about db's center across the flow (S8 Lanes). Under DOWN and
	// UP db is an even number of columns wide: its two ports fall on the
	// lines between cells and take the cells farther from its middle, so
	// they still mirror about its center (S8 Shape ports), while server's
	// and auth's lone ports each take the cell right of their middle line
	for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
		t.Run(dir.String(), func(t *testing.T) {
			pg := layText(t, clientServer(dir))
			s, a := edge(t, pg, "server->db#0").Points, edge(t, pg, "auth->db#0").Points
			require.Len(t, s, 4)
			require.Len(t, a, 4)
			var db model.PositionedNode
			for _, n := range pg.Nodes {
				if n.ID == "db" {
					db = n
				}
			}
			flow := func(p model.Point) float64 { along, _ := flowAxis(p, dir); return along }
			across := func(p model.Point) float64 { _, c := flowAxis(p, dir); return c }
			center := across(model.Point{X: db.X, Y: db.Y})
			assert.InDelta(t, flow(s[1]), flow(a[1]), 1e-6, "one lane")
			assert.InDelta(t, center, (across(s[3])+across(a[3]))/2, 1e-6, "ports mirrored about db")
			if dir == model.Down || dir == model.Up {
				return
			}
			assert.InDelta(t, center, (across(s[0])+across(a[0]))/2, 1e-6, "sources mirrored about db")
		})
	}
}

func TestLayout_ASidewaysCardinalityInACrowdFollowsItsRoute(t *testing.T) {
	// fleet, depot and chargeable relate to truck under RIGHT
	// (the class-relations example, pared down).
	// depot's wire turns into truck on a last run 24 px long, 16 px below
	// fleet's wire: every spot one to three steps from its end meets
	// truck, a wire or fleet's cardinality, placed first, so depot's
	// "0..*" once fell back to its least overlap. It follows its route
	// around the turn, clear, within four rings of it (S10). Every
	// cardinality resolved lies nearer its own wire than another's, beyond
	// the slack; fleet's "1..*", between chargeable's wire and depot's 16
	// px either side of its own, finds no such spot and falls back
	record := func(id, label string, attrs ...string) model.Node {
		m := &model.Members{}
		for _, a := range attrs {
			m.Attributes = append(m.Attributes, model.Member{Visibility: "-", Text: a})
		}
		return model.Node{ID: id, Label: label, Members: m}
	}
	g := model.Graph{Direction: model.Right,
		Nodes: []model.Node{record("truck", "Truck", "payload: t"), record("chargeable", "Chargeable"), record("fleet", "Fleet", "name: string"), record("depot", "Depot")},
		Edges: []model.Edge{
			{ID: "truck->chargeable#0", From: "chargeable", To: "truck", Relation: model.RelationRealization},
			{ID: "fleet->truck#0", From: "fleet", To: "truck", Relation: model.RelationComposition, FromCard: "1", ToCard: "1..*"},
			{ID: "depot->truck#0", From: "depot", To: "truck", Relation: model.RelationAggregation, ToCard: "0..*"},
		},
	}
	cfg := screen()
	pg := lay(t, g, cfg, model.Right)
	for i, e := range pg.Edges {
		for _, c := range []*model.EndLabel{e.FromCard, e.ToCard} {
			if c == nil {
				continue
			}
			require.NotNil(t, c.Pos, "%s %s", e.ID, c.Text)
			if e.ID == "fleet->truck#0" && c == e.ToCard {
				assert.True(t, c.Unresolved, "%s %s: no spot of its own", e.ID, c.Text)
				continue
			}
			assert.False(t, c.Unresolved, "%s %s: unresolved", e.ID, c.Text)
			b := [4]float64{c.Pos.X - c.Width/2, c.Pos.Y - c.Height/2, c.Pos.X + c.Width/2, c.Pos.Y + c.Height/2}
			assert.LessOrEqual(t, gapTo(b, e.Points), 4*cfg.CardStep, "%s %s: beside its own wire", e.ID, c.Text)
			assert.LessOrEqual(t, nearerOther(pg, i, b), cfg.OwnSlack, "%s %s: another wire lies nearer", e.ID, c.Text)
		}
	}
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
	var buf bytes.Buffer
	ctx := layoutdbg.NewContext(context.Background(), slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	pg, err := Layout(ctx, g, screen(), nil)
	require.NoError(t, err)
	rooms, denied, restarts := 0, 0, 0.0
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var rec map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &rec))
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

// BenchmarkLayout_DenseLabelledGraph is the record of the layout's time on
// dense labelled graphs, which the corpus lacks: at 40 nodes and 200 edges
// it once took 13.6 s, nearly all in S10's ladders, where golayout took
// 6.2 s. The bipartite graphs of rectangles run neither S6's loop
// transposition nor S7's side-exit forcing: the pinned case, forward
// edges between diamonds, circles and rectangles, runs S7's forcing and
// its recheck, and the cyclic case S6's loop transposition. The cascade
// case, 30 cascades of 8 decisions side by side, reaches S7's rooms for
// blocked secondaries: denying those diamonds their primaries instead,
// one restart per denied diamond took it to 1.09 s, and a round that
// denied every diamond it found blocked at once to 35 ms; the rooms, one
// more round and no restart, take 52 ms. It is not a gate.
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

// TestScale_Groups pins the text adapter (S14) for groups: a group's box,
// its title extents and its title offset scale from cells to px like
// every other box.
func TestScale_Groups(t *testing.T) {
	pg := &model.PositionedGraph{Groups: []model.PositionedGroup{{ID: "g", X: 2, Y: 1, Width: 20, Height: 6, LabelWidth: 5, LabelHeight: 1, LabelOffset: 3}}}
	scale(pg, model.Down, 8, 16)
	g := pg.Groups[0]
	assert.Equal(t, [7]float64{16, 16, 160, 96, 40, 16, 24}, [7]float64{g.X, g.Y, g.Width, g.Height, g.LabelWidth, g.LabelHeight, g.LabelOffset})
}

// invertOutputPoint maps a point in the OUTPUT frame back to the engine's,
// the exact inverse of frame.Apply's map (S11): DOWN keeps it, UP negates
// y, RIGHT swaps the axes, LEFT swaps them and negates the one that lands
// on x.
func invertOutputPoint(dir model.Direction, x, y float64) (float64, float64) {
	switch dir {
	case model.Up:
		return x, -y
	case model.Right:
		return y, x
	case model.Left:
		return y, -x
	}
	return x, y
}

// invertOutputBox maps an output-frame box (top-left x, y, and its width
// and height) back to an engine-frame box: it inverts two opposite
// corners and takes the low corner and the span on each axis, since a
// direction can flip an axis or swap width and height (frame.Apply, S11).
func invertOutputBox(dir model.Direction, x, y, w, h float64) (ex, ey, ew, eh float64) {
	x0, y0 := invertOutputPoint(dir, x, y)
	x1, y1 := invertOutputPoint(dir, x+w, y+h)
	return math.Min(x0, x1), math.Min(y0, y1), math.Abs(x1 - x0), math.Abs(y1 - y0)
}

// TestPlaceTitles_EveryDirectionMapsTheBand pins placeTitles' band mapping
// (S10) in every direction and both profiles. Nothing else in the suite
// crosses a title band under RIGHT or LEFT, so today swapping RIGHT's and
// LEFT's u, flipping their v, or dropping "width = gr.Height" for a
// sideways direction would still pass every other test.
//
// The scene is described in the OUTPUT frame: one titled group at the
// origin and one vertical wire crossing the group's top side inside the
// left slot, so the left slot is blocked and the title must take the
// right slot, Width - TitleInset - LabelWidth from the group's left side.
// It is inverted into the engine's frame with invertOutputBox and
// invertOutputPoint, run through placeTitles and frame.Apply (and the
// text adapter, in text), and the result is read back in the output
// frame, where it started.
func TestPlaceTitles_EveryDirectionMapsTheBand(t *testing.T) {
	for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
		for _, profile := range []string{"screen", "text"} {
			t.Run(dir.String()+"/"+profile, func(t *testing.T) {
				cfg := screen()
				w, h, lw, lh := 200.0, 120.0, 40.0, 14.0
				wireTop, wireBottom := -20.0, 40.0
				if profile == "text" {
					cfg = TextConfig(dir)
					w, h, lw, lh = 25, 8, 5, 1
					wireTop, wireBottom = -2, 3
				}
				wireU := cfg.TitleInset + 2 // a few units into the left slot

				gx, gy, gw, gh := invertOutputBox(dir, 0, 0, w, h)
				p0x, p0y := invertOutputPoint(dir, wireU, wireTop)
				p1x, p1y := invertOutputPoint(dir, wireU, wireBottom)
				pg := &model.PositionedGraph{
					Groups: []model.PositionedGroup{{ID: "g", Label: "G", X: gx, Y: gy, Width: gw, Height: gh, LabelWidth: lw, LabelHeight: lh}},
					Edges:  []model.PositionedEdge{{ID: "w1", From: "a", To: "b", Points: []model.Point{{X: p0x, Y: p0y}, {X: p1x, Y: p1y}}}},
				}

				placeTitles(context.Background(), dir, cfg, pg)
				frame.Apply(context.Background(), pg, dir, cfg.MarginX, cfg.MarginY, cfg.Text)
				if cfg.Text {
					scale(pg, dir, cellW, cellH)
				}

				want := w - cfg.TitleInset - lw
				if cfg.Text {
					want *= cellW
				}
				assert.InDelta(t, want, pg.Groups[0].LabelOffset, 1e-6, "the right slot, from the group's left side")

				lim := contract.ScreenLimits()
				if cfg.Text {
					lim = contract.TextLimits(dir)
				}
				var c13 []contract.Violation
				for _, v := range contract.Check(pg, contract.Options{Limits: lim, Direction: dir, Text: cfg.Text}) {
					if v.Rule == "C13" {
						c13 = append(c13, v)
					}
				}
				assert.Empty(t, c13, "the wire keeps clear of the placed title")
			})
		}
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

func TestLayout_ClientServerTextCrossesTheBorderStraight(t *testing.T) {
	// under RIGHT and LEFT the proxy's three in-ports sit one row apart in
	// text. The Services level's terminals once kept the lane gap, two
	// rows, between them, three between their centers, so web's and
	// cli's wires jogged again inside Services: four bends each. With the
	// terminal gap, one row, each terminal sits on its port, and the
	// wires jog only between the frames, as on screen (S7, S9, S14)
	for _, dir := range []model.Direction{model.Right, model.Left} {
		t.Run(dir.String(), func(t *testing.T) {
			pg := layText(t, clientServer(dir))
			for _, id := range []string{"web->proxy#0", "cli->proxy#0"} {
				assert.Len(t, edge(t, pg, id).Points, 4, "%s: two bends", id)
			}
			assert.Len(t, edge(t, pg, "mobile->proxy#0").Points, 2, "mobile: straight")
		})
	}
}

func TestLayout_TextPortsMirrorAboutTheirFace(t *testing.T) {
	// The flow-loop example pared down, under RIGHT: the loop back from
	// inc counts on the decision's out-face, so
	// the decision is five rows tall, and its two exits leave its right
	// face. The face's two thirds fall on the lines between rows; each port
	// takes the row farther from the middle, one above it and one below,
	// mirrored about the decision's center (S8 Shape ports). They once took
	// the middle row and the row below it
	g := model.Graph{Direction: model.Right,
		Nodes: []model.Node{
			{ID: "init", Label: "i = 0", Shape: model.ShapeRect},
			{ID: "check", Label: "i < array.length?", Shape: model.ShapeDiamond},
			{ID: "process", Label: "Process array[i]", Shape: model.ShapeRect},
			{ID: "done", Label: "End", Shape: model.ShapeRounded}, {ID: "inc", Label: "i++", Shape: model.ShapeRect},
		},
		Edges: []model.Edge{
			{ID: "init->check#0", From: "init", To: "check"},
			{ID: "check->process#0", From: "check", To: "process", Label: "true"},
			{ID: "check->done#0", From: "check", To: "done", Label: "false"},
			{ID: "process->inc#0", From: "process", To: "inc"},
			{ID: "inc->check#0", From: "inc", To: "check"},
		},
	}
	pg := layText(t, g)
	var check model.PositionedNode
	for _, n := range pg.Nodes {
		if n.ID == "check" {
			check = n
		}
	}
	require.InDelta(t, 5*cellH, check.Height, 1e-6, "five rows")
	p, d := edge(t, pg, "check->process#0").Points[0], edge(t, pg, "check->done#0").Points[0]
	assert.InDelta(t, check.X+check.Width/2, p.X, 1e-6, "process's exit on the right face")
	assert.InDelta(t, check.X+check.Width/2, d.X, 1e-6, "done's exit on the right face")
	assert.InDelta(t, check.Y, (p.Y+d.Y)/2, 1e-6, "the exits mirror about the center row")
	assert.InDelta(t, 2*cellH, math.Abs(p.Y-d.Y), 1e-6, "one row above the middle and one below")
}

// seed388 is the random-graph probe's graph of seed 388 (TestProbe_Contract):
// G0 holds n2, n5 and n6, G1 n0, n1 and n4; n7 above feeds n0 twice, and
// n0 and n1 feed n5 back up.
func seed388(t *testing.T) model.Graph {
	g := graph(t, nil, "n0:cylinder", "n1:hexagon", "n2:hexagon", "n3:parallelogram", "n4:circle", "n5:cylinder", "n6:rounded", "n7:hexagon",
		"n7->n2", "n3->n3", "n5->n3", "n6->n0", "n1->n5", "n7->n0", "n7->n6", "n7->n0", "n0->n5")
	return grouped(g, model.Group{ID: "g0", Label: "G0", Contains: []string{"n2", "n5", "n6"}},
		model.Group{ID: "g1", Label: "G1", Contains: []string{"n0", "n1", "n4"}})
}

func TestLayout_TextLanesNeverPutTwoWiresOnOneColumn(t *testing.T) {
	// probe seed 388 in text: inside G1, n0->n5 leaves on the column of
	// n7->n0's port on n0, and n7->n0's two wires and n1->n5 hold the other
	// lanes of the channel above n0 in a cycle of constraints. The cycle
	// once released n7->n0, putting its jog above n0->n5's: both ran down
	// that column, a shared run (C9.1). It releases the crossing instead
	// (S8 Lanes), and the layout meets the text contract
	for _, dir := range []model.Direction{model.Down, model.Up} {
		t.Run(dir.String(), func(t *testing.T) {
			g := seed388(t)
			g.Direction = dir
			layText(t, g)
		})
	}
}

func TestLayout_TextALabelLeavesAGroupTooNarrowForIt(t *testing.T) {
	// api and db stacked in Backend, a group as narrow as its nodes, and
	// a class package as narrow around order and item: no spot inside the
	// frame fits the label. It once stayed unresolved and the text
	// renderer dropped it; the ladder runs again without the group (S10
	// Text cells), and the label is drawn
	flow := model.Graph{
		Nodes: []model.Node{{ID: "c", Label: "Client", Shape: model.ShapeRect}, {ID: "a", Label: "API", Shape: model.ShapeRect},
			{ID: "b", Label: "DB", Shape: model.ShapeCylinder}},
		Edges:  []model.Edge{{ID: "c->a#0", From: "c", To: "a"}, {ID: "a->b#0", From: "a", To: "b", Label: "HTTPS"}},
		Groups: []model.Group{{ID: "g", Label: "Backend", Contains: []string{"a", "b"}}},
	}
	class := model.Graph{
		Nodes:  []model.Node{{ID: "order", Label: "Order", Members: &model.Members{}}, {ID: "item", Label: "Item", Members: &model.Members{}}},
		Edges:  []model.Edge{{ID: "order->item#0", From: "order", To: "item", Label: "contains", Relation: model.RelationComposition}},
		Groups: []model.Group{{ID: "sales", Label: "Sales", Contains: []string{"order", "item"}}},
	}
	for _, tc := range []struct {
		name string
		g    model.Graph
		edge string
	}{{"flow", flow, "a->b#0"}, {"class", class, "order->item#0"}} {
		for _, dir := range []model.Direction{model.Down, model.Up} {
			t.Run(tc.name+"/"+dir.String(), func(t *testing.T) {
				g := tc.g
				g.Direction = dir
				pg := layText(t, g)
				e := edge(t, pg, tc.edge)
				require.NotNil(t, e.LabelPos)
				assert.False(t, e.LabelUnresolved, "resolved")
				res, err := textrender.Render(pg)
				require.NoError(t, err)
				assert.Empty(t, res.DroppedLabels, "drawn")
			})
		}
	}
}
