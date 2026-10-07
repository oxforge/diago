package layoutdbg

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func newCapturingCtx(level slog.Level) (context.Context, *bytes.Buffer) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: level}))
	return NewContext(context.Background(), logger), &buf
}

func parseLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("invalid JSON line: %q (%v)", line, err)
		}
		out = append(out, entry)
	}
	return out
}

func TestDecision_EmitsJSONWhenDebug(t *testing.T) {
	ctx, buf := newCapturingCtx(slog.LevelDebug)
	Decision(ctx, "sibling_blocks_check",
		"insider", "web", "sibling", "mobile", "blocks", false)
	entries := parseLines(t, buf)
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e["decision"] != "sibling_blocks_check" {
		t.Errorf("decision = %v, want sibling_blocks_check", e["decision"])
	}
	if e["insider"] != "web" {
		t.Errorf("insider = %v, want web", e["insider"])
	}
	if e["blocks"] != false {
		t.Errorf("blocks = %v, want false", e["blocks"])
	}
}

func TestDecision_DropsWhenInfo(t *testing.T) {
	ctx, buf := newCapturingCtx(slog.LevelInfo)
	Decision(ctx, "anything", "k", "v")
	if buf.Len() != 0 {
		t.Errorf("want empty buffer, got %q", buf.String())
	}
}

func TestWith_AttrsInheritedByDecision(t *testing.T) {
	ctx, buf := newCapturingCtx(slog.LevelDebug)
	ctx = With(ctx, "phase", "obstruction", "group", "clients")
	Decision(ctx, "sibling_blocks_check", "insider", "web")
	entries := parseLines(t, buf)
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e["phase"] != "obstruction" {
		t.Errorf("phase = %v, want obstruction (inherited from With)", e["phase"])
	}
	if e["group"] != "clients" {
		t.Errorf("group = %v, want clients", e["group"])
	}
}

func TestFrom_NilContextReturnsNopLogger(t *testing.T) {
	// Intentionally pass nil context to verify From's nil-safety contract.
	logger := From(nil) //nolint:staticcheck // SA1012: testing nil-context safety on purpose
	if logger == nil {
		t.Fatal("From(nil) returned nil")
	}
	// Calling Debug on the nop logger must not panic and must produce no output.
	logger.Debug("should be dropped")
}

func TestFrom_NoLoggerInContext(t *testing.T) {
	logger := From(context.Background())
	if logger == nil {
		t.Fatal("From(empty ctx) returned nil")
	}
	// Should be the nop logger.
	logger.Debug("should be dropped")
}
