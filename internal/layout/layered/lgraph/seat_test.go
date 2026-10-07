package lgraph_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/layout/layered/lgraph"
	"github.com/oxforge/diago/internal/layout/layered/lgraph/lgraphtest"
	"github.com/oxforge/diago/internal/layoutdbg"
)

// seat builds lv on layers, its edges reversed as reversed says, seats
// its corridors (S5) and returns the graph and its corridor_seated
// records.
func seat(t *testing.T, lv *lgraph.Level, layers []int, reversed []bool) (*lgraph.Graph, []map[string]any) {
	t.Helper()
	g, err := lgraph.Build(lv, layers, reversed, 8)
	require.NoError(t, err)
	var buf bytes.Buffer
	g.SeatCorridors(layoutdbg.NewContext(context.Background(), slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))))
	var recs []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &rec))
		if rec["decision"] == "corridor_seated" {
			recs = append(recs, rec)
		}
	}
	return g, recs
}

// layer returns the ids of layer l's vertices, in order.
func layer(g *lgraph.Graph, l int) []string {
	out := make([]string, len(g.Layers[l]))
	for i, v := range g.Layers[l] {
		out[i] = g.Vertices[v].ID
	}
	return out
}

// TestSeatCorridors_FlowNetwork pins S5's Corridor seats on
// the flow-network example: api -> db skips the middle row, where
// Database's parents are Redis Cache and Message Queue. Its column,
// seated last among the nodes, lands between them, at the mean of their
// positions 1 and 2.
func TestSeatCorridors_FlowNetwork(t *testing.T) {
	lv := lgraphtest.Level(t, "web->api", "api->auth", "api->db", "api->cache", "api->queue", "queue->db", "cache->db")
	g, recs := seat(t, lv, []int{0, 1, 2, 3, 2, 2}, nil)
	assert.Equal(t, []string{"auth", "cache", "api->db#0@2", "queue"}, layer(g, 2))
	for _, l := range []int{0, 1, 3} {
		assert.Len(t, g.Layers[l], 1, "layer %d", l)
	}
	require.Len(t, recs, 1)
	assert.Equal(t, "api->db#0@2", recs[0]["dummy"])
	assert.Equal(t, "db", recs[0]["end"])
	assert.Equal(t, []any{"cache", "queue"}, recs[0]["parents"])
	assert.Equal(t, 1.5, recs[0]["key"])
	assert.Equal(t, 2.0, recs[0]["index"])
	assert.Equal(t, "lgraph", recs[0]["phase"])
	assert.Equal(t, "S5", recs[0]["spec_ref"])
}

// TestSeatCorridors_AnOddCountFollowsTheMiddleParent pins the odd median:
// t's parents a, b and c sit at 1, 2 and 3 behind x, and r -> t's column
// takes b's key, 2, and follows b. a joins t twice but counts once: its
// two hops would make the count even and the key 1.5.
func TestSeatCorridors_AnOddCountFollowsTheMiddleParent(t *testing.T) {
	lv := lgraphtest.Level(t, "r->x", "r->a", "r->b", "r->c", "a->t", "a->t", "b->t", "c->t", "r->t")
	g, recs := seat(t, lv, []int{0, 1, 1, 1, 1, 2}, nil)
	assert.Equal(t, []string{"x", "a", "b", "r->t#0@1", "c"}, layer(g, 1))
	require.Len(t, recs, 1)
	assert.Equal(t, []any{"a", "b", "c"}, recs[0]["parents"])
	assert.Equal(t, 2.0, recs[0]["key"])
}

// TestSeatCorridors_OnlyAChainsLastDummy pins that a column is seated
// only on the layer just above its downstream end: r -> t's first dummy
// stays behind a and b, its last lands between t's parents p and q.
func TestSeatCorridors_OnlyAChainsLastDummy(t *testing.T) {
	lv := lgraphtest.Level(t, "r->a", "r->b", "a->p", "b->q", "p->t", "q->t", "r->t")
	g, recs := seat(t, lv, []int{0, 1, 1, 2, 2, 3}, nil)
	assert.Equal(t, []string{"a", "b", "r->t#0@1"}, layer(g, 1))
	assert.Equal(t, []string{"p", "r->t#0@2", "q"}, layer(g, 2))
	require.Len(t, recs, 1)
	assert.Equal(t, "r->t#0@2", recs[0]["dummy"])
}

// TestSeatCorridors_TheColumnsOfOneEndKeepEdgeOrder pins the tie between
// columns: both r -> t chains are keyed at 0.5, between t's parents a and
// b, and keep their edge order there.
func TestSeatCorridors_TheColumnsOfOneEndKeepEdgeOrder(t *testing.T) {
	lv := lgraphtest.Level(t, "r->a", "r->b", "a->t", "b->t", "r->t", "r->t")
	g, recs := seat(t, lv, []int{0, 1, 1, 2}, nil)
	assert.Equal(t, []string{"a", "r->t#0@1", "r->t#1@1", "b"}, layer(g, 1))
	require.Len(t, recs, 2)
	assert.Equal(t, 0.5, recs[0]["key"])
	assert.Equal(t, 0.5, recs[1]["key"])
}

// TestSeatCorridors_NeedTwoParents pins what stays in the declaration
// order: a downstream end with one parent, whose other upper neighbors
// are columns, not parents; and a reversed edge's column, a return path.
func TestSeatCorridors_NeedTwoParents(t *testing.T) {
	t.Run("one parent", func(t *testing.T) {
		lv := lgraphtest.Level(t, "r->a", "r->z", "a->t", "r->t")
		g, recs := seat(t, lv, []int{0, 1, 1, 2}, nil)
		assert.Equal(t, []string{"a", "z", "r->t#0@1"}, layer(g, 1))
		assert.Empty(t, recs)
	})
	t.Run("one parent and two columns", func(t *testing.T) {
		lv := lgraphtest.Level(t, "r->a", "r->z", "a->t", "r->t", "r->t")
		g, recs := seat(t, lv, []int{0, 1, 1, 2}, nil)
		assert.Equal(t, []string{"a", "z", "r->t#0@1", "r->t#1@1"}, layer(g, 1))
		assert.Empty(t, recs)
	})
	t.Run("a reversed edge", func(t *testing.T) {
		// t -> r runs against the flow: its chain goes from r down to t,
		// whose parents are a and b, but it is a return path.
		lv := lgraphtest.Level(t, "r->a", "r->b", "a->t", "b->t", "t->r")
		g, recs := seat(t, lv, []int{0, 1, 1, 2}, []bool{false, false, false, false, true})
		assert.Equal(t, []string{"a", "b", "t->r#0@1"}, layer(g, 1))
		assert.Empty(t, recs)
	})
}
