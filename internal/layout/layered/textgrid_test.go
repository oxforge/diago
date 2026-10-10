package layered

import (
	"context"
	"fmt"
	"math"
	"slices"
	"testing"

	"github.com/oxforge/diago/internal/layout/contract"
	"github.com/oxforge/diago/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestLayout_TextPortsMirrorAboutTheirFace(t *testing.T) {
	// The flow-loop example pared down, under RIGHT: the loop back from
	// inc counts on the decision's out-face, so
	// the decision is five rows tall, and its two exits leave its right
	// face. The face's two thirds fall on the lines between rows; each port
	// takes the row farther from the middle, one above it and one below,
	// mirrored about the decision's center (S8 Shape ports), not the middle
	// row and the row below it
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
	// releases the crossing (S8 Lanes): releasing n7->n0 would put its jog
	// above n0->n5's, both running down that column, a shared run (C9.1).
	// The layout meets the text contract
	for _, dir := range []model.Direction{model.Down, model.Up} {
		t.Run(dir.String(), func(t *testing.T) {
			g := seed388(t)
			g.Direction = dir
			layText(t, g)
		})
	}
}
