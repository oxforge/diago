package schema

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// envelopeKeys are top-level keys the spec structs do not declare but the
// API decoded from the same request body. They are never unknown.
var envelopeKeys = map[string]bool{"format": true, "store": true}

// Removed top-level fields, per spec type: a key that used to be part of
// the spec reports removed-field with the reason instead of unknown-field.
var (
	flowRemovedFields = map[string]string{
		"style":     "diago is orthogonal-only; the field is ignored",
		"hints":     "layout hints were removed; the field is ignored",
		"alignment": "alignment was removed; the field is ignored",
	}
	classRemovedFields = map[string]string{
		"style": "diago is orthogonal-only; the field is ignored",
	}
)

// maxSuggestDistance bounds the "did you mean" edit distance.
const maxSuggestDistance = 2

// unknownFields walks the decoded JSON alongside the spec struct type and
// reports every object key the struct does not declare at that position.
// Keys match case-insensitively, as encoding/json matches them. removed
// names top-level keys that used to be part of the spec: those report
// removed-field with the reason instead of unknown-field.
func unknownFields(data []byte, root reflect.Type, removed map[string]string) ([]Advisory, error) {
	var tree any
	if err := json.Unmarshal(data, &tree); err != nil {
		return nil, fmt.Errorf("check: invalid JSON: %w", err)
	}
	var out []Advisory
	walkUnknown(tree, root, "", true, removed, &out)
	return out, nil
}

// fieldSet is a struct's JSON keys in declaration order, with their types.
type fieldSet struct {
	names []string
	types map[string]reflect.Type // lower-cased name → field type
}

func jsonFields(t reflect.Type) fieldSet {
	fs := fieldSet{types: map[string]reflect.Type{}}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		name := strings.Split(f.Tag.Get("json"), ",")[0]
		if name == "-" {
			continue
		}
		if name == "" {
			name = f.Name
		}
		fs.names = append(fs.names, name)
		fs.types[strings.ToLower(name)] = f.Type
	}
	return fs
}

func walkUnknown(v any, t reflect.Type, path string, top bool, removed map[string]string, out *[]Advisory) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Struct:
		obj, ok := v.(map[string]any)
		if !ok {
			return
		}
		known := jsonFields(t)
		keys := make([]string, 0, len(obj))
		for k := range obj {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if ft, isKnown := known.types[strings.ToLower(k)]; isKnown {
				walkUnknown(obj[k], ft, joinPath(path, k), false, nil, out)
				continue
			}
			if top && envelopeKeys[k] {
				continue
			}
			if top {
				if msg, gone := removed[strings.ToLower(k)]; gone {
					*out = append(*out, Advisory{Rule: RuleRemovedField, Field: k, Message: msg})
					continue
				}
			}
			msg := fmt.Sprintf("unknown key %q is ignored", k)
			if s := suggest(k, known.names); s != "" {
				msg += fmt.Sprintf("; did you mean %q?", s)
			}
			*out = append(*out, Advisory{Rule: RuleUnknownField, Field: joinPath(path, k), Message: msg})
		}
	case reflect.Slice, reflect.Array:
		arr, ok := v.([]any)
		if !ok {
			return
		}
		for i, e := range arr {
			walkUnknown(e, t.Elem(), fmt.Sprintf("%s[%d]", path, i), false, nil, out)
		}
	}
}

func joinPath(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

// suggest returns the declared key closest to key within
// maxSuggestDistance, first in declaration order on ties, or "". A candidate
// only counts when its edit distance is also less than the rune length of
// the shorter of the two strings: on short keys (e.g. "wt", "kind"), an edit
// distance of 2 rewrites most or all of the string, which is not a safe
// rename to suggest to an agent that will act on it without a human check.
func suggest(key string, names []string) string {
	best, bestD := "", maxSuggestDistance+1
	kLen := len([]rune(key))
	for _, n := range names {
		nLen := len([]rune(n))
		shorter := kLen
		if nLen < shorter {
			shorter = nLen
		}
		d := levenshtein(strings.ToLower(key), strings.ToLower(n))
		if d >= shorter {
			continue
		}
		if d < bestD {
			best, bestD = n, d
		}
	}
	return best
}

// levenshtein is the edit distance between two strings, over runes.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}
