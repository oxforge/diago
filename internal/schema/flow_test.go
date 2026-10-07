package schema_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/oxforge/diago/internal/model"
	"github.com/oxforge/diago/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var validFlowSpec = `{
  "type": "flow",
  "direction": "DOWN",
  "nodes": [
    {"id": "a", "label": "Node A", "shape": "rect"},
    {"id": "b", "label": "Node B", "shape": "rounded"},
    {"id": "c", "label": "Node C", "shape": "cylinder"}
  ],
  "edges": [
    {"from": "a", "to": "b", "label": "A to B", "style": "solid"},
    {"from": "b", "to": "c", "style": "dashed"}
  ],
  "groups": [
    {"id": "g1", "label": "Group 1", "contains": ["b", "c"]}
  ]
}`

func TestParseFlowValid(t *testing.T) {
	g, err := schema.ParseFlow([]byte(validFlowSpec))
	require.NoError(t, err)
	require.NotNil(t, g)

	assert.Equal(t, model.Down, g.Direction)
	require.Len(t, g.Nodes, 3)
	assert.Equal(t, "a", g.Nodes[0].ID)
	assert.Equal(t, "Node A", g.Nodes[0].Label)
	assert.Equal(t, model.ShapeRect, g.Nodes[0].Shape)
	assert.Equal(t, "b", g.Nodes[1].ID)
	assert.Equal(t, model.ShapeRounded, g.Nodes[1].Shape)
	assert.Equal(t, model.ShapeCylinder, g.Nodes[2].Shape)

	require.Len(t, g.Edges, 2)
	assert.Equal(t, "a", g.Edges[0].From)
	assert.Equal(t, "b", g.Edges[0].To)
	assert.Equal(t, "A to B", g.Edges[0].Label)
	assert.Equal(t, model.EdgeSolid, g.Edges[0].Style)
	assert.Equal(t, model.EdgeDashed, g.Edges[1].Style)

	require.Len(t, g.Groups, 1)
	assert.Equal(t, "g1", g.Groups[0].ID)
	assert.Equal(t, "Group 1", g.Groups[0].Label)
	assert.Equal(t, []string{"b", "c"}, g.Groups[0].Contains)
}

func TestParseFlowDefaults(t *testing.T) {
	// direction defaults to UNDEFINED when omitted; shape defaults to rect
	spec := `{
		"type": "flow",
		"nodes": [{"id": "a", "label": "A"}],
		"edges": []
	}`
	g, err := schema.ParseFlow([]byte(spec))
	require.NoError(t, err)
	assert.Equal(t, model.Auto, g.Direction)
	assert.Equal(t, model.ShapeRect, g.Nodes[0].Shape)
}

// TestParseFlowMissingDirectionAccepted confirms that top-level direction is
// optional (contract: defaults to AUTO). Omitting it must not produce a
// validation error, matching the schema where direction is not in "required".
func TestParseFlowMissingDirectionAccepted(t *testing.T) {
	spec := `{"type": "flow", "nodes": [{"id": "a", "label": "A"}], "edges": []}`
	g, err := schema.ParseFlow([]byte(spec))
	require.NoError(t, err)
	assert.Equal(t, model.Auto, g.Direction)
}

// TestParseFlowEdgeDirectionValid confirms all documented edge directions parse
// (contract: forward (default), backward, both), and that an empty direction
// defaults to forward.
func TestParseFlowEdgeDirectionValid(t *testing.T) {
	cases := map[string]model.EdgeDirection{
		"":         model.EdgeForward,
		"forward":  model.EdgeForward,
		"backward": model.EdgeBackward,
		"both":     model.EdgeBoth,
	}
	for dir, want := range cases {
		t.Run(dir, func(t *testing.T) {
			spec := fmt.Sprintf(
				`{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":[{"from":"a","to":"b","direction":%q}]}`,
				dir,
			)
			g, err := schema.ParseFlow([]byte(spec))
			require.NoError(t, err)
			require.Len(t, g.Edges, 1)
			assert.Equal(t, want, g.Edges[0].Direction)
		})
	}
}

func TestParseFlowValidationErrors(t *testing.T) {
	tests := []struct {
		name       string
		spec       string
		fieldPath  string
		msgContain string
	}{
		{
			name:       "wrong type",
			spec:       `{"type": "sequence", "nodes": [{"id": "a", "label": "A"}], "edges": []}`,
			fieldPath:  "type",
			msgContain: "flow",
		},
		{
			name:       "invalid direction",
			spec:       `{"type": "flow", "direction": "up-down", "nodes": [{"id": "a", "label": "A"}], "edges": []}`,
			fieldPath:  "direction",
			msgContain: "invalid direction",
		},
		{
			name:       "empty nodes",
			spec:       `{"type": "flow", "nodes": [], "edges": []}`,
			fieldPath:  "nodes",
			msgContain: "at least one node",
		},
		{
			name:       "missing node id",
			spec:       `{"type": "flow", "nodes": [{"id": "", "label": "A"}], "edges": []}`,
			fieldPath:  "nodes[0].id",
			msgContain: "must not be empty",
		},
		{
			name:       "missing node label",
			spec:       `{"type": "flow", "nodes": [{"id": "a", "label": ""}], "edges": []}`,
			fieldPath:  "nodes[0].label",
			msgContain: "must not be empty",
		},
		{
			name:       "duplicate node id",
			spec:       `{"type": "flow", "nodes": [{"id": "a", "label": "A"}, {"id": "a", "label": "B"}], "edges": []}`,
			fieldPath:  "nodes[1].id",
			msgContain: "duplicate",
		},
		{
			name:       "unknown node shape",
			spec:       `{"type": "flow", "nodes": [{"id": "a", "label": "A", "shape": "octagon"}], "edges": []}`,
			fieldPath:  "nodes[0].shape",
			msgContain: "octagon",
		},
		{
			name:       "edge references unknown from",
			spec:       `{"type": "flow", "nodes": [{"id": "a", "label": "A"}], "edges": [{"from": "x", "to": "a"}]}`,
			fieldPath:  "edges[0].from",
			msgContain: "unknown node",
		},
		{
			name:       "edge references unknown to",
			spec:       `{"type": "flow", "nodes": [{"id": "a", "label": "A"}], "edges": [{"from": "a", "to": "z"}]}`,
			fieldPath:  "edges[0].to",
			msgContain: "unknown node",
		},
		{
			name:       "unknown edge style",
			spec:       `{"type": "flow", "nodes": [{"id": "a", "label": "A"}, {"id": "b", "label": "B"}], "edges": [{"from": "a", "to": "b", "style": "bold"}]}`,
			fieldPath:  "edges[0].style",
			msgContain: "bold",
		},
		{
			name:       "unknown edge direction",
			spec:       `{"type": "flow", "nodes": [{"id": "a", "label": "A"}, {"id": "b", "label": "B"}], "edges": [{"from": "a", "to": "b", "direction": "sideways"}]}`,
			fieldPath:  "edges[0].direction",
			msgContain: "sideways",
		},
		{
			name:       "group references unknown node",
			spec:       `{"type": "flow", "nodes": [{"id": "a", "label": "A"}], "edges": [], "groups": [{"id": "g1", "label": "G", "contains": ["x"]}]}`,
			fieldPath:  "groups[0].contains[0]",
			msgContain: "neither a valid node nor a valid group",
		},
		{
			name:       "node in multiple groups",
			spec:       `{"type": "flow", "nodes": [{"id": "a", "label": "A"}], "edges": [], "groups": [{"id": "g1", "label": "G1", "contains": ["a"]}, {"id": "g2", "label": "G2", "contains": ["a"]}]}`,
			fieldPath:  "groups[1].contains[0]",
			msgContain: "already belongs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := schema.ParseFlow([]byte(tt.spec))
			require.Error(t, err)

			var valErrs schema.ValidationErrors
			require.ErrorAs(t, err, &valErrs)

			found := false
			for _, ve := range valErrs {
				if ve.Field == tt.fieldPath {
					assert.Contains(t, ve.Message, tt.msgContain)
					found = true
					break
				}
			}
			assert.True(t, found, "expected error at field %q containing %q, got: %s", tt.fieldPath, tt.msgContain, err)
		})
	}
}

func TestParseFlow_NestedGroups(t *testing.T) {
	spec := `{
		"type": "flow", "direction": "DOWN",
		"nodes": [
			{"id": "n1", "label": "N1"},
			{"id": "n2", "label": "N2"},
			{"id": "n3", "label": "N3"}
		],
		"edges": [],
		"groups": [
			{"id": "outer", "label": "Outer", "contains": ["inner", "n3"]},
			{"id": "inner", "label": "Inner", "contains": ["n1", "n2"]}
		]
	}`
	g, err := schema.ParseFlow([]byte(spec))
	require.NoError(t, err)
	require.Len(t, g.Groups, 2)

	var outer, inner model.Group
	for _, grp := range g.Groups {
		switch grp.ID {
		case "outer":
			outer = grp
		case "inner":
			inner = grp
		}
	}

	assert.Equal(t, []string{"n3"}, outer.Contains)
	assert.Equal(t, []string{"inner"}, outer.Children)
	assert.Equal(t, 1, outer.Depth)

	assert.Equal(t, []string{"n1", "n2"}, inner.Contains)
	assert.Empty(t, inner.Children)
	assert.Equal(t, 2, inner.Depth)
}

func TestParseFlow_NestedGroups_MaxDepthExceeded(t *testing.T) {
	spec := `{
		"type": "flow", "direction": "DOWN",
		"nodes": [{"id": "n1", "label": "N1"}],
		"edges": [],
		"groups": [
			{"id": "g1", "label": "G1", "contains": ["g2"]},
			{"id": "g2", "label": "G2", "contains": ["g3"]},
			{"id": "g3", "label": "G3", "contains": ["g4"]},
			{"id": "g4", "label": "G4", "contains": ["n1"]}
		]
	}`
	_, err := schema.ParseFlow([]byte(spec))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum nesting depth of 3")
}

func TestParseFlow_NestedGroups_CycleDetected(t *testing.T) {
	spec := `{
		"type": "flow", "direction": "DOWN",
		"nodes": [{"id": "n1", "label": "N1"}],
		"edges": [],
		"groups": [
			{"id": "g1", "label": "G1", "contains": ["g2"]},
			{"id": "g2", "label": "G2", "contains": ["g1"]}
		]
	}`
	_, err := schema.ParseFlow([]byte(spec))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}

func TestParseFlow_NestedGroups_NodeGroupIDCollision(t *testing.T) {
	spec := `{
		"type": "flow", "direction": "DOWN",
		"nodes": [{"id": "x", "label": "X"}],
		"edges": [],
		"groups": [
			{"id": "x", "label": "G", "contains": []}
		]
	}`
	_, err := schema.ParseFlow([]byte(spec))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "collides with node id")
}

func TestParseFlow_NestedGroups_DuplicateGroupID(t *testing.T) {
	spec := `{
		"type": "flow", "direction": "DOWN",
		"nodes": [{"id": "n1", "label": "N1"}],
		"edges": [],
		"groups": [
			{"id": "g1", "label": "G1", "contains": ["n1"]},
			{"id": "g1", "label": "G1 dup", "contains": []}
		]
	}`
	_, err := schema.ParseFlow([]byte(spec))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate group id")
}

func TestParseFlow_NestedGroups_GroupInMultipleParents(t *testing.T) {
	spec := `{
		"type": "flow", "direction": "DOWN",
		"nodes": [{"id": "n1", "label": "N1"}],
		"edges": [],
		"groups": [
			{"id": "child", "label": "Child", "contains": ["n1"]},
			{"id": "p1", "label": "P1", "contains": ["child"]},
			{"id": "p2", "label": "P2", "contains": ["child"]}
		]
	}`
	_, err := schema.ParseFlow([]byte(spec))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already belongs to group")
}

func TestParseFlow_NestedGroups_ThreeLevelsValid(t *testing.T) {
	spec := `{
		"type": "flow", "direction": "DOWN",
		"nodes": [{"id": "n1", "label": "N1"}],
		"edges": [],
		"groups": [
			{"id": "l1", "label": "Level 1", "contains": ["l2"]},
			{"id": "l2", "label": "Level 2", "contains": ["l3"]},
			{"id": "l3", "label": "Level 3", "contains": ["n1"]}
		]
	}`
	g, err := schema.ParseFlow([]byte(spec))
	require.NoError(t, err)

	depthByID := make(map[string]int)
	for _, grp := range g.Groups {
		depthByID[grp.ID] = grp.Depth
	}
	assert.Equal(t, 1, depthByID["l1"])
	assert.Equal(t, 2, depthByID["l2"])
	assert.Equal(t, 3, depthByID["l3"])
}

func TestParseFlow_NestedGroups_MixedContents(t *testing.T) {
	spec := `{
		"type": "flow", "direction": "DOWN",
		"nodes": [
			{"id": "n1", "label": "N1"},
			{"id": "n2", "label": "N2"}
		],
		"edges": [],
		"groups": [
			{"id": "parent", "label": "Parent", "contains": ["child", "n1"]},
			{"id": "child", "label": "Child", "contains": ["n2"]}
		]
	}`
	g, err := schema.ParseFlow([]byte(spec))
	require.NoError(t, err)

	var parent model.Group
	for _, grp := range g.Groups {
		if grp.ID == "parent" {
			parent = grp
		}
	}
	assert.Equal(t, []string{"n1"}, parent.Contains)
	assert.Equal(t, []string{"child"}, parent.Children)
}

func TestParseFlow_NodeColor_Valid(t *testing.T) {
	spec := `{"type":"flow","direction":"DOWN","nodes":[{"id":"a","label":"A","color":"red"}],"edges":[]}`
	g, err := schema.ParseFlow([]byte(spec))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if g.Nodes[0].Color != "red" {
		t.Errorf("expected color %q, got %q", "red", g.Nodes[0].Color)
	}
}

func TestParseFlow_NodeColor_Hex(t *testing.T) {
	spec := `{"type":"flow","direction":"DOWN","nodes":[{"id":"a","label":"A","color":"#e63946"}],"edges":[]}`
	g, err := schema.ParseFlow([]byte(spec))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if g.Nodes[0].Color != "#e63946" {
		t.Errorf("expected color %q, got %q", "#e63946", g.Nodes[0].Color)
	}
}

func TestParseFlow_NodeColor_Invalid(t *testing.T) {
	spec := `{"type":"flow","direction":"DOWN","nodes":[{"id":"a","label":"A","color":"pink"}],"edges":[]}`
	_, err := schema.ParseFlow([]byte(spec))
	if err == nil {
		t.Fatal("expected validation error for invalid color")
	}
}

func TestParseFlow_EdgeColor(t *testing.T) {
	spec := `{"type":"flow","direction":"DOWN","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":[{"from":"a","to":"b","color":"blue"}]}`
	g, err := schema.ParseFlow([]byte(spec))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if g.Edges[0].Color != "blue" {
		t.Errorf("expected color %q, got %q", "blue", g.Edges[0].Color)
	}
}

func TestParseFlow_GroupColor(t *testing.T) {
	spec := `{"type":"flow","direction":"DOWN","nodes":[{"id":"a","label":"A"}],"edges":[],"groups":[{"id":"g","label":"G","contains":["a"],"color":"orange"}]}`
	g, err := schema.ParseFlow([]byte(spec))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if g.Groups[0].Color != "orange" {
		t.Errorf("expected color %q, got %q", "orange", g.Groups[0].Color)
	}
}

func TestValidationErrorsString(t *testing.T) {
	errs := schema.ValidationErrors{
		{Field: "nodes[0].id", Message: "must not be empty"},
		{Field: "nodes[0].label", Message: "must not be empty"},
	}
	s := errs.Error()
	assert.Contains(t, s, "nodes[0].id")
	assert.Contains(t, s, "nodes[0].label")
}

func TestParseFlowEdgeIDs_Derived(t *testing.T) {
	spec := `{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],
		"edges":[{"from":"a","to":"b"},{"from":"a","to":"b","label":"again"},{"from":"b","to":"a"}]}`
	g, err := schema.ParseFlow([]byte(spec))
	require.NoError(t, err)
	require.Len(t, g.Edges, 3)
	assert.Equal(t, "a->b#0", g.Edges[0].ID)
	assert.Equal(t, "a->b#1", g.Edges[1].ID)
	assert.Equal(t, "b->a#0", g.Edges[2].ID)
}

func TestParseFlowEdgeIDs_ExplicitKeepsOrdinals(t *testing.T) {
	// Naming the first edge must not renumber the second: n counts every edge of the pair.
	spec := `{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],
		"edges":[{"id":"first","from":"a","to":"b"},{"from":"a","to":"b"}]}`
	g, err := schema.ParseFlow([]byte(spec))
	require.NoError(t, err)
	assert.Equal(t, "first", g.Edges[0].ID)
	assert.Equal(t, "a->b#1", g.Edges[1].ID)
}

func TestParseFlowEdgeIDs_Errors(t *testing.T) {
	cases := []struct{ name, edges, field, contains string }{
		{"empty explicit", `[{"id":"","from":"a","to":"b"}]`, "edges[0].id", "must not be empty"},
		{"duplicate explicit", `[{"id":"x","from":"a","to":"b"},{"id":"x","from":"b","to":"a"}]`, "edges[1].id", "duplicate edge id"},
		{"collides with derived", `[{"from":"a","to":"b"},{"id":"a->b#0","from":"b","to":"a"}]`, "edges[1].id", "collides with the derived id of edges[0]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec := `{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":` + tc.edges + `}`
			_, err := schema.ParseFlow([]byte(spec))
			var ve schema.ValidationErrors
			require.ErrorAs(t, err, &ve)
			found := false
			for _, e := range ve {
				if e.Field == tc.field && strings.Contains(e.Message, tc.contains) {
					found = true
				}
			}
			assert.True(t, found, "want %s: %q in %v", tc.field, tc.contains, ve)
		})
	}
}

func TestParseFlow_Title(t *testing.T) {
	g, err := schema.ParseFlow([]byte(`{"type":"flow","title":"  Deploy pipeline ","nodes":[{"id":"a","label":"A"}],"edges":[]}`))
	require.NoError(t, err)
	assert.Equal(t, "Deploy pipeline", g.Title)
	g, err = schema.ParseFlow([]byte(`{"type":"flow","nodes":[{"id":"a","label":"A"}],"edges":[]}`))
	require.NoError(t, err)
	assert.Equal(t, "", g.Title)
}

func TestParseFlow_EdgeDirectionNone(t *testing.T) {
	g, err := schema.ParseFlow([]byte(`{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":[{"from":"a","to":"b","direction":"none"}]}`))
	require.NoError(t, err)
	assert.Equal(t, model.EdgeNone, g.Edges[0].Direction)
}

func TestParseFlow_HintsIgnored(t *testing.T) {
	// A contradictory hint pair used to be a validation error; the field
	// is gone, so the spec parses without error and the removed-field
	// advisory reports it (schema.Check, not exercised here).
	_, err := schema.ParseFlow([]byte(`{"type":"flow",
		"nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],
		"edges":[{"from":"a","to":"b"}],
		"hints":[{"type":"below","node":"a","relative_to":"b"},{"type":"above","node":"a","relative_to":"b"}]}`))
	require.NoError(t, err)
}

func TestParseFlow_EdgeFlat(t *testing.T) {
	g, err := schema.ParseFlow([]byte(`{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":[{"from":"a","to":"b","flat":true}]}`))
	require.NoError(t, err)
	assert.True(t, g.Edges[0].Flat)

	g, err = schema.ParseFlow([]byte(`{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":[{"from":"a","to":"b"}]}`))
	require.NoError(t, err)
	assert.False(t, g.Edges[0].Flat, "flat defaults to false when absent")
}

func TestParseFlow_FlatSelfLoopRejected(t *testing.T) {
	_, err := schema.ParseFlow([]byte(`{"type":"flow","nodes":[{"id":"a","label":"A"}],"edges":[{"from":"a","to":"a","flat":true}]}`))
	var ve schema.ValidationErrors
	require.ErrorAs(t, err, &ve)
	found := false
	for _, e := range ve {
		if e.Field == "edges[0].flat" && strings.Contains(e.Message, "a flat edge cannot be a self-loop") {
			found = true
		}
	}
	assert.True(t, found, "want edges[0].flat: %q in %v", "a flat edge cannot be a self-loop", ve)
}

func TestParseFlow_OrdinarySelfLoopStillAllowed(t *testing.T) {
	// A self-loop with no flat flag is unaffected by the new rule.
	g, err := schema.ParseFlow([]byte(`{"type":"flow","nodes":[{"id":"a","label":"A"}],"edges":[{"from":"a","to":"a"}]}`))
	require.NoError(t, err)
	assert.False(t, g.Edges[0].Flat)
}
