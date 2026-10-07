package diff

import (
	"slices"

	"github.com/oxforge/diago/internal/model"
)

// MemberRecord is one non-same member of a class union, the footer's raw
// material: the after-side member for added and changed, the before-side
// member for removed.
type MemberRecord struct {
	Compartment string // "attributes" or "methods"
	Status      Status
	Member      model.Member
}

// MemberUnion is a changed class node's union member lists with a status
// per union member.
type MemberUnion struct {
	Members      model.Members
	AttrStatus   []Status // aligned with Members.Attributes
	MethodStatus []Status // aligned with Members.Methods
	Records      []MemberRecord
	Changed      bool
}

// sameMember reports whether two members are equal in every field.
func sameMember(a, b model.Member) bool {
	return a.Text == b.Text && a.Visibility == b.Visibility && a.Static == b.Static && a.Abstract == b.Abstract
}

// unionCompartment unions one compartment. Identity is Text alone but not
// unique: the k-th occurrence of a text in after pairs with the k-th
// occurrence of that text in before; an after occurrence beyond before's
// count is added, a before occurrence beyond after's is removed. Removed
// members are bucketed right after the after index their nearest paired
// before-predecessor mapped to (bucket 0 when none), keeping before order
// within a bucket.
func unionCompartment(compartment string, before, after []model.Member) ([]model.Member, []Status, []MemberRecord) {
	beforeByText := map[string][]int{}
	for i, bm := range before {
		beforeByText[bm.Text] = append(beforeByText[bm.Text], i)
	}
	used := map[string]int{}
	beforeToAfter := map[int]int{}
	afterStatus := make([]Status, len(after))
	for j, am := range after {
		k := used[am.Text]
		idxs := beforeByText[am.Text]
		if k >= len(idxs) {
			afterStatus[j] = Added
			continue
		}
		used[am.Text] = k + 1
		bi := idxs[k]
		beforeToAfter[bi] = j
		if sameMember(before[bi], am) {
			afterStatus[j] = Same
		} else {
			afterStatus[j] = Changed
		}
	}
	buckets := make([][]model.Member, len(after)+1)
	for i, bm := range before {
		if _, paired := beforeToAfter[i]; paired {
			continue
		}
		anchor := 0
		for k := i - 1; k >= 0; k-- {
			if aj, ok := beforeToAfter[k]; ok {
				anchor = aj + 1
				break
			}
		}
		buckets[anchor] = append(buckets[anchor], bm)
	}
	var union []model.Member
	var status []Status
	for j := 0; j <= len(after); j++ {
		for _, rm := range buckets[j] {
			union = append(union, rm)
			status = append(status, Removed)
		}
		if j < len(after) {
			union = append(union, after[j])
			status = append(status, afterStatus[j])
		}
	}
	var records []MemberRecord
	for i, s := range status {
		if s != Same {
			records = append(records, MemberRecord{Compartment: compartment, Status: s, Member: union[i]})
		}
	}
	return union, status, records
}

// UnionMembers unions two member sets; the union
// carries the after side's stereotype and type parameters.
func UnionMembers(before, after *model.Members) MemberUnion {
	attrs, attrStatus, attrRecords := unionCompartment("attributes", before.Attributes, after.Attributes)
	methods, methodStatus, methodRecords := unionCompartment("methods", before.Methods, after.Methods)
	u := MemberUnion{
		Members:      model.Members{Stereotype: after.Stereotype, TypeParams: after.TypeParams, Attributes: attrs, Methods: methods},
		AttrStatus:   attrStatus,
		MethodStatus: methodStatus,
		Records:      append(attrRecords, methodRecords...),
	}
	u.Changed = len(u.Records) > 0 || before.Stereotype != after.Stereotype || !slices.Equal(before.TypeParams, after.TypeParams)
	return u
}

// membersChanged is DiffGraphs' node rule: changed when exactly one side has
// members, or both do and their union changed.
func membersChanged(before, after *model.Members) bool {
	switch {
	case before == nil && after == nil:
		return false
	case before == nil || after == nil:
		return true
	}
	return UnionMembers(before, after).Changed
}

// memberText is the composed member line without mark or suffix: the
// footer's wording.
// Member-line composition exists in three places that must agree: this
// one, schema.memberLine (internal/schema/check_class.go) and
// size.classLines (internal/layout/layered/size/class.go). Change one,
// check the others.
func memberText(m model.Member) string {
	if m.Visibility == "" {
		return m.Text
	}
	return m.Visibility + " " + m.Text
}
