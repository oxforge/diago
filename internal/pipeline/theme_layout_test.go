package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/diff"
	"github.com/oxforge/diago/internal/model"
	"github.com/oxforge/diago/internal/schema"
	"github.com/oxforge/diago/internal/theme"
	"github.com/oxforge/diago/themes"
)

// roomyTheme writes a copy of the default theme named "roomy" to a theme
// directory (DIAGO_THEME_DIR) with node padding 30 / 20, group padding 24
// and activation width 16.
func roomyTheme(t *testing.T) {
	t.Helper()
	data, err := themes.FS.ReadFile("default.json")
	require.NoError(t, err)
	s := string(data)
	for _, r := range [][2]string{
		{`"padding": { "x": 20, "y": 12 }`, `"padding": { "x": 30, "y": 20 }`},
		{`"padding": 12`, `"padding": 24`},
		{`"name": "default"`, `"name": "roomy"`},
	} {
		require.Contains(t, s, r[0])
		s = strings.Replace(s, r[0], r[1], 1)
	}
	require.Contains(t, s, `"width": 10`)
	s = strings.Replace(s, `"width": 10`, `"width": 16`, 1)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "roomy.json"), []byte(s), 0o600))
	t.Setenv("DIAGO_THEME_DIR", dir)
	th, err := theme.Load("roomy")
	require.NoError(t, err)
	require.Equal(t, 24.0, th.Group.Padding)
}

const themedFlow = `{"type":"flow","direction":"DOWN",
"nodes":[{"id":"a","label":"Application Server"},{"id":"b","label":"Backend Service"}],
"edges":[{"from":"a","to":"b"}],
"groups":[{"id":"g","label":"G","contains":["a","b"]}]}`

const themedClass = `{"type":"class","direction":"DOWN",
"classes":[{"id":"c","label":"C","attributes":[{"visibility":"-","text":"id: int"}]}]}`

const themedSeq = `{"type":"sequence",
"actors":[{"id":"a","label":"Application Server"},{"id":"b","label":"Backend Service"}],
"interactions":[{"from":"a","to":"b","label":"hi"},{"from":"b","to":"a","label":"ok"}]}`

func nodeByID(pg *model.PositionedGraph, id string) model.PositionedNode {
	for _, n := range pg.Nodes {
		if n.ID == id {
			return n
		}
	}
	return model.PositionedNode{}
}

func groupByID(pg *model.PositionedGraph, id string) model.PositionedGroup {
	for _, g := range pg.Groups {
		if g.ID == id {
			return g
		}
	}
	return model.PositionedGroup{}
}

// groupInset is the group frame's left inset from its content: the gap
// between the frame's left side and its leftmost node.
func groupInset(pg *model.PositionedGraph) float64 {
	g := groupByID(pg, "g")
	n := nodeByID(pg, "a")
	left := n.X - n.Width/2
	return left - g.X
}

func TestThemePadding_ReachesFlowLayout(t *testing.T) {
	roomyTheme(t)
	ctx := context.Background()
	def, _, err := layoutGraph(ctx, []byte(themedFlow), "flow", schema.ParseFlow, "default", nil)
	require.NoError(t, err)
	roomy, _, err := layoutGraph(ctx, []byte(themedFlow), "flow", schema.ParseFlow, "roomy", nil)
	require.NoError(t, err)

	// The box grows by twice the padding difference: 2 * (30 - 20) wide, 2 * (20 - 12) tall.
	da, ra := nodeByID(def, "a"), nodeByID(roomy, "a")
	assert.InDelta(t, 20, ra.Width-da.Width, 1e-9, "node width")
	assert.InDelta(t, 16, ra.Height-da.Height, 1e-9, "node height")
	// The group frame is 12 px further from its content on each side.
	assert.InDelta(t, 12, groupInset(roomy)-groupInset(def), 1e-9, "group inset")
}

func TestThemePadding_LeavesClassBoxAlone(t *testing.T) {
	roomyTheme(t)
	ctx := context.Background()
	def, _, err := layoutGraph(ctx, []byte(themedClass), "class", schema.ParseClass, "default", nil)
	require.NoError(t, err)
	roomy, _, err := layoutGraph(ctx, []byte(themedClass), "class", schema.ParseClass, "roomy", nil)
	require.NoError(t, err)
	d, r := nodeByID(def, "c"), nodeByID(roomy, "c")
	assert.Equal(t, d.Width, r.Width)
	assert.Equal(t, d.Height, r.Height)
}

func TestThemeActivationWidth_ReachesSequenceLayout(t *testing.T) {
	roomyTheme(t)
	d, err := schema.ParseSequence([]byte(themedSeq))
	require.NoError(t, err)
	for name, want := range map[string]float64{"default": 10, "roomy": 16} {
		th, err := theme.Load(name)
		require.NoError(t, err)
		ps := layoutSequenceScreen(*d, th)
		require.NotEmpty(t, ps.Activations, name)
		for _, a := range ps.Activations {
			assert.Equal(t, want, a.Width, name)
		}
	}
}

func TestThemePadding_ReachesFlowDiff(t *testing.T) {
	roomyTheme(t)
	before, err := schema.ParseFlow([]byte(themedFlow))
	require.NoError(t, err)
	after, err := schema.ParseFlow([]byte(`{"type":"flow","direction":"DOWN",
"nodes":[{"id":"a","label":"Application Server"},{"id":"b","label":"Backend Service"},{"id":"c","label":"C"}],
"edges":[{"from":"a","to":"b"},{"from":"b","to":"c"}],
"groups":[{"id":"g","label":"G","contains":["a","b"]}]}`))
	require.NoError(t, err)
	u := diff.BuildUnion(before, after)
	ctx := context.Background()
	var pgs []*model.PositionedGraph
	for _, name := range []string{"default", "roomy"} {
		th, err := theme.Load(name)
		require.NoError(t, err)
		pg, _, err := layoutFlowDiff(ctx, u, before, th, false)
		require.NoError(t, err)
		pgs = append(pgs, pg)
	}
	assert.InDelta(t, 20, nodeByID(pgs[1], "a").Width-nodeByID(pgs[0], "a").Width, 1e-9, "node width")
	assert.InDelta(t, 12, groupInset(pgs[1])-groupInset(pgs[0]), 1e-9, "group inset")
}
