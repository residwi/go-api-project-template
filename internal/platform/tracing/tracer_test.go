package tracing_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/tracing"
	"github.com/residwi/go-api-project-template/internal/testutil"
)

func TestRecord(t *testing.T) {
	t.Run("leaves a successful span unset", func(t *testing.T) {
		recorder, tracer := newRecordingTracer()
		_, span := tracer.Start(context.Background(), "cart.Add")

		tracing.Record(span, nil)
		span.End()

		got := onlySpan(t, recorder)
		assert.Equal(t, codes.Unset, got.Status().Code)
		assert.Empty(t, got.Events())
	})

	t.Run("records a business error without failing the span", func(t *testing.T) {
		recorder, tracer := newRecordingTracer()
		_, span := tracer.Start(context.Background(), "cart.Add")

		tracing.Record(span, fmt.Errorf("%w: cart is empty", errs.ErrConflict))
		span.End()

		got := onlySpan(t, recorder)
		assert.Equal(t, codes.Unset, got.Status().Code)
		assert.Len(t, got.Events(), 1)
		assert.Contains(t, attrValues(got.Attributes()), "conflict")
	})

	t.Run("fails the span for an unrecognised error", func(t *testing.T) {
		recorder, tracer := newRecordingTracer()
		_, span := tracer.Start(context.Background(), "cart.Add")

		tracing.Record(span, errors.New("connection refused"))
		span.End()

		got := onlySpan(t, recorder)
		assert.Equal(t, codes.Error, got.Status().Code)
		assert.Equal(t, "connection refused", got.Status().Description)
		assert.Len(t, got.Events(), 1)
	})
}

func TestSetup(t *testing.T) {
	t.Run("registers nothing when the exporter is unset", func(t *testing.T) {
		t.Setenv("OTEL_TRACES_EXPORTER", "")

		beforeProvider := otel.GetTracerProvider()
		beforePropagator := otel.GetTextMapPropagator()

		shutdown, err := tracing.Setup(t.Context(), "test-api", "test", testutil.DiscardLogger())
		require.NoError(t, err)
		t.Cleanup(shutdown)

		assert.Same(t, beforeProvider, otel.GetTracerProvider())
		assert.Equal(t, beforePropagator, otel.GetTextMapPropagator())
	})

	t.Run("registers nothing when the exporter is none", func(t *testing.T) {
		t.Setenv("OTEL_TRACES_EXPORTER", "none")

		beforeProvider := otel.GetTracerProvider()
		beforePropagator := otel.GetTextMapPropagator()

		shutdown, err := tracing.Setup(t.Context(), "test-api", "test", testutil.DiscardLogger())
		require.NoError(t, err)
		t.Cleanup(shutdown)

		assert.Same(t, beforeProvider, otel.GetTracerProvider())
		assert.Equal(t, beforePropagator, otel.GetTextMapPropagator())
	})

	t.Run("registers a provider and a propagator when an exporter is configured", func(t *testing.T) {
		t.Setenv("OTEL_TRACES_EXPORTER", "console")

		shutdown, err := tracing.Setup(t.Context(), "test-api", "test", testutil.DiscardLogger())
		require.NoError(t, err)
		t.Cleanup(shutdown)

		assert.NotEmpty(t, otel.GetTextMapPropagator().Fields())
	})
}

func newRecordingTracer() (*tracetest.SpanRecorder, trace.Tracer) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))

	return recorder, provider.Tracer("test")
}

func onlySpan(t *testing.T, recorder *tracetest.SpanRecorder) sdktrace.ReadOnlySpan {
	t.Helper()

	ended := recorder.Ended()
	require.Len(t, ended, 1)

	return ended[0]
}

func attrValues(attrs []attribute.KeyValue) []string {
	values := make([]string, 0, len(attrs))
	for _, attr := range attrs {
		values = append(values, attr.Value.String())
	}

	return values
}
