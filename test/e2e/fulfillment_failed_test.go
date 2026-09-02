package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mockgatewayserver "github.com/residwi/go-api-project-template/cmd/mockgateway/mockserver"
	"github.com/residwi/go-api-project-template/internal/features/payment"

	"github.com/residwi/go-api-project-template/internal/testutil"
)

func TestE2ELatePaymentSuccessOnCancelledOrder(t *testing.T) {
	setup(t)
	mockMux := http.NewServeMux()
	mockgatewayserver.RegisterRoutes(mockMux, testutil.DiscardLogger())
	mockServer := httptest.NewServer(mockMux)
	defer mockServer.Close()

	customPaymentCfg := payment.Config{
		Gateway:        "mock",
		GatewayURL:     mockServer.URL + "/mock/payment",
		GatewayTimeout: 5 * time.Second,
	}
	handler := newTestRouter(customPaymentCfg)
	ctx := context.Background()

	catID := uuid.New()
	_, err := testPool.Exec(ctx,
		`INSERT INTO categories (id, name, slug, active) VALUES ($1, 'LateSuccess Cat', $2, true)`,
		catID, "latesuccess-cat-"+catID.String()[:8])
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM categories WHERE id = $1`, catID) })

	// 4099 x1 makes the order total end in 99, so the mock gateway declines the
	// synchronous charge and the order stays awaiting_payment -- which is what
	// makes it cancellable, and therefore terminal when the webhook lands.
	prodID := uuid.New()
	_, err = testPool.Exec(ctx,
		`INSERT INTO products (id, name, slug, description, price, currency, status, category_id)
		 VALUES ($1, 'LateSuccess Product', $2, 'desc', 4099, 'USD', 'published', $3)`,
		prodID, "latesuccess-prod-"+prodID.String()[:8], catID)
	require.NoError(t, err)
	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM inventory_levels WHERE product_id = $1`, prodID)
		testPool.Exec(ctx, `DELETE FROM products WHERE id = $1`, prodID)
	})
	seedInventoryLevel(t, prodID, 100, 0)

	email := "latesuccess-flow@example.com"
	regBody := `{"email":"` + email + `","password":"Password123!","first_name":"Late","last_name":"User"}`
	regReq := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(regBody))
	regReq.Header.Set("Content-Type", "application/json")
	regW := httptest.NewRecorder()
	handler.ServeHTTP(regW, regReq)
	require.Equal(t, http.StatusCreated, regW.Code)

	var regResp map[string]any
	require.NoError(t, json.NewDecoder(regW.Body).Decode(&regResp))
	token := regResp["data"].(map[string]any)["access_token"].(string)

	t.Cleanup(func() {
		testPool.Exec(
			ctx,
			`DELETE FROM payments WHERE order_id IN (SELECT id FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = $1))`,
			email,
		)
		testPool.Exec(ctx, `DELETE FROM notifications WHERE user_id IN (SELECT id FROM users WHERE email = $1)`, email)
		testPool.Exec(
			ctx,
			`DELETE FROM order_items WHERE order_id IN (SELECT id FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = $1))`,
			email,
		)
		testPool.Exec(
			ctx,
			`DELETE FROM cart_items WHERE cart_id IN (SELECT id FROM carts WHERE user_id IN (SELECT id FROM users WHERE email = $1))`,
			email,
		)
		testPool.Exec(ctx, `DELETE FROM carts WHERE user_id IN (SELECT id FROM users WHERE email = $1)`, email)
		testPool.Exec(ctx, `DELETE FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = $1)`, email)
		testPool.Exec(ctx, `DELETE FROM users WHERE email = $1`, email)
	})

	cartBody := `{"product_id":"` + prodID.String() + `","quantity":1}`
	cartReq := httptest.NewRequest(http.MethodPost, "/api/cart/items", strings.NewReader(cartBody))
	cartReq.Header.Set("Content-Type", "application/json")
	cartReq.Header.Set("Authorization", "Bearer "+token)
	cartW := httptest.NewRecorder()
	handler.ServeHTTP(cartW, cartReq)
	require.Equal(t, http.StatusCreated, cartW.Code)

	// Place order. The charge declines, so the order stays awaiting_payment.
	orderBody := `{"payment_method_id":"pm_test_123"}`
	orderReq := httptest.NewRequest(http.MethodPost, "/api/orders", strings.NewReader(orderBody))
	orderReq.Header.Set("Content-Type", "application/json")
	orderReq.Header.Set("Authorization", "Bearer "+token)
	orderReq.Header.Set("Idempotency-Key", uuid.New().String())
	orderW := httptest.NewRecorder()
	handler.ServeHTTP(orderW, orderReq)
	require.Equal(t, http.StatusCreated, orderW.Code)

	var orderResp map[string]any
	require.NoError(t, json.NewDecoder(orderW.Body).Decode(&orderResp))
	orderID := orderResp["data"].(map[string]any)["order"].(map[string]any)["id"].(string)

	var paymentID uuid.UUID
	err = testPool.QueryRow(ctx, `SELECT id FROM payments WHERE order_id = $1`, orderID).Scan(&paymentID)
	require.NoError(t, err)

	var orderStatus string
	require.NoError(t, testPool.QueryRow(ctx, `SELECT status FROM orders WHERE id = $1`, orderID).Scan(&orderStatus))
	require.Equal(t, "awaiting_payment", orderStatus, "the declined charge must leave the order cancellable")

	// Cancel the order through the API, making it terminal.
	cancelReq := httptest.NewRequest(http.MethodPost, "/api/orders/"+orderID+"/cancel", nil)
	cancelReq.Header.Set("Authorization", "Bearer "+token)
	cancelW := httptest.NewRecorder()
	handler.ServeHTTP(cancelW, cancelReq)
	require.Equal(t, http.StatusNoContent, cancelW.Code)

	require.NoError(t, testPool.QueryRow(ctx, `SELECT status FROM orders WHERE id = $1`, orderID).Scan(&orderStatus))
	require.Equal(t, "cancelled", orderStatus)

	t.Run("late success webhook flags the cancelled order fulfillment_failed", func(t *testing.T) {
		webhookBody := fmt.Sprintf(
			`{"event":"success","metadata":{"payment_id":"%s"},"transaction_id":"txn_late_success"}`,
			paymentID,
		)
		req := httptest.NewRequest(http.MethodPost, "/api/payments/webhook", strings.NewReader(webhookBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code, "the webhook is acked so the gateway stops retrying")

		// The order reaches fulfillment_failed through production code, not SQL.
		var status string
		require.NoError(t, testPool.QueryRow(ctx, `SELECT status FROM orders WHERE id = $1`, orderID).Scan(&status))
		assert.Equal(t, "fulfillment_failed", status)

		// The payment is parked for review rather than left looking successful:
		// funds are captured but the order can never be fulfilled.
		var paymentStatus string
		require.NoError(
			t,
			testPool.QueryRow(ctx, `SELECT status FROM payments WHERE id = $1`, paymentID).Scan(&paymentStatus),
		)
		assert.Equal(t, "requires_review", paymentStatus)

		var pendingRefunds int
		require.NoError(t, testPool.QueryRow(ctx,
			`SELECT COUNT(*) FROM river_job
			 WHERE kind = 'payment.refund' AND $1 = ANY(tags) AND state = 'available'`,
			"order-"+orderID).Scan(&pendingRefunds))
		assert.Equal(t, 1, pendingRefunds)
	})

	t.Run("the enqueued refund job then refunds the order", func(t *testing.T) {
		// Cancelling already released the reservation, so the refund's inventory
		// reversal must be a no-op rather than a second release.
		stockBefore, reservedBefore := inventoryLevelOf(t, prodID)
		require.Equal(t, 100, stockBefore)
		require.Equal(t, 0, reservedBefore)

		svc := newPaymentService(t, mockServer.URL+"/mock/payment")

		require.NoError(t, svc.SettleRefund(ctx, paymentID, uuid.MustParse(orderID)))

		var status string
		require.NoError(t, testPool.QueryRow(ctx, `SELECT status FROM orders WHERE id = $1`, orderID).Scan(&status))
		assert.Equal(t, "refunded", status)

		var paymentStatus string
		require.NoError(
			t,
			testPool.QueryRow(ctx, `SELECT status FROM payments WHERE id = $1`, paymentID).Scan(&paymentStatus),
		)
		assert.Equal(t, "refunded", paymentStatus)

		stockAfter, reservedAfter := inventoryLevelOf(t, prodID)
		assert.Equal(t, 100, stockAfter, "cancel already released the hold; the refund must not release it twice")
		assert.Equal(t, 0, reservedAfter)
	})
}
