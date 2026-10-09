package layered

import (
	"context"
	"math"
	"testing"

	"github.com/oxforge/diago/internal/layout/contract"
	"github.com/oxforge/diago/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLayout_TextLabelsAreDrawnOnTheirBoxes(t *testing.T) {
	// web and cli, in a titled group, feed api above them under UP
	// (the flow-client-server example, pared down). web's label once sat
	// beside the middle of its wire's first run, a box straddling the
	// group's top border row and the row above it: the text renderer
	// started it on the border row, found the
	// frame there and drew it a row up. In the text profile a label's box
	// is whole cells, clear of every cell drawn there (S10), so the
	// renderer draws the label on it (C14.5); it now takes the row next to
	// its wire's lane, left of the wire's last run
	g := model.Graph{Direction: model.Up,
		Nodes: []model.Node{
			{ID: "web", Label: "Web", Shape: model.ShapeRect}, {ID: "cli", Label: "CLI", Shape: model.ShapeRect},
			{ID: "api", Label: "API", Shape: model.ShapeRect},
		},
		Edges:  []model.Edge{{ID: "web->api#0", From: "web", To: "api", Label: "HTTPS"}, {ID: "cli->api#0", From: "cli", To: "api"}},
		Groups: []model.Group{{ID: "clients", Label: "Clients", Contains: []string{"web", "cli"}}},
	}
	pg := layText(t, g) // the text contract holds: the box is whole cells, clear of every frame side and wire (C14)
	web := edge(t, pg, "web->api#0")
	require.NotNil(t, web.LabelPos)
	assert.False(t, web.LabelUnresolved)
	left, top := web.LabelPos.X-web.LabelWidth/2, web.LabelPos.Y-web.LabelHeight/2
	assert.Equal(t, [2]float64{8 * cellW, 5 * cellH}, [2]float64{left, top}, "columns 8..12 of row 5, the row above the lane, a blank column left of the last run")
}

func TestLayout_ClientServerTextLabelsSitByTheirWires(t *testing.T) {
	// under RIGHT and LEFT the gap between the two frames is narrower than
	// "HTTPS" and its blanks: web's and cli's labels once climbed out of
	// the frames, four rows from their wires, and "/assets/*" left
	// Services below its bottom side. A text label now takes the row next
	// to a wire along its row, covers a frame side that crosses its row
	// when its ring holds no clear spot, and stays in its edge's group
	// (S10)
	for _, dir := range []model.Direction{model.Right, model.Left} {
		t.Run(dir.String(), func(t *testing.T) {
			pg := layText(t, clientServer(dir))
			var services model.PositionedGroup
			for _, gr := range pg.Groups {
				if gr.ID == "services" {
					services = gr
				}
			}
			for _, id := range []string{"web->proxy#0", "cli->proxy#0", "proxy->assets#0"} {
				e := edge(t, pg, id)
				require.NotNil(t, e.LabelPos, id)
				assert.False(t, e.LabelUnresolved, id)
				b := [4]float64{e.LabelPos.X - e.LabelWidth/2, e.LabelPos.Y - e.LabelHeight/2, e.LabelPos.X + e.LabelWidth/2, e.LabelPos.Y + e.LabelHeight/2}
				assert.InDelta(t, cellH/2, gapTo(b, e.Points), 1e-6, "%s: on the row next to its wire", id)
			}
			e := edge(t, pg, "proxy->assets#0")
			assert.True(t, e.LabelPos.X-e.LabelWidth/2 > services.X && e.LabelPos.X+e.LabelWidth/2 < services.X+services.Width &&
				e.LabelPos.Y-e.LabelHeight/2 > services.Y && e.LabelPos.Y+e.LabelHeight/2 < services.Y+services.Height,
				"/assets/* inside Services")
		})
	}
}

func TestHomes_PicksTheInnermostGroupHoldingBothEnds(t *testing.T) {
	// depth as the groups carry it: 1 top-level, 2 nested. outer holds
	// inner (a, b) and the node c directly; other holds d (S10)
	outer := model.PositionedGroup{ID: "outer", Contains: []string{"c"}, Children: []string{"inner"}, Depth: 1}
	inner := model.PositionedGroup{ID: "inner", Contains: []string{"a", "b"}, Depth: 2}
	other := model.PositionedGroup{ID: "other", Contains: []string{"d"}, Depth: 1}
	for _, tc := range []struct {
		name   string
		groups []model.PositionedGroup
		a, b   string
		want   string // the group's id, "" for none
	}{
		{"both in inner, outer listed first", []model.PositionedGroup{outer, inner, other}, "a", "b", "inner"},
		{"both in inner, outer listed second", []model.PositionedGroup{inner, outer, other}, "a", "b", "inner"},
		{"one in inner, one in outer, outer first", []model.PositionedGroup{outer, inner, other}, "a", "c", "outer"},
		{"one in inner, one in outer, outer second", []model.PositionedGroup{inner, outer, other}, "a", "c", "outer"},
		{"ends in no common group", []model.PositionedGroup{outer, inner, other}, "a", "d", ""},
		{"an end in no group", []model.PositionedGroup{outer, inner, other}, "a", "z", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			k, ok := homes(tc.groups)(tc.a, tc.b)
			if tc.want == "" {
				assert.False(t, ok)
				return
			}
			require.True(t, ok)
			assert.Equal(t, tc.want, tc.groups[k].ID)
		})
	}
}

func TestLayout_TextANestedGroupsLabelStaysInsideTheInnerFrame(t *testing.T) {
	// a and b lie in inner, nested in outer beside the node c; the edge
	// a->b has both ends in inner, so in text its label stays inside
	// inner's frame, one cell clear of each side, resolved (S10)
	for _, dir := range []model.Direction{model.Down, model.Right} {
		t.Run(dir.String(), func(t *testing.T) {
			g := graph(t, nil, "a->b", "c->a", "c->b")
			g.Direction = dir
			g.Edges[0].Label = "long label"
			g.Groups = []model.Group{
				{ID: "outer", Label: "Outer", Contains: []string{"c"}, Children: []string{"inner"}, Depth: 1},
				{ID: "inner", Label: "Inner", Contains: []string{"a", "b"}, Depth: 2},
			}
			pg := layText(t, g)
			e := edge(t, pg, g.Edges[0].ID)
			require.NotNil(t, e.LabelPos)
			assert.False(t, e.LabelUnresolved)
			in := group(t, pg, "inner")
			assert.GreaterOrEqual(t, e.LabelPos.X-e.LabelWidth/2, in.X+cellW, "left")
			assert.LessOrEqual(t, e.LabelPos.X+e.LabelWidth/2, in.X+in.Width-cellW, "right")
			assert.GreaterOrEqual(t, e.LabelPos.Y-e.LabelHeight/2, in.Y+cellH, "top")
			assert.LessOrEqual(t, e.LabelPos.Y+e.LabelHeight/2, in.Y+in.Height-cellH, "bottom")
		})
	}
}

// gapTo is the distance from box b, left/top/right/bottom, to the nearest
// segment of route.
func gapTo(b [4]float64, route []model.Point) float64 {
	d := math.Inf(1)
	for i := 0; i+1 < len(route); i++ {
		p, q := route[i], route[i+1]
		dx := max(b[0]-max(p.X, q.X), min(p.X, q.X)-b[2], 0)
		dy := max(b[1]-max(p.Y, q.Y), min(p.Y, q.Y)-b[3], 0)
		d = min(d, math.Hypot(dx, dy))
	}
	return d
}

// nearerOther is how much nearer than its own wire the nearest wire of
// another edge lies to box b, a label of edge i in pg: positive when a
// reader would take the label for the other edge's (S10).
func nearerOther(pg *model.PositionedGraph, i int, b [4]float64) float64 {
	other := math.Inf(1)
	for j, e := range pg.Edges {
		if j != i {
			other = min(other, gapTo(b, e.Points))
		}
	}
	return gapTo(b, pg.Edges[i].Points) - other
}

func TestLayout_ASidewaysFanOutsLabelsSitBesideTheirWires(t *testing.T) {
	// gw fans out to four services under RIGHT and LEFT, each wire
	// labelled. A label's length lies along the flow there, longer than
	// any run between the two layers, so every label once fell back to
	// its least overlap. Each now slides beside its own wire, past a turn
	// or beside a jog, clear, within the eight rings of the slide, where
	// no other wire lies nearer: "/billing" once slid to 12 from its own
	// wire and 4 from search's, and now sits under billing's last run
	// (S10)
	for _, dir := range []model.Direction{model.Right, model.Left} {
		t.Run(dir.String(), func(t *testing.T) {
			g := graph(t, nil, "gw->users", "gw->orders", "gw->billing", "gw->search")
			g.Direction = dir
			for i := range g.Edges {
				g.Edges[i].Label = "/" + g.Edges[i].To
			}
			cfg := screen()
			pg := lay(t, g, cfg, dir)
			for i, e := range pg.Edges {
				require.NotNil(t, e.LabelPos, e.ID)
				assert.False(t, e.LabelUnresolved, "%s: unresolved", e.ID)
				b := [4]float64{e.LabelPos.X - e.LabelWidth/2, e.LabelPos.Y - e.LabelHeight/2, e.LabelPos.X + e.LabelWidth/2, e.LabelPos.Y + e.LabelHeight/2}
				assert.LessOrEqual(t, gapTo(b, e.Points), 8*cfg.LabelGap, "%s: beside its own wire", e.ID)
				assert.LessOrEqual(t, nearerOther(pg, i, b), cfg.OwnSlack, "%s: another wire lies nearer", e.ID)
			}
		})
	}
}

// clientServer is the flow-client-server example under dir: three clients in a group feed a reverse proxy in another,
// which fans out to three services, two of them backed by one database.
func clientServer(dir model.Direction) model.Graph {
	return model.Graph{
		Direction: dir,
		Nodes: []model.Node{
			{ID: "web", Label: "Web Client", Shape: model.ShapeRounded}, {ID: "mobile", Label: "Mobile Client", Shape: model.ShapeRounded},
			{ID: "cli", Label: "CLI Client", Shape: model.ShapeRounded}, {ID: "proxy", Label: "Reverse Proxy", Shape: model.ShapeHexagon},
			{ID: "server", Label: "App Server", Shape: model.ShapeRect}, {ID: "auth", Label: "Auth Service", Shape: model.ShapeRect},
			{ID: "assets", Label: "Assets Service", Shape: model.ShapeRect}, {ID: "db", Label: "Database", Shape: model.ShapeCylinder},
		},
		Edges: []model.Edge{
			{ID: "web->proxy#0", From: "web", To: "proxy", Label: "HTTPS"},
			{ID: "mobile->proxy#0", From: "mobile", To: "proxy", Label: "HTTPS"},
			{ID: "cli->proxy#0", From: "cli", To: "proxy", Label: "HTTPS"},
			{ID: "proxy->server#0", From: "proxy", To: "server", Label: "/api/*", Direction: model.EdgeBoth},
			{ID: "proxy->auth#0", From: "proxy", To: "auth", Label: "/auth/*", Direction: model.EdgeBoth},
			{ID: "proxy->assets#0", From: "proxy", To: "assets", Label: "/assets/*", Direction: model.EdgeBoth},
			{ID: "server->db#0", From: "server", To: "db", Label: "query/result", Direction: model.EdgeBoth},
			{ID: "auth->db#0", From: "auth", To: "db", Label: "query/result", Direction: model.EdgeBoth},
		},
		Groups: []model.Group{
			{ID: "clients", Label: "Clients", Contains: []string{"web", "mobile", "cli"}},
			{ID: "services", Label: "Services", Contains: []string{"proxy", "server", "auth", "assets", "db"}},
		},
	}
}

func TestLayout_ClientServerSidewaysLabelsNameTheirOwnWires(t *testing.T) {
	// flow-client-server under RIGHT and LEFT: "/auth/*" once slid 24 from
	// its own wire and 3.6 from proxy->assets', reading as that edge's. A
	// label resolved sits where no other wire lies nearer than its own,
	// beyond the slack, or falls back, marked unresolved (S10); only
	// proxy->auth's falls back, so the check covers every other label
	for _, dir := range []model.Direction{model.Right, model.Left} {
		t.Run(dir.String(), func(t *testing.T) {
			g := clientServer(dir)
			cfg := screen()
			pg := lay(t, g, cfg, dir)
			var unresolved []string
			for i, e := range pg.Edges {
				require.NotNil(t, e.LabelPos, e.ID)
				if e.LabelUnresolved {
					unresolved = append(unresolved, e.ID)
					continue
				}
				b := [4]float64{e.LabelPos.X - e.LabelWidth/2, e.LabelPos.Y - e.LabelHeight/2, e.LabelPos.X + e.LabelWidth/2, e.LabelPos.Y + e.LabelHeight/2}
				assert.LessOrEqual(t, nearerOther(pg, i, b), cfg.OwnSlack, "%s: another wire lies nearer", e.ID)
			}
			assert.Equal(t, []string{"proxy->auth#0"}, unresolved, "the labels that fall back")
		})
	}
}

// layText lays g out under the text profile and checks the result against
// the contract's text limits.
func layText(t *testing.T, g model.Graph) *model.PositionedGraph {
	t.Helper()
	pg, err := Layout(context.Background(), g, TextConfig(g.Direction), nil)
	require.NoError(t, err)
	for _, v := range contract.Check(pg, contract.Options{Limits: contract.TextLimits(g.Direction), Direction: g.Direction, EdgeIDs: ids(g), Text: true}) {
		t.Errorf("text: %s %s: %s", v.Rule, v.Subject, v.Detail)
	}
	return pg
}

func TestLayout_ClientServerTextQueryResultWiresMirror(t *testing.T) {
	// server and auth sit either side of db, and under RIGHT and LEFT
	// their wires into db jog exactly the in-lane gap apart in text: they
	// once took two lanes, auth's turning two columns past server's. They
	// share one lane, so both turn at one flow coordinate and mirror each
	// other about db's center across the flow (S8 Lanes). Under DOWN and
	// UP db is an even number of columns wide: its two ports fall on the
	// lines between cells and take the cells farther from its middle, so
	// they still mirror about its center (S8 Shape ports), while server's
	// and auth's lone ports each take the cell right of their middle line
	for _, dir := range []model.Direction{model.Down, model.Up, model.Right, model.Left} {
		t.Run(dir.String(), func(t *testing.T) {
			pg := layText(t, clientServer(dir))
			s, a := edge(t, pg, "server->db#0").Points, edge(t, pg, "auth->db#0").Points
			require.Len(t, s, 4)
			require.Len(t, a, 4)
			var db model.PositionedNode
			for _, n := range pg.Nodes {
				if n.ID == "db" {
					db = n
				}
			}
			flow := func(p model.Point) float64 { along, _ := flowAxis(p, dir); return along }
			across := func(p model.Point) float64 { _, c := flowAxis(p, dir); return c }
			center := across(model.Point{X: db.X, Y: db.Y})
			assert.InDelta(t, flow(s[1]), flow(a[1]), 1e-6, "one lane")
			assert.InDelta(t, center, (across(s[3])+across(a[3]))/2, 1e-6, "ports mirrored about db")
			if dir == model.Down || dir == model.Up {
				return
			}
			assert.InDelta(t, center, (across(s[0])+across(a[0]))/2, 1e-6, "sources mirrored about db")
		})
	}
}

func TestLayout_ASidewaysCardinalityInACrowdFollowsItsRoute(t *testing.T) {
	// fleet, depot and chargeable relate to truck under RIGHT
	// (the class-relations example, pared down).
	// depot's wire turns into truck on a last run 24 px long, 16 px below
	// fleet's wire: every spot one to three steps from its end meets
	// truck, a wire or fleet's cardinality, placed first, so depot's
	// "0..*" once fell back to its least overlap. It follows its route
	// around the turn, clear, within four rings of it (S10). Every
	// cardinality resolved lies nearer its own wire than another's, beyond
	// the slack; fleet's "1..*", between chargeable's wire and depot's 16
	// px either side of its own, finds no such spot and falls back
	record := func(id, label string, attrs ...string) model.Node {
		m := &model.Members{}
		for _, a := range attrs {
			m.Attributes = append(m.Attributes, model.Member{Visibility: "-", Text: a})
		}
		return model.Node{ID: id, Label: label, Members: m}
	}
	g := model.Graph{Direction: model.Right,
		Nodes: []model.Node{record("truck", "Truck", "payload: t"), record("chargeable", "Chargeable"), record("fleet", "Fleet", "name: string"), record("depot", "Depot")},
		Edges: []model.Edge{
			{ID: "truck->chargeable#0", From: "chargeable", To: "truck", Relation: model.RelationRealization},
			{ID: "fleet->truck#0", From: "fleet", To: "truck", Relation: model.RelationComposition, FromCard: "1", ToCard: "1..*"},
			{ID: "depot->truck#0", From: "depot", To: "truck", Relation: model.RelationAggregation, ToCard: "0..*"},
		},
	}
	cfg := screen()
	pg := lay(t, g, cfg, model.Right)
	for i, e := range pg.Edges {
		for _, c := range []*model.EndLabel{e.FromCard, e.ToCard} {
			if c == nil {
				continue
			}
			require.NotNil(t, c.Pos, "%s %s", e.ID, c.Text)
			if e.ID == "fleet->truck#0" && c == e.ToCard {
				assert.True(t, c.Unresolved, "%s %s: no spot of its own", e.ID, c.Text)
				continue
			}
			assert.False(t, c.Unresolved, "%s %s: unresolved", e.ID, c.Text)
			b := [4]float64{c.Pos.X - c.Width/2, c.Pos.Y - c.Height/2, c.Pos.X + c.Width/2, c.Pos.Y + c.Height/2}
			assert.LessOrEqual(t, gapTo(b, e.Points), 4*cfg.CardStep, "%s %s: beside its own wire", e.ID, c.Text)
			assert.LessOrEqual(t, nearerOther(pg, i, b), cfg.OwnSlack, "%s %s: another wire lies nearer", e.ID, c.Text)
		}
	}
}

func TestLayout_ClientServerTextCrossesTheBorderStraight(t *testing.T) {
	// under RIGHT and LEFT the proxy's three in-ports sit one row apart in
	// text. The Services level's terminals once kept the lane gap, two
	// rows, between them, three between their centers, so web's and
	// cli's wires jogged again inside Services: four bends each. With the
	// terminal gap, one row, each terminal sits on its port, and the
	// wires jog only between the frames, as on screen (S7, S9, S14)
	for _, dir := range []model.Direction{model.Right, model.Left} {
		t.Run(dir.String(), func(t *testing.T) {
			pg := layText(t, clientServer(dir))
			for _, id := range []string{"web->proxy#0", "cli->proxy#0"} {
				assert.Len(t, edge(t, pg, id).Points, 4, "%s: two bends", id)
			}
			assert.Len(t, edge(t, pg, "mobile->proxy#0").Points, 2, "mobile: straight")
		})
	}
}

func TestLayout_TextALabelLeavesAGroupTooNarrowForIt(t *testing.T) {
	// api and db stacked in Backend, a group as narrow as its nodes, and
	// a class package as narrow around order and item: no spot inside the
	// frame fits the label. It once stayed unresolved and the text
	// renderer dropped it; the ladder runs again without the group (S10
	// Text cells), and the label is drawn
	flow := model.Graph{
		Nodes: []model.Node{{ID: "c", Label: "Client", Shape: model.ShapeRect}, {ID: "a", Label: "API", Shape: model.ShapeRect},
			{ID: "b", Label: "DB", Shape: model.ShapeCylinder}},
		Edges:  []model.Edge{{ID: "c->a#0", From: "c", To: "a"}, {ID: "a->b#0", From: "a", To: "b", Label: "HTTPS"}},
		Groups: []model.Group{{ID: "g", Label: "Backend", Contains: []string{"a", "b"}}},
	}
	class := model.Graph{
		Nodes:  []model.Node{{ID: "order", Label: "Order", Members: &model.Members{}}, {ID: "item", Label: "Item", Members: &model.Members{}}},
		Edges:  []model.Edge{{ID: "order->item#0", From: "order", To: "item", Label: "contains", Relation: model.RelationComposition}},
		Groups: []model.Group{{ID: "sales", Label: "Sales", Contains: []string{"order", "item"}}},
	}
	for _, tc := range []struct {
		name string
		g    model.Graph
		edge string
	}{{"flow", flow, "a->b#0"}, {"class", class, "order->item#0"}} {
		for _, dir := range []model.Direction{model.Down, model.Up} {
			t.Run(tc.name+"/"+dir.String(), func(t *testing.T) {
				g := tc.g
				g.Direction = dir
				pg := layText(t, g)
				e := edge(t, pg, tc.edge)
				require.NotNil(t, e.LabelPos)
				assert.False(t, e.LabelUnresolved, "resolved")
			})
		}
	}
}
