package schema

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// DiagramType reads the type a spec declares. Data that is not a JSON object
// is a validation error on the whole document (field ""); a missing type, or
// one other than flow, sequence or class, is one on "type".
func DiagramType(data []byte) (string, error) {
	var probe struct {
		Type string `json:"type"`
	}
	if err := decode(data, &probe); err != nil {
		return "", err
	}
	switch probe.Type {
	case "flow", "sequence", "class":
		return probe.Type, nil
	case "":
		return "", ValidationErrors{{Field: "type", Message: "must be given: flow, sequence or class"}}
	}
	return "", ValidationErrors{{Field: "type", Message: fmt.Sprintf("unknown diagram type %q: want flow, sequence or class", probe.Type)}}
}

// decode unmarshals data into v. A value of the wrong JSON type is a
// validation error on its path ("edges[0].flat"); data that is not JSON is
// one on the whole document.
func decode(data []byte, v any) error {
	err := json.Unmarshal(data, v)
	if err == nil {
		return nil
	}
	var te *json.UnmarshalTypeError
	if errors.As(err, &te) {
		return ValidationErrors{{Field: typeErrorPath(data, te), Message: fmt.Sprintf("must be %s, got %s", jsonKind(te.Type), te.Value)}}
	}
	return ValidationErrors{{Field: "", Message: "invalid JSON: " + err.Error()}}
}

// typeErrorPath names the value an UnmarshalTypeError rejected, as validation
// errors name fields ("edges[1].flat"). The decoder's own Field omits array
// indices and map keys on some Go versions, but its Offset is the same on all,
// so the path comes from walking data's tokens to the value that ends at, or
// for a container opens at, that offset. Should the walk fail it falls back
// to the decoder's Field.
func typeErrorPath(data []byte, te *json.UnmarshalTypeError) string {
	type frame struct {
		object    bool
		key       string
		index     int
		expectKey bool
	}
	var stack []frame
	path := func() string {
		var b strings.Builder
		for _, f := range stack {
			switch {
			case !f.object:
				fmt.Fprintf(&b, "[%d]", f.index)
			case b.Len() > 0:
				b.WriteByte('.')
				fallthrough
			default:
				b.WriteString(f.key)
			}
		}
		return b.String()
	}
	// done marks the value just completed in the enclosing container.
	done := func() {
		if n := len(stack); n > 0 {
			if stack[n-1].object {
				stack[n-1].expectKey = true
			} else {
				stack[n-1].index++
			}
		}
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if err != nil {
			return jsonPath(te.Field)
		}
		d, isDelim := tok.(json.Delim)
		if isDelim && (d == ']' || d == '}') {
			stack = stack[:len(stack)-1]
			done()
			continue
		}
		if n := len(stack); n > 0 && stack[n-1].object && stack[n-1].expectKey {
			stack[n-1].key, _ = tok.(string)
			stack[n-1].expectKey = false
			continue
		}
		if dec.InputOffset() >= te.Offset {
			return path()
		}
		if isDelim {
			stack = append(stack, frame{object: d == '{', expectKey: d == '{'})
			continue
		}
		done()
	}
}

// jsonPath writes encoding/json's dotted path ("edges.0.flat") the way
// validation errors name fields ("edges[0].flat").
func jsonPath(dotted string) string {
	if dotted == "" {
		return ""
	}
	var b strings.Builder
	for i, part := range strings.Split(dotted, ".") {
		if _, err := strconv.Atoi(part); err == nil {
			fmt.Fprintf(&b, "[%s]", part)
			continue
		}
		if i > 0 {
			b.WriteByte('.')
		}
		b.WriteString(part)
	}
	return b.String()
}

// jsonKind names the JSON value a Go type decodes from.
func jsonKind(t reflect.Type) string {
	switch t.Kind() {
	case reflect.Bool:
		return "a boolean"
	case reflect.String:
		return "a string"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "an integer"
	case reflect.Float32, reflect.Float64:
		return "a number"
	case reflect.Slice, reflect.Array:
		return "an array"
	case reflect.Map, reflect.Struct:
		return "an object"
	case reflect.Pointer:
		return jsonKind(t.Elem())
	}
	return "a " + t.String()
}
