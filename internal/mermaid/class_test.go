package mermaid

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/oxforge/diago/internal/pipeline"
	"github.com/oxforge/diago/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const zooSrc = `---
title: Zoo
---
classDiagram
  direction LR
  class Animal~T~ {
    <<abstract>>
    -name: string
    +speak() string*
    +count() int$
  }
  Animal <|-- Dog
  Dog --|> Pet
  Owner "1" *-- "many" Dog : keeps
  Dog o-- Toy
  Dog ..> Food
  Vet -- Clinic
  Dog --> Leash
  Bowl <-- Dog
  Cat <|.. Feline
  Kennel --* Dog
  Pack --o Dog
  Groomer <.. Dog
`

func parseCls(t *testing.T, src string) (schema.ClassSpec, []Report) {
	t.Helper()
	res, err := Parse([]byte(src))
	require.NoError(t, err)
	require.Equal(t, KindClass, res.Kind)
	requireValid(t, res.Kind, res.JSON)
	var spec schema.ClassSpec
	require.NoError(t, json.Unmarshal(res.JSON, &spec))
	return spec, res.Reports
}

func rel(from, to, kind string) schema.RelationSpec {
	return schema.RelationSpec{From: from, To: to, Kind: kind}
}

func TestClass_Zoo(t *testing.T) {
	spec, reports := parseCls(t, zooSrc)
	assert.Empty(t, reports)
	// title, direction, class block with members
	assert.Equal(t, "class", spec.Type)
	assert.Equal(t, "Zoo", spec.Title)
	assert.Equal(t, "RIGHT", spec.Direction)
	require.Equal(t, "Animal", spec.Classes[0].ID)
	animal := spec.Classes[0]
	assert.Equal(t, []string{"T"}, animal.TypeParams)
	assert.Equal(t, "abstract", animal.Stereotype)
	assert.Equal(t, "", animal.Label, "label defaults to the id and is not written")
	assert.Equal(t, []schema.MemberSpec{{Visibility: "-", Text: "name: string"}}, animal.Attributes)
	assert.Equal(t, []schema.MemberSpec{
		{Visibility: "+", Text: "speak() string", Abstract: true},
		{Visibility: "+", Text: "count() int", Static: true},
	}, animal.Methods)

	// operators normalize to canonical orientation: from is the subtype,
	// the implementer, the owner or the dependent
	yes := true
	assert.Equal(t, []schema.RelationSpec{
		rel("Dog", "Animal", "inheritance"),
		rel("Dog", "Pet", "inheritance"),
		{From: "Owner", To: "Dog", Kind: "composition", FromCard: "1", ToCard: "many", Label: "keeps"},
		rel("Dog", "Toy", "aggregation"),
		rel("Dog", "Food", "dependency"),
		rel("Vet", "Clinic", "association"),
		{From: "Dog", To: "Leash", Kind: "association", Directed: &yes},
		{From: "Dog", To: "Bowl", Kind: "association", Directed: &yes},
		rel("Feline", "Cat", "realization"),
		rel("Dog", "Kennel", "composition"),
		rel("Dog", "Pack", "aggregation"),
		rel("Dog", "Groomer", "dependency"),
	}, spec.Relations)

	// relation endpoints exist as classes even when undeclared, in first-mention order
	ids := make([]string, 0, len(spec.Classes))
	for _, c := range spec.Classes {
		ids = append(ids, c.ID)
	}
	assert.Equal(t, []string{"Animal", "Dog", "Pet", "Owner", "Toy", "Food", "Vet", "Clinic", "Leash", "Bowl", "Cat", "Feline", "Kennel", "Pack", "Groomer"}, ids)
}

func TestClass_BareAndStandalone(t *testing.T) {
	// bare "class Name" declaration with no block; standalone "<<word>> Name"
	spec, _ := parseCls(t, "classDiagram\n  class Marker\n  <<interface>> Marker\n")
	require.Len(t, spec.Classes, 1)
	assert.Equal(t, "Marker", spec.Classes[0].ID)
	assert.Equal(t, "interface", spec.Classes[0].Stereotype)
	assert.Nil(t, spec.Classes[0].Attributes)
	assert.Nil(t, spec.Classes[0].Methods)

	// inline members, generics inside a member, every visibility
	spec, _ = parseCls(t, "classDiagram\n  Box : +items List~T~\n  Box : #count() int\n  Box : ~hidden\n  Box : plain()\n")
	assert.Equal(t, []schema.MemberSpec{{Visibility: "+", Text: "items List<T>"}, {Visibility: "~", Text: "hidden"}}, spec.Classes[0].Attributes)
	assert.Equal(t, []schema.MemberSpec{{Visibility: "#", Text: "count() int"}, {Text: "plain()"}}, spec.Classes[0].Methods)

	// the four directions
	for src, want := range map[string]string{"TB": "DOWN", "LR": "RIGHT", "BT": "UP", "RL": "LEFT"} {
		spec, _ = parseCls(t, "classDiagram\n  direction "+src+"\n  class A\n")
		assert.Equal(t, want, spec.Direction, src)
	}
	spec, _ = parseCls(t, "classDiagram\n  class A\n")
	assert.Equal(t, "", spec.Direction)
}

func TestClass_Errors(t *testing.T) {
	// unsupported constructs error with line number and name
	pe := errOf(t, "classDiagram\n  note \"hi\"\n")
	assert.Equal(t, 2, pe.Line)
	assert.Equal(t, `"note" is not supported by diago's class subset`, pe.Message)
	assert.Equal(t, `"namespace" is not supported by diago's class subset`, errOf(t, "classDiagram\nnamespace N {\n").Message)
	assert.Equal(t, `bidirectional arrows ("<-->") are not supported by diago's class subset`, errOf(t, "classDiagram\nA <--> B").Message)
	assert.Equal(t, `lollipop interfaces ("()--") are not supported by diago's class subset`, errOf(t, "classDiagram\nA ()-- B").Message)
	assert.Equal(t, `expected a class, relation, or member near "what"`, errOf(t, "classDiagram\nwhat").Message)
	pe = errOf(t, "classDiagram\nclass A {\n  +x\n")
	assert.Equal(t, `unclosed class block "A"`, pe.Message)
	assert.Equal(t, 4, pe.Line)
	pe = errOf(t, "classDiagram\nclass A {\n  +x\nclass B {\n  +y\n}\n")
	assert.Equal(t, `unclosed class block "A"`, pe.Message)
	assert.Equal(t, 4, pe.Line)
}

// The double-swap trap: the importer swaps once into the JSON's canonical
// orientation (from: subtype), and schema.ParseClass swaps once more for
// layout, so the supertype lands above the subtype in DOWN.
func TestClass_InheritanceRendersSupertypeAbove(t *testing.T) {
	res, err := Parse([]byte("classDiagram\n  Animal <|-- Dog\n"))
	require.NoError(t, err)
	var spec schema.ClassSpec
	require.NoError(t, json.Unmarshal(res.JSON, &spec))
	assert.Equal(t, []schema.RelationSpec{rel("Dog", "Animal", "inheritance")}, spec.Relations)
	pg, err := pipeline.LayoutClassForFormat(context.Background(), res.JSON, "svg", nil)
	require.NoError(t, err)
	var animalY, dogY float64
	for _, n := range pg.Nodes {
		switch n.ID {
		case "Animal":
			animalY = n.Y
		case "Dog":
			dogY = n.Y
		}
	}
	assert.Less(t, animalY, dogY, "supertype above subtype")
}
