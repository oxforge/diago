package schema

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/oxforge/diago/internal/model"
)

// FlowSpec is the JSON-tagged input struct for flow diagram specs.
// It is separate from the model types to keep JSON concerns out of the core model.
type FlowSpec struct {
	Type      string      `json:"type"`
	Title     string      `json:"title,omitempty" jsonschema:"description=Diagram title\\, rendered as a band above the diagram."`
	Theme     string      `json:"theme,omitempty" jsonschema:"enum=default,enum=dark,enum=midnight,enum=sketch,description=Theme the render uses unless the -theme flag overrides it. Defaults to the default theme."`
	Direction string      `json:"direction,omitempty" jsonschema:"enum=DOWN,enum=UP,enum=RIGHT,enum=LEFT,enum=AUTO,description=Layout direction. Defaults to AUTO (engine chooses based on graph shape)."`
	Nodes     []NodeSpec  `json:"nodes"`
	Edges     []EdgeSpec  `json:"edges"`
	Groups    []GroupSpec `json:"groups,omitempty"`
	Ignore    []string    `json:"ignore,omitempty" jsonschema:"description=Advisory rules to silence for this spec (see diago check). Each entry must be a rule name."`
}

// NodeSpec is the JSON representation of a node.
type NodeSpec struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Shape string `json:"shape,omitempty" jsonschema:"enum=rect,enum=rounded,enum=circle,enum=diamond,enum=cylinder,enum=hexagon,enum=parallelogram"`
	Color string `json:"color,omitempty"`
}

// EdgeSpec is the JSON representation of an edge.
type EdgeSpec struct {
	ID        *string `json:"id,omitempty" jsonschema:"description=Optional stable identifier. Keep ids stable across edits because diffs and layout anchoring key on them. Defaults to from->to#n where n is the edge's ordinal among edges with the same endpoints in spec order."`
	From      string  `json:"from"`
	To        string  `json:"to"`
	Label     string  `json:"label,omitempty"`
	Style     string  `json:"style,omitempty" jsonschema:"enum=solid,enum=dashed,enum=dotted,enum=thick"`
	Direction string  `json:"direction,omitempty" jsonschema:"enum=forward,enum=backward,enum=both,enum=none,description=Arrowhead direction. forward (default): arrow at target. backward: arrow at source. both: arrows at both ends. none: no arrowhead at either end."`
	Color     string  `json:"color,omitempty"`
	Flat      bool    `json:"flat,omitempty" jsonschema:"description=Optional. When true\\, this link does not imply order: its ends stay where the rest of the diagram puts them\\, side by side when it allows\\, and the edge is routed between them. Use it for links between peers\\, such as replication between two sites. Not allowed on a self-loop."`
}

// GroupSpec is the JSON representation of a group.
type GroupSpec struct {
	ID       string   `json:"id"`
	Label    string   `json:"label"`
	Contains []string `json:"contains" jsonschema:"description=Node IDs or group IDs belonging to this group. Referencing a group ID creates a nested child group (max 3 levels deep)."`
	Color    string   `json:"color,omitempty"`
}

// ParseFlow parses and validates a JSON flow diagram spec.
// Returns a model.Graph on success, or ValidationErrors if the spec is invalid.
// Multiple validation errors are collected and returned together.
func ParseFlow(data []byte) (*model.Graph, error) {
	var spec FlowSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("parse flow: %w", err)
	}

	var errs ValidationErrors

	// Validate type field
	if spec.Type != "flow" {
		errs = append(errs, ValidationError{
			Field:   "type",
			Message: fmt.Sprintf("must be %q, got %q", "flow", spec.Type),
		})
	}

	// Validate direction
	direction, dirErr := model.ParseDirection(spec.Direction)
	if dirErr != nil {
		errs = append(errs, ValidationError{
			Field:   "direction",
			Message: fmt.Sprintf("invalid direction %q; must be one of: DOWN, UP, RIGHT, LEFT, AUTO", spec.Direction),
		})
		direction = model.Auto
	}

	// Validate nodes
	nodes, nodeIDs, nodeErrs := validateNodes(spec.Nodes)
	errs = append(errs, nodeErrs...)

	// Validate edges
	edges, edgeErrs := validateEdges(spec.Edges, nodeIDs)
	errs = append(errs, edgeErrs...)

	// Validate groups
	groups, groupErrs := validateGroups(spec.Groups, nodeIDs)
	errs = append(errs, groupErrs...)

	errs = append(errs, validateIgnore(spec.Ignore)...)

	if len(errs) > 0 {
		return nil, errs
	}

	return &model.Graph{
		Title:     strings.TrimSpace(spec.Title),
		Direction: direction,
		Nodes:     nodes,
		Edges:     edges,
		Groups:    groups,
	}, nil
}
