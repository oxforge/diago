package layered

import (
	"bytes"
	"context"
	"log/slog"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/layout/contract"
	"github.com/oxforge/diago/internal/layout/layered/labels"
	"github.com/oxforge/diago/internal/layout/layered/lgraph"
	"github.com/oxforge/diago/internal/layout/layered/size"
	"github.com/oxforge/diago/internal/layoutdbg"
	"github.com/oxforge/diago/internal/model"
)

// grouped is g with groups.
func grouped(g model.Graph, groups ...model.Group) model.Graph {
	g.Groups = groups
	return g
}

// group returns pg's group with id.
func group(t *testing.T, pg *model.PositionedGraph, id string) model.PositionedGroup {
	t.Helper()
	for _, g := range pg.Groups {
		if g.ID == id {
			return g
		}
	}
	require.Failf(t, "no group", "%s", id)
	return model.PositionedGroup{}
}

// positioned returns pg's node with id.
func positioned(t *testing.T, pg *model.PositionedGraph, id string) model.PositionedNode {
	t.Helper()
	for _, n := range pg.Nodes {
		if n.ID == id {
			return n
		}
	}
	require.Failf(t, "no node", "%s", id)
	return model.PositionedNode{}
}

// box is a node's box: left, top, right, bottom.
func box(n model.PositionedNode) [4]float64 {
	return [4]float64{n.X - n.Width/2, n.Y - n.Height/2, n.X + n.Width/2, n.Y + n.Height/2}
}

// band is the height of a titled group's title band under cfg (S9).
func band(title string, cfg Config) float64 {
	_, h := size.Title(title, cfg.Size)
	return cfg.TitleMid + h/2 + cfg.TitleGap
}

// TestLayout_AGroupWrapsItsContent pins S9's box: GroupPadX beside the
// content, GroupPadY below it, and the title band above it, in every
// direction, the band on the output's top side.
func TestLayout_AGroupWrapsItsContent(t *testing.T) {
	for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
		t.Run(dir.String(), func(t *testing.T) {
			g := grouped(graph(t, nil, "a", "x->a"), model.Group{ID: "g", Label: "Group", Contains: []string{"a"}})
			g.Direction = dir
			pg := lay(t, g, screen(), dir)
			a, gr := box(positioned(t, pg, "a")), group(t, pg, "g")
			assert.InDelta(t, 12, a[0]-gr.X, 1e-9, "left")
			assert.InDelta(t, 12, gr.X+gr.Width-a[2], 1e-9, "right")
			assert.InDelta(t, band("Group", screen()), a[1]-gr.Y, 1e-9, "top: the title band")
			assert.InDelta(t, 12, gr.Y+gr.Height-a[3], 1e-9, "bottom")
			w, h := size.Title("Group", screen().Size)
			assert.Equal(t, [2]float64{w, h}, [2]float64{gr.LabelWidth, gr.LabelHeight}, "the title's extents, in the output frame")
		})
	}
}

// TestLayout_AnUntitledGroupPadsItsTopLikeItsBottom pins S9's box without
// a title: GroupPadY above the content too.
func TestLayout_AnUntitledGroupPadsItsTopLikeItsBottom(t *testing.T) {
	pg := lay(t, grouped(graph(t, nil, "a"), model.Group{ID: "g", Contains: []string{"a"}}), screen(), model.Down)
	a, gr := box(positioned(t, pg, "a")), group(t, pg, "g")
	assert.InDelta(t, 12, a[1]-gr.Y, 1e-9)
	assert.Zero(t, gr.LabelWidth)
}

// TestLayout_AnEdgelessNodeJoinsARowWithRoom pins S3's isolated nodes in
// a group: iso, connected to nothing, joins the short last row beside c
// instead of widening the first row, where every source sits, so the
// group is as wide as without it.
func TestLayout_AnEdgelessNodeJoinsARowWithRoom(t *testing.T) {
	for _, dir := range []model.Direction{model.Down, model.Right} {
		t.Run(dir.String(), func(t *testing.T) {
			members := []string{"a", "b", "c", "d", "e", "f", "g"}
			chains := []string{"a->b", "b->c", "d->e", "e->f", "g->h"}
			without := grouped(graph(t, nil, chains...), model.Group{ID: "grp", Contains: append(members, "h")})
			with := grouped(graph(t, nil, append(chains, "iso")...), model.Group{ID: "grp", Contains: append(members, "h", "iso")})
			without.Direction, with.Direction = dir, dir
			pgWithout, pg := lay(t, without, screen(), dir), lay(t, with, screen(), dir)
			iso, c := box(positioned(t, pg, "iso")), box(positioned(t, pg, "c"))
			grWithout, gr := group(t, pgWithout, "grp"), group(t, pg, "grp")
			if dir == model.Down {
				assert.Equal(t, c[1], iso[1], "on c's row")
				assert.InDelta(t, grWithout.Width, gr.Width, 1e-9, "no wider")
				return
			}
			assert.Equal(t, c[0], iso[0], "on c's column")
			assert.InDelta(t, grWithout.Height, gr.Height, 1e-9, "no taller")
		})
	}
}

// TestLayout_AnEdgelessNodePacksBesideItsRow pins S7's act 1 in a group:
// cp, connected to nothing, finds no row with room (S3) and stays on the
// first, which the entering wire's terminal pulls toward it; cp keeps the
// node gap from ing, not the place the packing from 0 gave it
// (platform-topology's clusters).
func TestLayout_AnEdgelessNodePacksBesideItsRow(t *testing.T) {
	labels := map[string]string{"cp": "Control plane x 3", "ing": "Ingress x 2", "w": "Worker x 2+"}
	for _, dir := range []model.Direction{model.Down, model.Right} {
		t.Run(dir.String(), func(t *testing.T) {
			g := grouped(graph(t, labels, "lb->ing:hexagon", "ing->w", "w->db", "cp"),
				model.Group{ID: "cluster", Contains: []string{"cp", "ing", "w"}})
			g.Direction = dir
			pg := lay(t, g, screen(), dir)
			cp, ing := box(positioned(t, pg, "cp")), box(positioned(t, pg, "ing"))
			lo, hi := 0, 2 // the cross axis: x under DOWN, y under RIGHT
			if dir == model.Right {
				lo, hi = 1, 3
			}
			assert.Equal(t, ing[1-lo], cp[1-lo], "on ing's row")
			gap := max(cp[lo]-ing[hi], ing[lo]-cp[hi])
			assert.InDelta(t, screen().NodeGap, gap, 1e-9)
		})
	}
}

// TestLayout_ALongTitleWidensTheGroup pins S9's box: a title wider than
// its content widens the box on the output's horizontal axis to
// 2 × TitleInset + its width, the content centered.
func TestLayout_ALongTitleWidensTheGroup(t *testing.T) {
	const title = "A title much wider than the node it holds"
	w, _ := size.Title(title, screen().Size)
	for _, dir := range []model.Direction{model.Down, model.Right} {
		g := grouped(graph(t, nil, "a"), model.Group{ID: "g", Label: title, Contains: []string{"a"}})
		g.Direction = dir
		pg := lay(t, g, screen(), dir)
		a, gr := positioned(t, pg, "a"), group(t, pg, "g")
		assert.InDelta(t, 16+w, gr.Width, 1e-9, "%s", dir)
		assert.InDelta(t, gr.X+gr.Width/2, a.X, 1e-9, "%s: centered", dir)
	}
}

// TestLayout_AWireReachesItsRealNode pins S9's terminals and joined
// routes: a wire into a node two groups deep runs straight down through
// both borders from its lane above, and one out of it straight down
// through both to its lane below.
func TestLayout_AWireReachesItsRealNode(t *testing.T) {
	g := grouped(graph(t, nil, "x->a", "a->y"),
		model.Group{ID: "outer", Children: []string{"inner"}},
		model.Group{ID: "inner", Contains: []string{"a"}},
	)
	pg := lay(t, g, screen(), model.Down)
	a, outer := box(positioned(t, pg, "a")), group(t, pg, "outer")
	in := edge(t, pg, "x->a#0").Points
	last, before := in[len(in)-1], in[len(in)-2]
	assert.Equal(t, a[1], last.Y, "on a's top")
	assert.Equal(t, last.X, before.X, "a straight last segment")
	assert.Less(t, before.Y, outer.Y, "which starts above outer")
	out := edge(t, pg, "a->y#0").Points
	assert.Equal(t, a[3], out[0].Y, "from a's bottom")
	assert.Equal(t, out[0].X, out[1].X)
	assert.Greater(t, out[1].Y, outer.Y+outer.Height, "straight past outer's bottom")
}

// TestLayout_ABackEdgeLeavesThroughTheFace pins S9 with S8: a back edge
// from inside a group leaves through the group's top, where its terminal
// is, and enters its target from the side.
func TestLayout_ABackEdgeLeavesThroughTheFace(t *testing.T) {
	g := grouped(graph(t, nil, "a->b", "b->c", "c->a"), model.Group{ID: "g", Contains: []string{"b", "c"}})
	pg := lay(t, g, screen(), model.Down)
	gr, a := group(t, pg, "g"), box(positioned(t, pg, "a"))
	back := edge(t, pg, "c->a#0").Points
	crossed := false
	for i := 1; i < len(back); i++ {
		p, q := back[i-1], back[i]
		if p.X == q.X && math.Min(p.Y, q.Y) < gr.Y && math.Max(p.Y, q.Y) > gr.Y {
			crossed = true
		}
	}
	assert.True(t, crossed, "a vertical run crosses g's top")
	end := back[len(back)-1]
	assert.True(t, end.X == a[0] || end.X == a[2], "enters a at a side: %v", end)
}

// TestLayout_StretchedGroupsCenterTheirContent pins S4 and S9: two groups
// in one row take the taller one's height, and the shorter one's content
// centers on the flow axis.
func TestLayout_StretchedGroupsCenterTheirContent(t *testing.T) {
	g := grouped(graph(t, nil, "r->a", "r->b", "b->c"),
		model.Group{ID: "short", Contains: []string{"a"}},
		model.Group{ID: "tall", Contains: []string{"b", "c"}},
	)
	pg := lay(t, g, screen(), model.Down)
	short, tall, a := group(t, pg, "short"), group(t, pg, "tall"), positioned(t, pg, "a")
	assert.Equal(t, tall.Height, short.Height)
	assert.Equal(t, tall.Y, short.Y, "a row shares its top")
	assert.InDelta(t, short.Y+short.Height/2, a.Y, 1e-9, "a centered")
}

// TestLayout_SiblingGroupsLineUp pins S9's seed and bottom-up order:
// entries into a group follow the exits of the sibling group upstream,
// so wires between two groups do not cross; child groups are laid out
// in declaration order.
func TestLayout_SiblingGroupsLineUp(t *testing.T) {
	g := grouped(graph(t, nil, "gw->cat", "gw->cart", "gw->ord", "ord->pay", "pay->stripe", "cat->pg", "ord->pg", "cart->redis"),
		model.Group{ID: "svc", Contains: []string{"cat", "cart", "ord", "pay"}},
		model.Group{ID: "data", Contains: []string{"pg", "redis"}},
	)
	pg := lay(t, g, screen(), model.Down)
	for _, e := range pg.Edges {
		assert.Empty(t, e.Crossings, "%s", e.ID)
	}

	// Regression check: with data declared (and so laid out) first, the
	// wires between the groups still do not cross.
	g = grouped(graph(t, nil, "pg", "redis", "gw->cat", "gw->ord", "gw->cart", "cat->pg", "ord->pg", "cart->redis"),
		model.Group{ID: "data", Contains: []string{"pg", "redis"}},
		model.Group{ID: "svc", Contains: []string{"cat", "ord", "cart"}},
	)
	pg = lay(t, g, screen(), model.Down)
	for _, e := range pg.Edges {
		assert.Empty(t, e.Crossings, "data declared first: %s", e.ID)
	}
}

// TestLayout_TextGroupsSitOnTheGrid pins S9 in the text profile: group
// boxes on cell boundaries, and wires crossing their borders in the
// middle of a cell.
func TestLayout_TextGroupsSitOnTheGrid(t *testing.T) {
	for _, dir := range []model.Direction{model.Down, model.Right} {
		g := grouped(graph(t, nil, "x->a", "a->b", "b->y", "x->y", "b->b"),
			model.Group{ID: "outer", Label: "Outer", Children: []string{"inner"}},
			model.Group{ID: "inner", Label: "In", Contains: []string{"a", "b"}},
		)
		g.Direction = dir
		pg, err := Layout(context.Background(), g, TextConfig(dir), nil)
		require.NoError(t, err)
		assert.Empty(t, contract.Check(pg, contract.Options{Limits: contract.TextLimits(dir), Direction: dir, EdgeIDs: ids(g), Text: true}), "%s", dir)
		for _, gr := range pg.Groups {
			for _, v := range []float64{gr.X, gr.X + gr.Width} {
				assert.Zero(t, math.Mod(v, 8), "%s: %s x %v", dir, gr.ID, v)
			}
			for _, v := range []float64{gr.Y, gr.Y + gr.Height} {
				assert.Zero(t, math.Mod(v, 16), "%s: %s y %v", dir, gr.ID, v)
			}
		}
		for _, e := range pg.Edges {
			for i := 1; i < len(e.Points); i++ {
				p, q := e.Points[i-1], e.Points[i]
				for _, gr := range pg.Groups {
					if dir == model.Down && p.X == q.X {
						for _, y := range []float64{gr.Y, gr.Y + gr.Height} {
							if min(p.Y, q.Y) < y && y < max(p.Y, q.Y) {
								assert.Equal(t, 4.0, math.Mod(p.X, 8), "%s: %s crosses %s in a column's middle", dir, e.ID, gr.ID)
							}
						}
					}
					if dir == model.Right && p.Y == q.Y {
						for _, x := range []float64{gr.X, gr.X + gr.Width} {
							if min(p.X, q.X) < x && x < max(p.X, q.X) {
								assert.Equal(t, 8.0, math.Mod(p.Y, 16), "%s: %s crosses %s in a row's middle", dir, e.ID, gr.ID)
							}
						}
					}
				}
			}
		}
	}
}

// TestLayout_TextASelfLoopKeepsOffItsGroupsFrame lays out a self-loop
// whose run bounds its group's content: right of its node, so on the
// group's right under DOWN and UP and on its bottom under RIGHT and LEFT.
// The run lies on a cell's middle, half a cell beyond Reach (S8,
// Self-loops), so the group's box, rounded out to whole cells, wraps the
// cell the text art draws it in, and the frame stays off it (C6.2).
func TestLayout_TextASelfLoopKeepsOffItsGroupsFrame(t *testing.T) {
	g := grouped(graph(t, nil, "x->a", "a->a"), model.Group{ID: "g", Label: "G", Contains: []string{"a"}})
	for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
		g.Direction = dir
		pg, err := Layout(context.Background(), g, TextConfig(dir), nil)
		require.NoError(t, err, "%s", dir)
		assert.Empty(t, contract.Check(pg, contract.Options{Limits: contract.TextLimits(dir), Direction: dir, EdgeIDs: ids(g), Text: true}), "%s", dir)
		loop := edge(t, pg, "a->a#0").Points
		require.Len(t, loop, 4, "%s", dir)
		if dir == model.Right || dir == model.Left {
			assert.Equal(t, loop[1].Y, loop[2].Y, "%s: the run is horizontal", dir)
			assert.Equal(t, 8.0, math.Mod(loop[1].Y, 16), "%s: the run lies on a row's middle", dir)
		} else {
			assert.Equal(t, loop[1].X, loop[2].X, "%s: the run is vertical", dir)
			assert.Equal(t, 4.0, math.Mod(loop[1].X, 8), "%s: the run lies on a column's middle", dir)
		}
	}
}

// TestLayout_AnchoredPortsMeetInText reproduces an anchoring snap:
// S8 can move an anchored port within Snap of another fixed end (here, a
// back edge's side column and an entering edge's anchored port), while the
// child level's own route does not move with it, so the join (nest's
// expand) emits a diagonal segment (C3). In the text profile a side
// column and an anchor differ by exactly Snap (half a cell), so they
// always meet this way, and the text renderer fails with exit 2.
func TestLayout_AnchoredPortsMeetInText(t *testing.T) {
	for _, dir := range []model.Direction{model.Down, model.Up} {
		g := grouped(graph(t, nil, "n0->n5", "n4->n0"),
			model.Group{ID: "g1", Label: "G", Contains: []string{"n4", "n5"}},
		)
		g.Direction = dir
		pg, err := Layout(context.Background(), g, TextConfig(dir), nil)
		require.NoError(t, err, "%s", dir)
		var c3 []contract.Violation
		for _, v := range contract.Check(pg, contract.Options{Limits: contract.TextLimits(dir), Direction: dir, EdgeIDs: ids(g), Text: true}) {
			if v.Rule == "C3" {
				c3 = append(c3, v)
			}
		}
		assert.Empty(t, c3, "%s: no diagonal segment at the anchored-port join", dir)
	}
}

// TestLayout_AnEmptyGroupIsItsPadding pins S9's box for a group with no
// content: its padding and title band only.
func TestLayout_AnEmptyGroupIsItsPadding(t *testing.T) {
	pg := lay(t, grouped(graph(t, nil, "a"), model.Group{ID: "g", Label: "G"}), screen(), model.Down)
	w, _ := size.Title("G", screen().Size)
	gr := group(t, pg, "g")
	assert.InDelta(t, max(24, 16+w), gr.Width, 1e-9)
	assert.InDelta(t, band("G", screen())+12, gr.Height, 1e-9)
}

// TestLayout_ATitleLeavesTheSlotAWireCrosses pins S10's title slots in
// both profiles: under DOWN and UP the wire into a, on the left of the
// group, crosses the title face under the left slot, so the title takes
// the right slot; under RIGHT and LEFT nothing crosses the title's band,
// so it keeps the left slot.
func TestLayout_ATitleLeavesTheSlotAWireCrosses(t *testing.T) {
	for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
		for _, cfg := range []Config{screen(), TextConfig(dir)} {
			e := "x->a"
			if dir == model.Up {
				e = "a->x"
			}
			g := grouped(graph(t, nil, e, "b"), model.Group{ID: "g", Label: "Services", Contains: []string{"a", "b"}})
			g.Direction = dir
			pg, err := Layout(context.Background(), g, cfg, nil)
			require.NoError(t, err)
			lim, cell := contract.ScreenLimits(), 1.0
			if cfg.Text {
				lim, cell = contract.TextLimits(dir), cellW
			}
			assert.Empty(t, contract.Check(pg, contract.Options{Limits: lim, Direction: dir, EdgeIDs: ids(g), Text: cfg.Text}), "%s text=%v", dir, cfg.Text)
			gr := group(t, pg, "g")
			want := cfg.TitleInset * cell
			if dir == model.Down || dir == model.Up {
				want = gr.Width - cfg.TitleInset*cell - gr.LabelWidth
			}
			assert.InDelta(t, want, gr.LabelOffset, 1e-9, "%s text=%v", dir, cfg.Text)
		}
	}
}

// TestLayout_ABlockedTitleStaysLeft pins S10's fallback: a title wider
// than every free span of its band stays in its left slot, marked
// LabelBlocked and recorded as group_title_blocked, and C13 exempts the
// wire through it (C0).
func TestLayout_ABlockedTitleStaysLeft(t *testing.T) {
	g := grouped(graph(t, nil, "x->a"), model.Group{ID: "g", Label: "A title wider than half its group", Contains: []string{"a"}})
	var buf bytes.Buffer
	ctx := layoutdbg.NewContext(context.Background(), slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	pg, err := Layout(ctx, g, screen(), nil)
	require.NoError(t, err)
	gr := group(t, pg, "g")
	assert.Equal(t, screen().TitleInset, gr.LabelOffset)
	assert.True(t, gr.LabelBlocked)
	assert.Contains(t, buf.String(), `"decision":"group_title_blocked"`)
	assert.Empty(t, contract.Check(pg, contract.Options{Limits: contract.ScreenLimits(), Direction: model.Down, EdgeIDs: ids(g)}))
}

// TestLayout_NoGroupBlocksItsOwnOrAChildsTitle pins S10's blocked spans:
// a group's own box and its ancestors' span the whole band, so they block
// no title.
func TestLayout_NoGroupBlocksItsOwnOrAChildsTitle(t *testing.T) {
	g := grouped(graph(t, nil, "a"),
		model.Group{ID: "outer", Label: "Outer", Children: []string{"inner"}},
		model.Group{ID: "inner", Label: "Inner", Contains: []string{"a"}},
	)
	var buf bytes.Buffer
	ctx := layoutdbg.NewContext(context.Background(), slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	pg, err := Layout(ctx, g, screen(), nil)
	require.NoError(t, err)
	assert.NotContains(t, buf.String(), "group_title_blocked")
	for _, id := range []string{"outer", "inner"} {
		assert.Equal(t, screen().TitleInset, group(t, pg, id).LabelOffset, id)
		assert.False(t, group(t, pg, id).LabelBlocked, id)
	}
}

// TestTitleBox pins S10's title obstacle in the engine's frame: the output
// frame's title box, LabelOffset from the group's left side (C13), turned
// back by the direction.
func TestTitleBox(t *testing.T) {
	cfg := screen()
	gr := model.PositionedGroup{X: 100, Y: 200, Width: 300, Height: 400, LabelWidth: 50, LabelHeight: 14, LabelOffset: 20}
	for _, tt := range []struct {
		dir  model.Direction
		want labels.Rect
	}{
		{model.Down, labels.Rect{X: 120, Y: 209, W: 50, H: 14}},
		{model.Up, labels.Rect{X: 120, Y: 577, W: 50, H: 14}},
		{model.Right, labels.Rect{X: 109, Y: 220, W: 14, H: 50}},
		{model.Left, labels.Rect{X: 109, Y: 530, W: 14, H: 50}},
	} {
		got, ok := titleBox(gr, tt.dir, cfg)
		assert.True(t, ok)
		assert.Equal(t, tt.want, got, "%s", tt.dir)
	}
	gr.LabelWidth = 0
	_, ok := titleBox(gr, model.Down, cfg)
	assert.False(t, ok, "untitled")
}

// TestTitleBox_TextCoversTheBlanks pins S10's text title obstacle: the box
// grows by TitlePad, one cell, at both ends along the band, over the blank
// the text renderer writes on each side of the title.
func TestTitleBox_TextCoversTheBlanks(t *testing.T) {
	gr := model.PositionedGroup{X: 10, Y: 20, Width: 30, Height: 40, LabelWidth: 5, LabelHeight: 1, LabelOffset: 3}
	for _, tt := range []struct {
		dir  model.Direction
		want labels.Rect
	}{
		{model.Down, labels.Rect{X: 12, Y: 20, W: 7, H: 1}},
		{model.Up, labels.Rect{X: 12, Y: 59, W: 7, H: 1}},
		{model.Right, labels.Rect{X: 10, Y: 22, W: 1, H: 7}},
		{model.Left, labels.Rect{X: 10, Y: 51, W: 1, H: 7}},
	} {
		got, ok := titleBox(gr, tt.dir, TextConfig(tt.dir))
		assert.True(t, ok)
		assert.Equal(t, tt.want, got, "%s", tt.dir)
	}
}

// TestLayout_ATextTitlesBlanksBlockAnEdgeLabel pins S10's text title
// obstacle on a layout: the flow-client-server example pared down to its clients group and the proxy they call, under UP. Without
// the blanks among the label obstacles, web's "HTTPS" takes the spot beside
// its wire that ends in the cell of the blank before "Clients".
func TestLayout_ATextTitlesBlanksBlockAnEdgeLabel(t *testing.T) {
	g := grouped(graph(t, map[string]string{"web": "Web Client", "mobile": "Mobile Client", "cli": "CLI Client", "proxy": "Reverse Proxy"},
		"web->proxy", "mobile->proxy", "cli->proxy"),
		model.Group{ID: "clients", Label: "Clients", Contains: []string{"web", "mobile", "cli"}},
	)
	for i := range g.Edges {
		g.Edges[i].Label = "HTTPS"
	}
	g.Direction = model.Up
	pg, err := Layout(context.Background(), g, TextConfig(model.Up), nil)
	require.NoError(t, err)

	gr := group(t, pg, "clients")
	// The result is in the output frame and in px, 8 per column: the title
	// starts LabelOffset in, and its blanks take the column on each side.
	left := gr.X + gr.LabelOffset - 8
	title := [4]float64{left, gr.Y, left + gr.LabelWidth + 16, gr.Y + gr.LabelHeight}
	for _, e := range pg.Edges {
		require.NotNil(t, e.LabelPos, "%s: HTTPS has a spot", e.ID)
		lb := [4]float64{e.LabelPos.X - e.LabelWidth/2, e.LabelPos.Y - e.LabelHeight/2, e.LabelPos.X + e.LabelWidth/2, e.LabelPos.Y + e.LabelHeight/2}
		assert.False(t, boxesOverlap(lb, title), "%s: label %v overlaps clients's title and its blanks %v", e.ID, lb, title)
	}
}

// boxesOverlap reports whether two label-shaped boxes, left/top/right/
// bottom, overlap.
func boxesOverlap(a, b [4]float64) bool {
	return a[0] < b[2] && a[2] > b[0] && a[1] < b[3] && a[3] > b[1]
}

// TestLayout_GroupTitleBlocksAnEdgeLabel pins S10/C14.1: a group's title,
// where it was placed, is a hard obstacle for edge labels. This is the
// flow-groups example, pared down to its
// services and data groups, under UP: without the title box among the
// label obstacles, "query"'s natural spot for auth->db and users->db
// falls on services's title. Here services's title is wider than every
// free span of its band, so it stays in its left slot, marked
// LabelBlocked, and the wire from auth crosses it (C0).
func TestLayout_GroupTitleBlocksAnEdgeLabel(t *testing.T) {
	g := grouped(graph(t, nil, "gateway->auth", "gateway->users", "auth->db", "users->db", "users->cache"),
		model.Group{ID: "services", Label: "Backend Services", Contains: []string{"gateway", "auth", "users"}},
		model.Group{ID: "data", Label: "Data Layer", Contains: []string{"db", "cache"}},
	)
	for i, e := range g.Edges {
		switch e.From + "->" + e.To {
		case "auth->db", "users->db":
			g.Edges[i].Label = "query"
		case "users->cache":
			g.Edges[i].Label = "lookup"
		}
	}
	g.Direction = model.Up
	pg, err := Layout(context.Background(), g, screen(), nil)
	require.NoError(t, err)

	assert.Empty(t, contract.Check(pg, contract.Options{Limits: contract.ScreenLimits(), Direction: model.Up, EdgeIDs: ids(g)}))

	gr := group(t, pg, "services")
	assert.True(t, gr.LabelBlocked)
	// Layout's result is already in the OUTPUT frame, which titleBox reads
	// as DOWN (the identity map): DOWN here turns gr's OUTPUT-frame box
	// back into the same box, while UP would (wrongly) read it as an
	// ENGINE-frame box still awaiting frame.Apply and compute a box near
	// services's bottom instead of its top.
	tb, ok := titleBox(gr, model.Down, screen())
	require.True(t, ok, "services is titled")
	tbb := [4]float64{tb.X, tb.Y, tb.X + tb.W, tb.Y + tb.H}
	for _, id := range []string{"auth->db#0", "users->db#0"} {
		e := edge(t, pg, id)
		require.NotNil(t, e.LabelPos, "%s: query has a spot", id)
		lb := [4]float64{e.LabelPos.X - e.LabelWidth/2, e.LabelPos.Y - e.LabelHeight/2, e.LabelPos.X + e.LabelWidth/2, e.LabelPos.Y + e.LabelHeight/2}
		assert.False(t, boxesOverlap(lb, tbb), "%s: label %v overlaps services's title %v", id, lb, tbb)
	}
}

// TestNesting_KeysFollowTheSeed pins S9's seed keys: a group's level ranks
// a terminal by its edge's outside representative, then by the inside
// end, or binding, by the sibling group's own terminal.
func TestNesting_KeysFollowTheSeed(t *testing.T) {
	g := grouped(graph(t, nil, "gw->cat", "gw->ord", "cat->pg", "ord->pg"),
		model.Group{ID: "svc", Contains: []string{"cat", "ord"}},
		model.Group{ID: "data", Contains: []string{"pg"}},
	)
	n := testNesting(t, g)
	svc, data := n.levels[1], n.levels[2]
	sd := n.keys(1)
	checked := 0
	for le, ed := range svc.lv.Edges {
		if svc.lv.Nodes[ed.From].Terminal {
			inner := n.index[n.g.Edges[svc.carries[le]].To]
			assert.Equal(t, n.flat[n.index["gw"]], sd.key[ed.From], "keyed by gw")
			assert.Equal(t, n.flat[inner], sd.tie[ed.From], "tied by its inside end")
			assert.False(t, sd.hard[ed.From])
			checked++
		}
	}
	require.Equal(t, 2, checked, "svc's entering terminals")
	sd = n.keys(2)
	checked = 0
	svcMean := (n.flat[n.index["cat"]] + n.flat[n.index["ord"]]) / 2
	for le, ed := range data.lv.Edges {
		if data.lv.Nodes[ed.From].Terminal {
			assert.Equal(t, svcMean, sd.key[ed.From], "keyed by svc's mean (cat, ord)")
			assert.Equal(t, svc.at[data.carries[le]], sd.tie[ed.From], "tied by svc's own terminal")
			assert.True(t, sd.hard[ed.From], "and bound")
			checked++
		}
	}
	require.Equal(t, 2, checked, "data's entering terminals")
}

// TestNesting_CorridorSeatsReachAGroupThroughTheKeys pins how S5's
// Corridor seats reach a level of a graph with groups (S9): flow-network
// inside one group lays its level out like the flat graph, api -> db's
// column between Redis Cache and Message Queue. The whole-graph seed's
// sort alone puts it there: the level rebuilt in the declaration order,
// the column last, and sorted by the keys comes out so, since the
// whole-graph run seated the column and S7 lined api and db up on it.
func TestNesting_CorridorSeatsReachAGroupThroughTheKeys(t *testing.T) {
	g := grouped(flowNetwork(), model.Group{ID: "all", Contains: []string{"web", "api", "auth", "db", "cache", "queue"}})
	n := testNesting(t, g)
	c := n.tree.In[n.index["db"]]
	l := n.levels[c]
	ids := func(lg *lgraph.Graph) []string {
		var out []string
		for _, v := range lg.Layers[2] {
			out = append(out, lg.Vertices[v].ID)
		}
		return out
	}
	want := []string{"auth", "cache", "api->db#0@2", "queue"}
	assert.Equal(t, want, ids(l.out.Graph), "the group's level")
	lg, err := lgraph.Build(l.out.Level, l.out.Layers, l.reversed, screen().DummyWidth)
	require.NoError(t, err)
	require.Equal(t, []string{"auth", "cache", "queue", "api->db#0@2"}, ids(lg), "the declaration order")
	seedOrder(lg, n.keys(c))
	assert.Equal(t, want, ids(lg), "sorted by the whole-graph seed's keys")
}

// testNesting lays g out through S9 as Layout does and returns the
// levels, composed.
func testNesting(t *testing.T, g model.Graph) *nesting {
	t.Helper()
	cfg := screen()
	sizes := size.Measure(context.Background(), g.Nodes, cfg.Size)
	n, err := arrangeNested(context.Background(), g, sizes, cfg, model.Down, nil)
	require.NoError(t, err)
	return n
}
