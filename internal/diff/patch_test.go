package diff

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func apply(t *testing.T, old, patch, name string) (string, error) {
	t.Helper()
	out, err := ApplyUnifiedDiff([]byte(old), []byte(patch), name)
	return string(out), err
}

func TestApplyUnifiedDiff(t *testing.T) {
	const old = "a\nb\nc\nd\n"
	cases := []struct {
		name, old, patch, file, want, errContains string
	}{
		{"simple replacement", old, "--- a\n+++ b\n@@ -2,1 +2,1 @@\n-b\n+B\n", "", "a\nB\nc\nd\n", ""},
		{"insertion-only", old, "@@ -2,0 +3,1 @@\n+X\n", "", "a\nb\nX\nc\nd\n", ""},
		{"multiple hunks in order", old, "@@ -1,1 +1,1 @@\n-a\n+A\n@@ -4,1 +4,1 @@\n-d\n+D\n", "", "A\nb\nc\nD\n", ""},
		{"context mismatch names the hunk", old, "@@ -2,1 +2,1 @@\n-z\n+B\n", "", "", `hunk 1 (@@ -2,1 +2,1 @@): expected line 2 to be "z", found "b"`},
		{"multi-file selects by name", old, "--- a/x.json\n+++ b/x.json\n@@ -1,1 +1,1 @@\n-a\n+X\n--- a/y.json\n+++ b/y.json\n@@ -1,1 +1,1 @@\n-a\n+Y\n", "y.json", "Y\nb\nc\nd\n", ""},
		{"multi-file without a match", old, "--- a/x.json\n+++ b/x.json\n@@ -1,1 +1,1 @@\n-a\n+X\n--- a/y.json\n+++ b/y.json\n@@ -1,1 +1,1 @@\n-a\n+Y\n", "z.json", "", `no section matches "z.json" (sections: x.json, y.json)`},
		{"no-newline markers tolerated", "a\nb", "@@ -2,1 +2,1 @@\n-b\n\\ No newline at end of file\n+B\n\\ No newline at end of file\n", "", "a\nB", ""},
		{"bare hunks are one section", old, "@@ -1,1 +1,1 @@\n-a\n+A\n", "whatever.json", "A\nb\nc\nd\n", ""},
		{"CRLF patch on CRLF content", "a\r\nb\r\n", "@@ -1,1 +1,1 @@\n-a\r\n+A\r\n", "", "A\r\nb\r\n", ""},
		{"LF patch on CRLF content fails strictly", "a\r\nb\r\n", "@@ -1,1 +1,1 @@\n-a\n+A\n", "", "", `expected line 1 to be "a", found "a\r"`},
		{"header overstates body length with no trailing newline", old, "@@ -1,5 +1,5 @@\n-a\n+A", "", "", "fewer lines than its header declares"},
		{"hunks out of order", old, "@@ -3,1 +3,1 @@\n-c\n+C\n@@ -2,1 +2,1 @@\n-b\n+B\n", "", "", "hunks overlap or are out of order"},
		{"start beyond end of input", old, "@@ -9,1 +9,1 @@\n-x\n+X\n", "", "", "start beyond end of input"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := apply(t, tc.old, tc.patch, tc.file)
			if tc.errContains != "" {
				require.Error(t, err)
				var pe *PatchError
				assert.True(t, errors.As(err, &pe), "PatchError, got %T", err)
				assert.Contains(t, err.Error(), tc.errContains)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestApplyUnifiedDiff_NoHunks(t *testing.T) {
	_, err := ApplyUnifiedDiff([]byte("a\n"), []byte("just text\n"), "")
	assert.ErrorContains(t, err, "contains no hunks")
}
