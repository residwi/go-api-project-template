package e2e_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func TestTraceShapeOfAPublicRequest(t *testing.T) {
	setup(t)

	router := newTestRouter(testPaymentCfg)

	req := httptest.NewRequest(http.MethodGet, "/api/products", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	root := findSpan(t, "GET /api/products")
	assert.Equal(t, trace.SpanKindServer, root.SpanKind())
	assert.False(t, root.Parent().IsValid())

	query := findSpanDescendedFrom(t, root, "query SELECT")
	assert.Equal(t, trace.SpanKindClient, query.SpanKind())
}

func findSpan(t *testing.T, name string) sdktrace.ReadOnlySpan {
	t.Helper()

	for _, span := range testSpans.Ended() {
		if span.Name() == name {
			return span
		}
	}

	t.Fatalf("no span named %q in %v", name, spanNames())

	return nil
}

func findSpanDescendedFrom(t *testing.T, root sdktrace.ReadOnlySpan, name string) sdktrace.ReadOnlySpan {
	t.Helper()

	for _, span := range testSpans.Ended() {
		if span.Name() == name && span.SpanContext().TraceID() == root.SpanContext().TraceID() {
			return span
		}
	}

	t.Fatalf("no span named %q in the trace of %q; saw %v", name, root.Name(), spanNames())

	return nil
}

func spanNames() []string {
	names := make([]string, 0, len(testSpans.Ended()))
	for _, span := range testSpans.Ended() {
		names = append(names, span.Name())
	}

	return names
}
