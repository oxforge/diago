package layered

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/layout/layered/frame"
	"github.com/oxforge/diago/internal/layout/layered/lgraph"
	"github.com/oxforge/diago/internal/layout/layered/lgraph/lgraphtest"
	"github.com/oxforge/diago/internal/layout/layered/ports"
	"github.com/oxforge/diago/internal/layout/layered/size"
	"github.com/oxforge/diago/internal/layoutdbg"
	"github.com/oxforge/diago/internal/model"
)

func screen() Config {
	return ScreenConfig(size.Font{Family: "Inter", Size: 14}, size.Font{Family: "Inter", Size: 12}, size.Font{Family: "Inter", Size: 12}, size.Font{Family: "Inter", Size: 12})
}

// graph builds a DOWN flow graph from lgraphtest specs; labels default to
// the ids and labels overrides them.
func graph(t *testing.T, labels map[string]string, specs ...string) model.Graph {
	t.Helper()
	lv := lgraphtest.Level(t, specs...)
	g := model.Graph{Direction: model.Down}
	for _, n := range lv.Nodes {
		label, ok := labels[n.ID]
		if !ok {
			label = n.ID
		}
		g.Nodes = append(g.Nodes, model.Node{ID: n.ID, Label: label, Shape: n.Shape})
	}
	for _, e := range lv.Edges {
		g.Edges = append(g.Edges, model.Edge{ID: e.ID, From: lv.Nodes[e.From].ID, To: lv.Nodes[e.To].ID})
	}
	return g
}

func arrange(t *testing.T, g model.Graph, cfg Config) (*Arrangement, func(id string) float64) {
	t.Helper()
	a, err := Arrange(context.Background(), g, cfg)
	require.NoError(t, err)
	return a, func(id string) float64 {
		for i, n := range a.Level.Nodes {
			if n.ID == id {
				return a.X[i]
			}
		}
		require.Failf(t, "no node", "%s", id)
		return math.NaN()
	}
}

// node returns the level node with id.
func node(t *testing.T, a *Arrangement, id string) int {
	t.Helper()
	for i, n := range a.Level.Nodes {
		if n.ID == id {
			return i
		}
	}
	require.Failf(t, "no node", "%s", id)
	return -1
}

// The following tests rerun place's own symmetry properties on measured sizes carried through every stage.
func TestArrange_AChainIsStraight(t *testing.T) {
	_, x := arrange(t, graph(t, nil, "a->b", "b->c", "c->d"), screen())
	for _, id := range []string{"b", "c", "d"} {
		assert.Equal(t, x("a"), x(id), id)
	}
}

func TestArrange_DiamondBranchesAreSymmetric(t *testing.T) {
	_, x := arrange(t, graph(t, nil, "a->b", "a->c", "b->d", "c->d"), screen())
	assert.InDelta(t, x("a"), x("d"), 1e-6)
	left, right := min(x("b"), x("c")), max(x("b"), x("c"))
	assert.InDelta(t, x("a")-left, right-x("a"), 1e-6)
}

func TestArrange_AFanIsCenteredAndEven(t *testing.T) {
	_, x := arrange(t, graph(t, nil, "root->a", "root->b", "root->c", "root->d"), screen())
	assert.InDelta(t, (x("a")+x("d"))/2, x("root"), 1e-6)
	assert.InDelta(t, x("b")-x("a"), x("c")-x("b"), 1e-6)
	assert.InDelta(t, x("b")-x("a"), x("d")-x("c"), 1e-6)
}

func TestArrange_ABinaryTreeIsMirrorSymmetric(t *testing.T) {
	_, x := arrange(t, graph(t, nil, "r->a", "r->b", "a->aa", "a->ab", "b->ba", "b->bb"), screen())
	axis := x("r")
	assert.InDelta(t, 2*axis, x("aa")+x("bb"), 1e-6)
	assert.InDelta(t, 2*axis, x("ab")+x("ba"), 1e-6)
}

func TestArrange_ParentsAlignOnTheOffCenterPort(t *testing.T) {
	long := "a-target-node-with-a-rather-long-unbroken-label-that-keeps-on-going"
	a, x := arrange(t, graph(t, map[string]string{"t": long}, "a->t", "b->t"), screen())
	w := a.Level.Nodes[node(t, a, "t")].W
	require.Greater(t, w/3, 120.0, "a third of the face clears the parents' separation")
	assert.InDelta(t, x("t")-w/6, x("a"), 1e-6)
	assert.InDelta(t, x("t")+w/6, x("b"), 1e-6)
}

func TestArrange_TwoExitsRouteAsLs(t *testing.T) {
	a, x := arrange(t, graph(t, nil, "m:diamond->yes", "m->no"), screen())
	column := a.Level.Nodes[node(t, a, "m")].W/2 + 20
	assert.InDelta(t, x("m")-column, x("yes"), 1e-9)
	assert.InDelta(t, x("m")+column, x("no"), 1e-9)
}

func TestArrange_ThreeExitsFanFromThreeVertices(t *testing.T) {
	a, x := arrange(t, graph(t, nil, "m:diamond->a", "m->b", "m->c"), screen())
	column := a.Level.Nodes[node(t, a, "m")].W/2 + 20
	assert.InDelta(t, x("m"), x("b"), 1e-9)
	assert.InDelta(t, x("m")-x("a"), x("c")-x("m"), 1e-9)
	assert.GreaterOrEqual(t, x("m")-x("a"), column-1e-9)
}

// flowNetwork is the flow-network example laid out DOWN: api -> db
// skips the middle row, where Database's parents Redis Cache and Message
// Queue sit beside Auth Service.
func flowNetwork() model.Graph {
	return model.Graph{
		Direction: model.Down,
		Nodes: []model.Node{
			{ID: "web", Label: "Web Server", Shape: model.ShapeRect}, {ID: "api", Label: "API Service", Shape: model.ShapeRect},
			{ID: "auth", Label: "Auth Service", Shape: model.ShapeRect}, {ID: "db", Label: "Database", Shape: model.ShapeCylinder},
			{ID: "cache", Label: "Redis Cache", Shape: model.ShapeHexagon}, {ID: "queue", Label: "Message Queue", Shape: model.ShapeParallelogram},
		},
		Edges: []model.Edge{
			{ID: "web->api#0", From: "web", To: "api", Label: "REST"},
			{ID: "api->auth#0", From: "api", To: "auth", Label: "verify"},
			{ID: "api->db#0", From: "api", To: "db", Label: "query"},
			{ID: "api->cache#0", From: "api", To: "cache", Label: "lookup"},
			{ID: "api->queue#0", From: "api", To: "queue", Label: "publish"},
			{ID: "queue->db#0", From: "queue", To: "db", Label: "persist"},
			{ID: "cache->db#0", From: "cache", To: "db", Label: "fallback"},
		},
	}
}

// TestArrange_ALongEdgesColumnStartsAmongItsTargetsParents pins S5's
// Corridor seats through S6 on flow-network: the seed puts api -> db's
// column between Database's parents Redis Cache and Message Queue, and
// S6 keeps it, the seed being crossing-free. The declaration order put it
// last, where S7's straight chain then pulled Web, API and Database to
// the canvas's edge.
func TestArrange_ALongEdgesColumnStartsAmongItsTargetsParents(t *testing.T) {
	a, x := arrange(t, flowNetwork(), screen())
	var middle []string
	for _, v := range a.Graph.Layers[2] {
		middle = append(middle, a.Graph.Vertices[v].ID)
	}
	assert.Equal(t, []string{"auth", "cache", "api->db#0@2", "queue"}, middle)
	assert.Less(t, x("cache"), x("db"))
	assert.Less(t, x("db"), x("queue"))
}

// flowLoop is the flow-loop example: the loop check -> Process -> i++
// -> check, whose back edge i++ -> check is reversed, and check's other
// exit, End.
func flowLoop(dir model.Direction) model.Graph {
	return model.Graph{
		Direction: dir,
		Nodes: []model.Node{
			{ID: "start", Label: "Start", Shape: model.ShapeCircle}, {ID: "init", Label: "i = 0", Shape: model.ShapeRect},
			{ID: "check", Label: "i < array.length?", Shape: model.ShapeDiamond},
			{ID: "process", Label: "Process array[i]", Shape: model.ShapeRect},
			{ID: "increment", Label: "i++", Shape: model.ShapeRect}, {ID: "end", Label: "End", Shape: model.ShapeCircle},
		},
		Edges: []model.Edge{
			{ID: "start->init#0", From: "start", To: "init"}, {ID: "init->check#0", From: "init", To: "check"},
			{ID: "check->process#0", From: "check", To: "process", Label: "true"},
			{ID: "process->increment#0", From: "process", To: "increment"},
			{ID: "increment->check#0", From: "increment", To: "check", Style: model.EdgeDashed},
			{ID: "check->end#0", From: "check", To: "end", Label: "false"},
		},
	}
}

// TestArrange_FlowLoopKeepsItsLoopTogether pins S6's Loops kept together
// on flow-loop in every direction, both profiles: the loop's return
// column, the dummy of i++ -> check, sits next to Process, the loop's
// body, and End outside the two. The declaration seed put End between
// them.
func TestArrange_FlowLoopKeepsItsLoopTogether(t *testing.T) {
	for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
		for _, cfg := range []Config{screen(), TextConfig(dir)} {
			a, _ := arrange(t, flowLoop(dir), cfg)
			var row []string
			for _, v := range a.Graph.Layers[a.Layers[node(t, a, "process")]] {
				row = append(row, a.Graph.Vertices[v].ID)
			}
			assert.Equal(t, []string{"end", "process", "increment->check#0@3"}, row, "%s text=%v", dir, cfg.Text)
		}
	}
}

func TestArrange_Right(t *testing.T) {
	g := graph(t, nil, "a->b")
	g.Direction = model.Right
	a, _ := arrange(t, g, screen())
	assert.Equal(t, model.Right, a.Dir)
	assert.Equal(t, a.Sizes[0].H, a.Level.Nodes[0].W, "the cross axis is vertical")
	assert.Equal(t, a.Sizes[0].W, a.Level.Nodes[0].H)
}

func TestArrange_Auto(t *testing.T) {
	g := graph(t, nil, "a", "b", "c", "a->b")
	g.Direction = model.Auto
	a, _ := arrange(t, g, screen())
	assert.Equal(t, model.Right, a.Dir)
}

func TestArrange_TextProfile(t *testing.T) {
	a, x := arrange(t, graph(t, nil, "abc->de", "abc->f"), TextConfig(model.Down))
	assert.Equal(t, size.Size{W: 8, H: 3}, a.Sizes[0], "cells")
	assert.GreaterOrEqual(t, math.Abs(x("f")-x("de")), 8+5-1e-9)
}

func TestArrange_Classes(t *testing.T) {
	g := model.Graph{Direction: model.Down,
		Nodes: []model.Node{
			{ID: "animal", Label: "Animal", Members: &model.Members{Methods: []model.Member{{Visibility: "+", Text: "speak(): string"}}}},
			{ID: "dog", Label: "Dog", Members: &model.Members{}},
			{ID: "cat", Label: "Cat", Members: &model.Members{Attributes: []model.Member{{Text: "lives: int"}}}},
		},
		Edges: []model.Edge{
			{ID: "dog->animal#0", From: "animal", To: "dog", Relation: model.RelationInheritance},
			{ID: "cat->animal#0", From: "animal", To: "cat", Relation: model.RelationInheritance},
		},
	}
	a, x := arrange(t, g, screen())
	require.NotNil(t, a.Sizes[0].Members)
	assert.True(t, a.Level.Nodes[0].Record)
	assert.Equal(t, []int{0, 1, 1}, a.Layers, "the supertype sits on top")
	assert.NotEqual(t, a.Level.Nodes[1].H, a.Level.Nodes[2].H, "record boxes are never equalized")
	assert.InDelta(t, x("animal"), (x("dog")+x("cat"))/2, 1e-6)
}

func TestArrange_Errors(t *testing.T) {
	g := graph(t, nil, "a->b")
	g.Groups = []model.Group{{ID: "grp", Contains: []string{"a"}}}
	_, err := Arrange(context.Background(), g, screen())
	assert.ErrorContains(t, err, "groups")

	g = graph(t, nil, "a->b")
	g.Edges[0].To = "ghost"
	_, err = Arrange(context.Background(), g, screen())
	assert.ErrorContains(t, err, "ghost")
}

func TestArrange_TextConfigMustMatchTheDirection(t *testing.T) {
	down := graph(t, nil, "a->b")
	auto := graph(t, nil, "a", "b", "c", "a->b")
	auto.Direction = model.Auto // resolves to RIGHT
	up, right := down, down
	up.Direction, right.Direction = model.Up, model.Right

	for _, tt := range []struct {
		name string
		g    model.Graph
		cfg  Config
	}{
		{"built for AUTO", down, TextConfig(model.Auto)},
		{"built for AUTO, on a graph that resolves AUTO", auto, TextConfig(model.Auto)},
		{"DOWN's values on a graph that resolves to RIGHT", auto, TextConfig(model.Down)},
		{"DOWN's values under RIGHT", right, TextConfig(model.Down)},
		{"LEFT's values under UP", up, TextConfig(model.Left)},
		{"a hand-built text Config names no direction", down, Config{Text: true, Size: TextConfig(model.Down).Size}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Arrange(context.Background(), tt.g, tt.cfg)
			assert.ErrorContains(t, err, "text Config built for")
		})
	}

	for _, tt := range []struct {
		name string
		g    model.Graph
		cfg  Config
	}{
		{"DOWN's values under UP: the same axis", up, TextConfig(model.Down)},
		{"LEFT's values under RIGHT: the same axis", right, TextConfig(model.Left)},
		{"the resolved direction of an AUTO graph", auto, TextConfig(frame.Resolve(context.Background(), auto))},
		{"the screen profile ignores Dir", right, screen()},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Arrange(context.Background(), tt.g, tt.cfg)
			assert.NoError(t, err)
		})
	}
}

// fanOut is hub feeding n leaves l0, l1, …, then the extra specs.
func fanOut(t *testing.T, n int, extra ...string) model.Graph {
	t.Helper()
	var specs []string
	for i := range n {
		specs = append(specs, fmt.Sprintf("hub->l%d", i))
	}
	return graph(t, nil, append(specs, extra...)...)
}

func TestArrange_TextNodesGrowToHoldTheirPorts(t *testing.T) {
	width := func(g model.Graph, cfg Config) float64 {
		a, _ := arrange(t, g, cfg)
		return a.Level.Nodes[node(t, a, "hub")].W
	}
	g := fanOut(t, 10)
	assert.Equal(t, 12.0, width(g, TextConfig(model.Down)), "ten ports and two corner cells")
	g.Direction = model.Right
	assert.Equal(t, 12.0, width(g, TextConfig(model.Right)), "sideways, in rows")

	g = fanOut(t, 11)
	assert.Equal(t, 14.0, width(g, TextConfig(model.Down)), "rounded up to even under DOWN, as S1 keeps widths")
	g.Direction = model.Right
	assert.Equal(t, 13.0, width(g, TextConfig(model.Right)), "rows keep an odd count")

	assert.Equal(t, 8.0, width(fanOut(t, 6), TextConfig(model.Down)), "a face with room keeps its size")
	assert.Equal(t, 8.0, width(fanOut(t, 6, "hub->hub"), TextConfig(model.Down)), "a self-loop takes no port on a face")
	assert.Equal(t, 80.0, width(fanOut(t, 10), screen()), "a screen face with room keeps its size")
}

// crowded is hub, of shape (a rect when ""), feeding l0 to l(n-1), and
// the extra specs.
func crowded(t *testing.T, shape string, n int, extra ...string) model.Graph {
	t.Helper()
	var specs []string
	for i := range n {
		from := "hub"
		if i == 0 && shape != "" {
			from += ":" + shape
		}
		specs = append(specs, fmt.Sprintf("%s->l%d", from, i))
	}
	return graph(t, nil, append(specs, extra...)...)
}

// TestArrange_ScreenNodesGrowToKeepTheirPortsApart pins S8's Shape ports on
// screen: a node with faces whose busier face holds n chain ends is widened
// across the flow, before S3, until that face's usable span is at least
// (n + 1) InLaneGap / 2, so its evenly spread ports keep a rake's 4 px
// apart (C9.2). Thirty ends want a span of 124 px.
func TestArrange_ScreenNodesGrowToKeepTheirPortsApart(t *testing.T) {
	hub := func(g model.Graph, dir model.Direction) lgraph.Node {
		g.Direction = dir
		a, _ := arrange(t, g, screen())
		return a.Level.Nodes[node(t, a, "hub")]
	}
	const span = 31 * 4.0
	var back []string
	for i := range 15 {
		back = append(back, fmt.Sprintf("l%d->hub", i))
	}
	for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
		t.Run(dir.String(), func(t *testing.T) {
			assert.Equal(t, span, hub(crowded(t, "", 30), dir).W, "a rect's face is its whole side")
			assert.Equal(t, span, hub(crowded(t, "rounded", 30), dir).W, "so is a rounded box's")
			hexagon := hub(crowded(t, "hexagon", 30), dir)
			parallelogram := hub(crowded(t, "parallelogram", 30), dir)
			cylinder := hub(crowded(t, "cylinder", 30), dir)
			if dir == model.Down || dir == model.Up {
				assert.Equal(t, 2*span, hexagon.W, "a hexagon's flat face is the middle half")
				assert.InDelta(t, span+ports.Slant*parallelogram.H, parallelogram.W, 1e-9, "a parallelogram's edge loses its slant")
				assert.Equal(t, span, cylinder.W, "a cylinder's cap: its whole side")
			} else {
				assert.Equal(t, span, hexagon.W, "a hexagon's angled side: its whole side")
				assert.Equal(t, span, parallelogram.W, "a parallelogram's slanted side: its whole side")
				assert.InDelta(t, span/(1-2*ports.CapRatio), cylinder.W, 1e-9, "a cylinder's side line runs between its caps")
			}
			assert.Equal(t, span, hub(crowded(t, "", 15, back...), dir).W, "a counter-flow end counts on its face")
			assert.Equal(t, span, hub(crowded(t, "", 30, "hub->hub"), dir).W, "a self-loop takes no port on a face")
			assert.Equal(t, hub(crowded(t, "diamond", 1), dir).W, hub(crowded(t, "diamond", 30), dir).W,
				"a diamond's ends share its vertices (C8)")
			assert.Equal(t, hub(crowded(t, "", 1), dir).W, hub(crowded(t, "", 9), dir).W, "a face with room keeps its size")
		})
	}
}

// TestFaceRoom_AGroupOrATerminalNeverGrows pins the exemption of S8's
// Shape ports, in both profiles: a group is sized by its content and
// anchors its ports, and a terminal holds one end (S9).
func TestFaceRoom_AGroupOrATerminalNeverGrows(t *testing.T) {
	var specs []string
	for i := range 30 {
		specs = append(specs, fmt.Sprintf("hub->l%d", i))
	}
	for _, cfg := range []Config{screen(), TextConfig(model.Right)} {
		for _, kind := range []string{"group", "terminal"} {
			lv := lgraphtest.Level(t, specs...)
			lv.Nodes[0].W = 10 // narrower than thirty ends want, in px or cells
			lv.Nodes[0].Group, lv.Nodes[0].Terminal = kind == "group", kind == "terminal"
			faceRoom(context.Background(), lv, make([]bool, len(lv.Edges)), cfg, model.Right)
			assert.Equal(t, 10.0, lv.Nodes[0].W, "text=%v: a %s", cfg.Text, kind)
		}
	}
}

// TestArrange_LogsAWidenedFace pins face_widened (S8, Debug phase): the
// node, the ends of its busier face and its width before and after.
func TestArrange_LogsAWidenedFace(t *testing.T) {
	var buf bytes.Buffer
	ctx := layoutdbg.NewContext(context.Background(), slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	g := crowded(t, "", 30)
	g.Direction = model.Right
	_, err := Arrange(ctx, g, screen())
	require.NoError(t, err)
	var got []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var rec map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &rec))
		if rec["decision"] == "face_widened" {
			got = append(got, rec)
		}
	}
	require.Len(t, got, 1)
	assert.Equal(t, "route", got[0]["phase"])
	assert.Equal(t, "S8", got[0]["spec_ref"])
	assert.Equal(t, "hub", got[0]["node"])
	assert.Equal(t, 30.0, got[0]["ends"])
	assert.Equal(t, 46.59375, got[0]["from"], "hub as S1 measures it, across the flow")
	assert.Equal(t, 124.0, got[0]["to"])
}

// TestArrange_NestedSelfLoopsLengthenTheirNode pins S8's self-loops: a
// node with faces and n loops is at least LoopMinH and (4n - 2) LaneGap
// long on the flow axis, set before S4; a diamond's loops leave it as S1
// sized it.
func TestArrange_NestedSelfLoopsLengthenTheirNode(t *testing.T) {
	length := func(cfg Config, dir model.Direction, specs ...string) float64 {
		g := graph(t, nil, append(specs, "a->b")...)
		g.Direction = dir
		a, _ := arrange(t, g, cfg)
		return a.Level.Nodes[node(t, a, "a")].H
	}
	loops := func(n int, shape string) []string {
		specs := []string{"a" + shape + "->a"}
		for range n - 1 {
			specs = append(specs, "a->a")
		}
		return specs
	}
	for _, tt := range []struct {
		name   string
		cfg    Config
		dir    model.Direction
		specs  []string
		length float64
	}{
		{"one loop: S1's 46.6, over LoopMinH", screen(), model.Down, loops(1, ""), length(screen(), model.Down, "a")},
		{"two loops: 6 lane gaps", screen(), model.Down, loops(2, ""), 48},
		{"three loops: 10 lane gaps", screen(), model.Down, loops(3, ""), 80},
		{"text, one loop: LoopMinH", TextConfig(model.Down), model.Down, loops(1, ""), 5},
		{"text, two loops: 6 rows", TextConfig(model.Down), model.Down, loops(2, ""), 6},
		{"text sideways, two loops: 6 lane gaps of 2 columns", TextConfig(model.Right), model.Right, loops(2, ""), 12},
		{"a diamond keeps its size", screen(), model.Down, loops(3, ":diamond"), length(screen(), model.Down, "a:diamond")},
	} {
		assert.Equal(t, tt.length, length(tt.cfg, tt.dir, tt.specs...), tt.name)
	}
}

func TestArrange_Properties(t *testing.T) {
	rng := rand.New(rand.NewPCG(19, 23))
	shapes := []model.Shape{model.ShapeRect, model.ShapeRounded, model.ShapeDiamond, model.ShapeCircle,
		model.ShapeCylinder, model.ShapeHexagon, model.ShapeParallelogram}
	words := []string{"Load", "the", "customer", "record", "and", "validate", "payment", "details", "retry"}
	dirs := []model.Direction{model.Down, model.Up, model.Right, model.Left, model.Auto}
	for trial := range 80 {
		n := 1 + rng.IntN(18)
		g := model.Graph{Direction: dirs[rng.IntN(len(dirs))]}
		for i := range n {
			label := ""
			for range 1 + rng.IntN(8) {
				label += words[rng.IntN(len(words))] + " "
			}
			g.Nodes = append(g.Nodes, model.Node{ID: fmt.Sprintf("n%d", i), Label: label, Shape: shapes[rng.IntN(len(shapes))]})
		}
		for e := range rng.IntN(2*n + 1) {
			from, to := rng.IntN(n), rng.IntN(n)
			g.Edges = append(g.Edges, model.Edge{ID: fmt.Sprintf("e%d", e), From: g.Nodes[from].ID, To: g.Nodes[to].ID})
		}
		cfg := screen()
		if trial%3 == 0 {
			cfg = TextConfig(frame.Resolve(context.Background(), g)) // the text Config needs the resolved direction
		}
		a, err := Arrange(context.Background(), g, cfg)
		require.NoError(t, err, "trial %d", trial)

		for e := range a.Level.Edges {
			if a.Level.SelfLoop(e) {
				continue
			}
			upper, lower := a.Level.Oriented(e, a.Graph.Reversed)
			assert.Greater(t, a.Layers[lower], a.Layers[upper], "trial %d: edge %d descends", trial, e)
		}
		for l, layer := range a.Graph.Layers {
			assert.NotEmpty(t, layer, "trial %d: layer %d", trial, l)
			for i := 1; i < len(layer); i++ {
				p, q := layer[i-1], layer[i]
				need := (a.Graph.Vertices[p].W+a.Graph.Vertices[q].W)/2 + cfg.NodeGap
				assert.GreaterOrEqual(t, a.X[q]-a.X[p], need-1e-6, "trial %d layer %d", trial, l)
			}
		}
		require.Len(t, a.X, len(a.Graph.Vertices))
		for v, xv := range a.X {
			assert.False(t, math.IsNaN(xv) || math.IsInf(xv, 0), "trial %d vertex %d", trial, v)
		}
	}
}
