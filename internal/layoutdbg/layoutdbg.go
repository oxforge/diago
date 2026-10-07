// Package layoutdbg provides a thin helper for layout/routing decision
// logging. A *slog.Logger is carried in context.Context and accessed via
// From/With/Decision. When the logger's level is above DEBUG, Decision
// short-circuits before any allocation, so call sites are cheap when
// debug is off.
package layoutdbg

import (
	"context"
	"io"
	"log/slog"
)

type ctxKey struct{}

// nopLogger is a shared no-op logger returned by From when no logger is
// attached to ctx. It writes to io.Discard at LevelError (so any future
// non-debug log call is also dropped).
var nopLogger = slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{
	Level: slog.LevelError,
}))

// NewContext returns a child of parent with logger attached.
func NewContext(parent context.Context, logger *slog.Logger) context.Context {
	if logger == nil {
		return parent
	}
	return context.WithValue(parent, ctxKey{}, logger)
}

// From returns the logger attached to ctx, or a no-op logger if none.
// Never returns nil.
func From(ctx context.Context) *slog.Logger {
	if ctx == nil {
		return nopLogger
	}
	if logger, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok {
		return logger
	}
	return nopLogger
}

// With returns a child ctx whose attached logger has the given attrs
// pre-attached via slog.Logger.With. Subsequent Decision calls inherit
// these attrs automatically. Used at function boundaries to scope a
// phase, group, or other identifier.
//
// Example:
//
//	ctx = layoutdbg.With(ctx, "phase", "nest", "group", g.ID)
//	layoutdbg.Decision(ctx, "level_arranged", "nodes", len(lv.Nodes))
func With(ctx context.Context, attrs ...any) context.Context {
	logger := From(ctx)
	return NewContext(ctx, logger.With(attrs...))
}

// Armed reports whether a debug logger attached to ctx is enabled at DEBUG
// level — i.e. whether Decision calls on this ctx will actually emit records.
// Callers use it to gate work that is only worth doing for debug output
// (e.g. running the output-contract checker). Cost when unarmed is one
// context lookup and one level comparison.
func Armed(ctx context.Context) bool {
	return From(ctx).Enabled(ctx, slog.LevelDebug)
}

// Decision emits a single layout/routing decision via slog.Debug. The
// "decision" field is filled automatically. The slog "msg" is fixed at
// "layout decision" so consumers grep on the "decision" field, not msg.
//
// When the logger's level is above LevelDebug, Decision short-circuits
// before constructing attrs. Cost in that path is one method call and
// one comparison.
func Decision(ctx context.Context, decision string, attrs ...any) {
	logger := From(ctx)
	if !logger.Enabled(ctx, slog.LevelDebug) {
		return
	}
	args := make([]any, 0, len(attrs)+2)
	args = append(args, "decision", decision)
	args = append(args, attrs...)
	logger.Debug("layout decision", args...)
}
