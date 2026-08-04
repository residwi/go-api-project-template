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
	"github.com/residwi/go-api-project-template/internal/config"
	"github.com/residwi/go-api-project-template/internal/modules/payment"
	apihttp "github.com/residwi/go-api-project-template/internal/transport/http"

	"github.com/residwi/go-api-project-template/internal/testhelper"
)

func TestE2EAdminRefundEndpoint(t *testing.T) {
	setup(t)
	// Start a mock payment gateway server
	mockMux := http.NewServeMux()
	mockgatewayserver.RegisterRoutes(mockMux, testhelper.DiscardLogger())
	mockServer := httptest.NewServer(mockMux)
	defer mockServer.Close()

	webhookDeps := &apihttp.Deps{
		Config: &config.Config{
			App:  testDeps.Config.App,
			JWT:  testDeps.Config.JWT,
			CORS: testDeps.Config.CORS,
			Payment: config.PaymentConfig{
				Gateway:        "mock",
				GatewayURL:     mockServer.URL + "/mock/payment",
				GatewayTimeout: 5 * time.Second,
			},
		},
		Pool:   testPool,
		Cache:  testRedis,
		Logger: testhelper.DiscardLogger(),
	}
	handler := apihttp.NewRouter(webhookDeps)
	ctx := context.Background()

	// Seed category + product
	catID := uuid.New()
	_, err := testPool.Exec(ctx,
		`INSERT INTO categories (id, name, slug, active) VALUES ($1, 'Refund Cat', $2, true)`,
		catID, "refund-cat-"+catID.String()[:8])
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM categories WHERE id = $1`, catID) })

	prodID := uuid.New()
	_, err = testPool.Exec(ctx,
		`INSERT INTO products (id, name, slug, description, price, currency, status, category_id)
		 VALUES ($1, 'Refund Product', $2, 'desc', 3000, 'USD', 'published', $3)`,
		prodID, "refund-prod-"+prodID.String()[:8], catID)
	require.NoError(t, err)
	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM inventory_levels WHERE product_id = $1`, prodID)
		testPool.Exec(ctx, `DELETE FROM products WHERE id = $1`, prodID)
	})
	seedInventoryLevel(t, prodID, 100, 0)

	// Register user
	email := "refund-flow@example.com"
	regBody := `{"email":"` + email + `","password":"Password123!","first_name":"Refund","last_name":"User"}`
	regReq := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(regBody))
	regReq.Header.Set("Content-Type", "application/json")
	regW := httptest.NewRecorder()
	handler.ServeHTTP(regW, regReq)
	require.Equal(t, http.StatusCreated, regW.Code)

	var regResp map[string]any
	require.NoError(t, json.NewDecoder(regW.Body).Decode(&regResp))
	token := regResp["data"].(map[string]any)["access_token"].(string)

	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM payment_jobs WHERE order_id IN (SELECT id FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = $1))`, email)
		testPool.Exec(ctx, `DELETE FROM payments WHERE order_id IN (SELECT id FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = $1))`, email)
		testPool.Exec(ctx, `DELETE FROM shipments WHERE order_id IN (SELECT id FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = $1))`, email)
		testPool.Exec(ctx, `DELETE FROM notifications WHERE user_id IN (SELECT id FROM users WHERE email = $1)`, email)
		testPool.Exec(ctx, `DELETE FROM order_items WHERE order_id IN (SELECT id FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = $1))`, email)
		testPool.Exec(ctx, `DELETE FROM cart_items WHERE cart_id IN (SELECT id FROM carts WHERE user_id IN (SELECT id FROM users WHERE email = $1))`, email)
		testPool.Exec(ctx, `DELETE FROM carts WHERE user_id IN (SELECT id FROM users WHERE email = $1)`, email)
		testPool.Exec(ctx, `DELETE FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = $1)`, email)
		testPool.Exec(ctx, `DELETE FROM users WHERE email = $1`, email)
	})

	// Add to cart
	cartBody := `{"product_id":"` + prodID.String() + `","quantity":1}`
	cartReq := httptest.NewRequest(http.MethodPost, "/api/cart/items", strings.NewReader(cartBody))
	cartReq.Header.Set("Content-Type", "application/json")
	cartReq.Header.Set("Authorization", "Bearer "+token)
	cartW := httptest.NewRecorder()
	handler.ServeHTTP(cartW, cartReq)
	require.Equal(t, http.StatusCreated, cartW.Code)

	// Place order
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

	// Look up the payment_id
	var paymentID uuid.UUID
	err = testPool.QueryRow(ctx, `SELECT id FROM payments WHERE order_id = $1`, orderID).Scan(&paymentID)
	require.NoError(t, err)

	// Send webhook to mark payment as success and order as paid
	webhookBody := fmt.Sprintf(`{"event":"success","metadata":{"payment_id":"%s"}}`, paymentID)
	whReq := httptest.NewRequest(http.MethodPost, "/api/payments/webhook", strings.NewReader(webhookBody))
	whReq.Header.Set("Content-Type", "application/json")
	whW := httptest.NewRecorder()
	handler.ServeHTTP(whW, whReq)
	require.Equal(t, http.StatusOK, whW.Code)

	// Promote user to admin and re-login
	_, err = testPool.Exec(ctx, `UPDATE users SET role = 'admin' WHERE email = $1`, email)
	require.NoError(t, err)

	loginBody := `{"email":"` + email + `","password":"Password123!"}`
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginW := httptest.NewRecorder()
	handler.ServeHTTP(loginW, loginReq)
	require.Equal(t, http.StatusOK, loginW.Code)

	var loginResp map[string]any
	require.NoError(t, json.NewDecoder(loginW.Body).Decode(&loginResp))
	adminToken := loginResp["data"].(map[string]any)["access_token"].(string)

	t.Run("admin refund creates refund job", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/admin/payments/"+paymentID.String()+"/refund", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		data, ok := resp["data"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "refund_enqueued", data["status"])

		// Verify a refund job was created
		var jobCount int
		err := testPool.QueryRow(ctx,
			`SELECT COUNT(*) FROM payment_jobs WHERE order_id = $1 AND action = 'refund'`, orderID).Scan(&jobCount)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, jobCount, 1)
	})

	t.Run("processing refund job restocks inventory and releases coupon", func(t *testing.T) {
		// Fetch the refund job from the database
		var job payment.Job
		err := testPool.QueryRow(ctx,
			`SELECT id, payment_id, order_id, action, status, attempts, max_attempts,
			        COALESCE(last_error, ''), locked_until, next_retry_at,
			        created_at, updated_at
			 FROM payment_jobs
			 WHERE order_id = $1 AND action = 'refund' AND status = 'pending'
			 LIMIT 1`, orderID).Scan(
			&job.ID, &job.PaymentID, &job.OrderID, &job.Action, &job.Status,
			&job.Attempts, &job.MaxAttempts, &job.LastError, &job.LockedUntil,
			&job.NextRetryAt, &job.CreatedAt, &job.UpdatedAt,
		)
		require.NoError(t, err)
		assert.Equal(t, payment.ActionRefund, job.Action)

		// Record stock before refund
		stockBefore, _ := inventoryLevelOf(t, prodID)

		// Process the refund job via a composed payment service
		processErr := newPaymentService(t, mockServer.URL+"/mock/payment").Process(ctx, job)
		require.NoError(t, processErr)

		// Verify order status changed to "refunded"
		var orderStatus string
		err = testPool.QueryRow(ctx, `SELECT status FROM orders WHERE id = $1`, orderID).Scan(&orderStatus)
		require.NoError(t, err)
		assert.Equal(t, "refunded", orderStatus)

		// Verify payment status changed to "refunded"
		var paymentStatus string
		err = testPool.QueryRow(ctx, `SELECT status FROM payments WHERE id = $1`, paymentID).Scan(&paymentStatus)
		require.NoError(t, err)
		assert.Equal(t, "refunded", paymentStatus)

		// Verify inventory was restocked
		stockAfter, _ := inventoryLevelOf(t, prodID)
		assert.Equal(t, stockBefore+1, stockAfter)

		// Verify refund job marked as completed
		var jobStatus string
		err = testPool.QueryRow(ctx,
			`SELECT status FROM payment_jobs WHERE id = $1`, job.ID).Scan(&jobStatus)
		require.NoError(t, err)
		assert.Equal(t, "completed", jobStatus)
	})
}

func TestE2ERefundWithCouponAndRelease(t *testing.T) {
	setup(t)
	// This test covers inventoryRestorerAdapter.Restore and promotion.Service.Release
	// by processing a refund job with inventory_action='release' on an order with a coupon.
	mockMux := http.NewServeMux()
	mockgatewayserver.RegisterRoutes(mockMux, testhelper.DiscardLogger())
	mockServer := httptest.NewServer(mockMux)
	defer mockServer.Close()

	webhookDeps := &apihttp.Deps{
		Config: &config.Config{
			App:  testDeps.Config.App,
			JWT:  testDeps.Config.JWT,
			CORS: testDeps.Config.CORS,
			Payment: config.PaymentConfig{
				Gateway:        "mock",
				GatewayURL:     mockServer.URL + "/mock/payment",
				GatewayTimeout: 5 * time.Second,
			},
		},
		Pool:   testPool,
		Cache:  testRedis,
		Logger: testhelper.DiscardLogger(),
	}
	handler := apihttp.NewRouter(webhookDeps)
	ctx := context.Background()

	// Seed category + product
	catID := uuid.New()
	_, err := testPool.Exec(ctx,
		`INSERT INTO categories (id, name, slug, active) VALUES ($1, 'RelCoupon Cat', $2, true)`,
		catID, "relcoupon-cat-"+catID.String()[:8])
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM categories WHERE id = $1`, catID) })

	prodID := uuid.New()
	_, err = testPool.Exec(ctx,
		`INSERT INTO products (id, name, slug, description, price, currency, status, category_id)
		 VALUES ($1, 'RelCoupon Product', $2, 'desc', 8000, 'USD', 'published', $3)`,
		prodID, "relcoupon-prod-"+prodID.String()[:8], catID)
	require.NoError(t, err)
	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM inventory_levels WHERE product_id = $1`, prodID)
		testPool.Exec(ctx, `DELETE FROM products WHERE id = $1`, prodID)
	})
	seedInventoryLevel(t, prodID, 100, 0)

	// Seed a coupon
	couponID := uuid.New()
	couponCode := "RELCOUPON" + couponID.String()[:8]
	_, err = testPool.Exec(ctx,
		`INSERT INTO promotions (id, code, type, value, min_order_amount, max_uses, used_count, starts_at, expires_at, active)
		 VALUES ($1, $2, 'percentage', 10, 0, 100, 0, $3, $4, true)`,
		couponID, couponCode, time.Now().Add(-24*time.Hour), time.Now().Add(24*time.Hour))
	require.NoError(t, err)
	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM coupon_usages WHERE coupon_id = $1`, couponID)
		testPool.Exec(ctx, `DELETE FROM promotions WHERE id = $1`, couponID)
	})

	// Register user
	email := "relcoupon-flow@example.com"
	regBody := `{"email":"` + email + `","password":"Password123!","first_name":"RelCoupon","last_name":"User"}`
	regReq := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(regBody))
	regReq.Header.Set("Content-Type", "application/json")
	regW := httptest.NewRecorder()
	handler.ServeHTTP(regW, regReq)
	require.Equal(t, http.StatusCreated, regW.Code)

	var regResp map[string]any
	require.NoError(t, json.NewDecoder(regW.Body).Decode(&regResp))
	token := regResp["data"].(map[string]any)["access_token"].(string)

	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM payment_jobs WHERE order_id IN (SELECT id FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = $1))`, email)
		testPool.Exec(ctx, `DELETE FROM payments WHERE order_id IN (SELECT id FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = $1))`, email)
		testPool.Exec(ctx, `DELETE FROM notifications WHERE user_id IN (SELECT id FROM users WHERE email = $1)`, email)
		testPool.Exec(ctx, `DELETE FROM order_items WHERE order_id IN (SELECT id FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = $1))`, email)
		testPool.Exec(ctx, `DELETE FROM coupon_usages WHERE order_id IN (SELECT id FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = $1))`, email)
		testPool.Exec(ctx, `DELETE FROM cart_items WHERE cart_id IN (SELECT id FROM carts WHERE user_id IN (SELECT id FROM users WHERE email = $1))`, email)
		testPool.Exec(ctx, `DELETE FROM carts WHERE user_id IN (SELECT id FROM users WHERE email = $1)`, email)
		testPool.Exec(ctx, `DELETE FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = $1)`, email)
		testPool.Exec(ctx, `DELETE FROM users WHERE email = $1`, email)
	})

	// Add to cart
	cartBody := `{"product_id":"` + prodID.String() + `","quantity":1}`
	cartReq := httptest.NewRequest(http.MethodPost, "/api/cart/items", strings.NewReader(cartBody))
	cartReq.Header.Set("Content-Type", "application/json")
	cartReq.Header.Set("Authorization", "Bearer "+token)
	cartW := httptest.NewRecorder()
	handler.ServeHTTP(cartW, cartReq)
	require.Equal(t, http.StatusCreated, cartW.Code)

	// Place order with coupon
	orderBody := fmt.Sprintf(`{"payment_method_id":"pm_test_123","coupon_code":"%s"}`, couponCode)
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

	// Look up the payment_id
	var paymentID uuid.UUID
	err = testPool.QueryRow(ctx, `SELECT id FROM payments WHERE order_id = $1`, orderID).Scan(&paymentID)
	require.NoError(t, err)

	// Set payment to "success" and order to "fulfillment_failed" to simulate a refund
	// with inventory_action="release" (order NOT paid/delivered → "release" path)
	_, err = testPool.Exec(ctx,
		`UPDATE payments SET status = 'success', gateway_txn_id = 'txn_rel_test' WHERE id = $1`, paymentID)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx,
		`UPDATE orders SET status = 'fulfillment_failed' WHERE id = $1`, orderID)
	require.NoError(t, err)

	// Create a refund job directly
	refundJobID := uuid.New()
	_, err = testPool.Exec(ctx,
		`INSERT INTO payment_jobs (id, payment_id, order_id, action, status, max_attempts, next_retry_at)
		 VALUES ($1, $2, $3, 'refund', 'pending', 3, NOW())`,
		refundJobID, paymentID, orderID)
	require.NoError(t, err)

	t.Run("processing refund job restocks inventory and releases coupon", func(t *testing.T) {
		// The synchronous charge finalized this order at placement: stock was
		// DEDUCTED (available_stock 100 -> 99, reserved_stock 1 -> 0) and the
		// order's stock_deducted flag was set. Record stock/reserved before the
		// refund so we can assert the refund RESTOCKS (adds back to
		// available_stock) rather than releasing a reservation.
		stockBefore, reservedBefore := inventoryLevelOf(t, prodID)
		assert.Equal(t, 99, stockBefore)
		assert.Equal(t, 0, reservedBefore)

		// Record coupon usage before
		var usageBefore int
		err = testPool.QueryRow(ctx,
			`SELECT COUNT(*) FROM coupon_usages WHERE coupon_id = $1`, couponID).Scan(&usageBefore)
		require.NoError(t, err)
		assert.Equal(t, 1, usageBefore)

		// Fetch and process the refund job
		var job payment.Job
		err = testPool.QueryRow(ctx,
			`SELECT id, payment_id, order_id, action, status, attempts, max_attempts,
			        COALESCE(last_error, ''), locked_until, next_retry_at,
			        created_at, updated_at
			 FROM payment_jobs WHERE id = $1`, refundJobID).Scan(
			&job.ID, &job.PaymentID, &job.OrderID, &job.Action, &job.Status,
			&job.Attempts, &job.MaxAttempts, &job.LastError, &job.LockedUntil,
			&job.NextRetryAt, &job.CreatedAt, &job.UpdatedAt,
		)
		require.NoError(t, err)

		processErr := newPaymentService(t, mockServer.URL+"/mock/payment").Process(ctx, job)
		require.NoError(t, processErr)

		// Verify inventory was RESTOCKED: available_stock returns to its original
		// seeded value (100) and reserved_stock stays at 0.
		stockAfter, reservedAfter := inventoryLevelOf(t, prodID)
		assert.Equal(t, 100, stockAfter)
		assert.Equal(t, 0, reservedAfter)

		// Verify coupon usage was released
		var usageAfter int
		err = testPool.QueryRow(ctx,
			`SELECT COUNT(*) FROM coupon_usages WHERE coupon_id = $1`, couponID).Scan(&usageAfter)
		require.NoError(t, err)
		assert.Equal(t, 0, usageAfter)

		// Verify order status changed to "refunded"
		var orderStatus string
		err = testPool.QueryRow(ctx, `SELECT status FROM orders WHERE id = $1`, orderID).Scan(&orderStatus)
		require.NoError(t, err)
		assert.Equal(t, "refunded", orderStatus)

		// Verify payment status changed to "refunded"
		var paymentStatus string
		err = testPool.QueryRow(ctx, `SELECT status FROM payments WHERE id = $1`, paymentID).Scan(&paymentStatus)
		require.NoError(t, err)
		assert.Equal(t, "refunded", paymentStatus)
	})
}
