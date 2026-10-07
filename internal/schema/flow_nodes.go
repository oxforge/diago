package schema

import (
	"fmt"

	"github.com/oxforge/diago/internal/model"
)

// validateNodes validates node specs and returns model nodes, a set of node IDs, and any validation errors.
func validateNodes(specs []NodeSpec) ([]model.Node, map[string]bool, ValidationErrors) {
	var errs ValidationErrors

	if len(specs) == 0 {
		errs = append(errs, ValidationError{
			Field:   "nodes",
			Message: "must have at least one node",
		})
	}

	nodeIDs := make(map[string]bool)
	nodes := make([]model.Node, 0, len(specs))
	for i, n := range specs {
		field := fmt.Sprintf("nodes[%d]", i)
		if n.ID == "" {
			errs = append(errs, ValidationError{
				Field:   field + ".id",
				Message: "must not be empty",
			})
		} else if nodeIDs[n.ID] {
			errs = append(errs, ValidationError{
				Field:   field + ".id",
				Message: fmt.Sprintf("duplicate node id %q", n.ID),
			})
		} else {
			nodeIDs[n.ID] = true
		}

		if n.Label == "" {
			errs = append(errs, ValidationError{
				Field:   field + ".label",
				Message: "must not be empty",
			})
		}

		shape, shapeErr := model.ParseShape(n.Shape)
		if shapeErr != nil {
			errs = append(errs, ValidationError{
				Field:   field + ".shape",
				Message: fmt.Sprintf("unknown shape %q", n.Shape),
			})
			shape = model.ShapeRect
		}

		if err := validateColor(field+".color", n.Color); err != nil {
			errs = append(errs, *err)
		}

		nodes = append(nodes, model.Node{
			ID:    n.ID,
			Label: n.Label,
			Shape: shape,
			Color: n.Color,
		})
	}

	return nodes, nodeIDs, errs
}
