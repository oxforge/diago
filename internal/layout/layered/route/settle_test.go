package route

import (
	"testing"

	"github.com/oxforge/diago/internal/layout/layered/lgraph"
	"github.com/oxforge/diago/internal/layout/layered/lgraph/lgraphtest"
	"github.com/oxforge/diago/internal/layout/layered/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoute_StopsWithinSnapAreOneX(t *testing.T) {
	g := graph(t, nil, "a->b")
	_, route := routed(t, g, screen, map[string]float64{"a": 100, "b": 100.3})
	assertRoute(t, pts(100, 40, 100, 80), route("a->b#0"))

	// a pinned end keeps its vertex: the other end moves onto it
	g = graph(t, nil, "a->m:diamond")
	_, route = routed(t, g, screen, map[string]float64{"a": 100.3, "m": 100})
	assertRoute(t, pts(100, 40, 100, 80), route("a->m#0"))
}

// TestRoute_ASideColumnNeverGivesWayInward pins S8's stops: when two
// locked stops of one wire meet within Snap, the side that may move gives
// way, a side column only outward: r's column, Reach out, does not give
// way inward, and p's moves out onto it. When neither may move, the one
// that does not carry the arrowhead gives way all the same, a side column
// even inward: here r's, since the arrowhead is at p.
func TestRoute_ASideColumnNeverGivesWayInward(t *testing.T) {
	// r->p is reversed; replayed, it leaves r's right face and enters p's.
	// p is 40 wide, so its column sits over the dummy at 159.7, farther out
	// than Reach; r's column sits Reach out, at 160. The two meet within
	// Snap: p's moves out onto r's, which would otherwise be 0.3 px short.
	g := graph(t, map[string]float64{"p": 40}, "p->q", "q->r", "r->p", "n")
	o := screen
	o.Side = [][2]ports.Vertex{{ports.Bottom, ports.Bottom}, {ports.Bottom, ports.Bottom}, {ports.Right, ports.Right}}
	xs := map[string]float64{"p": 100, "q": 100, "r": 100, "r->p#0@1": 159.7, "n": 300}
	_, route := routed(t, g, o, xs)
	got := route("r->p#0")
	assert.Equal(t, 140.0, got[0].X, "leaves r's right face: %v", got)
	assert.Equal(t, 160.0, got[1].X, "on r's column, Reach out: %v", got)
	assert.Equal(t, 160.0, got[len(got)-2].X, "p's column moved out onto it: %v", got)

	// n sits in p's row, 12.1 px from p's column: moving that column out
	// would bring it within the node clearance, so p's column keeps its row
	// clear. Where r's column then lies is not asserted here.
	xs["n"] = 159.7 + 12.1 + 40
	_, route = routed(t, g, o, xs)
	got = route("r->p#0")
	assert.InDelta(t, 159.7, got[len(got)-2].X, 1e-9, "p's column keeps its row clear: %v", got)
}

// TestRoute_StopsSnapOntoAnAnchoredPort pins S8's stops (S9): a stop
// within Snap of an anchored port takes the port's x, and the port keeps
// it.
func TestRoute_StopsSnapOntoAnAnchoredPort(t *testing.T) {
	g := shaped(t, func(lv *lgraph.Level) {
		lv.Nodes[1].Group, lv.Nodes[1].W, lv.Nodes[1].H = true, 300, 200
		lv.Edges[0].Anchor[1] = lgraph.Anchor{On: true, At: 0.3}
	}, "p->g")
	_, route := routed(t, g, screen, map[string]float64{"p": 0, "g": 0})
	assert.Equal(t, pts(0.3, 40, 0.3, 80), route("p->g#0"), "straight, on the anchor")
}

// TestRoute_ASideColumnKeepsReachAgainstAnAnchoredPort pins S8's stops
// (S9): a back edge's side column meets its anchored port within Snap,
// inward. The port never moves, and against it a side column does not
// move inward either: it keeps Reach, the stub C7 asks, and the 0.3 px jog
// between them stays and takes a lane. The column is the wire's upper
// stop with the group below, its lower stop with the group above.
func TestRoute_ASideColumnKeepsReachAgainstAnAnchoredPort(t *testing.T) {
	g := shaped(t, func(lv *lgraph.Level) {
		lv.Nodes[1].Group, lv.Nodes[1].W, lv.Nodes[1].H = true, 300, 200
		lv.Edges[0].Anchor[1] = lgraph.Anchor{On: true, At: -60}
		lv.Edges[1].Anchor[0] = lgraph.Anchor{On: true, At: 59.7}
	}, "a->g", "g->a")
	res, route := routed(t, g, screen, map[string]float64{"a": 0, "g": 0})
	require.Equal(t, ports.Right, res.Side[1][ports.Head], "a's end attaches at its right side, its column 60")
	// from g's top face at its anchor up into the channel's one lane, across
	// to the column Reach outside a's right face, and into it at mid-height
	assertRoute(t, pts(59.7, 88, 59.7, 64, 60, 64, 60, 20, 40, 20), route("g->a#0"))
	assert.Equal(t, []int{1}, res.Lanes, "one lane: the 0.3 px jog shares it with a->g's, far away")

	// the group above, the node below
	g = shaped(t, func(lv *lgraph.Level) {
		lv.Nodes[0].Group, lv.Nodes[0].W, lv.Nodes[0].H = true, 300, 200
		lv.Edges[0].Anchor[0] = lgraph.Anchor{On: true, At: -60}
		lv.Edges[1].Anchor[1] = lgraph.Anchor{On: true, At: 59.7}
	}, "g->a", "a->g")
	res, route = routed(t, g, screen, map[string]float64{"g": 0, "a": 0})
	require.Equal(t, ports.Right, res.Side[1][ports.Tail], "a's end attaches at its right side, its column 60")
	assert.Equal(t, []int{1}, res.Lanes, "one lane: the 0.3 px jog shares it with g->a's, far away")
	// from a's right face at mid-height out to the column Reach outside it,
	// up into the channel's one lane, across to g's anchor and into g's
	// bottom face there
	assertRoute(t, pts(40, 268, 60, 268, 60, 224, 59.7, 224, 59.7, 200), route("a->g#0"))
}

// TestRoute_AnchoredPortsKeepTheirJog pins S8's stops and lanes (S9): two
// anchored ports within Snap never move, and the jog between them takes
// a lane; within a millionth of a unit they are one x.
func TestRoute_AnchoredPortsKeepTheirJog(t *testing.T) {
	fix := func(at float64) func(lv *lgraph.Level) {
		return func(lv *lgraph.Level) {
			for v := range lv.Nodes {
				lv.Nodes[v].Group, lv.Nodes[v].W, lv.Nodes[v].H = true, 300, 200
			}
			lv.Edges[0].Anchor = [2]lgraph.Anchor{{On: true, At: 0}, {On: true, At: at}}
		}
	}
	res, route := routed(t, shaped(t, fix(0.3), "g->k"), screen, map[string]float64{"g": 0, "k": 0})
	got := route("g->k#0")
	assertRoute(t, pts(0, 200, 0, 224, 0.3, 224, 0.3, 248), got)
	assert.Equal(t, []int{1}, res.Lanes, "the jog takes a lane")
	_, route = routed(t, shaped(t, fix(1e-9), "g->k"), screen, map[string]float64{"g": 0, "k": 0})
	assert.Len(t, route("g->k#0"), 2, "float noise: one x")
}

// TestRoute_StopsSnappedOntoAnAnchoredPortStayWithIt pins S8's stops
// (S9): the stops snapped onto an anchored port are locked with it, and
// when the run they make meets another anchored port within Snap, the
// whole run counts as the port's side, so the jog stays where the two
// meet. g->h leaves g's face anchored at 0 and enters h's anchored at
// 0.45, through dummies at 0.9 and 10: no straightening moves an anchored
// end, and walking up from h's port moves the second dummy onto it, within
// Snap of the first. Settling, the second dummy snaps back onto the first
// and the two, a run on 0.9, onto h's port; that run, three stops on 0.45,
// meets g's port 0.45 away, and the jog stays there, in the channel below
// g, rather than splitting the run and moving down to h.
func TestRoute_StopsSnappedOntoAnAnchoredPortStayWithIt(t *testing.T) {
	lv := lgraphtest.Level(t, "g->h")
	for v := range lv.Nodes {
		lv.Nodes[v].Group, lv.Nodes[v].W, lv.Nodes[v].H = true, 300, 200
	}
	lv.Edges[0].Anchor = [2]lgraph.Anchor{{On: true, At: 0}, {On: true, At: 0.45}}
	// layers: g 0, the dummies 1 and 2, h 3
	g, err := lgraph.Build(lv, []int{0, 3}, nil, 8)
	require.NoError(t, err)
	res, route := routed(t, g, screen, map[string]float64{"g": 0, "g->h#0@1": 0.9, "g->h#0@2": 10, "h": 0})
	assertRoute(t, pts(0, 200, 0, 224, 0.45, 224, 0.45, 328), route("g->h#0"))
	assert.Equal(t, []int{1, 0, 0}, res.Lanes, "the jog takes a lane in the channel below g")
}
