package pipeline

import (
	"errors"
	"fmt"
)

var (
	// ErrInvalidJSON indicates the request body is not valid JSON.
	ErrInvalidJSON = errors.New("request body is not valid JSON")

	// ErrMissingType indicates the 'type' field is missing from the spec.
	ErrMissingType = errors.New("type field is required")
)

// ErrUnknownType indicates an unsupported diagram type was requested.
type ErrUnknownType struct {
	Type string
}

func (e *ErrUnknownType) Error() string {
	return fmt.Sprintf("unknown diagram type: %s", e.Type)
}
