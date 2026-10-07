package model

import "fmt"

// SupportedFormats maps diagram types to their supported output formats.
var SupportedFormats = map[string]map[string]bool{
	"flow": {
		"svg":        true,
		"png":        true,
		"txt":        true,
		"drawio":     true,
		"excalidraw": true,
	},
	"sequence": {
		"svg": true,
		"png": true,
	},
}

// ValidateFormat checks if the requested format is supported for the diagram type.
func ValidateFormat(diagType, format string) error {
	formats, ok := SupportedFormats[diagType]
	if !ok {
		return fmt.Errorf("unknown diagram type: %s", diagType)
	}
	if !formats[format] {
		return fmt.Errorf("format %q is not supported for %s diagrams", format, diagType)
	}
	return nil
}
