package diff

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const loginV1 = `{"type":"sequence",
 "actors":[{"id":"user","label":"User"},{"id":"web","label":"Web App"},{"id":"auth","label":"Auth Service"}],
 "interactions":[
  {"from":"user","to":"web","label":"submit form"},
  {"from":"web","to":"auth","label":"check credentials"},
  {"from":"auth","to":"web","label":"ok","style":"dashed"},
  {"from":"web","to":"user","label":"welcome","style":"dashed"},
  {"from":"auth","to":"web","label":"denied","style":"dashed"},
  {"from":"web","to":"user"}],
 "fragments":[{"type":"alt","over":["user","web","auth"],"sections":[{"label":"valid","start":2,"end":3},{"label":"invalid","start":4,"end":5}]}]}`

const loginV2 = `{"type":"sequence",
 "actors":[{"id":"user","label":"User"},{"id":"web","label":"Web App"},{"id":"auth","label":"Auth Service"},{"id":"audit","label":"Audit Log"}],
 "interactions":[
  {"from":"user","to":"web","label":"submit credentials"},
  {"from":"web","to":"auth","label":"check credentials"},
  {"from":"auth","to":"web","label":"ok","style":"dashed"},
  {"from":"web","to":"audit","label":"record login","style":"async"},
  {"from":"web","to":"user","label":"welcome","style":"dashed"},
  {"from":"auth","to":"web","label":"denied","style":"dashed"},
  {"from":"web","to":"user","style":"async"},
  {"from":"auth","to":"web","label":"locked","style":"dashed"}],
 "fragments":[{"type":"alt","over":["user","web","auth"],"sections":[{"label":"valid","start":2,"end":4},{"label":"invalid","start":5,"end":6},{"label":"locked","start":7,"end":7}]}]}`

func TestStampSequenceLabels(t *testing.T) {
	u := BuildUnionSequence(parseSeq(t, loginV1), parseSeq(t, loginV2))
	d := StampSequenceLabels(u)
	assert.Equal(t, "+ Audit Log", d.Actors[3].Label)
	assert.Equal(t, "User", d.Actors[0].Label)
	assert.Equal(t, "~ submit credentials", d.Interactions[0].Label)
	assert.Equal(t, "check credentials", d.Interactions[1].Label)
	assert.Equal(t, "+ record login", d.Interactions[3].Label)
	assert.Equal(t, "~", d.Interactions[6].Label, "unlabeled changed message gets the bare marker")
	assert.Equal(t, "+ locked", d.Interactions[7].Label)
	secs := d.Fragments[0].Sections
	assert.Equal(t, "~ valid", secs[0].Label, "frame changed, first guard not marked on its own account")
	assert.Equal(t, "invalid", secs[1].Label)
	assert.Equal(t, "+ locked", secs[2].Label)
	assert.Equal(t, "submit credentials", u.Diagram.Interactions[0].Label, "the union itself is untouched")
}

func TestStampSequenceLabels_FrameDoesNotDoubleStamp(t *testing.T) {
	before := parseSeq(t, `{"type":"sequence","actors":[{"id":"u","label":"U"},{"id":"w","label":"W"}],
 "interactions":[{"from":"u","to":"w","label":"a"},{"from":"u","to":"w","label":"b"}]}`)
	after := parseSeq(t, `{"type":"sequence","actors":[{"id":"u","label":"U"},{"id":"w","label":"W"}],
 "interactions":[{"from":"u","to":"w","label":"a"},{"from":"u","to":"w","label":"b"}],
 "fragments":[{"type":"loop","over":["u","w"],"sections":[{"label":"retry","start":0,"end":1}]}]}`)
	d := StampSequenceLabels(BuildUnionSequence(before, after))
	assert.Equal(t, "+ retry", d.Fragments[0].Sections[0].Label, "an added section stamps itself; the added frame does not stamp again")
}

func TestSequenceLegend_OrderAndWording(t *testing.T) {
	u := BuildUnionSequence(parseSeq(t, loginV1), parseSeq(t, loginV2))
	assert.Equal(t, []string{
		"added: actor Audit Log",
		`added: message Web App -> Audit Log "record login"`,
		`added: message Auth Service -> Web App "locked"`,
		"added: section [locked] of alt",
		`changed: message User -> Web App: label "submit form" → "submit credentials"`,
		"changed: message Web App -> User: style solid → async",
	}, SequenceLegend(u))
}

func TestSequenceLegend_RemovedNamesBeforeLabels(t *testing.T) {
	before := parseSeq(t, `{"type":"sequence","actors":[{"id":"u","label":"User"},{"id":"m","label":"Mailer"}],
 "interactions":[{"from":"u","to":"m","label":"send"}],
 "fragments":[{"type":"opt","over":["u","m"],"sections":[{"label":"async","start":0,"end":0}]}]}`)
	after := parseSeq(t, `{"type":"sequence","actors":[{"id":"u","label":"Person"}],"interactions":[]}`)
	assert.Equal(t, []string{
		"removed: actor Mailer",
		`removed: message User -> Mailer "send"`,
		"removed: fragment opt",
		`changed: actor Person: label "User" → "Person"`,
	}, SequenceLegend(BuildUnionSequence(before, after)))
}

func TestSequenceLegend_EmptyWhenIdentical(t *testing.T) {
	assert.Empty(t, SequenceLegend(BuildUnionSequence(parseSeq(t, loginV1), parseSeq(t, loginV1))))
}

func TestSequenceLegend_LabelsWithNewlineAndQuote(t *testing.T) {
	const v1 = `{"type":"sequence","actors":[{"id":"a","label":"A\nB"},{"id":"b","label":"C"}],
	 "interactions":[{"from":"a","to":"b","label":"x"}]}`
	const v2 = `{"type":"sequence","actors":[{"id":"a","label":"A\nB","color":"red"},{"id":"b","label":"C"}],
	 "interactions":[{"from":"a","to":"b","label":"x"},{"from":"b","to":"a","label":"Say \"hi\""}]}`
	got := SequenceLegend(BuildUnionSequence(parseSeq(t, v1), parseSeq(t, v2)))
	assert.Equal(t, []string{
		`added: message C -> A B "Say \"hi\""`,
		`changed: actor A B: color none → red`,
	}, got)
}
