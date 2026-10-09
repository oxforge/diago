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
	if err := decode(data, &spec); err != nil {
		return nil, err
	}

	var errs ValidationErrors

	// Validate type field.
	if spec.Type != "sequence" {
		errs = append(errs, ValidationError{
			Field:   "type",
			Message: fmt.Sprintf("must be %q, got %q", "sequence", spec.Type),
		})
	}

	actors, actorIDs, actorErrs := validateActors(spec.Actors)
	errs = append(errs, actorErrs...)

	interactions, interactionErrs := validateInteractions(spec.Interactions, actorIDs)
	errs = append(errs, interactionErrs...)

	fragments, fragmentErrs := validateFragments(spec.Fragments, sectionKeys(data), actorIDs, len(spec.Interactions))
	errs = append(errs, fragmentErrs...)

	errs = append(errs, validateIgnore(spec.Ignore)...)

	if len(errs) > 0 {
		return nil, errs
	}

	activations := true
	if spec.Activations != nil {
		activations = *spec.Activations
	}

	return &model.SequenceDiagram{
		Title:        singleLine(strings.TrimSpace(spec.Title)),
		Actors:       actors,
		Interactions: interactions,
		Fragments:    fragments,
		Activations:  activations,
	}, nil
}

// sectionKeys returns the keys each fragment section in data gives, by
// fragment and then section index: decoded into an int, a missing start or
// end would read as 0. ParseSequence calls it on data it decoded, so this
// decode succeeds too.
func sectionKeys(data []byte) [][]map[string]json.RawMessage {
	var given struct {
		Fragments []struct {
			Sections []map[string]json.RawMessage `json:"sections"`
		} `json:"fragments"`
	}
	_ = json.Unmarshal(data, &given)
	keys := make([][]map[string]json.RawMessage, len(given.Fragments))
	for i, f := range given.Fragments {
		keys[i] = f.Sections
	}
	return keys
}

// validateActors validates the actors and returns them with the set of
// their ids.
func validateActors(specs []ActorSpec) ([]model.Actor, map[string]bool, ValidationErrors) {
	var errs ValidationErrors
	if len(specs) == 0 {
		errs = append(errs, ValidationError{
			Field:   "actors",
			Message: "must have at least one actor",
		})
	}

	actorIDs := make(map[string]bool, len(specs))
	actors := make([]model.Actor, 0, len(specs))
	for i, a := range specs {
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
		if msg := idProblem(a.ID, false); a.ID != "" && msg != "" {
			errs = append(errs, ValidationError{Field: field + ".id", Message: msg})
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
			Label: singleLine(a.Label),
			Color: a.Color,
		})
	}
	return actors, actorIDs, errs
}

// validateInteractions validates the interactions against the actor ids
// and returns them, an unknown style read as solid.
func validateInteractions(specs []InteractionSpec, actorIDs map[string]bool) ([]model.Interaction, ValidationErrors) {
	var errs ValidationErrors
	interactions := make([]model.Interaction, 0, len(specs))
	for i, it := range specs {
		field := fmt.Sprintf("interactions[%d]", i)

		for _, end := range [...]struct{ key, id string }{{"from", it.From}, {"to", it.To}} {
			if end.id == "" {
				errs = append(errs, ValidationError{
					Field:   field + "." + end.key,
					Message: "must not be empty",
				})
			} else if !actorIDs[end.id] {
				errs = append(errs, ValidationError{
					Field:   field + "." + end.key,
					Message: fmt.Sprintf("references unknown actor id %q", end.id),
				})
			}
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
			Label: singleLine(it.Label),
			Style: style,
			Color: it.Color,
		})
	}
	return interactions, errs
}

// validateFragments validates the fragments against the actor ids, the
// keys each section gives (sectionKeys) and the number of interactions,
// and returns them, an unknown type read as alt. Fragments nest or stay
// apart: two whose spans cross would draw one frame through the other.
func validateFragments(specs []FragmentSpec, keys [][]map[string]json.RawMessage, actorIDs map[string]bool, interactionCount int) ([]model.Fragment, ValidationErrors) {
	var errs ValidationErrors
	fragments := make([]model.Fragment, 0, len(specs))
	spans := make([][2]int, 0, len(specs))
	for i, f := range specs {
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

		sections, sectionErrs := validateSections(field, f.Sections, keys[i], interactionCount)
		errs = append(errs, sectionErrs...)

		fragments = append(fragments, model.Fragment{
			Type:     fragType,
			Over:     f.Over,
			Sections: sections,
		})
		span := [2]int{-1, -1}
		if len(sections) > 0 && len(sections) == len(f.Sections) {
			span = [2]int{sections[0].Start, sections[len(sections)-1].End}
		}
		spans = append(spans, span)
	}
	return fragments, append(errs, crossingFragments(spans)...)
}

// validateSections validates the sections of fragment field against the
// keys each gives and the number of interactions, and returns those that
// give both bounds.
func validateSections(field string, specs []FragmentSectionSpec, keys []map[string]json.RawMessage, interactionCount int) ([]model.FragmentSection, ValidationErrors) {
	var errs ValidationErrors
	sections := make([]model.FragmentSection, 0, len(specs))
	prevEnd := -1
	for j, sec := range specs {
		secField := fmt.Sprintf("%s.sections[%d]", field, j)
		missing := false
		for _, bound := range []string{"start", "end"} {
			if _, ok := keys[j][bound]; !ok {
				errs = append(errs, ValidationError{Field: secField + "." + bound, Message: "must be given"})
				missing = true
			}
		}
		if missing {
			continue
		}

		errs = append(errs, validateSection(secField, sec, interactionCount, prevEnd)...)
		prevEnd = max(prevEnd, sec.End)

		sections = append(sections, model.FragmentSection{
			Label: singleLine(sec.Label),
			Start: sec.Start,
			End:   sec.End,
		})
	}
	return sections, errs
}

// validateSection checks a section that gives both bounds: each is an
// interaction index, start is no later than end, and the section starts
// after prevEnd, where the sections before it end.
func validateSection(secField string, sec FragmentSectionSpec, interactionCount, prevEnd int) ValidationErrors {
	var errs ValidationErrors
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

	if sec.Start <= prevEnd {
		errs = append(errs, ValidationError{
			Field:   secField + ".start",
			Message: fmt.Sprintf("must come after the previous section's end (%d): sections run in order without overlapping", prevEnd),
		})
	}
	return errs
}

// crossingFragments reports every pair of fragments whose spans cross,
// under the later one. A fragment without a span ({-1, -1}: no sections,
// or one that did not give both bounds) crosses none.
func crossingFragments(spans [][2]int) ValidationErrors {
	var errs ValidationErrors
	for j, b := range spans {
		for i, a := range spans[:j] {
			if a[0] < 0 || b[0] < 0 {
				continue
			}
			if (a[0] < b[0] && b[0] <= a[1] && a[1] < b[1]) || (b[0] < a[0] && a[0] <= b[1] && b[1] < a[1]) {
				errs = append(errs, ValidationError{
					Field:   fmt.Sprintf("fragments[%d]", j),
					Message: fmt.Sprintf("spans interactions %d..%d, which cross fragments[%d]'s %d..%d: fragments must nest or stay apart", b[0], b[1], i, a[0], a[1]),
				})
			}
		}
	}
	return errs
}
