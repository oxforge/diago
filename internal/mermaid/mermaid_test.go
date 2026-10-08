package mermaid

import (
	"errors"
	"testing"

	"github.com/oxforge/diago/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The header forms Sniff recognizes, plus the frontmatter and comment-only
// forms.
func TestSniff(t *testing.T) {
	cases := []struct {
		src  string
		want Kind
	}{
		{"sequenceDiagram\nA->>B: x", KindSequence},
		{"---\ntitle: t\n---\nsequenceDiagram", KindSequence},
		{"graph TD\na-->b", KindFlowchart},
		{"\ufeffgraph TD\na-->b", KindFlowchart},
		{"flowchart TD\na-->b", KindFlowchart},
		{"flowchart BT\na-->b", KindFlowchart},
		{"%% comment\n\n  classDiagram\n", KindClass},
		{`{ "nodes": [] }`, ""},
		{"", ""},
		{"%% only a comment\n", ""},
		{"---\ntitle: unclosed\ngraph TD", ""},
		{"graphs TD", ""},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, Sniff([]byte(c.src)), c.src)
	}
}

func TestSplitLines(t *testing.T) {
	got := splitLines([]byte("graph TD\r\n  a --> b %% trailing\n\n%% whole\n"))
	require.Len(t, got, 5)
	assert.Equal(t, line{1, "graph TD"}, got[0])
	assert.Equal(t, line{2, "a --> b"}, got[1])
	assert.Equal(t, line{3, ""}, got[2])
	assert.Equal(t, line{4, ""}, got[3])
	assert.Equal(t, line{5, ""}, got[4])
}

func TestSplitLines_BOM(t *testing.T) {
	got := splitLines([]byte("\ufeffgraph TD\n  a --> b\n"))
	require.NotEmpty(t, got)
	assert.Equal(t, line{1, "graph TD"}, got[0])
}

func TestUnquote(t *testing.T) {
	assert.Equal(t, `Quoted [label]`, unquote(`"Quoted [label]"`))
	assert.Equal(t, `plain`, unquote(` plain `))
	assert.Equal(t, `"`, unquote(`"`))
}

func TestParseFrontmatter(t *testing.T) {
	title, body, err := parseFrontmatter(splitLines([]byte("---\ntitle: Deploy pipeline\n---\ngraph TD\n")))
	require.NoError(t, err)
	assert.Equal(t, "Deploy pipeline", title)
	// The trailing blank line (the split artifact of the source's final
	// "\n") is kept so later error line numbers match the source.
	require.Len(t, body, 2)
	assert.Equal(t, line{4, "graph TD"}, body[0])
	assert.Equal(t, line{5, ""}, body[1])

	title, _, err = parseFrontmatter(splitLines([]byte("---\ntitle: \"Order: states\"\n---\ngraph TD")))
	require.NoError(t, err)
	assert.Equal(t, "Order: states", title)

	title, body, err = parseFrontmatter(splitLines([]byte("graph TD\n  a --> b")))
	require.NoError(t, err)
	assert.Equal(t, "", title)
	assert.Len(t, body, 2)

	_, _, err = parseFrontmatter(splitLines([]byte("---\ntitle: Ok\nconfig: nope\n---\ngraph TD")))
	var pe *ParseError
	require.True(t, errors.As(err, &pe))
	assert.Equal(t, 3, pe.Line)
	assert.Contains(t, pe.Message, "config: nope")

	_, _, err = parseFrontmatter(splitLines([]byte("---\ntitle: Dangling\ngraph TD")))
	require.True(t, errors.As(err, &pe))
	assert.Equal(t, 1, pe.Line)
	assert.Equal(t, `unclosed frontmatter block (missing closing "---")`, pe.Message)
}

func TestParse_NotMermaid(t *testing.T) {
	for _, src := range []string{`{"type":"flow"}`, "", "%% only a comment\n", "---\ntitle: t\n---\n\n"} {
		res, err := Parse([]byte(src))
		assert.Nil(t, res)
		var pe *ParseError
		require.True(t, errors.As(err, &pe), src)
		assert.Equal(t, 1, pe.Line)
		assert.Equal(t, "not a Mermaid source", pe.Message)
		assert.Equal(t, "line 1: not a Mermaid source", err.Error())
	}
}

// TestParse_UnregisteredDialect pins the dispatch shape for a dialect that
// has no registered parser, by temporarily removing one. It mutates the
// package-level dialects map, so it must never run in parallel with the
// dialect tests; nothing in this package calls t.Parallel.
func TestParse_UnregisteredDialect(t *testing.T) {
	saved := dialects[KindFlowchart]
	delete(dialects, KindFlowchart)
	t.Cleanup(func() {
		if saved != nil {
			dialects[KindFlowchart] = saved
		}
	})
	_, err := Parse([]byte("graph TD\n  a --> b"))
	var pe *ParseError
	require.True(t, errors.As(err, &pe))
	assert.Equal(t, "the flowchart dialect is not supported", pe.Message)
}

// requireValid asserts the importer's JSON passes the dialect's parser and
// the advisory checker without error: the importer can never emit an
// invalid spec.
func requireValid(t *testing.T, kind Kind, js []byte) {
	t.Helper()
	var err error
	switch kind {
	case KindFlowchart:
		_, err = schema.ParseFlow(js)
	case KindSequence:
		_, err = schema.ParseSequence(js)
	case KindClass:
		_, err = schema.ParseClass(js)
	}
	require.NoError(t, err, string(js))
	_, err = schema.Check(js, schema.CheckOptions{})
	require.NoError(t, err)
}

// TestCorpus_InlineSourcesAreValid is a smoke check on a handful of inline
// sources: the property itself is carried by parseFlow, parseSeq and
// parseCls, which call requireValid on every source they translate.
func TestCorpus_InlineSourcesAreValid(t *testing.T) {
	for _, src := range []string{
		"graph TD\n  a --> b\n", zooSrc,
		"sequenceDiagram\nA->>B: x\nloop l\n  B-->>A: y\nend\n",
	} {
		res, err := Parse([]byte(src))
		require.NoError(t, err)
		requireValid(t, res.Kind, res.JSON)
	}
}
