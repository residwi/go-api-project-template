package tracing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestDropRootClientSpans(t *testing.T) {
	t.Run("keeps a server span that starts a trace", func(t *testing.T) {
		recorder, tracer := newFilteredTracer()

		_, span := tracer.Start(context.Background(), "POST /api/checkout", trace.WithSpanKind(trace.SpanKindServer))
		span.End()

		assert.Len(t, recorder.Ended(), 1)
	})

	t.Run("keeps a client span that has a parent", func(t *testing.T) {
		recorder, tracer := newFilteredTracer()

		ctx, parent := tracer.Start(context.Background(), "order.Place")
		_, child := tracer.Start(ctx, "query SELECT", trace.WithSpanKind(trace.SpanKindClient))
		child.End()
		parent.End()

		assert.Len(t, recorder.Ended(), 2)
	})

	t.Run("drops a client span that starts its own trace", func(t *testing.T) {
		recorder, tracer := newFilteredTracer()

		_, span := tracer.Start(context.Background(), "query SELECT", trace.WithSpanKind(trace.SpanKindClient))
		span.End()

		assert.Empty(t, recorder.Ended())
	})
}

func newFilteredTracer() (*tracetest.SpanRecorder, trace.Tracer) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(dropRootClientSpans{recorder}),
	)

	return recorder, provider.Tracer("test")
}
