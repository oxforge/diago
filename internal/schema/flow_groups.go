package schema

import (
	"fmt"

	"github.com/oxforge/diago/internal/model"
)

// maxGroupNestingDepth is the maximum allowed depth of nested groups.
const maxGroupNestingDepth = 3

// groupMaps holds the maps built during group validation, used across its
// three internal phases (containment resolution, cycle detection, depth).
type groupMaps struct {
	groupIDs      map[string]bool
	nodeInGroup   map[string]string   // nodeID → groupID
	groupInGroup  map[string]string   // childGroupID → parentGroupID
	groupChildren map[string][]string // groupID → child group IDs
	groupContains map[string][]string // groupID → node IDs
}

// validateGroups validates flow groups; validateGroupsPrefixed is the same
// pass with the field prefix parameterised (class packages use "packages").
func validateGroups(specs []GroupSpec, nodeIDs map[string]bool) ([]model.Group, ValidationErrors) {
	return validateGroupsPrefixed(specs, nodeIDs, "groups")
}

// validateGroupsPrefixed validates group specs in three phases (ID validation, containment resolution,
// cycle detection/depth) and returns model groups and any validation errors.
func validateGroupsPrefixed(specs []GroupSpec, nodeIDs map[string]bool, prefix string) ([]model.Group, ValidationErrors) {
	var errs ValidationErrors

	gm := groupMaps{
		groupIDs:      make(map[string]bool),
		nodeInGroup:   make(map[string]string),
		groupInGroup:  make(map[string]string),
		groupChildren: make(map[string][]string),
		groupContains: make(map[string][]string),
	}

	// Phase 1: collect group IDs, check uniqueness and collisions.
	for i, g := range specs {
		field := fmt.Sprintf("%s[%d]", prefix, i)
		if g.ID == "" {
			errs = append(errs, ValidationError{Field: field + ".id", Message: "must not be empty"})
			continue
		}
		if gm.groupIDs[g.ID] {
			errs = append(errs, ValidationError{Field: field + ".id", Message: fmt.Sprintf("duplicate group id %q", g.ID)})
			continue
		}
		if nodeIDs[g.ID] {
			errs = append(errs, ValidationError{Field: field + ".id", Message: fmt.Sprintf("group id %q collides with node id", g.ID)})
			continue
		}
		if g.Label == "" {
			errs = append(errs, ValidationError{Field: field + ".label", Message: "must not be empty"})
		}
		if err := validateColor(field+".color", g.Color); err != nil {
			errs = append(errs, *err)
		}
		gm.groupIDs[g.ID] = true
	}

	// Phase 2: resolve contains entries, enforce single parent.
	for i, g := range specs {
		if g.ID == "" || !gm.groupIDs[g.ID] {
			continue
		}
		field := fmt.Sprintf("%s[%d]", prefix, i)
		for j, ref := range g.Contains {
			refField := fmt.Sprintf("%s.contains[%d]", field, j)
			if nodeIDs[ref] {
				if existingGroup, already := gm.nodeInGroup[ref]; already {
					errs = append(errs, ValidationError{
						Field:   refField,
						Message: fmt.Sprintf("node %q already belongs to group %q", ref, existingGroup),
					})
				} else {
					gm.nodeInGroup[ref] = g.ID
					gm.groupContains[g.ID] = append(gm.groupContains[g.ID], ref)
				}
			} else if gm.groupIDs[ref] {
				if existingParent, already := gm.groupInGroup[ref]; already {
					errs = append(errs, ValidationError{
						Field:   refField,
						Message: fmt.Sprintf("group %q already belongs to group %q", ref, existingParent),
					})
				} else {
					gm.groupInGroup[ref] = g.ID
					gm.groupChildren[g.ID] = append(gm.groupChildren[g.ID], ref)
				}
			} else {
				errs = append(errs, ValidationError{
					Field:   refField,
					Message: fmt.Sprintf("contains reference %q in group %q is neither a valid node nor a valid group", ref, g.ID),
				})
			}
		}
	}

	// Phase 3: detect cycles and compute top-down depth.
	groupDepth := make(map[string]int)
	visitedGroups := make(map[string]bool)
	visitingGroups := make(map[string]bool)

	var assignDepth func(id string, depth int) error
	assignDepth = func(id string, depth int) error {
		if visitingGroups[id] {
			return fmt.Errorf("group containment cycle detected involving %q", id)
		}
		if visitedGroups[id] {
			return nil
		}
		visitingGroups[id] = true
		groupDepth[id] = depth
		for _, childID := range gm.groupChildren[id] {
			if err := assignDepth(childID, depth+1); err != nil {
				return err
			}
		}
		visitingGroups[id] = false
		visitedGroups[id] = true
		return nil
	}

	for _, g := range specs {
		if g.ID == "" || !gm.groupIDs[g.ID] {
			continue
		}
		if _, hasParent := gm.groupInGroup[g.ID]; !hasParent {
			if err := assignDepth(g.ID, 1); err != nil {
				errs = append(errs, ValidationError{Field: prefix, Message: err.Error()})
			}
		}
	}

	// Any group not visited is part of a cycle among non-root groups.
	for _, g := range specs {
		if g.ID != "" && gm.groupIDs[g.ID] && !visitedGroups[g.ID] {
			errs = append(errs, ValidationError{
				Field:   prefix,
				Message: fmt.Sprintf("group containment cycle detected involving %q", g.ID),
			})
		}
	}

	// Enforce max depth of 3.
	for id, depth := range groupDepth {
		if depth > maxGroupNestingDepth {
			errs = append(errs, ValidationError{
				Field:   prefix,
				Message: fmt.Sprintf("group %q exceeds maximum nesting depth of %d", id, maxGroupNestingDepth),
			})
		}
	}

	// Build model groups.
	groups := make([]model.Group, 0, len(specs))
	for _, g := range specs {
		if g.ID == "" || !gm.groupIDs[g.ID] {
			continue
		}
		groups = append(groups, model.Group{
			ID:       g.ID,
			Label:    g.Label,
			Contains: gm.groupContains[g.ID],
			Children: gm.groupChildren[g.ID],
			Depth:    groupDepth[g.ID],
			Color:    g.Color,
		})
	}

	return groups, errs
}
