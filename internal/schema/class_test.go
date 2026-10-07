package schema

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/model"
)

const classSpecOK = `{
  "type": "class",
  "direction": "DOWN",
  "classes": [
    {"id": "order", "label": "Order", "stereotype": "entity", "type_params": ["T"], "color": "blue",
     "attributes": [{"visibility": "-", "text": "id: string"}],
     "methods": [{"visibility": "+", "text": "total(): Money", "static": true}]},
    {"id": "item", "attributes": [{"text": "qty: int"}]},
    {"id": "base", "stereotype": "abstract"}
  ],
  "relations": [
    {"id": "r1", "from": "order", "to": "item", "kind": "composition", "label": "contains", "from_card": "1", "to_card": "*"},
    {"from": "item", "to": "base", "kind": "inheritance", "from_card": "sub", "to_card": "super"},
    {"from": "order", "to": "item", "kind": "dependency"},
    {"from": "order", "to": "item"},
    {"from": "order", "to": "order", "kind": "association", "directed": true, "color": "red"}
  ],
  "packages": [{"id": "domain", "label": "Domain", "contains": ["order", "item"], "color": "gray"}],
  "legend": true,
  "legend_labels": {"inheritance": "is-a", "dependency": ""},
  "ignore": ["deep-nesting"]
}`

func fields(t *testing.T, err error) map[string]string {
	t.Helper()
	var ve ValidationErrors
	require.True(t, errors.As(err, &ve), "want ValidationErrors, got %v", err)
	m := map[string]string{}
	for _, e := range ve {
		m[e.Field] = e.Message
	}
	return m
}

func TestParseClass_Graph(t *testing.T) {
	g, err := ParseClass([]byte(classSpecOK))
	require.NoError(t, err)
	assert.Equal(t, model.Down, g.Direction)
	require.Len(t, g.Nodes, 3)
	order := g.Nodes[0]
	assert.Equal(t, "Order", order.Label)
	assert.Equal(t, model.ShapeRect, order.Shape)
	assert.Equal(t, "blue", order.Color)
	require.NotNil(t, order.Members)
	assert.Equal(t, "entity", order.Members.Stereotype)
	assert.Equal(t, []string{"T"}, order.Members.TypeParams)
	assert.Equal(t, []model.Member{{Visibility: "-", Text: "id: string"}}, order.Members.Attributes)
	assert.Equal(t, []model.Member{{Visibility: "+", Text: "total(): Money", Static: true}}, order.Members.Methods)
	assert.Equal(t, "item", g.Nodes[1].Label, "label defaults to the id")
	assert.Equal(t, []model.Member{{Text: "qty: int"}}, g.Nodes[1].Members.Attributes)
	assert.Empty(t, g.Nodes[2].Members.Attributes)

	require.Len(t, g.Edges, 5)
	r1 := g.Edges[0]
	assert.Equal(t, "r1", r1.ID)
	assert.Equal(t, model.RelationComposition, r1.Relation)
	assert.Equal(t, model.EdgeSolid, r1.Style)
	assert.Equal(t, model.EdgeForward, r1.Direction)
	assert.False(t, r1.Directed)
	assert.Equal(t, "1", r1.FromCard)
	assert.Equal(t, "*", r1.ToCard)
	assert.Equal(t, "contains", r1.Label)

	inh := g.Edges[1]
	assert.Equal(t, "item->base#0", inh.ID, "id derived as written, before the swap")
	assert.Equal(t, "base", inh.From, "supertype becomes the source")
	assert.Equal(t, "item", inh.To)
	assert.Equal(t, "super", inh.FromCard, "cards swap with the ends")
	assert.Equal(t, "sub", inh.ToCard)
	assert.Equal(t, model.RelationInheritance, inh.Relation)
	assert.False(t, inh.Directed)

	dep := g.Edges[2]
	assert.Equal(t, "order->item#1", dep.ID, "derived ids count explicit ids of the pair")
	assert.Equal(t, model.RelationDependency, dep.Relation)
	assert.Equal(t, model.EdgeDashed, dep.Style)
	assert.True(t, dep.Directed, "dependency is directed by default")

	assoc := g.Edges[3]
	assert.Equal(t, "order->item#2", assoc.ID)
	assert.Equal(t, model.RelationAssociation, assoc.Relation, "absent kind is association")
	assert.False(t, assoc.Directed, "association is undirected by default")

	self := g.Edges[4]
	assert.True(t, self.Directed)
	assert.Equal(t, "red", self.Color)

	require.Len(t, g.Groups, 1)
	assert.Equal(t, "domain", g.Groups[0].ID)
	assert.Equal(t, []string{"order", "item"}, g.Groups[0].Contains)
	assert.Equal(t, 1, g.Groups[0].Depth)
	require.NotNil(t, g.Legend)
	assert.Equal(t, map[string]string{"inheritance": "is-a", "dependency": ""}, g.Legend.Labels)
}

func TestParseClass_NoLegendWhenAbsent(t *testing.T) {
	g, err := ParseClass([]byte(`{"type":"class","classes":[{"id":"a"}]}`))
	require.NoError(t, err)
	assert.Nil(t, g.Legend)
	assert.Equal(t, model.Down, g.Direction, "absent direction is DOWN, not AUTO")
	require.NotNil(t, g.Nodes[0].Members, "every class node carries Members, even when empty")
}

func TestParseClass_Direction(t *testing.T) {
	g, err := ParseClass([]byte(`{"type":"class","direction":"RIGHT","classes":[{"id":"a"}]}`))
	require.NoError(t, err)
	assert.Equal(t, model.Right, g.Direction)
	_, err = ParseClass([]byte(`{"type":"class","direction":"SIDEWAYS","classes":[{"id":"a"}]}`))
	f := fields(t, err)
	assert.Contains(t, f["direction"], "invalid direction")
}

func TestParseClass_Errors(t *testing.T) {
	cases := []struct{ name, spec, field, msg string }{
		{"type", `{"type":"flow","classes":[{"id":"a"}]}`, "type", `must be "class", got "flow"`},
		{"no classes", `{"type":"class"}`, "classes", "must have at least one class"},
		{"empty id", `{"type":"class","classes":[{"id":""}]}`, "classes[0].id", "must not be empty"},
		{"dup id", `{"type":"class","classes":[{"id":"a"},{"id":"a"}]}`, "classes[1].id", `duplicate class id "a"`},
		{"type param", `{"type":"class","classes":[{"id":"a","type_params":["T",""]}]}`, "classes[0].type_params[1]", "must not be empty"},
		{"color", `{"type":"class","classes":[{"id":"a","color":"mauve"}]}`, "classes[0].color", "unknown color"},
		{"attr text", `{"type":"class","classes":[{"id":"a","attributes":[{"text":""}]}]}`, "classes[0].attributes[0].text", "must not be empty"},
		{"method text", `{"type":"class","classes":[{"id":"a","methods":[{"visibility":"+"}]}]}`, "classes[0].methods[0].text", "must not be empty"},
		{"visibility", `{"type":"class","classes":[{"id":"a","attributes":[{"visibility":"*","text":"x"}]}]}`, "classes[0].attributes[0].visibility", "must be one of +, -, #, ~"},
		{"rel empty id", `{"type":"class","classes":[{"id":"a"}],"relations":[{"id":"","from":"a","to":"a"}]}`, "relations[0].id", "must not be empty when present"},
		{"rel dup id", `{"type":"class","classes":[{"id":"a"}],"relations":[{"id":"r","from":"a","to":"a"},{"id":"r","from":"a","to":"a"}]}`, "relations[1].id", `duplicate relation id "r" (also relations[0])`},
		{"rel id collides", `{"type":"class","classes":[{"id":"a"}],"relations":[{"id":"a->a#1","from":"a","to":"a"},{"from":"a","to":"a"}]}`, "relations[0].id", `"a->a#1" collides with the derived id of relations[1]`},
		{"rel from empty", `{"type":"class","classes":[{"id":"a"}],"relations":[{"from":"","to":"a"}]}`, "relations[0].from", "must not be empty"},
		{"rel to unknown", `{"type":"class","classes":[{"id":"a"}],"relations":[{"from":"a","to":"zz"}]}`, "relations[0].to", `references unknown class id "zz"`},
		{"rel kind", `{"type":"class","classes":[{"id":"a"}],"relations":[{"from":"a","to":"a","kind":"friendship"}]}`, "relations[0].kind", `unknown relation kind "friendship"`},
		{"directed on inheritance", `{"type":"class","classes":[{"id":"a"},{"id":"b"}],"relations":[{"from":"a","to":"b","kind":"inheritance","directed":true}]}`, "relations[0].directed", `not allowed for kind "inheritance": the adornment replaces the arrowhead`},
		{"directed false on composition", `{"type":"class","classes":[{"id":"a"},{"id":"b"}],"relations":[{"from":"a","to":"b","kind":"composition","directed":false}]}`, "relations[0].directed", `not allowed for kind "composition": the adornment replaces the arrowhead`},
		{"self inheritance", `{"type":"class","classes":[{"id":"a"}],"relations":[{"from":"a","to":"a","kind":"realization"}]}`, "relations[0]", "a class cannot inherit from or realize itself"},
		{"rel color", `{"type":"class","classes":[{"id":"a"}],"relations":[{"from":"a","to":"a","color":"#12"}]}`, "relations[0].color", "unknown color"},
		{"package label", `{"type":"class","classes":[{"id":"a"}],"packages":[{"id":"p","contains":["a"]}]}`, "packages[0].label", "must not be empty"},
		{"package ref", `{"type":"class","classes":[{"id":"a"}],"packages":[{"id":"p","label":"P","contains":["zz"]}]}`, "packages[0].contains[0]", "neither a valid node nor a valid group"},
		{"package twice", `{"type":"class","classes":[{"id":"a"}],"packages":[{"id":"p","label":"P","contains":["a"]},{"id":"q","label":"Q","contains":["a"]}]}`, "packages[1].contains[0]", `already belongs to group "p"`},
		{"legend label kind", `{"type":"class","classes":[{"id":"a"}],"legend_labels":{"friendship":"x"}}`, "legend_labels.friendship", `unknown relation kind "friendship"`},
		{"ignore", `{"type":"class","classes":[{"id":"a"}],"ignore":["nope"]}`, "ignore[0]", "unknown advisory rule"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseClass([]byte(c.spec))
			f := fields(t, err)
			assert.Contains(t, f[c.field], c.msg, "fields: %v", f)
		})
	}
}

func TestParseClass_SelfAssociationAllowed(t *testing.T) {
	g, err := ParseClass([]byte(`{"type":"class","classes":[{"id":"a"}],"relations":[{"from":"a","to":"a","kind":"aggregation"}]}`))
	require.NoError(t, err)
	assert.Equal(t, "a", g.Edges[0].From)
}

func TestParseClass_PackagesNestAndLimit(t *testing.T) {
	spec := `{"type":"class","classes":[{"id":"a"}],"packages":[
	  {"id":"p1","label":"1","contains":["p2"]},{"id":"p2","label":"2","contains":["p3"]},
	  {"id":"p3","label":"3","contains":["p4"]},{"id":"p4","label":"4","contains":["a"]}]}`
	_, err := ParseClass([]byte(spec))
	f := fields(t, err)
	assert.Contains(t, f["packages"], "exceeds maximum nesting depth of 3")
}

func TestDerivePairIDs(t *testing.T) {
	r := "r"
	ids := derivePairIDs([]*string{nil, &r, nil}, []string{"a", "a", "a"}, []string{"b", "b", "b"})
	assert.Equal(t, []string{"a->b#0", "r", "a->b#2"}, ids)
}

func TestParseFlow_StillDerivesIDs(t *testing.T) {
	g, err := ParseFlow([]byte(`{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":[{"from":"a","to":"b"},{"id":"x","from":"a","to":"b"},{"from":"a","to":"b"}]}`))
	require.NoError(t, err)
	assert.Equal(t, "a->b#0", g.Edges[0].ID)
	assert.Equal(t, "x", g.Edges[1].ID)
	assert.Equal(t, "a->b#2", g.Edges[2].ID)
}

func TestParseClass_Title(t *testing.T) {
	g, err := ParseClass([]byte(`{"type":"class","title":"Domain","classes":[{"id":"a"}]}`))
	require.NoError(t, err)
	assert.Equal(t, "Domain", g.Title)
}
