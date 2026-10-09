package layered

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"slices"

	"github.com/oxforge/diago/internal/layout/layered/equalize"
	"github.com/oxforge/diago/internal/layout/layered/frame"
	"github.com/oxforge/diago/internal/layout/layered/lgraph"
	"github.com/oxforge/diago/internal/layout/layered/order"
	"github.com/oxforge/diago/internal/layout/layered/place"
	"github.com/oxforge/diago/internal/layout/layered/ports"
	"github.com/oxforge/diago/internal/layout/layered/rank"
	"github.com/oxforge/diago/internal/layout/layered/route"
	"github.com/oxforge/diago/internal/layout/layered/size"
	"github.com/oxforge/diago/internal/layoutdbg"
	"github.com/oxforge/diago/internal/model"
)

// levelLayout is one level laid out through routing (S3 to S8), in the
// level's own top-to-bottom frame.
type levelLayout struct {
	Level  *lgraph.Level // with equalized extents (S4)
	Layers []int
	Graph  *lgraph.Graph
	X      []float64
	Route  *route.Result
}

// arrangeLevel runs S3 to S8 over lv, whose edges reversed orients (S2):
// the room self-loops and crowded faces need, the layers, equalized extents,
// the layered graph, its order, the placement and the routes, place and
// route a second time when the router attaches an end at a side. A
// non-nil seed gives each level node's position in the whole-graph seed
// (S9), which orders the layers before S6; fans are the level's
// congruence fans, whose spread runs route pairs on one lane (S12); a
// non-nil pv is the level's slice of a previous layout (S13).
func arrangeLevel(ctx context.Context, lv *lgraph.Level, reversed []bool, cfg Config, dir model.Direction, sd *seeding, fans [][]int, pv *prevScope) (*levelLayout, error) {
	pl, err := planLevel(ctx, lv, reversed, cfg, dir, sd, pv)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("layered: %w", err)
	}
	pl.route.Fans = fans
	lg, po, ro := pl.Graph, pl.place, pl.route
	x := place.Place(ctx, lg, po)
	routed := route.Route(ctx, lg, x, ro)
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("layered: %w", err)
	}
	if anySide(routed.Side) {
		po.Side, ro.Side = routed.Side, routed.Side
		x = place.Place(ctx, lg, po)
		routed = route.Route(ctx, lg, x, ro)
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("layered: %w", err)
		}
	}
	pl.X, pl.Route = x, routed
	return &pl.levelLayout, nil
}

// plan is a level through S6, with the options S7 and S8 take.
type plan struct {
	levelLayout
	place place.Options
	route route.Options
}

// planLevel runs S3 to S6 over lv: the room self-loops and crowded faces
// need, the layers with the isolated nodes placed, equalized extents, the
// layered graph with its corridors seated (S5) and its order, anchored on
// pv when it is not nil (S13): its layers, aligned, where the level allows
// them, its order as the seed, each slot on the layer its row's kept
// nodes held, and when it seeds any vertex an anchored order (S6: the
// anchored sweep budget and no loop transposition).
func planLevel(ctx context.Context, lv *lgraph.Level, reversed []bool, cfg Config, dir model.Direction, sd *seeding, pv *prevScope) (*plan, error) {
	loopRoom(lv, cfg)
	faceRoom(ctx, lv, reversed, cfg, dir)
	var previous []int
	if pv != nil {
		previous = pv.layers(lv)
	}
	layers, kept := rank.AssignKept(ctx, lv, reversed, previous)
	layers = rank.Isolated(ctx, lv, reversed, layers, kept, rank.Room{Gap: cfg.NodeGap, Dummy: cfg.DummyWidth})
	// A stretched parallelogram widens by its slant's gain at each end, so
	// the stretch its top and bottom edges share, its faces' usable span
	// (S8), keeps its width.
	slant := 0.0
	if !cfg.Text && (dir == model.Down || dir == model.Up) {
		slant = 2 * ports.Slant
	}
	lv = equalize.Apply(ctx, lv, layers, slant)
	lg, err := lgraph.Build(lv, layers, reversed, cfg.DummyWidth)
	if err != nil {
		return nil, fmt.Errorf("layered: %w", err)
	}
	lg.SeatCorridors(ctx)
	if sd != nil {
		seedOrder(lg, sd)
	}
	anchored := false
	if pv != nil {
		anchored = pv.seed(ctx, lg, kept) > 0
	}
	lg.Layers = order.Minimize(ctx, lg, anchored)
	return &plan{
		levelLayout: levelLayout{Level: lv, Layers: layers, Graph: lg},
		place: place.Options{
			Gap: cfg.NodeGap, TerminalGap: cfg.TerminalGap, Span: cfg.Span, Reach: cfg.Reach, Clearance: cfg.Clearance,
			InLaneGap: cfg.InLaneGap, Dir: dir, Text: cfg.Text,
		},
		route: route.Options{
			Clearance: cfg.Clearance, Span: cfg.Span, Reach: cfg.Reach,
			BaseGap: cfg.BaseGap, LaneGap: cfg.LaneGap, InLaneGap: cfg.InLaneGap, Snap: cfg.Snap,
			Dir: dir, Text: cfg.Text,
		},
	}, nil
}

// seeding places a level's nodes in the whole-graph seed (S9): per level
// node, a key, a tie-break, and whether the tie-break binds a terminal.
type seeding struct {
	key, tie []float64
	hard     []bool
}

// seedOrder sorts every layer of lg by the whole-graph seed (S9): a node
// by its key and then its tie-break, a dummy by its place between its
// chain's ends' keys, and what stays tied in the declaration seed. Then
// every run of terminals forms bands that S6 keeps in order: one per key,
// and one of its own for each terminal whose tie-break binds.
func seedOrder(lg *lgraph.Graph, sd *seeding) {
	key := make([]float64, len(lg.Vertices))
	sub := make([]float64, len(lg.Vertices))
	for v, vx := range lg.Vertices {
		if vx.Node >= 0 {
			key[v], sub[v] = sd.key[vx.Node], sd.tie[vx.Node]
			continue
		}
		chain := lg.Chains[vx.Edge]
		upper, lower := lg.Vertices[chain[0]], lg.Vertices[chain[len(chain)-1]]
		t := float64(vx.Layer-upper.Layer) / float64(lower.Layer-upper.Layer)
		key[v] = sd.key[upper.Node] + t*(sd.key[lower.Node]-sd.key[upper.Node])
	}
	lg.Bands = make([]int, len(lg.Vertices))
	band := -1
	for _, layer := range lg.Layers {
		slices.SortStableFunc(layer, func(a, b int) int {
			return cmp.Or(cmp.Compare(key[a], key[b]), cmp.Compare(sub[a], sub[b]))
		})
		prev := -1
		for _, v := range layer {
			lg.Bands[v] = -1
			n := lg.Vertices[v].Node
			if n < 0 || !lg.Level.Nodes[n].Terminal {
				prev = -1
				continue
			}
			if prev < 0 || key[prev] != key[v] || sd.hard[n] || sd.hard[lg.Vertices[prev].Node] {
				band++
			}
			lg.Bands[v], prev = band, v
		}
	}
}

// resolve returns the direction g lays out in (S11), or an error when cfg
// is a text Config built for AUTO or for the other axis (S14).
func resolve(ctx context.Context, g model.Graph, cfg Config) (model.Direction, error) {
	dir := frame.Resolve(ctx, g)
	if cfg.Text && (cfg.Dir == model.Auto || sideways(cfg.Dir) != sideways(dir)) {
		return dir, fmt.Errorf("layered: text Config built for %s, graph lays out %s; build it with TextConfig for the resolved direction", cfg.Dir, dir)
	}
	return dir, nil
}

// loopRoom gives every node with self-loops that is not pinned at least
// cfg.LoopMinH on the flow axis, and with n loops at least (4n - 2)
// LaneGaps, room for the stubs of its nested loops (S8): the innermost
// loop's two ports then keep LaneGap apart.
func loopRoom(lv *lgraph.Level, cfg Config) {
	for v, n := range lv.SelfLoops() {
		node := &lv.Nodes[v]
		if n == 0 || ports.Pinned(node.Shape, cfg.Text) {
			continue
		}
		node.H = max(node.H, cfg.LoopMinH, float64(4*n-2)*cfg.LaneGap)
	}
}

// faceRoom widens every node with faces whose busier face, its in-face or
// its out-face, holds more chain ends than it has room for (S8, Shape
// ports): in the text profile more than it has cells inside its corners,
// to two cells more than those ends, rounded up to even under DOWN and UP,
// where S1 keeps a node's width even, so every port gets its own cell; on
// screen to the width whose usable span spreads them InLaneGap / 2 apart,
// a rake's port gap (C9.2). A face's ends are every edge's end on it but a
// self-loop's, a counter-flow end counted even where the router later
// attaches it at a side. Each node that grows is logged.
func faceRoom(ctx context.Context, lv *lgraph.Level, reversed []bool, cfg Config, dir model.Direction) {
	ends := make([][2]int, len(lv.Nodes))
	for e := range lv.Edges {
		if lv.SelfLoop(e) {
			continue
		}
		upper, lower := lv.Oriented(e, reversed)
		ends[upper][ports.Head]++
		ends[lower][ports.Tail]++
	}
	for i, n := range ends {
		node := &lv.Nodes[i]
		// A group is sized by its content and anchors its ports, and a
		// terminal holds one end (S9): neither grows. A pinned node's
		// ends share its vertices (C8).
		if node.Group || node.Terminal || ports.Pinned(node.Shape, cfg.Text) {
			continue
		}
		busier := max(n[0], n[1])
		w := ports.Room(node.Shape, float64(busier+1)*cfg.InLaneGap/2, node.H, dir)
		if cfg.Text {
			w = float64(busier + 2)
			if !sideways(cfg.Dir) {
				w += math.Mod(w, 2)
			}
		}
		if w <= node.W {
			continue
		}
		layoutdbg.Decision(ctx, "face_widened",
			"phase", "route", "module", "diago", "spec_ref", "S8",
			"node", node.ID, "ends", busier, "from", node.W, "to", w)
		node.W = w
	}
}

// anySide reports whether any chain end is attached at a side: a side face
// or a pinned node's side vertex.
func anySide(side [][2]ports.Vertex) bool {
	for _, s := range side {
		if s[0] != ports.Bottom || s[1] != ports.Bottom {
			return true
		}
	}
	return false
}

// level is g as the engine's single level: node extents in the frame (the
// measured extents, swapped under RIGHT and LEFT, where the cross axis is
// vertical) and edges by node index.
func level(g model.Graph, sizes []size.Size, dir model.Direction) (*lgraph.Level, error) {
	across := sideways(dir)
	index := make(map[string]int, len(g.Nodes))
	lv := &lgraph.Level{Nodes: make([]lgraph.Node, len(g.Nodes)), Edges: make([]lgraph.Edge, len(g.Edges))}
	for i, n := range g.Nodes {
		index[n.ID] = i
		w, h := sizes[i].W, sizes[i].H
		if across {
			w, h = h, w
		}
		lv.Nodes[i] = lgraph.Node{ID: n.ID, Shape: n.Shape, Record: n.Members != nil, W: w, H: h}
	}
	for i, e := range g.Edges {
		from, okFrom := index[e.From]
		to, okTo := index[e.To]
		if !okFrom || !okTo {
			return nil, fmt.Errorf("layered: edge %s joins %s and %s, not both nodes", e.ID, e.From, e.To)
		}
		lv.Edges[i] = lgraph.Edge{ID: e.ID, From: from, To: to}
	}
	return lv, nil
}
