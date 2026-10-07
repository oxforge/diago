// Package lgraphtest builds levels from a compact notation for the layered
// engine's stage tests. The engine never imports it.
package lgraphtest

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/layout/layered/lgraph"
	"github.com/oxforge/diago/internal/model"
)

// Level builds a level from specs and fails the test on a malformed one.
// Each spec is an edge "from->to" or a lone node "id". An id may carry a
// shape where it first appears, as in "m:diamond" (any name
// model.ParseShape accepts); later mentions omit it. Nodes come in order of
// first appearance, 80 wide and 40 tall. An edge's id is "from->to#n", n
// counting the earlier edges between the same pair.
func Level(t testing.TB, specs ...string) *lgraph.Level {
	t.Helper()
	lv, err := parse(specs)
	require.NoError(t, err)
	return lv
}

func parse(specs []string) (*lgraph.Level, error) {
	lv := &lgraph.Level{}
	index := map[string]int{}
	node := func(tok string) (int, error) {
		id, shapeName, hasShape := strings.Cut(strings.TrimSpace(tok), ":")
		if id == "" {
			return 0, fmt.Errorf("empty node id in %q", tok)
		}
		if i, ok := index[id]; ok {
			return i, nil
		}
		shape := model.ShapeRect
		if hasShape {
			s, err := model.ParseShape(shapeName)
			if err != nil {
				return 0, fmt.Errorf("node %s: %w", id, err)
			}
			shape = s
		}
		index[id] = len(lv.Nodes)
		lv.Nodes = append(lv.Nodes, lgraph.Node{ID: id, Shape: shape, W: 80, H: 40})
		return index[id], nil
	}
	repeats := map[[2]int]int{}
	for _, spec := range specs {
		fromTok, toTok, isEdge := strings.Cut(spec, "->")
		from, err := node(fromTok)
		if err != nil {
			return nil, err
		}
		if !isEdge {
			continue
		}
		to, err := node(toTok)
		if err != nil {
			return nil, err
		}
		n := repeats[[2]int{from, to}]
		repeats[[2]int{from, to}] = n + 1
		lv.Edges = append(lv.Edges, lgraph.Edge{
			ID:   fmt.Sprintf("%s->%s#%d", lv.Nodes[from].ID, lv.Nodes[to].ID, n),
			From: from, To: to,
		})
	}
	return lv, nil
}
