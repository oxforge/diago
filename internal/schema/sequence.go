package schema

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/oxforge/diago/internal/model"
)

// SequenceSpec is the JSON-tagged input struct for sequence diagram specs.
type SequenceSpec struct {
	Type         string            `json:"type"`
	Title        string            `json:"title,omitempty" jsonschema:"description=Diagram title\\, rendered as a band above the diagram."`
	Actors       []ActorSpec       `json:"actors"`
	Interactions []InteractionSpec `json:"interactions"`
	Fragments    []FragmentSpec    `json:"fragments,omitempty"`
	Activations  *bool             `json:"activations,omitempty"` // default true
	Theme        string            `json:"theme,omitempty" jsonschema:"enum=default,enum=dark,enum=midnight,enum=sketch,description=Theme the render uses unless the -theme flag overrides it. Defaults to the default theme."`
	Ignore       []string          `json:"ignore,omitempty" jsonschema:"description=Advisory rules to silence for this spec (see diago check). Each entry must be a rule name."`
}

// ActorSpec is the JSON representation of a sequence diagram actor.
type ActorSpec struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Color string `json:"color,omitempty"`
}

// InteractionSpec is the JSON representation of a sequence diagram interaction.
type InteractionSpec struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Label string `json:"label,omitempty"`
	Style string `json:"style,omitempty" jsonschema:"enum=solid,enum=dashed,enum=async"` // solid (default), dashed, async
	Color string `json:"color,omitempty"`
}

// FragmentSpec is the JSON representation of a sequence diagram fragment.
type FragmentSpec struct {
	Type     string                `json:"type" jsonschema:"enum=alt,enum=opt,enum=loop,enum=par,enum=break"`
	Over     []string              `json:"over"`
	Sections []FragmentSectionSpec `json:"sections"`
}

// FragmentSectionSpec is the JSON representation of one section within a fragment.
type FragmentSectionSpec struct {
	Label string `json:"label"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

// ParseSequence parses and validates a JSON sequence diagram spec.
// Returns a model.SequenceDiagram on success, or ValidationErrors if the spec is invalid.
// Multiple validation errors are collected and returned together.
func ParseSequence(data []byte) (*model.SequenceDiagram, error) {
	var spec SequenceSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("parse sequence: %w", err)
	}
	// The keys each section gives: decoded into an int, a missing start or
	// end would read as 0. The decode above succeeded, so this one does.
	var given struct {
		Fragments []struct {
			Sections []map[string]json.RawMessage `json:"sections"`
		} `json:"fragments"`
	}
	_ = json.Unmarshal(data, &given)

	var errs ValidationErrors

	// Validate type field.
	if spec.Type != "sequence" {
		errs = append(errs, ValidationError{
			Field:   "type",
			Message: fmt.Sprintf("must be %q, got %q", "sequence", spec.Type),
		})
	}

	// Validate actors.
	if len(spec.Actors) == 0 {
		errs = append(errs, ValidationError{
			Field:   "actors",
			Message: "must have at least one actor",
		})
	}

	actorIDs := make(map[string]bool, len(spec.Actors))
	actors := make([]model.Actor, 0, len(spec.Actors))
	for i, a := range spec.Actors {
		field := fmt.Sprintf("actors[%d]", i)
		if a.ID == "" {
			errs = append(errs, ValidationError{
				Field:   field + ".id",
				Message: "must not be empty",
			})
		} else if actorIDs[a.ID] {
			errs = append(errs, ValidationError{
				Field:   field + ".id",
				Message: fmt.Sprintf("duplicate actor id %q", a.ID),
			})
		} else {
			actorIDs[a.ID] = true
		}

		if a.Label == "" {
			errs = append(errs, ValidationError{
				Field:   field + ".label",
				Message: "must not be empty",
			})
		}

		if err := validateColor(field+".color", a.Color); err != nil {
			errs = append(errs, *err)
		}

		actors = append(actors, model.Actor{
			ID:    a.ID,
			Label: a.Label,
			Color: a.Color,
		})
	}

	// Validate interactions.
	interactions := make([]model.Interaction, 0, len(spec.Interactions))
	for i, it := range spec.Interactions {
		field := fmt.Sprintf("interactions[%d]", i)

		if it.From == "" {
			errs = append(errs, ValidationError{
				Field:   field + ".from",
				Message: "must not be empty",
			})
		} else if !actorIDs[it.From] {
			errs = append(errs, ValidationError{
				Field:   field + ".from",
				Message: fmt.Sprintf("references unknown actor id %q", it.From),
			})
		}

		if it.To == "" {
			errs = append(errs, ValidationError{
				Field:   field + ".to",
				Message: "must not be empty",
			})
		} else if !actorIDs[it.To] {
			errs = append(errs, ValidationError{
				Field:   field + ".to",
				Message: fmt.Sprintf("references unknown actor id %q", it.To),
			})
		}

		style, styleErr := model.ParseInteractionStyle(it.Style)
		if styleErr != nil {
			errs = append(errs, ValidationError{
				Field:   field + ".style",
				Message: fmt.Sprintf("unknown interaction style %q", it.Style),
			})
			style = model.InteractionSolid
		}

		if err := validateColor(field+".color", it.Color); err != nil {
			errs = append(errs, *err)
		}

		interactions = append(interactions, model.Interaction{
			From:  it.From,
			To:    it.To,
			Label: it.Label,
			Style: style,
			Color: it.Color,
		})
	}

	// Validate fragments.
	interactionCount := len(spec.Interactions)
	fragments := make([]model.Fragment, 0, len(spec.Fragments))
	for i, f := range spec.Fragments {
		field := fmt.Sprintf("fragments[%d]", i)

		fragType, typeErr := model.ParseFragmentType(f.Type)
		if typeErr != nil {
			errs = append(errs, ValidationError{
				Field:   field + ".type",
				Message: fmt.Sprintf("unknown fragment type %q", f.Type),
			})
			fragType = model.FragmentAlt
		}

		if len(f.Over) == 0 {
			errs = append(errs, ValidationError{
				Field:   field + ".over",
				Message: "must reference at least one actor",
			})
		}
		for j, actorID := range f.Over {
			if !actorIDs[actorID] {
				errs = append(errs, ValidationError{
					Field:   fmt.Sprintf("%s.over[%d]", field, j),
					Message: fmt.Sprintf("references unknown actor id %q", actorID),
				})
			}
		}

		if len(f.Sections) == 0 {
			errs = append(errs, ValidationError{
				Field:   field + ".sections",
				Message: "must have at least one section",
			})
		}

		sections := make([]model.FragmentSection, 0, len(f.Sections))
		// Track interaction index ranges to detect overlaps.
		type indexRange struct{ start, end int }
		usedRanges := make([]indexRange, 0, len(f.Sections))

		for j, sec := range f.Sections {
			secField := fmt.Sprintf("%s.sections[%d]", field, j)
			missing := false
			for _, bound := range []string{"start", "end"} {
				if _, ok := given.Fragments[i].Sections[j][bound]; !ok {
					errs = append(errs, ValidationError{Field: secField + "." + bound, Message: "must be given"})
					missing = true
				}
			}
			if missing {
				continue
			}

			if interactionCount == 0 {
				errs = append(errs, ValidationError{
					Field:   secField,
					Message: "fragment sections require at least one interaction",
				})
			} else {
				if sec.Start < 0 || sec.Start >= interactionCount {
					errs = append(errs, ValidationError{
						Field:   secField + ".start",
						Message: fmt.Sprintf("interaction index %d is out of bounds (0..%d)", sec.Start, interactionCount-1),
					})
				}
				if sec.End < 0 || sec.End >= interactionCount {
					errs = append(errs, ValidationError{
						Field:   secField + ".end",
						Message: fmt.Sprintf("interaction index %d is out of bounds (0..%d)", sec.End, interactionCount-1),
					})
				}
			}
			if sec.Start > sec.End {
				errs = append(errs, ValidationError{
					Field:   secField + ".start",
					Message: fmt.Sprintf("start %d must be <= end %d", sec.Start, sec.End),
				})
			}

			// Check for overlaps with already-validated sections.
			for _, r := range usedRanges {
				if sec.Start <= r.end && sec.End >= r.start {
					errs = append(errs, ValidationError{
						Field:   secField,
						Message: fmt.Sprintf("section range [%d..%d] overlaps with another section", sec.Start, sec.End),
					})
					break
				}
			}
			usedRanges = append(usedRanges, indexRange{sec.Start, sec.End})

			sections = append(sections, model.FragmentSection{
				Label: sec.Label,
				Start: sec.Start,
				End:   sec.End,
			})
		}

		fragments = append(fragments, model.Fragment{
			Type:     fragType,
			Over:     f.Over,
			Sections: sections,
		})
	}

	errs = append(errs, validateIgnore(spec.Ignore)...)

	if len(errs) > 0 {
		return nil, errs
	}

	activations := true
	if spec.Activations != nil {
		activations = *spec.Activations
	}

	return &model.SequenceDiagram{
		Title:        strings.TrimSpace(spec.Title),
		Actors:       actors,
		Interactions: interactions,
		Fragments:    fragments,
		Activations:  activations,
	}, nil
}
