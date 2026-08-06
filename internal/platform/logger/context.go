package logger

import (
	"context"
	"log/slog"
	"slices"
)

type ctxKey struct{}

// WithAttrs returns a context carrying attrs on top of any already there.
// Nothing reads them back out; they exist for ContextHandler.
func WithAttrs(ctx context.Context, attrs ...slog.Attr) context.Context {
	prev, _ := ctx.Value(ctxKey{}).([]slog.Attr)

	// Clip forces append to allocate. Without it two contexts derived from the same
	// parent share a backing array and overwrite each other's attributes.
	return context.WithValue(ctx, ctxKey{}, append(slices.Clip(prev), attrs...))
}

// ContextHandler merges the attributes WithAttrs stashed in the context into
// every record.
type ContextHandler struct {
	slog.Handler
}

func (h ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if attrs, ok := ctx.Value(ctxKey{}).([]slog.Attr); ok {
		// A caller may hand the same record to several handlers, whose appends would
		// otherwise land in one shared slot. Clone only clips capacity, so this costs
		// no allocation.
		r = r.Clone()
		r.AddAttrs(attrs...)
	}

	return h.Handler.Handle(ctx, r)
}

// WithAttrs rewraps. The promoted method would return the inner handler, so
// every logger built by [slog.Logger.With] would silently stop emitting context
// attributes.
func (h ContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return ContextHandler{Handler: h.Handler.WithAttrs(attrs)}
}

// WithGroup rewraps for the same reason as WithAttrs.
func (h ContextHandler) WithGroup(name string) slog.Handler {
	return ContextHandler{Handler: h.Handler.WithGroup(name)}
}
