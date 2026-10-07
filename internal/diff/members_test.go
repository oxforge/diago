package diff

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/oxforge/diago/internal/model"
)

func mem(texts ...string) []model.Member {
	out := make([]model.Member, len(texts))
	for i, t := range texts {
		out[i] = model.Member{Text: t}
	}
	return out
}

func m(attrs, methods []model.Member) *model.Members {
	return &model.Members{Attributes: attrs, Methods: methods}
}

func texts(ms []model.Member) []string {
	out := make([]string, len(ms))
	for i, x := range ms {
		out[i] = x.Text
	}
	return out
}

func TestUnionMembers_Classifies(t *testing.T) {
	u := UnionMembers(
		m(mem("id: int", "legacy: str"), []model.Member{{Visibility: "+", Text: "go()"}}),
		m(mem("id: int"), []model.Member{{Visibility: "-", Text: "go()"}, {Text: "stop()"}}),
	)
	assert.Equal(t, []Status{Same, Removed}, u.AttrStatus)
	assert.Equal(t, []string{"id: int", "legacy: str"}, texts(u.Members.Attributes))
	assert.Equal(t, []Status{Changed, Added}, u.MethodStatus)
	assert.Equal(t, []MemberRecord{
		{Compartment: "attributes", Status: Removed, Member: model.Member{Text: "legacy: str"}},
		{Compartment: "methods", Status: Changed, Member: model.Member{Visibility: "-", Text: "go()"}},
		{Compartment: "methods", Status: Added, Member: model.Member{Text: "stop()"}},
	}, u.Records)
	assert.True(t, u.Changed)
}

func TestUnionMembers_RemovedAfterSurvivingPredecessor(t *testing.T) {
	u := UnionMembers(m(mem("a", "gone", "b"), nil), m(mem("a", "b"), nil))
	assert.Equal(t, []string{"a", "gone", "b"}, texts(u.Members.Attributes))
	assert.Equal(t, []Status{Same, Removed, Same}, u.AttrStatus)
}

func TestUnionMembers_RemovedWithoutPredecessorGoesFirst(t *testing.T) {
	u := UnionMembers(m(mem("gone", "a"), nil), m(mem("a"), nil))
	assert.Equal(t, []string{"gone", "a"}, texts(u.Members.Attributes))
}

func TestUnionMembers_TailRemoval(t *testing.T) {
	u := UnionMembers(m(mem("a", "gone"), nil), m(mem("a"), nil))
	assert.Equal(t, []string{"a", "gone"}, texts(u.Members.Attributes))
	assert.Equal(t, []Status{Same, Removed}, u.AttrStatus)
}

func TestUnionMembers_StereotypeOrTypeParamsChange(t *testing.T) {
	u := UnionMembers(&model.Members{Stereotype: "abstract"}, &model.Members{})
	assert.True(t, u.Changed)
	assert.Empty(t, u.Records)
	u = UnionMembers(&model.Members{TypeParams: []string{"T"}}, &model.Members{TypeParams: []string{"T", "U"}})
	assert.True(t, u.Changed)
	assert.Equal(t, []string{"T", "U"}, u.Members.TypeParams, "the union carries the after side")
	u = UnionMembers(&model.Members{Stereotype: "x", TypeParams: []string{"T"}}, &model.Members{Stereotype: "x", TypeParams: []string{"T"}})
	assert.False(t, u.Changed)
}

func TestUnionMembers_Identical(t *testing.T) {
	u := UnionMembers(m(mem("x"), nil), m(mem("x"), nil))
	assert.False(t, u.Changed)
	assert.Equal(t, []Status{Same}, u.AttrStatus)
	assert.Empty(t, u.Records)
}

func TestUnionMembers_DuplicateTextRemovalKept(t *testing.T) {
	u := UnionMembers(
		m([]model.Member{{Text: "x"}, {Text: "x", Visibility: "+"}}, nil),
		m([]model.Member{{Text: "x", Visibility: "+"}}, nil),
	)
	assert.True(t, u.Changed)
	assert.Equal(t, []Status{Changed, Removed}, u.AttrStatus)
	assert.Equal(t, []MemberRecord{
		{Compartment: "attributes", Status: Changed, Member: model.Member{Text: "x", Visibility: "+"}},
		{Compartment: "attributes", Status: Removed, Member: model.Member{Text: "x", Visibility: "+"}},
	}, u.Records)
}

func TestUnionMembers_OverloadsAllSame(t *testing.T) {
	u := UnionMembers(m(nil, mem("f(x)", "f(x)")), m(nil, mem("f(x)", "f(x)")))
	assert.False(t, u.Changed)
	assert.Equal(t, []Status{Same, Same}, u.MethodStatus)
}

func TestUnionMembers_SharedAnchorKeepsBeforeOrder(t *testing.T) {
	u := UnionMembers(m(mem("a", "gone1", "gone2", "b"), nil), m(mem("a", "b"), nil))
	assert.Equal(t, []string{"a", "gone1", "gone2", "b"}, texts(u.Members.Attributes))
	assert.Equal(t, []Status{Same, Removed, Removed, Same}, u.AttrStatus)
}

func TestUnionMembers_StaticAbstractChange(t *testing.T) {
	u := UnionMembers(m(nil, []model.Member{{Text: "f()", Static: true}}), m(nil, []model.Member{{Text: "f()"}}))
	assert.Equal(t, []Status{Changed}, u.MethodStatus)
	u = UnionMembers(m(nil, []model.Member{{Text: "f()"}}), m(nil, []model.Member{{Text: "f()", Abstract: true}}))
	assert.Equal(t, []Status{Changed}, u.MethodStatus)
}

func TestUnionMembers_AttributesBeforeMethodsInRecords(t *testing.T) {
	u := UnionMembers(m(mem("a"), mem("m")), m(mem("a", "b"), mem("m", "n")))
	assert.Equal(t, "attributes", u.Records[0].Compartment)
	assert.Equal(t, "methods", u.Records[1].Compartment)
}

func TestUnionMembers_EmptySides(t *testing.T) {
	u := UnionMembers(&model.Members{}, &model.Members{})
	assert.False(t, u.Changed)
	assert.Empty(t, u.AttrStatus)
	u = UnionMembers(&model.Members{}, m(mem("a"), nil))
	assert.Equal(t, []Status{Added}, u.AttrStatus)
}
