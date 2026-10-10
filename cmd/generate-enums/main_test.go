package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGenerate_MatchesTheCheckedInFile runs the generator as go generate
// does, in a directory of its own, and compares what it writes with the
// model package's enums_gen.go: an enum or template change without a
// regeneration, or a hand edit of the generated file, fails at the first
// line that differs.
func TestGenerate_MatchesTheCheckedInFile(t *testing.T) {
	want, err := os.ReadFile(filepath.Join("..", "..", "internal", "model", "enums_gen.go"))
	require.NoError(t, err)
	t.Chdir(t.TempDir())
	main()
	got, err := os.ReadFile("enums_gen.go")
	require.NoError(t, err)
	wl, gl := strings.Split(string(want), "\n"), strings.Split(string(got), "\n")
	for i := range max(len(wl), len(gl)) {
		var w, g string
		if i < len(wl) {
			w = wl[i]
		}
		if i < len(gl) {
			g = gl[i]
		}
		require.Equal(t, w, g, "internal/model/enums_gen.go line %d is stale: run go generate ./internal/model", i+1)
	}
}
