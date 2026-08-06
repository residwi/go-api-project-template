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
	"github.com/residwi/go-api-project-template/internal/modules/payment"
	apihttp "github.com/residwi/go-api-project-template/internal/transport/http"

	"github.com/residwi/go-api-project-template/internal/testhelper"
)

func TestE2EPaymentWebhookFlow(t *testing.T) {
	setup(t)
	mockMux := http.NewServeMux()
	mockgatewayserver.RegisterRoutes(mockMux, testhelper.DiscardLogger())
	mockServer := httptest.NewServer(mockMux)
	defer mockServer.Close()

	customPaymentCfg := payment.Config{
		Gateway:        "mock",
		GatewayURL:     mockServer.URL + "/mock/payment",
		GatewayTimeout: 5 * time.Second,
	}
	webhookDeps := &apihttp.Deps{
		Infra:   testDeps.Infra,
		Auth:    testDeps.Auth,
		Order:   testDeps.Order,
		Payment: customPaymentCfg,
		Pool:    testPool,
		Cache:   testRedis,
		Logger:  testhelper.DiscardLogger(),
	}
	handler := apihttp.NewRouter(webhookDeps, newTestApp(customPaymentCfg))
	ctx := context.Background()

	catID := uuid.New()
	_, err := testPool.Exec(ctx,
		`INSERT INTO categories (id, name, slug, active) VALUES ($1, 'Webhook Cat', $2, true)`,
		catID, "webhook-cat-"+catID.String()[:8])
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM categories WHERE id = $1`, catID) })

	prodID := uuid.New()
	_, err = testPool.Exec(ctx,
		`INSERT INTO products (id, name, slug, description, price, currency, status, category_id)
		 VALUES ($1, 'Webhook Product', $2, 'desc', 5000, 'USD', 'published', $3)`,
		prodID, "webhook-prod-"+prodID.String()[:8], catID)
	require.NoError(t, err)
	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM inventory_levels WHERE product_id = $1`, prodID)
		testPool.Exec(ctx, `DELETE FROM products WHERE id = $1`, prodID)
	})
	seedInventoryLevel(t, prodID, 100, 0)

	email := "webhook-flow@example.com"
	regBody := `{"email":"` + email + `","password":"Password123!","first_name":"Webhook","last_name":"User"}`
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
			`DELETE FROM payment_jobs WHERE order_id IN (SELECT id FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = $1))`,
			email,
		)
		testPool.Exec(
			ctx,
			`DELETE FROM payments WHERE order_id IN (SELECT id FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = $1))`,
			email,
		)
		testPool.Exec(
			ctx,
			`DELETE FROM shipments WHERE order_id IN (SELECT id FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = $1))`,
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

	cartBody := `{"product_id":"` + prodID.String() + `","quantity":2}`
	cartReq := httptest.NewRequest(http.MethodPost, "/api/cart/items", strings.NewReader(cartBody))
	cartReq.Header.Set("Content-Type", "application/json")
	cartReq.Header.Set("Authorization", "Bearer "+token)
	cartW := httptest.NewRecorder()
	handler.ServeHTTP(cartW, cartReq)
	require.Equal(t, http.StatusCreated, cartW.Code)

	// Place order (pm_test_123 triggers direct charge success in mock)
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
	data := orderResp["data"].(map[string]any)
	orderID := data["order"].(map[string]any)["id"].(string)

	var paymentID uuid.UUID
	err = testPool.QueryRow(ctx, `SELECT id FROM payments WHERE order_id = $1`, orderID).Scan(&paymentID)
	require.NoError(t, err)

	t.Run("webhook success updates order to paid", func(t *testing.T) {
		webhookBody := fmt.Sprintf(
			`{"event":"success","metadata":{"payment_id":"%s"},"transaction_id":"txn_test"}`,
			paymentID,
		)
		req := httptest.NewRequest(http.MethodPost, "/api/payments/webhook", strings.NewReader(webhookBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		var orderStatus string
		err := testPool.QueryRow(ctx, `SELECT status FROM orders WHERE id = $1`, orderID).Scan(&orderStatus)
		require.NoError(t, err)
		assert.Equal(t, "paid", orderStatus)

		var paymentStatus string
		err = testPool.QueryRow(ctx, `SELECT status FROM payments WHERE id = $1`, paymentID).Scan(&paymentStatus)
		require.NoError(t, err)
		assert.Equal(t, "success", paymentStatus)
	})
}

func TestE2EPaymentFailedWebhookFlow(t *testing.T) {
	setup(t)
	mockMux := http.NewServeMux()
	mockgatewayserver.RegisterRoutes(mockMux, testhelper.DiscardLogger())
	mockServer := httptest.NewServer(mockMux)
	defer mockServer.Close()

	customPaymentCfg := payment.Config{
		Gateway:        "mock",
		GatewayURL:     mockServer.URL + "/mock/payment",
		GatewayTimeout: 5 * time.Second,
	}
	deps := &apihttp.Deps{
		Infra:   testDeps.Infra,
		Auth:    testDeps.Auth,
		Order:   testDeps.Order,
		Payment: customPaymentCfg,
		Pool:    testPool,
		Cache:   testRedis,
		Logger:  testhelper.DiscardLogger(),
	}
	handler := apihttp.NewRouter(deps, newTestApp(customPaymentCfg))
	ctx := context.Background()

	catID := uuid.New()
	_, err := testPool.Exec(ctx,
		`INSERT INTO categories (id, name, slug, active) VALUES ($1, 'Fail Cat', $2, true)`,
		catID, "fail-cat-"+catID.String()[:8])
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM categories WHERE id = $1`, catID) })

	prodID := uuid.New()
	_, err = testPool.Exec(ctx,
		`INSERT INTO products (id, name, slug, description, price, currency, status, category_id)
		 VALUES ($1, 'Fail Product', $2, 'desc', 2000, 'USD', 'published', $3)`,
		prodID, "fail-prod-"+prodID.String()[:8], catID)
	require.NoError(t, err)
	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM inventory_levels WHERE product_id = $1`, prodID)
		testPool.Exec(ctx, `DELETE FROM products WHERE id = $1`, prodID)
	})
	seedInventoryLevel(t, prodID, 50, 0)

	// 2000*2 + 99 = 4099, and 99 mod 100 makes the mock gateway decline: the order
	// stays awaiting_payment with stock only reserved, never deducted. prodID's
	// quantity stays 2 for the reserved_stock assertion below.
	prod2ID := uuid.New()
	_, err = testPool.Exec(ctx,
		`INSERT INTO products (id, name, slug, description, price, currency, status, category_id)
		 VALUES ($1, 'Fail Product 2', $2, 'desc', 99, 'USD', 'published', $3)`,
		prod2ID, "fail-prod2-"+prod2ID.String()[:8], catID)
	require.NoError(t, err)
	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM inventory_levels WHERE product_id = $1`, prod2ID)
		testPool.Exec(ctx, `DELETE FROM products WHERE id = $1`, prod2ID)
	})
	seedInventoryLevel(t, prod2ID, 50, 0)

	email := "fail-flow@example.com"
	regBody := `{"email":"` + email + `","password":"Password123!","first_name":"Fail","last_name":"User"}`
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
			`DELETE FROM payment_jobs WHERE order_id IN (SELECT id FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = $1))`,
			email,
		)
		testPool.Exec(
			ctx,
			`DELETE FROM payments WHERE order_id IN (SELECT id FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = $1))`,
			email,
		)
		testPool.Exec(
			ctx,
			`DELETE FROM shipments WHERE order_id IN (SELECT id FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = $1))`,
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

	// prodID qty 2 plus prod2ID qty 1 is the 4099 total.
	cartBody := `{"product_id":"` + prodID.String() + `","quantity":2}`
	cartReq := httptest.NewRequest(http.MethodPost, "/api/cart/items", strings.NewReader(cartBody))
	cartReq.Header.Set("Content-Type", "application/json")
	cartReq.Header.Set("Authorization", "Bearer "+token)
	cartW := httptest.NewRecorder()
	handler.ServeHTTP(cartW, cartReq)
	require.Equal(t, http.StatusCreated, cartW.Code)

	cart2Body := `{"product_id":"` + prod2ID.String() + `","quantity":1}`
	cart2Req := httptest.NewRequest(http.MethodPost, "/api/cart/items", strings.NewReader(cart2Body))
	cart2Req.Header.Set("Content-Type", "application/json")
	cart2Req.Header.Set("Authorization", "Bearer "+token)
	cart2W := httptest.NewRecorder()
	handler.ServeHTTP(cart2W, cart2Req)
	require.Equal(t, http.StatusCreated, cart2W.Code)

	// Place order. Total is 4099 (%100 == 99) so the synchronous charge fails and
	// the order stays awaiting_payment with a pending payment.
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

	t.Run("webhook failed cancels payment and jobs", func(t *testing.T) {
		// Before the webhook: the synchronous charge failed (total ended in 99),
		// so the order is still awaiting_payment with a pending payment and the
		// stock is only RESERVED (prodID's reserved_stock == 2).
		_, reservedBefore := inventoryLevelOf(t, prodID)
		assert.Equal(t, 2, reservedBefore)

		var orderStatusBefore string
		err = testPool.QueryRow(ctx, `SELECT status FROM orders WHERE id = $1`, orderID).Scan(&orderStatusBefore)
		require.NoError(t, err)
		assert.Equal(t, "awaiting_payment", orderStatusBefore)

		var paymentStatusBefore string
		err = testPool.QueryRow(ctx, `SELECT status FROM payments WHERE id = $1`, paymentID).Scan(&paymentStatusBefore)
		require.NoError(t, err)
		assert.Equal(t, "pending", paymentStatusBefore)

		webhookBody := fmt.Sprintf(
			`{"event":"failed","metadata":{"payment_id":"%s"},"transaction_id":"txn_fail"}`,
			paymentID,
		)
		req := httptest.NewRequest(http.MethodPost, "/api/payments/webhook", strings.NewReader(webhookBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		var paymentStatus string
		err = testPool.QueryRow(ctx, `SELECT status FROM payments WHERE id = $1`, paymentID).Scan(&paymentStatus)
		require.NoError(t, err)
		assert.Equal(t, "cancelled", paymentStatus)

		var pendingJobs int
		err = testPool.QueryRow(
			ctx,
			`SELECT COUNT(*) FROM payment_jobs WHERE order_id = $1 AND status IN ('pending','processing')`,
			orderID,
		).Scan(&pendingJobs)
		require.NoError(t, err)
		assert.Equal(t, 0, pendingJobs)

		// A failed webhook cancels the order and releases its reservation too, not just
		// the payment row.
		var orderStatus string
		err = testPool.QueryRow(ctx, `SELECT status FROM orders WHERE id = $1`, orderID).Scan(&orderStatus)
		require.NoError(t, err)
		assert.Equal(t, "cancelled", orderStatus)

		_, reservedAfter := inventoryLevelOf(t, prodID)
		assert.Equal(t, 0, reservedAfter)
	})
}
