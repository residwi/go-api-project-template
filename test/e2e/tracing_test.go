package e2e_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
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

func TestTraceShapeOfCheckout(t *testing.T) {
	setup(t)
	handler := newTestRouter(testPaymentCfg)
	ctx := context.Background()

	catID := uuid.New()
	_, err := testPool.Exec(ctx,
		`INSERT INTO categories (id, name, slug, active) VALUES ($1, 'Trace Cat', $2, true)`,
		catID, "trace-cat-"+catID.String()[:8])
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM categories WHERE id = $1`, catID) })

	prodID := uuid.New()
	_, err = testPool.Exec(ctx,
		`INSERT INTO products (id, name, slug, description, price, currency, status, category_id)
		 VALUES ($1, 'Trace Product', $2, 'desc', 5000, 'USD', 'published', $3)`,
		prodID, "trace-prod-"+prodID.String()[:8], catID)
	require.NoError(t, err)
	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM inventory_levels WHERE product_id = $1`, prodID)
		testPool.Exec(ctx, `DELETE FROM products WHERE id = $1`, prodID)
	})
	seedInventoryLevel(t, prodID, 100, 0)

	email := "trace-checkout@example.com"
	_, token := registerE2EUser(t, handler, email)
	t.Cleanup(func() {
		testPool.Exec(
			ctx,
			`DELETE FROM cart_items WHERE cart_id IN (SELECT id FROM carts WHERE user_id IN (SELECT id FROM users WHERE email = $1))`,
			email,
		)
		testPool.Exec(ctx, `DELETE FROM carts WHERE user_id IN (SELECT id FROM users WHERE email = $1)`, email)
		testPool.Exec(
			ctx,
			`DELETE FROM order_items WHERE order_id IN (SELECT id FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = $1))`,
			email,
		)
		testPool.Exec(
			ctx,
			`DELETE FROM payments WHERE order_id IN (SELECT id FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = $1))`,
			email,
		)
		testPool.Exec(ctx, `DELETE FROM notifications WHERE user_id IN (SELECT id FROM users WHERE email = $1)`, email)
		testPool.Exec(ctx, `DELETE FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = $1)`, email)
	})

	addBody := `{"product_id":"` + prodID.String() + `","quantity":1}`
	addReq := httptest.NewRequest(http.MethodPost, "/api/cart/items", strings.NewReader(addBody))
	addReq.Header.Set("Content-Type", "application/json")
	addReq.Header.Set("Authorization", "Bearer "+token)
	addW := httptest.NewRecorder()
	handler.ServeHTTP(addW, addReq)
	require.Equal(t, http.StatusCreated, addW.Code)

	orderBody := `{"payment_method_id":"pm_test_123"}`
	orderReq := httptest.NewRequest(http.MethodPost, "/api/checkout", strings.NewReader(orderBody))
	orderReq.Header.Set("Content-Type", "application/json")
	orderReq.Header.Set("Authorization", "Bearer "+token)
	orderReq.Header.Set("Idempotency-Key", uuid.New().String())
	orderW := httptest.NewRecorder()
	handler.ServeHTTP(orderW, orderReq)
	require.Equal(t, http.StatusCreated, orderW.Code)

	root := findSpan(t, "POST /api/checkout")
	place := findSpanDescendedFrom(t, root, "checkout.PlaceOrder")

	assert.Equal(t, root.SpanContext().SpanID(), place.Parent().SpanID())

	orderPlace := findSpanDescendedFrom(t, root, "order.Place")
	assert.Equal(t, place.SpanContext().SpanID(), orderPlace.Parent().SpanID())

	query := findSpanDescendedFrom(t, root, "query SELECT")
	assert.Equal(t, orderPlace.SpanContext().SpanID(), query.Parent().SpanID())
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
