package layered

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/layoutdbg"
	"github.com/oxforge/diago/internal/model"
)

// TestArrange_Deterministic is C1 for the stages built so far: 50
// in-process runs of each adversarial graph give byte-identical
// arrangements. Each case aims at a tie-break: equal crossings (S6), the
// chain of an interleaved pair that moves to the other side (S6, of equal
// spans the later), a symmetric fan (S7 tie batches), cycles entered by
// declaration order (S2), forced side exits (S7), arriving ends giving
// way to a pinned node's exits (S8), a side rail with a self-loop on it,
// nested self-loops, facing side columns of two pinned nodes (S7, S8),
// ports sliding onto the far end of a wire within Span, in-port first
// (S8), isolated nodes placed in declaration order, of equal rooms on the
// earliest row, and riding with their rows (S3, S7), long edges, parallel
// edges, self-loops, record boxes, both profiles and every direction. The
// replayed side exits add no tie-break of their own: that case guards the
// forced reruns of S7's recheck and its second-pass prediction; nor do
// the rooms of side-by-side cascades (S7), a room's fallback to a denial
// given back, or a level whose parts interleave, which S7's packing
// leaves alone, each a case of its own.
func TestArrange_Deterministic(t *testing.T) {
	cases := []struct {
		name  string
		specs []string
		dir   model.Direction
		text  bool
	}{
		{"equal crossings", []string{"b", "a", "d", "c", "b->d", "b->c", "a->d", "a->c"}, model.Down, false},
		{"interleaved back edges", []string{"a->b", "b->c", "c->d", "d->e", "d->a", "e->b"}, model.Right, false},
		{"interleaved back edges in text", []string{"a->b", "b->c", "c->d", "d->e", "d->a", "e->b"}, model.Down, true},
		{"symmetric fan", []string{"r->a", "r->b", "r->c", "r->d", "a->e", "d->e"}, model.Down, false},
		{"cycles", []string{"a->b", "b->c", "c->a", "c->d", "d->b", "d->d"}, model.Up, false},
		{"decisions", []string{"m:diamond->a", "m->b", "a->n:circle", "b->n", "n->x", "n->y", "n->z", "z->m"}, model.Down, false},
		{"arrivals giving way", []string{"n0:diamond->n1:diamond", "n1->n0", "n2:diamond->n0", "n0->n2"}, model.Down, false},
		{"a side rail with a self-loop", []string{"m:diamond->a", "m->b", "m->c", "m->m"}, model.Down, false},
		{"nested self-loops", []string{"a->a", "a->a", "a->b", "b->a"}, model.Up, false},
		{"nested self-loops in text", []string{"a->a", "a->a", "a->b", "b->a"}, model.Right, true},
		{"facing side columns", []string{"top:diamond->l:diamond", "top->r:diamond", "l->top", "r->top"}, model.Right, false},
		{"long and parallel edges", []string{"a->b", "b->c", "c->d", "a->d", "a->d", "a->c", "b->d"}, model.Right, false},
		{"sliding ports", []string{"workstation->api", "workstation->api", "api->db:cylinder", "workstation->db"}, model.Down, false},
		{"sliding ports in text", []string{"client->api", "client->api"}, model.Down, true},
		{"isolated nodes", []string{"a->b", "b->c", "d->e", "iso1", "iso2", "iso3"}, model.Down, false},
		{"isolated nodes in text", []string{"a->b", "b->c", "d->e", "iso1", "iso2", "iso3"}, model.Right, true},
		{"text", []string{"start->check:diamond", "check->ok", "check->retry", "retry->start", "ok->ok"}, model.Left, true},
		{"replayed side exits", []string{"n0:diamond", "n1:diamond", "n2:circle", "n4:rounded", "n5",
			"n1->n5", "n0->n2", "n1->n0", "n1->n0", "n1->n2", "n4->n2"}, model.Right, false},
		{"a primary exit beside a back edge", []string{"s->x", "x->m:diamond", "m->main", "m->end", "main->next", "end->x"}, model.Down, false},
		{"a cascade's rooms", []string{"s->a:diamond", "a->b:diamond", "a->ea", "b->c:diamond", "b->eb", "c->ok", "c->ec", "ok->done"}, model.Right, false},
		{"a room giving way to a forcing", []string{"n0->n4", "n1:diamond", "n2:diamond", "n1->n4", "n1->n3", "n2->n4", "n2->n6", "n3->n7", "n6->n8"}, model.Down, false},
		{"a primary given back", []string{"s->m:diamond", "m->mainmainmainmain", "m->endendee", "z->endendee", "mainmainmainmain->next"}, model.Down, false},
		{"rooms side by side", []string{"s->a:diamond", "a->b:diamond", "a->ea", "b->ok", "b->eb", "ok->done",
			"s->c:diamond", "c->d:diamond", "c->ec", "d->ok2", "d->ed", "ok2->done2"}, model.Down, false},
		{"interleaved parts", interleavedParts, model.Down, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := graph(t, nil, tc.specs...)
			g.Direction = tc.dir
			cfg := screen()
			if tc.text {
				cfg = TextConfig(tc.dir)
			}
			first := run(t, g, cfg)
			for range 49 {
				require.Equal(t, first, run(t, g, cfg))
			}
		})
	}
}

// TestArrange_DeterministicForcedSideExit is the forced-side-exit case
// TestArrange_Deterministic's comment claims: two exits of a pinned node,
// which place can only satisfy by forcing (the fixture of
// TestArrange_TwoExitsRouteAsLs). It confirms the forcing actually happens
// (a side_exit_forced decision) before checking that the arrangement still
// repeats byte-identically.
func TestArrange_DeterministicForcedSideExit(t *testing.T) {
	g := graph(t, nil, "m:diamond->yes", "m->no")
	cfg := screen()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := layoutdbg.NewContext(context.Background(), logger)
	a, err := Arrange(ctx, g, cfg)
	require.NoError(t, err)

	forced := 0
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		if entry["decision"] == "side_exit_forced" {
			forced++
		}
	}
	require.Greater(t, forced, 0, "the fixture must actually force a side exit")

	first, err := json.Marshal(a)
	require.NoError(t, err)
	for range 49 {
		require.Equal(t, string(first), run(t, g, cfg))
	}
}

// TestArrange_DeterministicRecordBoxes is the record-box case
// TestArrange_Deterministic's comment claims: an inheritance pair whose
// record boxes are never equalized (TestArrange_Classes), repeated
// byte-identically.
func TestArrange_DeterministicRecordBoxes(t *testing.T) {
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
	cfg := screen()
	first := run(t, g, cfg)
	for range 49 {
		require.Equal(t, first, run(t, g, cfg))
	}
}

// TestArrange_DeterministicCorridorSeats is C1 for S5's Corridor seats:
// columns keyed at a parent's position (r -> t's two, at b's, t having
// three parents), tied with each other (both at b's), and a column keyed
// between two parents (r -> s's). It confirms the three seats happen (a
// corridor_seated decision each) before checking that the arrangement
// repeats byte-identically, 50 times, under DOWN and RIGHT.
func TestArrange_DeterministicCorridorSeats(t *testing.T) {
	g := graph(t, nil, "r->x", "r->a", "r->b", "r->c", "a->t", "b->t", "c->t", "r->t", "r->t", "a->s", "b->s", "r->s")
	var buf bytes.Buffer
	ctx := layoutdbg.NewContext(context.Background(), slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	_, err := Arrange(ctx, g, screen())
	require.NoError(t, err)
	var seated []string
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		if entry["decision"] == "corridor_seated" {
			id, _ := entry["dummy"].(string)
			seated = append(seated, id)
		}
	}
	require.Equal(t, []string{"r->t#0@1", "r->t#1@1", "r->s#0@1"}, seated, "the fixture must seat its three columns")
	for _, dir := range []model.Direction{model.Down, model.Right} {
		g.Direction = dir
		first := run(t, g, screen())
		for range 49 {
			require.Equal(t, first, run(t, g, screen()))
		}
	}
}

// TestArrange_DeterministicPackedParts is C1 for S7's Packing: a level of
// four parts, numbered by their first vertex in layer order, that close up
// toward its weighted center: o's part, which act 1 drifts, c->d, e->f and
// an isolated node. It confirms the packing moves parts (a parts_packed
// decision) before checking that the arrangement repeats byte-identically,
// 50 times, under DOWN and RIGHT, in both profiles.
func TestArrange_DeterministicPackedParts(t *testing.T) {
	g := graph(t, nil, "o->p", "p->q", "o->q", "c->d", "e->f", "iso")
	var buf bytes.Buffer
	ctx := layoutdbg.NewContext(context.Background(), slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	_, err := Arrange(ctx, g, screen())
	require.NoError(t, err)
	moved := 0.0
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		if entry["decision"] == "parts_packed" {
			n, _ := entry["moved"].(float64)
			moved += n
		}
	}
	require.Positive(t, moved, "the fixture must pack its parts")
	for _, dir := range []model.Direction{model.Down, model.Right} {
		for _, text := range []bool{false, true} {
			g.Direction = dir
			cfg := screen()
			if text {
				cfg = TextConfig(dir)
			}
			first := run(t, g, cfg)
			for range 49 {
				require.Equal(t, first, run(t, g, cfg))
			}
		}
	}
}

// dependencyGraph is the corpus's neat/dependency-graph in direction dir:
// under RIGHT, S8's give-way moves pkg-0a's exit to pkg-1b past its exit to
// pkg-1f, and the face's ports then take their positions in their order.
func dependencyGraph(t *testing.T, dir model.Direction) model.Graph {
	g := graph(t, map[string]string{
		"l0n0": "pkg-0a", "l0n1": "pkg-0b", "l0n2": "pkg-0c", "l0n3": "pkg-0d", "l1n0": "pkg-1a", "l1n1": "pkg-1b",
		"l1n2": "pkg-1c", "l1n3": "pkg-1d", "l1n4": "pkg-1e", "l1n5": "pkg-1f", "l2n0": "pkg-2a", "l2n1": "pkg-2b",
		"l2n2": "pkg-2c", "l2n3": "pkg-2d", "l2n4": "pkg-2e", "l2n5": "pkg-2f", "l3n0": "pkg-3a", "l3n1": "pkg-3b",
		"l3n2": "pkg-3c", "l3n3": "pkg-3d",
	},
		"l0n0", "l0n1", "l0n2", "l0n3", "l1n0", "l1n1", "l1n2", "l1n3", "l1n4", "l1n5",
		"l2n0", "l2n1", "l2n2", "l2n3", "l2n4", "l2n5", "l3n0", "l3n1", "l3n2", "l3n3",
		"l0n0->l1n0", "l0n0->l1n1", "l0n3->l1n2", "l0n2->l1n3", "l0n2->l1n4", "l0n1->l1n5", "l0n0->l1n5",
		"l0n1->l1n1", "l0n2->l1n2", "l0n3->l1n1", "l1n3->l2n0", "l1n5->l2n1", "l1n4->l2n2", "l1n0->l2n3",
		"l1n1->l2n4", "l1n2->l2n5", "l1n0->l2n1", "l1n0->l2n2", "l1n1->l2n5", "l1n1->l3n3", "l1n2->l2n1",
		"l1n2->l2n4", "l1n3->l2n4", "l2n5->l3n0", "l2n3->l3n1", "l2n4->l3n2", "l2n2->l3n3", "l2n1->l3n1",
		"l2n1->l3n2", "l2n2->l3n2", "l2n5->l3n1")
	g.Direction = dir
	return g
}

// TestArrange_DeterministicPortOrder is C1 for S8's port give-way: a spot
// that passes no other end of the node wins over an earlier one that
// does, in the order the spots are tried, and when every clear spot
// passes one, the face's ports share out their positions in the order
// they held, ties by edge order (neat/dependency-graph). It confirms that
// a face is reordered (a port_gave_way decision) before checking that
// the arrangement repeats byte-identically, 50 times, under RIGHT and
// DOWN, in both profiles.
func TestArrange_DeterministicPortOrder(t *testing.T) {
	var buf bytes.Buffer
	ctx := layoutdbg.NewContext(context.Background(), slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	_, err := Arrange(ctx, dependencyGraph(t, model.Right), screen())
	require.NoError(t, err)
	reordered := 0.0
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		if entry["decision"] == "port_gave_way" {
			n, _ := entry["reordered"].(float64)
			reordered += n
		}
	}
	require.Positive(t, reordered, "the fixture must reorder a face")
	for _, dir := range []model.Direction{model.Right, model.Down} {
		for _, text := range []bool{false, true} {
			g := dependencyGraph(t, dir)
			cfg := screen()
			if text {
				cfg = TextConfig(dir)
			}
			first := run(t, g, cfg)
			for range 49 {
				require.Equal(t, first, run(t, g, cfg))
			}
		}
	}
}

// TestArrange_DeterministicLoops is C1 for S6's Loops kept together: the
// loop transposition's swaps, in layer and pair order, and its side-aware
// count, sides held in edge order (flow-loop, where End leaves the loop,
// and neat/ml-training, whose two-cycle beside a longer loop has the
// side-aware count keep a swap out). It confirms the transposition swaps
// (a loops_kept decision) before checking that the arrangement repeats
// byte-identically, 50 times, under DOWN and RIGHT, in both profiles.
func TestArrange_DeterministicLoops(t *testing.T) {
	cases := []model.Graph{
		flowLoop(model.Down),
		graph(t, nil, "raw->split", "split->feat", "feat->train", "train->tune", "tune->train", "train->eval",
			"eval->good:diamond", "good->reg", "good->feat", "reg->deploy", "deploy->monitor", "monitor->retrain",
			"retrain->train", "eval->report"),
	}
	var buf bytes.Buffer
	ctx := layoutdbg.NewContext(context.Background(), slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	_, err := Arrange(ctx, cases[0], screen())
	require.NoError(t, err)
	kept := 0
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		if entry["decision"] == "loops_kept" {
			kept++
		}
	}
	require.Positive(t, kept, "the fixture must swap in the loop transposition")
	for _, g := range cases {
		for _, dir := range []model.Direction{model.Down, model.Right} {
			for _, text := range []bool{false, true} {
				g.Direction = dir
				cfg := screen()
				if text {
					cfg = TextConfig(dir)
				}
				first := run(t, g, cfg)
				for range 49 {
					require.Equal(t, first, run(t, g, cfg))
				}
			}
		}
	}
}

// TestLayout_Deterministic is C1 for the whole engine: 50 in-process runs
// of each adversarial graph give byte-identical positioned graphs. On top
// of TestArrange_Deterministic's tie-breaks, each case aims at one of S8,
// S10 and S11's: side attachments and the replayed second pass, lane
// packing with a released cycle, rails into one in-vertex, self-loops on a
// rect and on a diamond, labels competing for one spot, cardinalities and
// adornments, crossings, the text adapter with nodes grown for their
// ports, text labels snapped to whole cells and tested against the cells
// drawn on an axis the frame turns over, labels sliding along their
// routes, and every direction.
func TestLayout_Deterministic(t *testing.T) {
	cases := []struct {
		name  string
		specs []string
		dir   model.Direction
		text  bool
	}{
		{"back edges on both kinds of node", []string{"s->d:diamond", "d->a", "d->b", "a->s", "b->d", "top->bot", "bot->top"}, model.Down, false},
		{"lanes with a released cycle", []string{"a->d", "a->e", "a->f", "b->d", "b->e", "b->f", "c->d", "c->e", "c->f"}, model.Up, false},
		{"rails and self-loops", []string{"a->m:diamond", "b->m", "c->m", "m->m", "m->x", "x->x"}, model.Right, false},
		{"labels in one spot", []string{"a->b", "a->b", "a->b", "b->c", "a->c"}, model.Left, false},
		{"text", []string{"start->check:diamond", "check->ok", "check->retry", "retry->start", "ok->ok", "check->check"}, model.Down, true},
		{"text sideways", []string{"a->b", "a->c", "b->d", "c->d", "d->a", "a->d"}, model.Left, true},
		{"text faces grown for their ports", []string{"h->a", "h->b", "h->c", "h->d", "h->e", "h->f", "a->z", "b->z", "c->z", "d->z"}, model.Right, true},
		{"text labels on turned cells", []string{"a->b", "a->c", "a->d", "b->e", "c->e", "d->e", "a->e"}, model.Up, true},
		{"labels sliding past their turns", []string{"gw->users", "gw->orders", "gw->billing", "gw->search"}, model.Right, false},
		{"text labels sliding past their turns", []string{"gw->users", "gw->orders", "gw->billing", "gw->search"}, model.Left, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := graph(t, nil, tc.specs...)
			g.Direction = tc.dir
			for i := range g.Edges {
				g.Edges[i].Label = "label"
			}
			cfg := screen()
			if tc.text {
				cfg = TextConfig(tc.dir)
			}
			first := runLayout(t, g, cfg)
			for range 49 {
				require.Equal(t, first, runLayout(t, g, cfg))
			}
		})
	}
}

// TestLayout_DeterministicCompactLabels is C1 for S1's Compact labels:
// labels whose balanced splits tie on their widest line ("go go go" on two
// lines), whose line counts tie on their box ("a b" on a circle's and a
// hexagon's MinW) and that compact onto two lines, laid out 50 times.
func TestLayout_DeterministicCompactLabels(t *testing.T) {
	g := graph(t, map[string]string{"d": "Metrics healthy?", "h": "Redis Cache", "c": "go go go", "t": "a b", "u": "a b"},
		"d:diamond->h:hexagon", "d->c:circle", "h->t:circle", "c->u:hexagon", "t->d")
	pg, err := Layout(context.Background(), g, screen(), nil)
	require.NoError(t, err)
	lines := map[string][]string{}
	for _, n := range pg.Nodes {
		lines[n.ID] = n.Lines
	}
	require.Equal(t, map[string][]string{"d": {"Metrics", "healthy?"}, "h": {"Redis", "Cache"}, "c": {"go go", "go"}, "t": nil, "u": nil},
		lines, "the fixture must compact and tie")
	for _, dir := range []model.Direction{model.Down, model.Right} {
		g.Direction = dir
		first := runLayout(t, g, screen())
		for range 49 {
			require.Equal(t, first, runLayout(t, g, screen()))
		}
	}
}

// TestLayout_DeterministicClassDiagram is C1 for record boxes,
// adornments and cardinalities, among them a cardinality that follows its
// route around a turn (S10) under RIGHT.
func TestLayout_DeterministicClassDiagram(t *testing.T) {
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
	for _, cfg := range []Config{screen(), TextConfig(model.Down)} {
		first := runLayout(t, g, cfg)
		for range 49 {
			require.Equal(t, first, runLayout(t, g, cfg))
		}
	}
	crowd := model.Graph{Direction: model.Right,
		Nodes: []model.Node{
			{ID: "truck", Label: "Truck", Members: &model.Members{Attributes: []model.Member{{Visibility: "-", Text: "payload: t"}}}},
			{ID: "chargeable", Label: "Chargeable", Members: &model.Members{}},
			{ID: "fleet", Label: "Fleet", Members: &model.Members{Attributes: []model.Member{{Visibility: "-", Text: "name: string"}}}},
			{ID: "depot", Label: "Depot", Members: &model.Members{}},
		},
		Edges: []model.Edge{
			{ID: "truck->chargeable#0", From: "chargeable", To: "truck", Relation: model.RelationRealization},
			{ID: "fleet->truck#0", From: "fleet", To: "truck", Relation: model.RelationComposition, FromCard: "1", ToCard: "1..*"},
			{ID: "depot->truck#0", From: "depot", To: "truck", Relation: model.RelationAggregation, ToCard: "0..*"},
		},
	}
	for _, cfg := range []Config{screen(), TextConfig(model.Right)} {
		first := runLayout(t, crowd, cfg)
		for range 49 {
			require.Equal(t, first, runLayout(t, crowd, cfg))
		}
	}
}

func runLayout(t *testing.T, g model.Graph, cfg Config) string {
	t.Helper()
	pg, err := Layout(context.Background(), g, cfg, nil)
	require.NoError(t, err)
	out, err := json.Marshal(pg)
	require.NoError(t, err)
	return string(out)
}

func run(t *testing.T, g model.Graph, cfg Config) string {
	t.Helper()
	a, err := Arrange(context.Background(), g, cfg)
	require.NoError(t, err)
	out, err := json.Marshal(a)
	require.NoError(t, err)
	return string(out)
}

// TestLayout_DeterministicGroups is C1 for S9: 50 in-process runs of each
// nested graph give byte-identical positioned graphs. Each case aims at a
// nest tie-break: sibling groups whose seed keys tie (bands, S6), wires
// through two borders, back edges out of a group, parallel edges into one
// group (a bundle on its anchors, S8), wires into a group whose node's
// ports slide onto their terminals (S8), a group stretched in its row
// (S4), a title off its left slot (S10), an edgeless node riding with its
// row (S3, S7), and both profiles.
func TestLayout_DeterministicGroups(t *testing.T) {
	cases := []struct {
		name   string
		specs  []string
		groups []model.Group
		dir    model.Direction
		text   bool
	}{
		{"sibling groups", []string{"gw->cat", "gw->cart", "gw->ord", "ord->pay", "pay->stripe", "cat->pg", "ord->pg", "cart->redis"},
			[]model.Group{{ID: "svc", Label: "Services", Contains: []string{"cat", "cart", "ord", "pay"}}, {ID: "data", Label: "Data", Contains: []string{"pg", "redis"}}}, model.Down, false},
		{"two borders and a back edge", []string{"x->a", "a->b", "b->y", "y->a", "x->y"},
			[]model.Group{{ID: "outer", Label: "Outer", Contains: []string{"y"}, Children: []string{"inner"}}, {ID: "inner", Contains: []string{"a", "b"}}}, model.Right, false},
		{"ports sliding onto terminals", []string{"p->db:cylinder", "q->db"},
			[]model.Group{{ID: "data", Label: "Data", Contains: []string{"db"}}}, model.Right, false},
		{"a bundle and a stretched group", []string{"p->a", "p->b", "p->c", "c->d"},
			[]model.Group{{ID: "g", Contains: []string{"a", "b"}}, {ID: "h", Contains: []string{"c", "d"}}}, model.Up, false},
		{"text", []string{"x->a", "a->b", "b->y", "y->a", "x->y", "b->b"},
			[]model.Group{{ID: "outer", Label: "Outer", Contains: []string{"y"}, Children: []string{"inner"}}, {ID: "inner", Label: "In", Contains: []string{"a", "b"}}}, model.Left, true},
		{"a title in its right slot", []string{"x->a", "b"},
			[]model.Group{{ID: "g", Label: "Services", Contains: []string{"a", "b"}}}, model.Down, true},
		{"an edgeless node beside its row", []string{"lb->ing:hexagon", "ing->w", "w->db", "cp"},
			[]model.Group{{ID: "cluster", Label: "PCI cluster", Contains: []string{"cp", "ing", "w"}}}, model.Down, false},
		{"a lane cycle released at a crossing", []string{"n0:cylinder", "n1:hexagon", "n2:hexagon", "n3:parallelogram", "n4:circle", "n5:cylinder", "n6:rounded", "n7:hexagon",
			"n7->n2", "n3->n3", "n5->n3", "n6->n0", "n1->n5", "n7->n0", "n7->n6", "n7->n0", "n0->n5"},
			[]model.Group{{ID: "g0", Label: "G0", Contains: []string{"n2", "n5", "n6"}}, {ID: "g1", Label: "G1", Contains: []string{"n0", "n1", "n4"}}}, model.Down, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := grouped(graph(t, nil, tc.specs...), tc.groups...)
			g.Direction = tc.dir
			cfg := screen()
			if tc.text {
				cfg = TextConfig(tc.dir)
			}
			first := runLayout(t, g, cfg)
			for range 49 {
				require.Equal(t, first, runLayout(t, g, cfg))
			}
		})
	}
	// flow-client-server in text: labels over a frame side, in their
	// home group, and sliding along their runs; terminals one cell apart;
	// two jogs sharing a lane at exactly the in-lane gap (S7, S8, S10)
	for _, dir := range []model.Direction{model.Right, model.Down} {
		t.Run("client-server text "+dir.String(), func(t *testing.T) {
			g := clientServer(dir)
			first := runLayout(t, g, TextConfig(dir))
			for range 49 {
				require.Equal(t, first, runLayout(t, g, TextConfig(dir)))
			}
		})
	}
}

// TestLayout_DeterministicCongruences is C1 for S12: 50 in-process runs of
// each congruent graph give byte-identical positioned graphs. The cases
// aim at its maps and choices: nested sets whose copies reach one group
// twice, a fan whose ranks share lanes, labels placed with their copies,
// labels and copies that cannot clear each other (S10), a nested set
// degraded because its members copy apart, and a fan pair released
// because it cannot agree (S8).
func TestLayout_DeterministicCongruences(t *testing.T) {
	cases := []struct {
		name string
		g    func(t *testing.T) model.Graph
		dir  model.Direction
		text bool
	}{
		{"nested sets", datacenters, model.Down, false},
		{"nested sets in text", datacenters, model.Right, true},
		{"a fan beside another wire", threeDCs, model.Up, false},
		{"labels that cannot clear their copies", wideAndNarrow, model.Down, false},
		{"a nested set copied apart", copiedApart, model.Down, false},
		{"a fan pair that cannot agree", func(t *testing.T) model.Graph {
			// in text DOWN the pair of n3's fan cannot share a lane (S8)
			return grouped(graph(t, nil, "n3:hexagon->n2", "src->n2", "n3->m2", "src->m2"),
				model.Group{ID: "a", Contains: []string{"n2"}},
				model.Group{ID: "b", Contains: []string{"m2"}})
		}, model.Down, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := tc.g(t)
			g.Direction = tc.dir
			cfg := screen()
			if tc.text {
				cfg = TextConfig(tc.dir)
			}
			first := runLayout(t, g, cfg)
			require.Contains(t, first, "rep_edges")
			for range 49 {
				require.Equal(t, first, runLayout(t, g, cfg))
			}
		})
	}
}

// TestLayout_DeterministicAnchored is C1 for S13: 50 in-process runs of
// each changed graph anchored on its predecessor's carrier give
// byte-identical positioned graphs, carrier included. The cases aim at
// its maps and tie-breaks: adopted and rejected layers, a pushed new node,
// order seeds among unseeded vertices, stale entries, a moved node, a
// group that loses its first terminal row, a copying level, a flat edge's
// fallback the carrier keeps and one a leaf newly causes (S9, S13), and
// both profiles.
func TestLayout_DeterministicAnchored(t *testing.T) {
	grown := func(g model.Graph) model.Graph {
		g.Nodes = append(slices.Clone(g.Nodes), model.Node{ID: "m", Label: "Metrics"})
		g.Edges = append(slices.Clone(g.Edges), model.Edge{ID: "c->m#0", From: "c", To: "m"}, model.Edge{ID: "m->in#0", From: "m", To: "in"})
		return g
	}
	cases := []struct {
		name          string
		before, after func(t *testing.T) model.Graph
		dir           model.Direction
		text          bool
	}{
		{"a leaf and a new cycle", base215, func(t *testing.T) model.Graph { return grown(base215(t)) }, model.Down, false},
		{"a deletion", base215, func(t *testing.T) model.Graph { return without(base215(t), "q") }, model.Right, true},
		{"a moved node", datacenters, func(t *testing.T) model.Graph {
			g := datacenters(t)
			g.Groups[0].Contains = slices.DeleteFunc(slices.Clone(g.Groups[0].Contains), func(s string) bool { return s == "d1p_wrk" })
			g.Groups[1].Contains = append(slices.Clone(g.Groups[1].Contains), "d1p_wrk")
			return g
		}, model.Down, false},
		{"a copying level", twoDCs, func(t *testing.T) model.Graph { return without(twoDCs(t), "b_db") }, model.Up, true},
		{"a fallback kept", blocked, func(t *testing.T) model.Graph {
			g := blocked(t)
			g.Nodes = append(slices.Clone(g.Nodes), model.Node{ID: "z", Label: "z"})
			g.Edges = append(slices.Clone(g.Edges), model.Edge{ID: "b->z#0", From: "b", To: "z"})
			return g
		}, model.Down, false},
		{"a flat edge newly ranked", func(t *testing.T) model.Graph { return flatten(twoChains(t), "a1->b1#0") }, func(t *testing.T) model.Graph {
			g := flatten(twoChains(t), "a1->b1#0")
			g.Nodes = append(slices.Clone(g.Nodes), model.Node{ID: "leaf", Label: "leaf"})
			g.Edges = append(slices.Clone(g.Edges), model.Edge{ID: "leaf->a2#0", From: "leaf", To: "a2"})
			return g
		}, model.Right, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before, after := tc.before(t), tc.after(t)
			before.Direction, after.Direction = tc.dir, tc.dir
			cfg := screen()
			if tc.text {
				cfg = TextConfig(tc.dir)
			}
			prev, err := Layout(context.Background(), before, cfg, nil)
			require.NoError(t, err)
			prev.LayoutHints.Scope("").Layers["ghost"] = 3
			run := func() string {
				pg, err := Layout(context.Background(), after, cfg, prev.LayoutHints)
				require.NoError(t, err)
				out, err := json.Marshal(pg)
				require.NoError(t, err)
				return string(out)
			}
			first := run()
			for range 49 {
				require.Equal(t, first, run())
			}
		})
	}
}

// TestLayout_DeterministicFlatEdges is C1 for S9's Flat edges: the
// router's candidates and positions, tried in order, a flat edge routed
// beside another flat route drawn before it, and the fallback, which
// ranks the first flat edge in edge order that no candidate routes and
// redoes the layout. It confirms the fixture falls back (a
// flat_edge_ranked record) and routes flat edges too (flat_edge_routed)
// before checking that the layout repeats byte-identically, 50 times,
// under DOWN and RIGHT, in both profiles.
func TestLayout_DeterministicFlatEdges(t *testing.T) {
	g := flatten(graph(t, nil, "x->x2", "y->y2", "z->z2", "x->y", "y->z", "a->m", "m->b", "a->b"), "x->y#0", "y->z#0", "a->b#0")
	for i := range g.Edges {
		g.Edges[i].Label = "label"
	}
	var buf bytes.Buffer
	ctx := layoutdbg.NewContext(context.Background(), slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	_, err := Layout(ctx, g, screen(), nil)
	require.NoError(t, err)
	require.Contains(t, buf.String(), `"decision":"flat_edge_ranked"`, "the fixture must fall back")
	require.Contains(t, buf.String(), `"decision":"flat_edge_routed"`, "the fixture must route a flat edge")
	for _, dir := range []model.Direction{model.Down, model.Right} {
		for _, text := range []bool{false, true} {
			g.Direction = dir
			cfg := screen()
			if text {
				cfg = TextConfig(dir)
			}
			first := runLayout(t, g, cfg)
			for range 49 {
				require.Equal(t, first, runLayout(t, g, cfg))
			}
		}
	}
}
