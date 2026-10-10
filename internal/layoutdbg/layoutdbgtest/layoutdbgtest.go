// Package layoutdbgtest captures the decision records layoutdbg emits, for
// tests. The engine never imports it.
package layoutdbgtest

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/layoutdbg"
)

// Capture returns a context whose decision records go to the returned
// buffer, one JSON object per line, as -debug writes them.
func Capture() (context.Context, *bytes.Buffer) {
	var log bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&log, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return layoutdbg.NewContext(context.Background(), logger), &log
}

// Records decodes the records in log, in the order they were logged.
func Records(t testing.TB, log *bytes.Buffer) []map[string]any {
	t.Helper()
	return Decode[map[string]any](t, log)
}

// Decode decodes each record in log into a T, in the order they were
// logged; a line that does not decode fails t.
func Decode[T any](t testing.TB, log *bytes.Buffer) []T {
	t.Helper()
	var out []T
	for line := range strings.Lines(log.String()) {
		var rec T
		require.NoError(t, json.Unmarshal([]byte(line), &rec), "decision record %s", line)
		out = append(out, rec)
	}
	return out
}
