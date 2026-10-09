package schema

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSingleLine_EscapesEveryControlCharacter(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"plain", "plain"},
		{"two\nlines", `two\nlines`},
		{"a\tb", `a\tb`},
		{"a\r\nb", `a\r\nb`},
		{"bell \u0007", `bell \u0007`},
		{"del \u007f", `del \u007f`},
		{"x\ufffe", `x\ufffe`},
		{"日本 é → ✓", "日本 é → ✓"},
	} {
		assert.Equal(t, tc.want, singleLine(tc.in), "%q", tc.in)
	}
}

func TestMultiLine_KeepsLineBreaks(t *testing.T) {
	assert.Equal(t, "API\nGateway", multiLine("API\nGateway"))
	assert.Equal(t, "API\nGateway", multiLine("API\r\nGateway"), "a CRLF breaks the line like LF")
	assert.Equal(t, `a\tb`+"\n"+`c\u0007`, multiLine("a\tb\nc\u0007"))
}

// controlSuffix gives a text field a bell, a tab and a newline, as JSON escapes.
const controlSuffix = `\u0007\t\n`

// TestParse_NoControlCharacterReachesTheModel gives every text field of each
// diagram type a bell, a tab and a newline, then walks the parsed model: only
// a flow node's label keeps its "\n".
func TestParse_NoControlCharacterReachesTheModel(t *testing.T) {
	const x = controlSuffix
	flow := `{"type":"flow","title":"T` + x + `","nodes":[{"id":"a","label":"A` + x + `"},{"id":"b","label":"B"}],` +
		`"edges":[{"from":"a","to":"b","label":"E` + x + `"}],"groups":[{"id":"g","label":"G` + x + `","contains":["a"]}]}`
	class := `{"type":"class","title":"T` + x + `","legend_labels":{"inheritance":"L` + x + `"},` +
		`"classes":[{"id":"a","label":"A` + x + `","stereotype":"S` + x + `","type_params":["P` + x + `"],` +
		`"attributes":[{"text":"f` + x + `"}],"methods":[{"text":"m` + x + `"}]},{"id":"b"}],` +
		`"relations":[{"from":"a","to":"b","kind":"inheritance","label":"R` + x + `","from_card":"1` + x + `","to_card":"2` + x + `"}],` +
		`"packages":[{"id":"p","label":"P` + x + `","contains":["a"]}]}`
	seq := `{"type":"sequence","title":"T` + x + `","actors":[{"id":"a","label":"A` + x + `"},{"id":"b","label":"B"}],` +
		`"interactions":[{"from":"a","to":"b","label":"M` + x + `"}],` +
		`"fragments":[{"type":"alt","over":["a","b"],"sections":[{"label":"S` + x + `","start":0,"end":0}]}]}`

	g, err := ParseFlow([]byte(flow))
	require.NoError(t, err)
	assertNoControls(t, reflect.ValueOf(g), "flow", func(path string) bool {
		return strings.HasPrefix(path, "flow.Nodes[") && strings.HasSuffix(path, ".Label")
	})
	c, err := ParseClass([]byte(class))
	require.NoError(t, err)
	assertNoControls(t, reflect.ValueOf(c), "class", func(string) bool { return false })
	s, err := ParseSequence([]byte(seq))
	require.NoError(t, err)
	assertNoControls(t, reflect.ValueOf(s), "sequence", func(string) bool { return false })
}

// assertNoControls walks v and fails on a string holding a control
// character; at a path keepsNewline accepts, a "\n" is allowed.
func assertNoControls(t *testing.T, v reflect.Value, path string, keepsNewline func(string) bool) {
	t.Helper()
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			assertNoControls(t, v.Elem(), path, keepsNewline)
		}
	case reflect.Struct:
		for i := range v.NumField() {
			assertNoControls(t, v.Field(i), path+"."+v.Type().Field(i).Name, keepsNewline)
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			assertNoControls(t, v.Index(i), fmt.Sprintf("%s[%d]", path, i), keepsNewline)
		}
	case reflect.Map:
		for it := v.MapRange(); it.Next(); {
			assertNoControls(t, it.Value(), fmt.Sprintf("%s[%v]", path, it.Key()), keepsNewline)
		}
	case reflect.String:
		s := v.String()
		if keepsNewline(path) {
			s = strings.ReplaceAll(s, "\n", "")
		}
		assert.False(t, strings.ContainsFunc(s, unicode.IsControl), "%s = %q", path, v.String())
	}
}
