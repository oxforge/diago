package route

import (
	"context"
	"testing"

	"github.com/oxforge/diago/internal/layout/layered/cycle"
	"github.com/oxforge/diago/internal/layout/layered/lgraph"
	"github.com/oxforge/diago/internal/layout/layered/lgraph/lgraphtest"
	"github.com/oxforge/diago/internal/layout/layered/ports"
	"github.com/oxforge/diago/internal/layout/layered/rank"
	"github.com/oxforge/diago/internal/layoutdbg/layoutdbgtest"
	"github.com/oxforge/diago/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var screen = Options{Clearance: 12, Span: 16, Reach: 20, BaseGap: 40, LaneGap: 8, InLaneGap: 8, Snap: 0.5, Dir: model.Down}

// graph runs S2 and S3 over a fixture level and builds its layered graph
// in the declaration seed; widths overrides node widths by id. Nodes are
// 80 x 40 (lgraphtest).
func graph(t *testing.T, widths map[string]float64, specs ...string) *lgraph.Graph {
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
	return g
}

// routed places every vertex of g at xs[its id] and routes g; it returns
// the result and an accessor of an edge's route by id.
func routed(t *testing.T, g *lgraph.Graph, o Options, xs map[string]float64) (*Result, func(id string) []model.Point) {
	t.Helper()
	x := make([]float64, len(g.Vertices))
	for v, vx := range g.Vertices {
		p, ok := xs[vx.ID]
		require.True(t, ok, "no x for %s", vx.ID)
		x[v] = p
	}
	r := Route(context.Background(), g, x, o)
	return r, func(id string) []model.Point {
		for e, ed := range g.Level.Edges {
			if ed.ID == id {
				return r.Edges[e]
			}
		}
		require.Failf(t, "no edge", "%s", id)
		return nil
	}
}

// twice routes g, then replays its side attachments, as Arrange's second
// pass does (without placing again): the attached ends leave their faces.
func twice(t *testing.T, g *lgraph.Graph, o Options, xs map[string]float64) (*Result, func(id string) []model.Point) {
	t.Helper()
	first, _ := routed(t, g, o, xs)
	o.Side = first.Side
	return routed(t, g, o, xs)
}

func pts(xy ...float64) []model.Point {
	out := make([]model.Point, 0, len(xy)/2)
	for i := 0; i+1 < len(xy); i += 2 {
		out = append(out, model.Point{X: xy[i], Y: xy[i+1]})
	}
	return out
}

func assertRoute(t *testing.T, want, got []model.Point) {
	t.Helper()
	require.Len(t, got, len(want), "%v", got)
	for i := range want {
		assert.InDelta(t, want[i].X, got[i].X, 1e-6, "point %d x of %v", i, got)
		assert.InDelta(t, want[i].Y, got[i].Y, 1e-6, "point %d y of %v", i, got)
	}
}

func TestRoute_AChainRunsStraightBetweenRows(t *testing.T) {
	g := graph(t, nil, "a->b", "b->c")
	r, route := routed(t, g, screen, map[string]float64{"a": 100, "b": 100, "c": 100})
	assert.Equal(t, []float64{0, 80, 160}, r.RowTop, "a row, then BaseGap and no lanes")
	assert.Equal(t, []float64{40, 40, 40}, r.RowH)
	assert.Equal(t, []int{0, 0}, r.Lanes)
	assertRoute(t, pts(100, 40, 100, 80), route("a->b#0"))
	assertRoute(t, pts(100, 120, 100, 160), route("b->c#0"))
}

func TestRoute_ARowIsItsTallestNodeAndEveryNodeStartsAtItsTop(t *testing.T) {
	lv := lgraphtest.Level(t, "a->c", "b->d")
	lv.Nodes[2].H = 60 // b
	ctx := context.Background()
	g, err := lgraph.Build(lv, rank.Assign(ctx, lv, nil, nil), nil, 8)
	require.NoError(t, err)
	r, route := routed(t, g, screen, map[string]float64{"a": 100, "b": 300, "c": 100, "d": 300})
	assert.Equal(t, []float64{60, 40}, r.RowH)
	assert.Equal(t, []float64{0, 100}, r.RowTop)
	assertRoute(t, pts(100, 40, 100, 100), route("a->c#0"))
	assertRoute(t, pts(300, 60, 300, 100), route("b->d#0"))
}

func TestRoute_PortsLandOnTheDrawnOutline(t *testing.T) {
	g := graph(t, nil, "a->c:cylinder", "b->c")
	_, route := routed(t, g, screen, map[string]float64{"a": 100, "b": 300, "c": 200})
	got := route("a->c#0")
	end := got[len(got)-1]
	assert.InDelta(t, 200-40.0/3, end.X, 1e-6)
	top := 88.0 // one lane in the channel
	assert.InDelta(t, top+ports.Inset(model.ShapeCylinder, 80, 40, model.Down, false, ports.FaceIn, -40.0/3), end.Y, 1e-9)
	assert.Greater(t, end.Y, top, "on the cap's arc, below the box's top")
}

func TestRoute_LogsItsDecisions(t *testing.T) {
	ctx, buf := layoutdbgtest.Capture()
	g := graph(t, nil, "m:diamond->a", "a->m")
	Route(ctx, g, []float64{200, 200}, screen)
	var decisions []string
	for _, rec := range layoutdbgtest.Records(t, buf) {
		assert.Equal(t, "route", rec["phase"])
		assert.Equal(t, "S8", rec["spec_ref"])
		decisions = append(decisions, rec["decision"].(string))
	}
	assert.Equal(t, []string{"side_attached", "side_attached", "routed"}, decisions)
}

// TestRoute_LogsAMergedFanOut pins S8's debug record of a circle's merged
// exits: the circle and how many exits leave its bottom vertex. The replay
// takes them from the first route and records nothing new.
func TestRoute_LogsAMergedFanOut(t *testing.T) {
	ctx, buf := layoutdbgtest.Capture()
	g := graph(t, nil, "m:circle->a", "m->b", "m->c")
	first := Route(ctx, g, []float64{200, 100, 200, 300}, screen)
	replay := screen
	replay.Side = first.Side
	Route(ctx, g, []float64{200, 100, 200, 300}, replay)
	var merged []map[string]any
	for _, rec := range layoutdbgtest.Records(t, buf) {
		if rec["decision"] == "exits_merged" {
			merged = append(merged, rec)
		}
	}
	require.Len(t, merged, 1)
	assert.Equal(t, "route", merged[0]["phase"])
	assert.Equal(t, "S8", merged[0]["spec_ref"])
	assert.Equal(t, "m", merged[0]["node"])
	assert.InDelta(t, 3, merged[0]["exits"], 0)
}

// beside is diamond m's level with a long edge s->c whose dummy runs
// through m's row: m on layer 1 above its two exits' ends a and b, s on
// layer 0 and c on layer 2. m is 80 wide at 200, so its right column lies
// at 260; a sits under m, b's lone in-port at 320, s and the dummy on one
// x, dummy, and c 200 beyond it, so that the router leaves the dummy
// where it is.
func beside(t *testing.T, dummy float64) func(id string) []model.Point {
	t.Helper()
	lv := lgraphtest.Level(t, "m:diamond->a", "m->b", "s->c")
	g, err := lgraph.Build(lv, []int{1, 2, 2, 0, 2}, nil, 8)
	require.NoError(t, err)
	_, route := routed(t, g, screen, map[string]float64{
		"m": 200, "a": 200, "b": 320, "s": dummy, "s->c#0@1": dummy, "c": dummy + 200,
	})
	return route
}

// shaped is graph with fix applied to the level before S2 and S3: the
// groups, terminals and anchors S9 sets.
func shaped(t *testing.T, fix func(lv *lgraph.Level), specs ...string) *lgraph.Graph {
	t.Helper()
	lv := lgraphtest.Level(t, specs...)
	fix(lv)
	ctx := context.Background()
	reversed := cycle.Break(ctx, lv, nil)
	g, err := lgraph.Build(lv, rank.Assign(ctx, lv, reversed, nil), reversed, 8)
	require.NoError(t, err)
	return g
}

// TestRoute_ATerminalChannelCloses pins S8 (S9): the channel below a row of
// terminals closes when it holds no lane, and keeps its height when a
// terminal's wire jogs there.
func TestRoute_ATerminalChannelCloses(t *testing.T) {
	fix := func(lv *lgraph.Level) { lv.Nodes[0].Terminal, lv.Nodes[0].W, lv.Nodes[0].H = true, 8, 0 }
	res, route := routed(t, shaped(t, fix, "t->n"), screen, map[string]float64{"t": 0, "n": 0})
	assert.Equal(t, []float64{0, 0}, res.RowTop, "straight: the rows meet")
	assert.Equal(t, pts(0, 0), route("t->n#0"), "a zero-length piece the join drops (S9)")
	res, _ = routed(t, shaped(t, fix, "t->n"), screen, map[string]float64{"t": 30, "n": 0})
	assert.Equal(t, []float64{0, 48}, res.RowTop, "a jog keeps BaseGap and its lane")
}

// text is the S14 text Config's routing under DOWN, in cells.
var text = Options{Clearance: 2, Span: 1, Reach: 3, BaseGap: 3, LaneGap: 1, InLaneGap: 2, Snap: 0.5, Dir: model.Down, Text: true}
