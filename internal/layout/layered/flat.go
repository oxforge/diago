package layered

import (
	"context"

	"github.com/oxforge/diago/internal/layout/layered/flat"
	"github.com/oxforge/diago/internal/layout/layered/size"
	"github.com/oxforge/diago/internal/layoutdbg"
	"github.com/oxforge/diago/internal/model"
)

// composed lays g out through S9 and routes its flat edges on the
// composed layout (S9, Flat edges): the levels hold the other edges alone,
// and a flat edge the router keeps no route for falls back to an ordinary
// edge, ranked on a redone layout. It returns the positioned graph in the
// engine's frame with every edge of g, in g's order, its carrier listing
// the flat edges that fell back (C18, ranked). An anchored layout (prev
// not nil) starts with the flat edges prev lists there already ranked
// (S13), so that it falls back wherever its previous layout did. A graph
// without flat edges is nested's layout itself.
func composed(ctx context.Context, g model.Graph, sizes []size.Size, cfg Config, dir model.Direction, prev *model.LayoutHints) (*model.PositionedGraph, error) {
	ranked := make([]bool, len(g.Edges))
	flats := false
	for i, e := range g.Edges {
		ranked[i] = !e.Flat
		flats = flats || e.Flat
	}
	keepRanked(ctx, g, prev, ranked)
	if !flats {
		return nested(ctx, g, sizes, cfg, dir, prev)
	}
	pg, err := settle(ctx, g, sizes, cfg, dir, prev, ranked)
	if err != nil {
		return nil, err
	}
	writeRanked(pg.LayoutHints, g, ranked)
	return pg, nil
}

// settle lays out g's ranked edges (ranked, per edge of g) and routes the
// others, its flat edges, on the composed layout in edge order (S9, Flat
// edges). When the router keeps no route for one, settle ranks it in
// ranked, records it, and lays out again, until every flat edge is routed
// or ranked: at most one redo per flat edge. Every flat edge it ranked, or
// that ranked held already, is marked FlatRanked.
func settle(ctx context.Context, g model.Graph, sizes []size.Size, cfg Config, dir model.Direction, prev *model.LayoutHints, ranked []bool) (*model.PositionedGraph, error) {
	o := flatOptions(cfg, dir)
	for {
		pg, err := nestedRanked(ctx, g, sizes, cfg, dir, prev, ranked)
		if err != nil {
			return nil, err
		}
		failed := -1
		for i := range g.Edges {
			if ranked[i] {
				continue
			}
			res := flat.Route(ctx, pg, i, o)
			if res.Kind == flat.None {
				failed = i
				break
			}
			pg.Edges[i].Points = res.Points
		}
		if failed < 0 {
			for i, e := range g.Edges {
				pg.Edges[i].FlatRanked = e.Flat && ranked[i]
			}
			return pg, nil
		}
		ranked[failed] = true
		layoutdbg.Decision(ctx, "flat_edge_ranked",
			"phase", "flat", "module", "diago", "spec_ref", "S9",
			"edge", g.Edges[failed].ID, "anchored", prev != nil)
	}
}

// nestedRanked lays out g with its ranked edges alone (S9) and returns the
// positioned graph with every edge of g, in g's order: a ranked edge with
// its route, any other with none yet. The congruences' edge indices (S12)
// are g's.
func nestedRanked(ctx context.Context, g model.Graph, sizes []size.Size, cfg Config, dir model.Direction, prev *model.LayoutHints, ranked []bool) (*model.PositionedGraph, error) {
	rg := g
	rg.Edges = make([]model.Edge, 0, len(g.Edges))
	var at []int // per edge of rg: its index in g
	for i, e := range g.Edges {
		if ranked[i] {
			rg.Edges = append(rg.Edges, e)
			at = append(at, i)
		}
	}
	pg, err := nested(ctx, rg, sizes, cfg, dir, prev)
	if err != nil {
		return nil, err
	}
	edges := make([]model.PositionedEdge, len(g.Edges))
	for i, e := range g.Edges {
		if !ranked[i] {
			edges[i] = positionedEdge(e)
		}
	}
	for j, i := range at {
		edges[i] = pg.Edges[j]
	}
	pg.Edges = edges
	remap := func(es []int) {
		for k, e := range es {
			es[k] = at[e]
		}
	}
	for c := range pg.AppliedCongruences {
		ac := &pg.AppliedCongruences[c]
		remap(ac.RepEdges)
		remap(ac.FanOut)
		for m := range ac.Members {
			remap(ac.Members[m].Edges)
		}
	}
	return pg, nil
}

// flatOptions are the distances a flat route keeps (S9, Flat edges), from
// cfg (S14): on screen each is one px value on both axes; in the text
// profile, where the engine's x counts columns and its y rows under DOWN
// and UP (the other way round under RIGHT and LEFT) and a row is two
// columns tall, the node clearance along the flow is as many px as across
// it, and the port gap one cell across the flow and as many px along it.
func flatOptions(cfg Config, dir model.Direction) flat.Options {
	o := flat.Options{
		Clearance: flat.XY{X: cfg.Clearance, Y: cfg.Clearance},
		Stub:      flat.XY{X: cfg.Reach, Y: (cfg.BaseGap + cfg.LaneGap) / 2},
		TrackGap:  flat.XY{X: cfg.InLaneGap, Y: cfg.LaneGap},
		PortGap:   flat.XY{X: cfg.InLaneGap / 2, Y: cfg.InLaneGap / 2},
		Dir:       dir,
		Text:      cfg.Text,
	}
	if cfg.Text {
		ux, uy := cellW, cellH // px per unit of the engine's x and y
		if sideways(dir) {
			ux, uy = cellH, cellW
		}
		o.Clearance.Y = cfg.Clearance * ux / uy
		o.PortGap = flat.XY{X: 1, Y: ux / uy}
	}
	return o
}
