package schema

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// Check returns the advisories for a flow, sequence or class spec, sorted, with
// the spec's ignore list applied. It is a separate pass over the raw JSON:
// the content rules read the decoded spec struct, unknown-field reads the
// generic tree. Callers run it after a successful parse; on a spec that
// fails validation Check makes no promise beyond not panicking.
//
// The error is non-nil only for JSON that does not decode or a type that
// is not flow or sequence.
func Check(data []byte, opts CheckOptions) ([]Advisory, error) {
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("check: invalid JSON: %w", err)
	}
	var out []Advisory
	var ignore []string
	switch probe.Type {
	case "flow":
		var spec FlowSpec
		if err := json.Unmarshal(data, &spec); err != nil {
			return nil, fmt.Errorf("check: %w", err)
		}
		unknown, err := unknownFields(data, reflect.TypeOf(spec), flowRemovedFields)
		if err != nil {
			return nil, err
		}
		out = append(out, unknown...)
		out = append(out, checkFlow(&spec, opts)...)
		ignore = spec.Ignore
	case "sequence":
		var spec SequenceSpec
		if err := json.Unmarshal(data, &spec); err != nil {
			return nil, fmt.Errorf("check: %w", err)
		}
		unknown, err := unknownFields(data, reflect.TypeOf(spec), nil)
		if err != nil {
			return nil, err
		}
		out = append(out, unknown...)
		out = append(out, checkSequence(&spec)...)
		ignore = spec.Ignore
	case "class":
		var spec ClassSpec
		if err := json.Unmarshal(data, &spec); err != nil {
			return nil, fmt.Errorf("check: %w", err)
		}
		unknown, err := unknownFields(data, reflect.TypeOf(spec), classRemovedFields)
		if err != nil {
			return nil, err
		}
		out = append(out, unknown...)
		out = append(out, checkClass(&spec, opts)...)
		ignore = spec.Ignore
	default:
		return nil, fmt.Errorf("check: unknown diagram type: %q", probe.Type)
	}
	out = dropIgnored(out, ignoreSet(ignore))
	SortAdvisories(out)
	return out, nil
}
