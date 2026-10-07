// Package schema provides JSON parsing and validation for diagram spec types.
package schema

import (
	"fmt"
	"strings"
)

// ValidationError represents a single field-level validation failure.
type ValidationError struct {
	Field   string `json:"field"`   // e.g., "nodes[2].shape"
	Message string `json:"message"` // e.g., "unknown shape: octagon"
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidationErrors is a collection of field-level validation errors.
type ValidationErrors []ValidationError

// Error implements the error interface by joining all errors.
func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return "validation failed"
	}
	msgs := make([]string, len(e))
	for i, ve := range e {
		msgs[i] = ve.Error()
	}
	return strings.Join(msgs, "; ")
}
