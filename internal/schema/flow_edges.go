package schema

import (
	"fmt"

	"github.com/oxforge/diago/internal/model"
)

// derivePairIDs returns each pair's effective id: the explicit id when
// present, else the derived from->to#n form, where n counts every pair
// with the same endpoints in spec order (explicit ids included, so naming
// one never renumbers its neighbours). It is the one implementation of the
// id contract shared by flow edges and class relations, by validation and
// by the advisory checker.
func derivePairIDs(explicit []*string, from, to []string) []string {
	ids := make([]string, len(from))
	pairCount := make(map[[2]string]int)
	for i := range from {
		n := pairCount[[2]string{from[i], to[i]}]
		pairCount[[2]string{from[i], to[i]}]++
		if explicit[i] != nil && *explicit[i] != "" {
			ids[i] = *explicit[i]
			continue
		}
		ids[i] = fmt.Sprintf("%s->%s#%d", from[i], to[i], n)
	}
	return ids
}

// deriveEdgeIDs is derivePairIDs over flow edge specs.
func deriveEdgeIDs(edges []EdgeSpec) []string {
	explicit := make([]*string, len(edges))
	from := make([]string, len(edges))
	to := make([]string, len(edges))
	for i, e := range edges {
		explicit[i], from[i], to[i] = e.ID, e.From, e.To
	}
	return derivePairIDs(explicit, from, to)
}

// validatePairIDs applies the explicit-id rules (present-but-empty,
// duplicate, collision with a derived id) and returns the effective ids.
// field is the array name ("edges", "relations"); noun names the element in
// messages ("edge", "relation").
func validatePairIDs(explicit []*string, from, to []string, field, noun string) ([]string, ValidationErrors) {
	var errs ValidationErrors
	seen := make(map[string]int, len(explicit)) // id → index of the element that owns it
	for i, id := range explicit {
		if id == nil {
			continue
		}
		f := fmt.Sprintf("%s[%d].id", field, i)
		if *id == "" {
			errs = append(errs, ValidationError{Field: f, Message: "must not be empty when present"})
			continue
		}
		if j, dup := seen[*id]; dup {
			errs = append(errs, ValidationError{Field: f,
				Message: fmt.Sprintf("duplicate %s id %q (also %s[%d])", noun, *id, field, j)})
			continue
		}
		seen[*id] = i
	}
	ids := derivePairIDs(explicit, from, to)
	for i := range ids {
		if explicit[i] != nil && *explicit[i] != "" {
			continue
		}
		if j, taken := seen[ids[i]]; taken {
			errs = append(errs, ValidationError{Field: fmt.Sprintf("%s[%d].id", field, j),
				Message: fmt.Sprintf("%q collides with the derived id of %s[%d]", ids[i], field, i)})
		}
	}
	return ids, errs
}

// validateEdges validates edge specs against known node IDs and returns model edges and any validation errors.
func validateEdges(specs []EdgeSpec, nodeIDs map[string]bool) ([]model.Edge, ValidationErrors) {
	explicit := make([]*string, len(specs))
	from := make([]string, len(specs))
	to := make([]string, len(specs))
	for i, e := range specs {
		explicit[i], from[i], to[i] = e.ID, e.From, e.To
	}
	ids, errs := validatePairIDs(explicit, from, to, "edges", "edge")

	edges := make([]model.Edge, 0, len(specs))
	for i, e := range specs {
		field := fmt.Sprintf("edges[%d]", i)

		if e.From == "" {
			errs = append(errs, ValidationError{
				Field:   field + ".from",
				Message: "must not be empty",
			})
		} else if !nodeIDs[e.From] {
			errs = append(errs, ValidationError{
				Field:   field + ".from",
				Message: fmt.Sprintf("references unknown node id %q", e.From),
			})
		}

		if e.To == "" {
			errs = append(errs, ValidationError{
				Field:   field + ".to",
				Message: "must not be empty",
			})
		} else if !nodeIDs[e.To] {
			errs = append(errs, ValidationError{
				Field:   field + ".to",
				Message: fmt.Sprintf("references unknown node id %q", e.To),
			})
		}

		edgeStyle, styleErr := model.ParseEdgeStyle(e.Style)
		if styleErr != nil {
			errs = append(errs, ValidationError{
				Field:   field + ".style",
				Message: fmt.Sprintf("unknown edge style %q", e.Style),
			})
			edgeStyle = model.EdgeSolid
		}

		edgeDir, dirErr := model.ParseEdgeDirection(e.Direction)
		if dirErr != nil {
			errs = append(errs, ValidationError{
				Field:   field + ".direction",
				Message: fmt.Sprintf("unknown edge direction %q", e.Direction),
			})
			edgeDir = model.EdgeForward
		}

		if err := validateColor(field+".color", e.Color); err != nil {
			errs = append(errs, *err)
		}

		if e.Flat && e.From != "" && e.From == e.To {
			errs = append(errs, ValidationError{
				Field:   field + ".flat",
				Message: "a flat edge cannot be a self-loop",
			})
		}

		edges = append(edges, model.Edge{
			ID:        ids[i],
			From:      e.From,
			To:        e.To,
			Label:     e.Label,
			Style:     edgeStyle,
			Direction: edgeDir,
			Color:     e.Color,
			Flat:      e.Flat,
		})
	}

	return edges, errs
}
