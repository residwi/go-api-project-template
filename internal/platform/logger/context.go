package logger

import (
	"context"
	"log/slog"
	"slices"

	"go.opentelemetry.io/otel/trace"
)

type ctxKey struct{}

func WithAttrs(ctx context.Context, attrs ...slog.Attr) context.Context {
	prev, _ := ctx.Value(ctxKey{}).([]slog.Attr)

	return context.WithValue(ctx, ctxKey{}, append(slices.Clip(prev), attrs...))
}

type ContextHandler struct {
	slog.Handler
}

func (h ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	attrs, _ := ctx.Value(ctxKey{}).([]slog.Attr)
	spanCtx := trace.SpanContextFromContext(ctx)

	if len(attrs) == 0 && !spanCtx.IsValid() {
		return h.Handler.Handle(ctx, r)
	}

	r = r.Clone()
	r.AddAttrs(attrs...)

	if spanCtx.IsValid() {
		r.AddAttrs(
			slog.String("trace_id", spanCtx.TraceID().String()),
			slog.String("span_id", spanCtx.SpanID().String()),
		)
	}

	return h.Handler.Handle(ctx, r)
}

func (h ContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return ContextHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h ContextHandler) WithGroup(name string) slog.Handler {
	return ContextHandler{Handler: h.Handler.WithGroup(name)}
}
