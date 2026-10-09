package layered

import (
	"context"
	"fmt"

	"github.com/oxforge/diago/internal/layout/layered/cycle"
	"github.com/oxforge/diago/internal/layout/layered/lgraph"
	"github.com/oxforge/diago/internal/layout/layered/route"
	"github.com/oxforge/diago/internal/layout/layered/size"
	"github.com/oxforge/diago/internal/model"
)

// Arrangement is a flow or class graph laid out through routing (S1 to
// S8), in the engine's top-to-bottom frame: what labels (S10) and frame
// (S11) consume. Lengths are in the Config's units.
type Arrangement struct {
	Dir    model.Direction // the resolved direction (S11)
	Sizes  []size.Size     // per node, as measured, in the output frame; Level holds the equalized extents
	Level  *lgraph.Level   // per node and edge, with frame extents (W on the cross axis) after S4
	Layers []int           // per node (S3)
	Graph  *lgraph.Graph   // the ordered layered graph (S5, S6)
	X      []float64       // per vertex, its cross-axis center (S7)
	Route  *route.Result   // the rows and every edge's route (S8)
}

// Arrange runs S1 to S8 over g under cfg. Place and route run a second
// time when the router attaches an end at a side: a side face or a pinned
// node's side vertex; the place then takes those ends off their faces and
// the route replays the first one's side attachments (S8). Arrange lays
// out one level and rejects a graph with groups; Layout nests them (S9).
// It rejects a flat edge too, which takes no part in a level and which
// Layout routes on the composed layout (S9, Flat edges). A text Config
// whose Dir is AUTO or lies on the other axis than g's resolved direction
// is also an error (S14).
func Arrange(ctx context.Context, g model.Graph, cfg Config) (*Arrangement, error) {
	if len(g.Groups) > 0 {
		return nil, fmt.Errorf("layered: %d groups given; Arrange lays out one level, Layout nests (S9)", len(g.Groups))
	}
	for _, e := range g.Edges {
		if e.Flat {
			return nil, fmt.Errorf("layered: edge %s is flat; Arrange lays out one level, Layout routes flat edges (S9)", e.ID)
		}
	}
	dir, err := resolve(ctx, g, cfg)
	if err != nil {
		return nil, err
	}
	sizes := size.Measure(ctx, g.Nodes, cfg.Size)
	lv, err := level(g, sizes, dir)
	if err != nil {
		return nil, err
	}
	reversed := cycle.Break(ctx, lv, nil)
	ll, err := arrangeLevel(ctx, lv, reversed, cfg, dir, nil, nil, nil)
	if err != nil {
		return nil, err
	}
	return &Arrangement{Dir: dir, Sizes: sizes, Level: ll.Level, Layers: ll.Layers, Graph: ll.Graph, X: ll.X, Route: ll.Route}, nil
}
