package schema_test

import (
	"errors"
	"testing"

	"github.com/oxforge/diago/internal/model"
	"github.com/oxforge/diago/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSequenceValid(t *testing.T) {
	input := []byte(`{
		"type": "sequence",
		"actors": [
			{"id": "client", "label": "Client"},
			{"id": "server", "label": "Server"}
		],
		"interactions": [
			{"from": "client", "to": "server", "label": "request", "style": "solid"},
			{"from": "server", "to": "client", "label": "response", "style": "dashed"}
		],
		"fragments": [
			{
				"type": "opt",
				"over": ["client", "server"],
				"sections": [{"label": "retry", "start": 0, "end": 1}]
			}
		]
	}`)
	got, err := schema.ParseSequence(input)
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Len(t, got.Actors, 2)
	assert.Equal(t, "client", got.Actors[0].ID)
	assert.Equal(t, "Client", got.Actors[0].Label)

	assert.Len(t, got.Interactions, 2)
	assert.Equal(t, "client", got.Interactions[0].From)
	assert.Equal(t, "server", got.Interactions[0].To)
	assert.Equal(t, "request", got.Interactions[0].Label)
	assert.Equal(t, model.InteractionSolid, got.Interactions[0].Style)
	assert.Equal(t, model.InteractionDashed, got.Interactions[1].Style)

	assert.Len(t, got.Fragments, 1)
	assert.Equal(t, model.FragmentOpt, got.Fragments[0].Type)
	assert.Equal(t, []string{"client", "server"}, got.Fragments[0].Over)
	assert.Len(t, got.Fragments[0].Sections, 1)
	assert.Equal(t, "retry", got.Fragments[0].Sections[0].Label)
	assert.Equal(t, 0, got.Fragments[0].Sections[0].Start)
	assert.Equal(t, 1, got.Fragments[0].Sections[0].End)
}

func TestParseSequenceMinimal(t *testing.T) {
	input := []byte(`{
		"type": "sequence",
		"actors": [{"id": "a", "label": "A"}],
		"interactions": [{"from": "a", "to": "a", "label": "ping"}]
	}`)
	got, err := schema.ParseSequence(input)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Len(t, got.Actors, 1)
	assert.Len(t, got.Interactions, 1)
	assert.Empty(t, got.Fragments)
	assert.True(t, got.Activations)
}

func TestParseSequenceSelfMessage(t *testing.T) {
	input := []byte(`{
		"type": "sequence",
		"actors": [
			{"id": "server", "label": "Server"}
		],
		"interactions": [
			{"from": "server", "to": "server", "label": "process()"}
		]
	}`)
	got, err := schema.ParseSequence(input)
	require.NoError(t, err)
	assert.Equal(t, "server", got.Interactions[0].From)
	assert.Equal(t, "server", got.Interactions[0].To)
}

func TestParseSequenceActivationsDefault(t *testing.T) {
	input := []byte(`{
		"type": "sequence",
		"actors": [{"id": "a", "label": "A"}],
		"interactions": []
	}`)
	got, err := schema.ParseSequence(input)
	require.NoError(t, err)
	assert.True(t, got.Activations)
}

func TestParseSequenceActivationsDisabled(t *testing.T) {
	input := []byte(`{
		"type": "sequence",
		"actors": [{"id": "a", "label": "A"}],
		"interactions": [],
		"activations": false
	}`)
	got, err := schema.ParseSequence(input)
	require.NoError(t, err)
	assert.False(t, got.Activations)
}

func TestParseSequenceEmptyInteractions(t *testing.T) {
	input := []byte(`{
		"type": "sequence",
		"actors": [
			{"id": "client", "label": "Client"},
			{"id": "server", "label": "Server"}
		],
		"interactions": []
	}`)
	got, err := schema.ParseSequence(input)
	require.NoError(t, err)
	assert.Empty(t, got.Interactions)
}

func TestParseSequenceInvalidType(t *testing.T) {
	input := []byte(`{
		"type": "flow",
		"actors": [{"id": "a", "label": "A"}],
		"interactions": []
	}`)
	_, err := schema.ParseSequence(input)
	require.Error(t, err)
	var ve schema.ValidationErrors
	require.True(t, errors.As(err, &ve))
	assert.Len(t, ve, 1)
	assert.Equal(t, "type", ve[0].Field)
}

func TestParseSequenceNoActors(t *testing.T) {
	input := []byte(`{
		"type": "sequence",
		"actors": [],
		"interactions": []
	}`)
	_, err := schema.ParseSequence(input)
	require.Error(t, err)
	var ve schema.ValidationErrors
	require.True(t, errors.As(err, &ve))
	assert.Contains(t, ve.Error(), "actors")
}

func TestParseSequenceBadActorRef(t *testing.T) {
	input := []byte(`{
		"type": "sequence",
		"actors": [{"id": "client", "label": "Client"}],
		"interactions": [
			{"from": "client", "to": "unknown_actor", "label": "msg"}
		]
	}`)
	_, err := schema.ParseSequence(input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown_actor")
}

func TestParseSequenceBadFragmentRange(t *testing.T) {
	input := []byte(`{
		"type": "sequence",
		"actors": [
			{"id": "a", "label": "A"},
			{"id": "b", "label": "B"}
		],
		"interactions": [
			{"from": "a", "to": "b", "label": "msg"}
		],
		"fragments": [
			{
				"type": "loop",
				"over": ["a", "b"],
				"sections": [{"label": "loop", "start": 1, "end": 0}]
			}
		]
	}`)
	_, err := schema.ParseSequence(input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "start")
}

func TestParseSequenceOutOfBoundsRange(t *testing.T) {
	input := []byte(`{
		"type": "sequence",
		"actors": [
			{"id": "a", "label": "A"},
			{"id": "b", "label": "B"}
		],
		"interactions": [
			{"from": "a", "to": "b", "label": "msg"}
		],
		"fragments": [
			{
				"type": "loop",
				"over": ["a", "b"],
				"sections": [{"label": "loop", "start": 0, "end": 5}]
			}
		]
	}`)
	_, err := schema.ParseSequence(input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "out of bounds")
}

func TestParseSequenceOverlappingSections(t *testing.T) {
	input := []byte(`{
		"type": "sequence",
		"actors": [
			{"id": "a", "label": "A"},
			{"id": "b", "label": "B"}
		],
		"interactions": [
			{"from": "a", "to": "b", "label": "msg1"},
			{"from": "b", "to": "a", "label": "msg2"},
			{"from": "a", "to": "b", "label": "msg3"}
		],
		"fragments": [
			{
				"type": "alt",
				"over": ["a", "b"],
				"sections": [
					{"label": "case1", "start": 0, "end": 1},
					{"label": "case2", "start": 1, "end": 2}
				]
			}
		]
	}`)
	_, err := schema.ParseSequence(input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "overlap")
}

func TestParseSequenceMultipleErrors(t *testing.T) {
	input := []byte(`{
		"type": "sequence",
		"actors": [],
		"interactions": [
			{"from": "unknown", "to": "also_unknown", "label": "msg", "style": "bad_style"}
		]
	}`)
	_, err := schema.ParseSequence(input)
	require.Error(t, err)
	var ve schema.ValidationErrors
	require.True(t, errors.As(err, &ve))
	// Should have: no actors + from ref + to ref + style errors
	assert.Greater(t, len(ve), 1)
}

func TestParseSequenceAsyncStyle(t *testing.T) {
	input := []byte(`{
		"type": "sequence",
		"actors": [
			{"id": "a", "label": "A"},
			{"id": "b", "label": "B"}
		],
		"interactions": [
			{"from": "a", "to": "b", "label": "fire and forget", "style": "async"}
		]
	}`)
	got, err := schema.ParseSequence(input)
	require.NoError(t, err)
	assert.Equal(t, model.InteractionAsync, got.Interactions[0].Style)
}

func TestParseSequenceStyleFieldIgnored(t *testing.T) {
	// Style field in JSON is silently ignored for sequence diagrams.
	input := []byte(`{"type":"sequence","style":"lawful","actors":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"interactions":[{"from":"a","to":"b","label":"msg"}]}`)
	d, err := schema.ParseSequence(input)
	require.NoError(t, err)
	assert.NotNil(t, d)
}

func TestParseSequence_ActorColor(t *testing.T) {
	spec := `{"type":"sequence","actors":[{"id":"a","label":"A","color":"green"}],"interactions":[]}`
	d, err := schema.ParseSequence([]byte(spec))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Actors[0].Color != "green" {
		t.Errorf("expected color %q, got %q", "green", d.Actors[0].Color)
	}
}

func TestParseSequence_InteractionColor(t *testing.T) {
	spec := `{"type":"sequence","actors":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"interactions":[{"from":"a","to":"b","color":"red"}]}`
	d, err := schema.ParseSequence([]byte(spec))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Interactions[0].Color != "red" {
		t.Errorf("expected color %q, got %q", "red", d.Interactions[0].Color)
	}
}

func TestParseSequenceAltMultipleSections(t *testing.T) {
	input := []byte(`{
		"type": "sequence",
		"actors": [
			{"id": "a", "label": "A"},
			{"id": "b", "label": "B"}
		],
		"interactions": [
			{"from": "a", "to": "b", "label": "msg1"},
			{"from": "b", "to": "a", "label": "msg2"},
			{"from": "a", "to": "b", "label": "msg3"},
			{"from": "b", "to": "a", "label": "msg4"}
		],
		"fragments": [
			{
				"type": "alt",
				"over": ["a", "b"],
				"sections": [
					{"label": "success", "start": 0, "end": 1},
					{"label": "failure", "start": 2, "end": 3}
				]
			}
		]
	}`)
	got, err := schema.ParseSequence(input)
	require.NoError(t, err)
	require.Len(t, got.Fragments, 1)
	assert.Equal(t, model.FragmentAlt, got.Fragments[0].Type)
	assert.Len(t, got.Fragments[0].Sections, 2)
	assert.Equal(t, "success", got.Fragments[0].Sections[0].Label)
	assert.Equal(t, "failure", got.Fragments[0].Sections[1].Label)
}

func TestParseSequence_Title(t *testing.T) {
	d, err := schema.ParseSequence([]byte(`{"type":"sequence","title":" Login ","actors":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"interactions":[{"from":"a","to":"b","label":"hi"}]}`))
	require.NoError(t, err)
	assert.Equal(t, "Login", d.Title)
}
