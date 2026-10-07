package layered

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"maps"
	"math"
	"math/rand"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/layout/contract"
	"github.com/oxforge/diago/internal/layout/metrics"
	"github.com/oxforge/diago/internal/layoutdbg"
	"github.com/oxforge/diago/internal/model"
	"github.com/oxforge/diago/internal/schema"
)

// The random-graph probe: a dev tool, not a gate. It lays out random
// graphs the corpus does not cover and counts their contract violations.
//
//	go test ./internal/layout/layered/ -run TestProbe_Contract -probe -v
//	go test ./internal/layout/layered/ -run TestProbe_Contract -probe -probe.flat -probe.rule C9.1 -v
//	go test ./internal/layout/layered/ -run TestProbe_Contract -probe -probe.congruent -v
//	go test ./internal/layout/layered/ -run TestProbe_Anchor -probe -v
//	go test ./internal/layout/layered/ -run TestProbe_Dump -probe -probe.seed 145 -probe.dir DOWN -probe.flat -v
//	go test ./internal/layout/layered/ -run 'TestProbe_Contract|TestProbe_Anchor' -probe -probe.flatedges -v
//	go test ./internal/layout/layered/ -run TestProbe_Contract -probe -probe.decisions -v
//	go test ./internal/layout/layered/ -run TestProbe_Dump -probe -probe.congruent -probe.seed 348 -probe.dir RIGHT -probe.spec /tmp/x.json
var (
	probe          = flag.Bool("probe", false, "run the random-graph probe")
	probeSeeds     = flag.Int("probe.seeds", 400, "how many random graphs")
	probeFlat      = flag.Bool("probe.flat", false, "drop the groups")
	probeCong      = flag.Bool("probe.congruent", false, "copy a group beside itself, fed by one new node (S12)")
	probeFlatEdges = flag.Bool("probe.flatedges", false, "mark a random tenth of the edges flat (S9, Flat edges)")
	probeDecisions = flag.Bool("probe.decisions", false, "lay out random flowcharts of decisions instead (S7, Side-exit forcing)")
	probeRule      = flag.String("probe.rule", "", "print every violation of this rule")
	probeSeed      = flag.Int64("probe.seed", -1, "TestProbe_Dump: the graph to dump")
	probeDir       = flag.String("probe.dir", "DOWN", "TestProbe_Dump: the direction")
	probeText      = flag.Bool("probe.text", false, "TestProbe_Dump: the text profile")
	probeSpec      = flag.String("probe.spec", "", "TestProbe_Dump: also write the graph as a flow spec to this file (an absolute path: a relative one resolves against the package directory)")
)

// randomGraph builds a random flow graph from seed: 3 to 9 nodes of every
// shape, edges between random nodes with parallel edges and a few
// self-loops, and 1 to 3 groups over a random subset of the nodes, a
// group sometimes nested in the one before it.
func randomGraph(seed int64) model.Graph {
	rng := rand.New(rand.NewSource(seed))
	n := 3 + rng.Intn(7)
	shapes := []model.Shape{model.ShapeRect, model.ShapeRounded, model.ShapeCircle, model.ShapeDiamond, model.ShapeCylinder, model.ShapeHexagon, model.ShapeParallelogram}
	var g model.Graph
	for i := range n {
		id := fmt.Sprintf("n%d", i)
		g.Nodes = append(g.Nodes, model.Node{ID: id, Label: id, Shape: shapes[rng.Intn(len(shapes))]})
	}
	count := map[string]int{}
	for range n - 1 + rng.Intn(n+2) {
		a, b := rng.Intn(n), rng.Intn(n)
		if a == b && rng.Intn(4) != 0 {
			continue
		}
		key := fmt.Sprintf("n%d->n%d", a, b)
		g.Edges = append(g.Edges, model.Edge{ID: fmt.Sprintf("%s#%d", key, count[key]), From: fmt.Sprintf("n%d", a), To: fmt.Sprintf("n%d", b)})
		count[key]++
	}
	groups := 1 + rng.Intn(3)
	owner := make([]int, n)
	for i := range owner {
		owner[i] = rng.Intn(groups+1) - 1 // -1: the root
	}
	for gi := range groups {
		gr := model.Group{ID: fmt.Sprintf("g%d", gi), Label: fmt.Sprintf("G%d", gi), Contains: []string{}}
		if rng.Intn(3) == 0 {
			gr.Label = ""
		}
		for i := range owner {
			if owner[i] == gi {
				gr.Contains = append(gr.Contains, fmt.Sprintf("n%d", i))
			}
		}
		g.Groups = append(g.Groups, gr)
	}
	for gi := 1; gi < groups; gi++ {
		if rng.Intn(2) == 0 {
			g.Groups[gi-1].Children = append(g.Groups[gi-1].Children, g.Groups[gi].ID)
		}
	}
	return g
}

// decisionGraph builds a random flowchart from seed: 8 to 21 nodes, about
// half of them diamonds whose two exits, "yes" and "no", lead to later
// nodes, so decisions chain and often share a target, and the others
// rectangles with one exit to a later node or none; no groups.
func decisionGraph(seed int64) model.Graph {
	rng := rand.New(rand.NewSource(seed))
	n := 8 + rng.Intn(14)
	var g model.Graph
	for i := range n {
		shape := model.ShapeRect
		if i < n-2 && rng.Intn(2) == 0 {
			shape = model.ShapeDiamond
		}
		id := fmt.Sprintf("n%d", i)
		g.Nodes = append(g.Nodes, model.Node{ID: id, Label: id, Shape: shape})
	}
	for i, node := range g.Nodes[:n-1] {
		exits := 0
		switch {
		case node.Shape == model.ShapeDiamond:
			exits = 2
		case rng.Intn(4) != 0:
			exits = 1
		}
		targets := rng.Perm(n - 1 - i)[:min(exits, n-1-i)]
		for k, t := range targets {
			to := g.Nodes[i+1+t].ID
			e := model.Edge{ID: node.ID + "->" + to + "#0", From: node.ID, To: to}
			if exits == 2 {
				e.Label = []string{"yes", "no"}[k]
			}
			g.Edges = append(g.Edges, e)
		}
	}
	return g
}

// decisionCounts tallies the decision probe: every diamond exit by its
// bends, straight (0), an L (1) or more, and S7's records.
type decisionCounts struct {
	layouts, straight, ls, more int
	records                     map[string]int
}

func (c *decisionCounts) String() string {
	var recs []string
	for _, k := range slices.Sorted(maps.Keys(c.records)) {
		recs = append(recs, fmt.Sprintf("%s %d", k, c.records[k]))
	}
	return fmt.Sprintf("%d screen layouts; diamond exits: %d straight, %d L, %d more bends; records: %s",
		c.layouts, c.straight, c.ls, c.more, strings.Join(recs, ", "))
}

// add counts pg, laid out from g with the S7 records in log.
func (c *decisionCounts) add(g model.Graph, pg *model.PositionedGraph, log string) {
	c.layouts++
	shape := map[string]model.Shape{}
	for _, n := range g.Nodes {
		shape[n.ID] = n.Shape
	}
	for i, e := range pg.Edges {
		if shape[g.Edges[i].From] != model.ShapeDiamond {
			continue
		}
		switch len(e.Points) - 2 {
		case 0:
			c.straight++
		case 1:
			c.ls++
		default:
			c.more++
		}
	}
	for _, k := range []string{"secondary_room", "primary_denied", "primary_restored", "side_exit_forced", "side_exit_dropped"} {
		c.records[k] += strings.Count(log, `"decision":"`+k+`"`)
	}
}

// congruentGraph is g with its first group that holds nodes and no
// groups copied once or twice beside itself, in the same parent (S12):
// each copy with the group's nodes, the edges inside it, and the edges
// across its border to the same outside node; and a new node, src, with an
// edge to one node of the group and to its counterpart in every copy. g
// is unchanged when no group qualifies.
func congruentGraph(g model.Graph, seed int64) model.Graph {
	rng := rand.New(rand.NewSource(seed))
	gi := slices.IndexFunc(g.Groups, func(gr model.Group) bool { return len(gr.Contains) > 0 && len(gr.Children) == 0 })
	if gi < 0 {
		return g
	}
	gr := g.Groups[gi]
	in := map[string]bool{}
	for _, id := range gr.Contains {
		in[id] = true
	}
	shape := map[string]model.Shape{}
	for _, n := range g.Nodes {
		shape[n.ID] = n.Shape
	}
	fed := gr.Contains[rng.Intn(len(gr.Contains))]
	g.Nodes = append(g.Nodes, model.Node{ID: "src", Label: "src", Shape: model.ShapeRect})
	g.Edges = append(g.Edges, model.Edge{ID: "src->" + fed, From: "src", To: fed})
	edges := slices.Clone(g.Edges)
	for c := range 1 + rng.Intn(2) {
		name := func(id string) string {
			if in[id] {
				return fmt.Sprintf("%s_c%d", id, c)
			}
			return id
		}
		cp := model.Group{ID: fmt.Sprintf("%s_c%d", gr.ID, c), Label: gr.Label, Contains: []string{}}
		for _, id := range gr.Contains {
			g.Nodes = append(g.Nodes, model.Node{ID: name(id), Label: id, Shape: shape[id]})
			cp.Contains = append(cp.Contains, name(id))
		}
		for _, e := range edges {
			if in[e.From] || in[e.To] {
				g.Edges = append(g.Edges, model.Edge{ID: fmt.Sprintf("%s_c%d", e.ID, c), From: name(e.From), To: name(e.To)})
			}
		}
		g.Groups = append(g.Groups, cp)
		for pi := range g.Groups {
			if slices.Contains(g.Groups[pi].Children, gr.ID) {
				g.Groups[pi].Children = append(g.Groups[pi].Children, cp.ID)
			}
		}
	}
	return g
}

// flatEdgesGraph is g with a random tenth of its edges between two nodes
// flat, rounded up, so at least one when it has such an edge: never a
// self-loop, which cannot be flat (S9, Flat edges).
func flatEdgesGraph(g model.Graph, seed int64) model.Graph {
	rng := rand.New(rand.NewSource(^seed)) // apart from randomGraph's draws
	var loose []int
	for i, e := range g.Edges {
		if e.From != e.To {
			loose = append(loose, i)
		}
	}
	g.Edges = slices.Clone(g.Edges)
	for _, k := range rng.Perm(len(loose))[:(len(loose)+9)/10] {
		g.Edges[loose[k]].Flat = true
	}
	return g
}

// probeGraph is randomGraph(seed) in direction dir, without its groups
// under -probe.flat, with a group copied beside itself under
// -probe.congruent, and a tenth of its edges flat under -probe.flatedges.
func probeGraph(seed int64, dir model.Direction) model.Graph {
	if *probeDecisions {
		g := decisionGraph(seed)
		g.Direction = dir
		return g
	}
	g := randomGraph(seed)
	if *probeFlat {
		g.Groups = nil
	}
	if *probeCong {
		g = congruentGraph(g, seed)
	}
	if *probeFlatEdges {
		g = flatEdgesGraph(g, seed)
	}
	g.Direction = dir
	return g
}

// flowSpec is g, a probe graph, as a flow spec the CLI lays out as the
// probe does (-probe.spec): every edge keeps its id, and a group the probe
// left unlabelled takes its id as its label, since a spec group needs one.
func flowSpec(g model.Graph) schema.FlowSpec {
	spec := schema.FlowSpec{Type: "flow", Direction: g.Direction.String(), Nodes: []schema.NodeSpec{}, Edges: []schema.EdgeSpec{}}
	for _, n := range g.Nodes {
		spec.Nodes = append(spec.Nodes, schema.NodeSpec{ID: n.ID, Label: n.Label, Shape: n.Shape.String()})
	}
	for _, e := range g.Edges {
		id := e.ID
		spec.Edges = append(spec.Edges, schema.EdgeSpec{ID: &id, From: e.From, To: e.To, Label: e.Label, Flat: e.Flat})
	}
	for _, gr := range g.Groups {
		label := gr.Label
		if label == "" {
			label = gr.ID
		}
		spec.Groups = append(spec.Groups, schema.GroupSpec{ID: gr.ID, Label: label, Contains: append(slices.Clone(gr.Contains), gr.Children...)})
	}
	return spec
}

// flatRoutes counts the flat edges of a layout by what became of them
// (S9, Flat edges): routed as each kind of candidate, or fallen back.
type flatRoutes struct {
	marked, straight, along, l, z, other, ranked int
}

// add counts the flat edges of g in pg, g's layout under dir: a route of
// two points is a straight run, across the flow or along it, of three an
// L, of four a Z; an edge that fell back is marked FlatRanked.
func (c *flatRoutes) add(g model.Graph, pg *model.PositionedGraph, dir model.Direction) {
	for i, e := range pg.Edges {
		if !g.Edges[i].Flat {
			continue
		}
		c.marked++
		switch {
		case e.FlatRanked:
			c.ranked++
		case len(e.Points) == 2:
			from, _ := flowAxis(e.Points[0], dir)
			to, _ := flowAxis(e.Points[1], dir)
			if math.Abs(from-to) < 1e-6 {
				c.straight++
			} else {
				c.along++
			}
		case len(e.Points) == 3:
			c.l++
		case len(e.Points) == 4:
			c.z++
		default:
			c.other++
		}
	}
}

func (c flatRoutes) String() string {
	return fmt.Sprintf("%d flat edges: %d straight, %d straight along, %d L, %d Z, %d other, %d fell back",
		c.marked, c.straight, c.along, c.l, c.z, c.other, c.ranked)
}

// onFlatRoute reports whether violation v lies on a flat edge's kept
// route in pg, g's layout: its subject is one, or it names one as the
// other edge.
func onFlatRoute(g model.Graph, pg *model.PositionedGraph, v contract.Violation) bool {
	routed := func(id string) bool {
		i := slices.IndexFunc(g.Edges, func(e model.Edge) bool { return e.ID == id })
		return i >= 0 && g.Edges[i].Flat && !pg.Edges[i].FlatRanked
	}
	if routed(v.Subject) {
		return true
	}
	words := strings.Fields(v.Detail)
	for i := 0; i+1 < len(words); i++ {
		if words[i] == "edge" && routed(strings.TrimRight(words[i+1], ",")) {
			return true
		}
	}
	return false
}

// ruleKey is a violation's key in the probes' counts: its profile and its
// rule, prefixed "flat route" under -probe.flatedges when it lies on a flat
// edge's kept route (onFlatRoute).
func ruleKey(g model.Graph, pg *model.PositionedGraph, v contract.Violation, text bool) string {
	key := fmt.Sprintf("text=%v %s", text, v.Rule)
	if *probeFlatEdges && onFlatRoute(g, pg, v) {
		key = "flat route " + key
	}
	return key
}

// ordinary is g with every edge's flat flag cleared: the graph the probe
// lays out without -probe.flatedges.
func ordinary(g model.Graph) model.Graph {
	g.Edges = slices.Clone(g.Edges)
	for i := range g.Edges {
		g.Edges[i].Flat = false
	}
	return g
}

// TestProbe_Contract counts the contract violations of every random graph
// in every direction, screen and text, by profile and rule, and names the
// smallest graph that breaks each rule. Under -probe.flatedges it counts
// apart the violations on a flat edge's kept route, and what became of
// the flat edges (S9, Flat edges), and times each layout against the same
// graph with every edge ordinary. Under -probe.decisions it lays out
// random flowcharts of decisions and counts how their diamonds' exits
// route on screen, with S7's records.
func TestProbe_Contract(t *testing.T) {
	if !*probe {
		t.Skip("the random-graph probe runs with -probe")
	}
	counts := map[string]int{}
	type example struct {
		size int
		line string
	}
	smallest := map[string]example{}
	applied := 0
	var routes flatRoutes
	var withFlat, without time.Duration
	type slow struct {
		where          string
		flat, ordinary time.Duration
	}
	var slowest, ratio slow
	decisions := decisionCounts{records: map[string]int{}}
	for seed := range int64(*probeSeeds) {
		for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
			for _, text := range []bool{false, true} {
				g := probeGraph(seed, dir)
				cfg, lim := screen(), contract.ScreenLimits()
				if text {
					cfg, lim = TextConfig(dir), contract.TextLimits(dir)
				}
				where := fmt.Sprintf("seed=%d dir=%s text=%v", seed, dir, text)
				ctx := context.Background()
				var log bytes.Buffer
				if *probeDecisions && !text {
					ctx = layoutdbg.NewContext(ctx, slog.New(slog.NewJSONHandler(&log, &slog.HandlerOptions{Level: slog.LevelDebug})))
				}
				start := time.Now()
				pg, err := Layout(ctx, g, cfg, nil)
				took := time.Since(start)
				require.NoError(t, err, where)
				if *probeDecisions && !text {
					decisions.add(g, pg, log.String())
				}
				if *probeFlatEdges {
					routes.add(g, pg, dir)
					start = time.Now()
					_, err := Layout(context.Background(), ordinary(g), cfg, nil)
					plain := time.Since(start)
					require.NoError(t, err, where)
					withFlat, without = withFlat+took, without+plain
					s := slow{where, took, plain}
					if took > slowest.flat {
						slowest = s
					}
					if took > time.Millisecond && (ratio.ordinary == 0 || float64(took)/float64(plain) > float64(ratio.flat)/float64(ratio.ordinary)) {
						ratio = s
					}
				}
				if len(pg.AppliedCongruences) > 0 {
					applied++
				}
				for _, v := range contract.Check(pg, contract.Options{Limits: lim, Direction: dir, EdgeIDs: ids(g), Text: text}) {
					key := ruleKey(g, pg, v, text)
					counts[key]++
					line := fmt.Sprintf("%s nodes=%d edges=%d groups=%d %s: %s", where, len(g.Nodes), len(g.Edges), len(g.Groups), v.Subject, v.Detail)
					if v.Rule == *probeRule {
						t.Log(line)
					}
					size := 10*len(g.Nodes) + len(g.Edges)
					if cur, ok := smallest[key]; !ok || size < cur.size {
						smallest[key] = example{size, line}
					}
				}
			}
		}
	}
	for _, key := range slices.Sorted(maps.Keys(counts)) {
		t.Logf("%5d  %s  (smallest: %s)", counts[key], key, smallest[key].line)
	}
	t.Logf("%d layouts report a congruence", applied)
	if *probeDecisions {
		t.Logf("%s", &decisions)
	}
	if *probeFlatEdges {
		t.Logf("%s", routes)
		t.Logf("layout time with the flat edges %v, with every edge ordinary %v", withFlat.Round(time.Millisecond), without.Round(time.Millisecond))
		t.Logf("slowest with the flat edges: %s, %v against %v ordinary", slowest.where, slowest.flat.Round(time.Microsecond), slowest.ordinary.Round(time.Microsecond))
		t.Logf("highest ratio over 1 ms: %s, %v against %v ordinary", ratio.where, ratio.flat.Round(time.Microsecond), ratio.ordinary.Round(time.Microsecond))
	}
}

// TestProbe_Dump prints one random layout: its graph, node boxes, edge
// points (a flat edge marked flat, or ranked when it fell back) and the
// place, route and flat decision records.
func TestProbe_Dump(t *testing.T) {
	if !*probe || *probeSeed < 0 {
		t.Skip("dumps a layout with -probe -probe.seed N")
	}
	dir, err := model.ParseDirection(*probeDir)
	require.NoError(t, err)
	g := probeGraph(*probeSeed, dir)
	if *probeSpec != "" {
		raw, err := json.MarshalIndent(flowSpec(g), "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(*probeSpec, append(raw, '\n'), 0o644))
	}
	cfg := screen()
	if *probeText {
		cfg = TextConfig(dir)
	}
	var buf bytes.Buffer
	ctx := layoutdbg.NewContext(context.Background(), slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	pg, err := Layout(ctx, g, cfg, nil)
	require.NoError(t, err)
	for _, n := range g.Nodes {
		t.Logf("node %s %s", n.ID, n.Shape)
	}
	for _, gr := range g.Groups {
		t.Logf("group %s contains %v children %v", gr.ID, gr.Contains, gr.Children)
	}
	for _, n := range pg.Nodes {
		t.Logf("  %s at (%v, %v) %v x %v", n.ID, n.X, n.Y, n.Width, n.Height)
	}
	for _, gr := range pg.Groups {
		t.Logf("  group %s at (%v, %v) %v x %v", gr.ID, gr.X, gr.Y, gr.Width, gr.Height)
	}
	for i, e := range pg.Edges {
		mark := ""
		switch {
		case e.FlatRanked:
			mark = " (flat, ranked)"
		case g.Edges[i].Flat:
			mark = " (flat)"
		}
		t.Logf("  %s%s %v", e.ID, mark, e.Points)
	}
	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.Contains(line, `"phase":"route"`) || strings.Contains(line, `"phase":"place"`) || strings.Contains(line, `"phase":"cycle"`) || strings.Contains(line, `"phase":"flat"`) {
			t.Log("  ", line)
		}
	}
}

// TestProbe_Anchor checks C18's guarantees on every random graph in every
// direction, screen and text (S13): anchored on its own carrier, the
// layout reproduces itself (1); with the middle node deleted, anchoring
// once and twice agree (5); and with a leaf added, under a node or feeding
// it, every reversed edge stays reversed (3) and every survivor keeps its
// layer against the others (4). It counts the failures of each, the pairs
// of survivors that trade places in a layer (which 4 allows only for
// fewer crossings), how far the placement moves the survivors on screen,
// which 4 does not bound (Q6, metrics.MeasureChurn), each for the leaf
// under and the leaf above, and the contract violations of the grown and
// the smaller anchored layouts beyond those of the same graphs laid out
// fresh, by rule. A leaf layout whose carrier ranks a flat edge the fresh
// layout routed flat is exempt from 3 and 4 (C18, metrics.NewlyRanked):
// its failures of them are counted apart, as exempt. Under -probe.flatedges
// it counts apart the violations on a flat edge's kept route, what became
// of the flat edges in the fresh and in the anchored layouts, and the
// failures of 3, 4 and 5 where the two layouts compared fell back on
// different flat edges (S9, Flat edges).
func TestProbe_Anchor(t *testing.T) {
	if !*probe {
		t.Skip("the random-graph probe runs with -probe")
	}
	encode := func(pg *model.PositionedGraph) string {
		b, err := json.Marshal(pg)
		require.NoError(t, err)
		return string(b)
	}
	sides := []string{"under", "above"}
	var exact, converge, sticks, total int
	var layers, swapped, exemptLayers [2]int
	exemptSticks := 0
	var across, along [2][]float64
	beyond := map[string]int{}
	var freshRoutes, anchoredRoutes flatRoutes
	// fellBack lists the flat edges pg ranked; differ counts, per
	// guarantee, the failures between two layouts that fell back on
	// different ones.
	fellBack := func(pg *model.PositionedGraph) []string {
		var out []string
		for _, e := range pg.Edges {
			if e.FlatRanked {
				out = append(out, e.ID)
			}
		}
		return out
	}
	differ := map[string]int{}
	var newlyLeaves, exemptLeaves int // leaf layouts where a flat edge newly falls back, and those of them that fail 3 or 4
	fallbacks := func(guarantee string, a, b *model.PositionedGraph, n int) string {
		if fa, fb := fellBack(a), fellBack(b); !slices.Equal(fa, fb) {
			differ[guarantee] += n
			return fmt.Sprintf(" (fell back on %v, then on %v)", fa, fb)
		}
		return ""
	}
	for seed := range int64(*probeSeeds) {
		for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
			for _, text := range []bool{false, true} {
				g := probeGraph(seed, dir)
				cfg := screen()
				if text {
					cfg = TextConfig(dir)
				}
				lay := func(g model.Graph, prev *model.LayoutHints) *model.PositionedGraph {
					pg, err := Layout(context.Background(), g, cfg, prev)
					require.NoError(t, err, "seed %d %s text=%v", seed, dir, text)
					return pg
				}
				where := fmt.Sprintf("seed=%d dir=%s text=%v", seed, dir, text)
				total++
				fresh := lay(g, nil)
				freshRoutes.add(g, fresh, dir)
				if encode(fresh) != encode(lay(g, fresh.LayoutHints)) {
					exact++
					t.Logf("C18.1 %s", where)
				}
				smaller := without(g, g.Nodes[len(g.Nodes)/2].ID)
				once := lay(smaller, fresh.LayoutHints)
				if twice := lay(smaller, once.LayoutHints); encode(once) != encode(twice) {
					converge++
					t.Logf("C18.5 %s%s", where, fallbacks("C18.5", once, twice, 1))
				}
				type anchoredLayout struct {
					g  model.Graph
					pg *model.PositionedGraph
				}
				all := []anchoredLayout{{smaller, once}}
				at := g.Nodes[int(seed)%len(g.Nodes)].ID
				for side, name := range sides {
					from, to := at, "leaf"
					if name == "above" {
						from, to = to, from
					}
					grown := g
					grown.Nodes = append(slices.Clone(g.Nodes), model.Node{ID: "leaf", Label: "Leaf"})
					grown.Edges = append(slices.Clone(g.Edges), model.Edge{ID: "leaf-edge", From: from, To: to})
					after := lay(grown, fresh.LayoutHints)
					all = append(all, anchoredLayout{grown, after})
					newly := metrics.NewlyRanked(fresh.LayoutHints, after.LayoutHints, flatIDs(g))
					if newly != nil {
						newlyLeaves++
					}
					exempt := false
					for _, id := range fresh.LayoutHints.Reversed {
						if !slices.Contains(after.LayoutHints.Reversed, id) {
							if newly != nil {
								exemptSticks++
								exempt = true
								t.Logf("C18.3 exempt %s leaf %s: %s, newly ranked %v", where, name, id, newly)
								continue
							}
							sticks++
							t.Logf("C18.3 %s leaf %s: %s%s", where, name, id, fallbacks("C18.3", fresh, after, 1))
						}
					}
					c := metrics.MeasureChurn(fresh, after, dir)
					switch {
					case c.Relayered > 0 && newly != nil:
						exemptLayers[side] += c.Relayered
						exempt = true
						t.Logf("C18.4 exempt %s leaf %s: %d survivors on another layer, newly ranked %v", where, name, c.Relayered, newly)
					case c.Relayered > 0:
						layers[side] += c.Relayered
						t.Logf("C18.4 %s leaf %s: %d survivors on another layer%s", where, name, c.Relayered, fallbacks("C18.4", fresh, after, c.Relayered))
					}
					if exempt {
						exemptLeaves++
					}
					swapped[side] += c.Swapped
					if !text {
						across[side], along[side] = append(across[side], c.Across), append(along[side], c.Along)
					}
				}
				lim := contract.ScreenLimits()
				if text {
					lim = contract.TextLimits(dir)
				}
				for _, anchored := range all {
					anchoredRoutes.add(anchored.g, anchored.pg, dir)
					o := contract.Options{Limits: lim, Direction: dir, EdgeIDs: ids(anchored.g), Text: text}
					was := map[string]int{}
					own := lay(anchored.g, nil)
					for _, v := range contract.Check(own, o) {
						was[ruleKey(anchored.g, own, v, text)]++
					}
					for _, v := range contract.Check(anchored.pg, o) {
						key := ruleKey(anchored.g, anchored.pg, v, text)
						if was[key]--; was[key] < 0 {
							beyond[key]++
							t.Logf("beyond %s: %s %s: %s", where, v.Rule, v.Subject, v.Detail)
						}
					}
				}
			}
		}
	}
	pct := func(v []float64, p int) float64 { return v[(len(v)-1)*p/100] }
	t.Logf("%d layouts: C18.1 %d, C18.5 %d, C18.3 %d, C18.4 layers %d under / %d above; survivor pairs swapped %d under / %d above",
		total, exact, converge, sticks, layers[0], layers[1], swapped[0], swapped[1])
	t.Logf("exempt, a flat edge newly falling back: C18.3 %d, C18.4 layers %d under / %d above", exemptSticks, exemptLayers[0], exemptLayers[1])
	for _, key := range slices.Sorted(maps.Keys(beyond)) {
		t.Logf("%5d  %s beyond the fresh layouts' own", beyond[key], key)
	}
	if *probeFlatEdges {
		t.Logf("fresh: %s", freshRoutes)
		t.Logf("anchored: %s", anchoredRoutes)
		t.Logf("of the C18 failures, between layouts that fell back on different flat edges: C18.5 %d, C18.3 %d, C18.4 layers %d",
			differ["C18.5"], differ["C18.3"], differ["C18.4"])
		t.Logf("leaf layouts where a flat edge newly falls back: %d, of which %d exempt from a failure of C18.3 or C18.4", newlyLeaves, exemptLeaves)
	}
	for side, name := range sides {
		slices.Sort(across[side])
		slices.Sort(along[side])
		t.Logf("leaf %s: screen survivors' worst move after the median shift, across / along the flow: median %.0f / %.0f px, p90 %.0f / %.0f, max %.0f / %.0f", name,
			pct(across[side], 50), pct(along[side], 50), pct(across[side], 90), pct(along[side], 90), pct(across[side], 100), pct(along[side], 100))
	}
}

// TestFlowSpec_ReadsBackAsTheProbeGraph writes probe graphs of every mode
// as flow specs (-probe.spec) and parses them back: nodes, edges and
// groups return as the probe made them, except that a group the probe
// left unlabelled, which a spec cannot express, returns labelled with its
// id.
func TestFlowSpec_ReadsBackAsTheProbeGraph(t *testing.T) {
	for seed := range int64(40) {
		for name, g := range map[string]model.Graph{
			"plain":      randomGraph(seed),
			"congruent":  congruentGraph(randomGraph(seed), seed),
			"flat edges": flatEdgesGraph(randomGraph(seed), seed),
			"decisions":  decisionGraph(seed),
		} {
			at := fmt.Sprintf("seed %d, %s", seed, name)
			g.Direction = model.Right
			raw, err := json.Marshal(flowSpec(g))
			require.NoError(t, err, at)
			back, err := schema.ParseFlow(raw)
			require.NoError(t, err, at)
			assert.Equal(t, model.Right, back.Direction, at)
			require.Len(t, back.Nodes, len(g.Nodes), at)
			for i, n := range g.Nodes {
				b := back.Nodes[i]
				assert.Equal(t, []any{n.ID, n.Label, n.Shape}, []any{b.ID, b.Label, b.Shape}, at)
			}
			require.Len(t, back.Edges, len(g.Edges), at)
			for i, e := range g.Edges {
				b := back.Edges[i]
				assert.Equal(t, []any{e.ID, e.From, e.To, e.Label, e.Flat}, []any{b.ID, b.From, b.To, b.Label, b.Flat}, at)
			}
			require.Len(t, back.Groups, len(g.Groups), at)
			for i, gr := range g.Groups {
				label := gr.Label
				if label == "" {
					label = gr.ID
				}
				b := back.Groups[i]
				assert.Equal(t, []any{gr.ID, label}, []any{b.ID, b.Label}, at)
				assert.ElementsMatch(t, gr.Contains, b.Contains, at)
				assert.ElementsMatch(t, gr.Children, b.Children, at)
			}
		}
	}
}

// TestOnFlatRoute pins onFlatRoute on the contract's own violations, so
// that a reworded detail cannot drop a flat route's violations from the
// probe's "flat route" counts unnoticed. Five parallel runs 4 px apart,
// under the track gap (C9.2), each pair reported once, under the earlier
// edge: q, ordinary, beside f, a flat edge routed flat, beside o, ordinary,
// beside r, a flat edge that fell back (FlatRanked), beside p, ordinary. A
// violation lies on a flat route when its subject is f (f beside o), or
// its detail names f as the other edge (q beside f); one between o and r,
// or r and p, does not.
func TestOnFlatRoute(t *testing.T) {
	ids := []string{"q", "f", "o", "r", "p"}
	g := model.Graph{}
	pg := &model.PositionedGraph{}
	for i, id := range ids {
		x := 42 + 4*float64(i)
		g.Edges = append(g.Edges, model.Edge{ID: id, From: id + "1", To: id + "2", Flat: id == "f" || id == "r"})
		pg.Edges = append(pg.Edges, model.PositionedEdge{ID: id, From: id + "1", To: id + "2", FlatRanked: id == "r",
			Points: []model.Point{{X: x, Y: 100}, {X: x, Y: 200}}})
		for j, end := range []string{id + "1", id + "2"} { // the ends, out of the way
			pg.Nodes = append(pg.Nodes, model.PositionedNode{ID: end, Shape: model.ShapeRect, X: 1000 + 100*float64(2*i+j), Width: 4, Height: 4})
		}
	}
	var on, off []string
	for _, v := range contract.Check(pg, contract.Options{Limits: contract.ScreenLimits(), Direction: model.Down}) {
		if v.Rule != "C9.2" {
			continue
		}
		if onFlatRoute(g, pg, v) {
			on = append(on, v.Subject+": "+v.Detail)
		} else {
			off = append(off, v.Subject+": "+v.Detail)
		}
	}
	assert.Equal(t, []string{
		"q: segment 0 is 4.00 px from segment 0 of edge f, under the 8 px track gap",
		"f: segment 0 is 4.00 px from segment 0 of edge o, under the 8 px track gap",
	}, on)
	assert.Equal(t, []string{
		"o: segment 0 is 4.00 px from segment 0 of edge r, under the 8 px track gap",
		"r: segment 0 is 4.00 px from segment 0 of edge p, under the 8 px track gap",
	}, off)
}
