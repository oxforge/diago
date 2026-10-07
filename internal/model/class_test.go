package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelation_ParseStringRoundTrip(t *testing.T) {
	for _, s := range []string{"association", "inheritance", "realization", "dependency", "aggregation", "composition"} {
		r, err := ParseRelation(s)
		require.NoError(t, err, s)
		assert.Equal(t, s, r.String())
		assert.NotEqual(t, RelationNone, r)
	}
	r, err := ParseRelation("")
	require.NoError(t, err)
	assert.Equal(t, RelationNone, r)
	assert.Equal(t, "", RelationNone.String())
	_, err = ParseRelation("friendship")
	require.Error(t, err)
}

func TestRelation_JSONOmittedWhenNone(t *testing.T) {
	b, err := json.Marshal(PositionedEdge{ID: "e"})
	require.NoError(t, err)
	assert.NotContains(t, string(b), `"relation"`)
	assert.NotContains(t, string(b), `"directed"`)
	b, err = json.Marshal(PositionedEdge{ID: "e", Relation: RelationComposition, Directed: true})
	require.NoError(t, err)
	assert.Contains(t, string(b), `"relation":"composition"`)
	var pe PositionedEdge
	require.NoError(t, json.Unmarshal(b, &pe))
	assert.Equal(t, RelationComposition, pe.Relation)
	assert.True(t, pe.Directed)
}

func TestRelation_Helpers(t *testing.T) {
	assert.True(t, RelationInheritance.Adorned())
	assert.True(t, RelationRealization.Adorned())
	assert.True(t, RelationAggregation.Adorned())
	assert.True(t, RelationComposition.Adorned())
	assert.False(t, RelationAssociation.Adorned())
	assert.False(t, RelationDependency.Adorned())
	assert.False(t, RelationNone.Adorned())
	assert.True(t, RelationRealization.Dashed())
	assert.True(t, RelationDependency.Dashed())
	assert.False(t, RelationInheritance.Dashed())
	assert.True(t, RelationInheritance.Swapped())
	assert.True(t, RelationRealization.Swapped())
	assert.False(t, RelationComposition.Swapped())
}

func TestPositionedMembers_JSONRoundTrip(t *testing.T) {
	pn := PositionedNode{ID: "c", Members: &PositionedMembers{
		Header:           []PositionedLine{{Text: "«abstract»", Stereotype: true}, {Text: "Shape<T>", Italic: true}},
		Attributes:       []PositionedLine{{Text: "- id: string"}},
		Methods:          []PositionedLine{{Text: "+ area(): float", Italic: true}, {Text: "+ count(): int", Underline: true}},
		HeaderLineHeight: 17, MemberLineHeight: 15,
	}}
	b, err := json.Marshal(pn)
	require.NoError(t, err)
	var back PositionedNode
	require.NoError(t, json.Unmarshal(b, &back))
	assert.Equal(t, pn, back)
	plain, err := json.Marshal(PositionedNode{ID: "n"})
	require.NoError(t, err)
	assert.NotContains(t, string(plain), `"members"`)
}

func TestEndLabelAndLegend_JSONRoundTrip(t *testing.T) {
	pg := PositionedGraph{
		Edges:  []PositionedEdge{{ID: "r", FromCard: &EndLabel{Text: "1", Pos: &Point{X: 3, Y: 4}, Width: 8, Height: 16}, ToCard: &EndLabel{Text: "*", Unresolved: true}}},
		Legend: []LegendEntry{{Kind: "composition", Label: "composition"}},
	}
	b, err := json.Marshal(pg)
	require.NoError(t, err)
	var back PositionedGraph
	require.NoError(t, json.Unmarshal(b, &back))
	assert.Equal(t, pg.Edges[0].FromCard, back.Edges[0].FromCard)
	assert.True(t, back.Edges[0].ToCard.Unresolved)
	assert.Equal(t, pg.Legend, back.Legend)
	plain, err := json.Marshal(PositionedGraph{})
	require.NoError(t, err)
	assert.NotContains(t, string(plain), `"legend"`)
}
