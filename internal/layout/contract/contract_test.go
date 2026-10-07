package contract

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/model"
)

// rect is an 80 x 40 rectangle centered on (x, y).
func rect(id string, x, y float64) model.PositionedNode {
	return model.PositionedNode{ID: id, Shape: model.ShapeRect, X: x, Y: y, Width: 80, Height: 40}
}

func shaped(id string, s model.Shape, x, y, w, h float64) model.PositionedNode {
	return model.PositionedNode{ID: id, Shape: s, X: x, Y: y, Width: w, Height: h}
}

// wire is an edge from→to through the points (x0,y0), (x1,y1), ...
func wire(from, to string, xy ...float64) model.PositionedEdge {
	e := model.PositionedEdge{ID: from + "->" + to + "#0", From: from, To: to}
	for i := 0; i+1 < len(xy); i += 2 {
		e.Points = append(e.Points, model.Point{X: xy[i], Y: xy[i+1]})
	}
	return e
}

func layout(w, h float64, nodes []model.PositionedNode, edges ...model.PositionedEdge) *model.PositionedGraph {
	return &model.PositionedGraph{Width: w, Height: h, Nodes: nodes, Edges: edges}
}

func check(pg *model.PositionedGraph) []Violation {
	return Check(pg, Options{Limits: ScreenLimits(), Direction: model.Down})
}

// chain is the smallest clean layout: a above b, one straight wire.
func chain() *model.PositionedGraph {
	return layout(120, 180,
		[]model.PositionedNode{rect("a", 60, 40), rect("b", 60, 140)},
		wire("a", "b", 60, 60, 60, 120))
}

// clean is a layout that satisfies every rule: a feeds diamond d, whose
// side vertices feed b and c inside group g; the d->c wire has a label.
func clean() *model.PositionedGraph {
	pg := layout(300, 350,
		[]model.PositionedNode{
			rect("a", 150, 40),
			shaped("d", model.ShapeDiamond, 150, 143, 80, 46),
			rect("b", 100, 290),
			rect("c", 220, 290),
		},
		wire("a", "d", 150, 60, 150, 120),
		wire("d", "b", 110, 143, 90, 143, 90, 270),
		wire("d", "c", 190, 143, 220, 143, 220, 270),
	)
	pg.Groups = []model.PositionedGroup{{
		ID: "g", Label: "Pool", X: 20, Y: 220, Width: 260, Height: 110,
		Contains: []string{"b", "c"}, Depth: 1, LabelWidth: 40, LabelHeight: 14,
	}}
	pg.Edges[2].Label = "yes"
	pg.Edges[2].LabelPos = &model.Point{X: 239, Y: 200}
	pg.Edges[2].LabelWidth, pg.Edges[2].LabelHeight = 24, 14
	return pg
}

func TestCheck_CleanLayouts(t *testing.T) {
	assert.Empty(t, check(chain()))
	assert.Empty(t, check(clean()))
	assert.Nil(t, Check(nil, Options{Limits: ScreenLimits()}))
}

func TestCheck_ReportsEveryRuleIDOnce(t *testing.T) {
	seen := map[string]bool{}
	for _, id := range RuleIDs {
		require.False(t, seen[id], "duplicate rule id %s", id)
		seen[id] = true
	}
}

// TestRuleIDs_MatchEmitters reads every non-test .go file in the package
// and collects the id of each c.add(...) call, so an emitted id missing
// from RuleIDs, or a RuleIDs entry nothing emits, fails.
func TestRuleIDs_MatchEmitters(t *testing.T) {
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	re := regexp.MustCompile(`c\.add\("([^"]+)"`)
	emitted := map[string]bool{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		require.NoError(t, err)
		for _, m := range re.FindAllSubmatch(src, -1) {
			emitted[string(m[1])] = true
		}
	}
	want := map[string]bool{}
	for _, id := range RuleIDs {
		want[id] = true
	}
	assert.Equal(t, want, emitted)
}

func TestCheck_Deterministic(t *testing.T) {
	pg := clean()
	pg.Nodes[2].X = 200 // b now overlaps c and leaves its wire detached
	first := check(pg)
	require.NotEmpty(t, first)
	for range 20 {
		assert.Equal(t, first, check(pg))
	}
}

func TestTextLimits(t *testing.T) {
	down := Limits{
		NodeClearance: 16, TrackGap: 16, PortGap: 8, StubMin: 24, NodeGap: 40,
		LabelGap: 8, Margin: 16, EnvelopeDepth: 8, EnvelopeHalf: 8, TitleInsetX: 24, TitleCenterY: 8, TitlePad: 8,
		CellW: 8, CellH: 16,
	}
	assert.Equal(t, down, TextLimits(model.Down))
	assert.Equal(t, down, TextLimits(model.Up))
	sideways := down
	sideways.PortGap, sideways.NodeGap = 16, 32
	assert.Equal(t, sideways, TextLimits(model.Right))
	assert.Equal(t, sideways, TextLimits(model.Left))
}

func TestCheck_TextReadsEveryShapeAsABox(t *testing.T) {
	// a enters diamond d on its top side 20 px left of the top vertex: off
	// the drawn diamond, but on the box the text profile draws.
	pg := layout(120, 250,
		[]model.PositionedNode{rect("a", 40, 40), shaped("d", model.ShapeDiamond, 60, 183, 80, 46)},
		wire("a", "d", 40, 60, 40, 160))
	screen := check(pg)
	assert.True(t, has(screen, "C2.2", "a->d#0"), "on screen the end misses the diamond's outline")
	assert.True(t, has(screen, "C8.1", "a->d#0"), "on screen the end misses the diamond's vertices")

	text := Check(pg, Options{Limits: ScreenLimits(), Direction: model.Down, Text: true})
	assert.False(t, has(text, "C2.2", "a->d#0"))
	assert.False(t, has(text, "C8.1", "a->d#0"))
	assert.Equal(t, model.ShapeDiamond, pg.Nodes[1].Shape, "the check leaves the layout untouched")
}
