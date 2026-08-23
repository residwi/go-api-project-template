package e2e_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mockgatewayserver "github.com/residwi/go-api-project-template/cmd/mockgateway/mockserver"
	"github.com/residwi/go-api-project-template/internal/modules/payment"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/server"
	"github.com/residwi/go-api-project-template/internal/testutil"
)

// TestE2EOrderIdempotencyReplay is the first test in this repo to replay an
// Idempotency-Key. Every other e2e mints a fresh uuid per request, which is why
// a refactor could split order's idempotency guard away from checkout's payment
// tail and leave the retry that header exists to make safe charging twice and
// flipping a paid order to fulfillment_failed, with every suite green.
func TestE2EOrderIdempotencyReplay(t *testing.T) {
	setup(t)

	var charges atomic.Int64
	mockMux := http.NewServeMux()
	mockgatewayserver.RegisterRoutes(mockMux, testutil.DiscardLogger())
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/charge") {
			charges.Add(1)
		}
		mockMux.ServeHTTP(w, r)
	}))
	defer mockServer.Close()

	customPaymentCfg := payment.Config{
		Gateway:        "mock",
		GatewayURL:     mockServer.URL + "/mock/payment",
		GatewayTimeout: 5 * time.Second,
	}
	deps := &server.Deps{
		Infra:   testDeps.Infra,
		Auth:    testDeps.Auth,
		Order:   testDeps.Order,
		Payment: customPaymentCfg,
		DB:      database.DB{Primary: testPool},
		Cache:   testRedis,
		Logger:  testutil.DiscardLogger(),
	}
	handler := server.NewRouter(deps, newTestApp(customPaymentCfg))
	ctx := context.Background()

	catID := uuid.New()
	_, err := testPool.Exec(ctx,
		`INSERT INTO categories (id, name, slug, active) VALUES ($1, 'Replay Cat', $2, true)`,
		catID, "replay-cat-"+catID.String()[:8])
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM categories WHERE id = $1`, catID) })

	// 2500 does not end in 99, so the mock gateway settles the charge
	// synchronously and the first POST leaves the order paid.
	prodID := uuid.New()
	_, err = testPool.Exec(ctx,
		`INSERT INTO products (id, name, slug, description, price, currency, status, category_id)
		 VALUES ($1, 'Replay Product', $2, 'desc', 2500, 'USD', 'published', $3)`,
		prodID, "replay-prod-"+prodID.String()[:8], catID)
	require.NoError(t, err)
	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM inventory_levels WHERE product_id = $1`, prodID)
		testPool.Exec(ctx, `DELETE FROM products WHERE id = $1`, prodID)
	})
	seedInventoryLevel(t, prodID, 100, 0)

	userID, token := registerE2EUser(t, handler, "replay-flow@example.com")
	t.Cleanup(func() { cleanupOrdersOf(userID) })

	cartBody := `{"product_id":"` + prodID.String() + `","quantity":1}`
	cartReq := httptest.NewRequest(http.MethodPost, "/api/cart/items", strings.NewReader(cartBody))
	cartReq.Header.Set("Content-Type", "application/json")
	cartReq.Header.Set("Authorization", "Bearer "+token)
	cartW := httptest.NewRecorder()
	handler.ServeHTTP(cartW, cartReq)
	require.Equal(t, http.StatusCreated, cartW.Code)

	idempotencyKey := uuid.New().String()
	placeOrder := func(t *testing.T) map[string]any {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/orders",
			strings.NewReader(`{"payment_method_id":"pm_test_replay"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Idempotency-Key", idempotencyKey)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		require.Equal(t, http.StatusCreated, w.Code, "a replay is a retry, not an error")

		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		return resp["data"].(map[string]any)["order"].(map[string]any)
	}

	first := placeOrder(t)
	orderID := uuid.MustParse(first["id"].(string))
	require.Equal(t, "paid", orderStatusOf(t, orderID))
	require.EqualValues(t, 1, charges.Load())

	t.Run("replaying the key returns the stored order without charging again", func(t *testing.T) {
		replayed := placeOrder(t)

		assert.Equal(t, first["id"], replayed["id"])
		assert.Equal(t, "paid", orderStatusOf(t, orderID))
		assert.EqualValues(t, 1, charges.Load(), "the gateway must not be charged twice")
		assert.Equal(t, 1, countRows(t, `SELECT COUNT(*) FROM orders WHERE user_id = $1`, userID))
		assert.Equal(t, 1, countRows(t, `SELECT COUNT(*) FROM payments WHERE order_id = $1`, orderID))
	})
}

func countRows(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, testPool.QueryRow(context.Background(), query, args...).Scan(&n))
	return n
}

// cleanupOrdersOf deletes an order graph child-first. Not a t.Cleanup itself so
// the caller keeps the registration order it wants against its own fixtures.
func cleanupOrdersOf(userID uuid.UUID) {
	ctx := context.Background()
	for _, q := range []string{
		`DELETE FROM cart_items WHERE cart_id IN (SELECT id FROM carts WHERE user_id = $1)`,
		`DELETE FROM carts WHERE user_id = $1`,
		`DELETE FROM payment_jobs WHERE order_id IN (SELECT id FROM orders WHERE user_id = $1)`,
		`DELETE FROM payments WHERE order_id IN (SELECT id FROM orders WHERE user_id = $1)`,
		`DELETE FROM order_items WHERE order_id IN (SELECT id FROM orders WHERE user_id = $1)`,
		`DELETE FROM notifications WHERE user_id = $1`,
		`DELETE FROM coupon_usages WHERE order_id IN (SELECT id FROM orders WHERE user_id = $1)`,
		`DELETE FROM orders WHERE user_id = $1`,
	} {
		testPool.Exec(ctx, q, userID)
	}
}
