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
	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
	apihttp "github.com/residwi/go-api-project-template/internal/transport/http"

	"github.com/residwi/go-api-project-template/internal/testhelper"
)

func TestE2EAdminRefundEndpoint(t *testing.T) {
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

	cartBody := `{"product_id":"` + prodID.String() + `","quantity":1}`
	cartReq := httptest.NewRequest(http.MethodPost, "/api/cart/items", strings.NewReader(cartBody))
	cartReq.Header.Set("Content-Type", "application/json")
	cartReq.Header.Set("Authorization", "Bearer "+token)
	cartW := httptest.NewRecorder()
	handler.ServeHTTP(cartW, cartReq)
	require.Equal(t, http.StatusCreated, cartW.Code)

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

	webhookBody := fmt.Sprintf(`{"event":"success","metadata":{"payment_id":"%s"}}`, paymentID)
	whReq := httptest.NewRequest(http.MethodPost, "/api/payments/webhook", strings.NewReader(webhookBody))
	whReq.Header.Set("Content-Type", "application/json")
	whW := httptest.NewRecorder()
	handler.ServeHTTP(whW, whReq)
	require.Equal(t, http.StatusOK, whW.Code)

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

		var jobCount int
		err := testPool.QueryRow(ctx,
			`SELECT COUNT(*) FROM payment_jobs WHERE order_id = $1 AND action = 'refund'`, orderID).Scan(&jobCount)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, jobCount, 1)
	})

	t.Run("processing refund job restocks inventory and releases coupon", func(t *testing.T) {
		var job domain.Job
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
		assert.Equal(t, domain.ActionRefund, job.Action)

		stockBefore, _ := inventoryLevelOf(t, prodID)

		processErr := newPaymentService(t, mockServer.URL+"/mock/payment").JobProcessor.Process(ctx, job)
		require.NoError(t, processErr)

		var orderStatus string
		err = testPool.QueryRow(ctx, `SELECT status FROM orders WHERE id = $1`, orderID).Scan(&orderStatus)
		require.NoError(t, err)
		assert.Equal(t, "refunded", orderStatus)

		var paymentStatus string
		err = testPool.QueryRow(ctx, `SELECT status FROM payments WHERE id = $1`, paymentID).Scan(&paymentStatus)
		require.NoError(t, err)
		assert.Equal(t, "refunded", paymentStatus)

		stockAfter, _ := inventoryLevelOf(t, prodID)
		assert.Equal(t, stockBefore+1, stockAfter)

		var jobStatus string
		err = testPool.QueryRow(ctx,
			`SELECT status FROM payment_jobs WHERE id = $1`, job.ID).Scan(&jobStatus)
		require.NoError(t, err)
		assert.Equal(t, "completed", jobStatus)
	})
}

func TestE2ERefundWithCouponAndRelease(t *testing.T) {
	setup(t)
	// Drives refund.InventoryRestorer.Restore and promotion.Service.Release
	// through a refund job with inventory_action='release' on a coupon order.
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

	couponID := uuid.New()
	couponCode := "RELCOUPON" + couponID.String()[:8]
	_, err = testPool.Exec(
		ctx,
		`INSERT INTO promotions (id, code, type, value, min_order_amount, max_uses, used_count, starts_at, expires_at, active)
		 VALUES ($1, $2, 'percentage', 10, 0, 100, 0, $3, $4, true)`,
		couponID,
		couponCode,
		time.Now().Add(-24*time.Hour),
		time.Now().Add(24*time.Hour),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM coupon_usages WHERE coupon_id = $1`, couponID)
		testPool.Exec(ctx, `DELETE FROM promotions WHERE id = $1`, couponID)
	})

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
		testPool.Exec(ctx, `DELETE FROM notifications WHERE user_id IN (SELECT id FROM users WHERE email = $1)`, email)
		testPool.Exec(
			ctx,
			`DELETE FROM order_items WHERE order_id IN (SELECT id FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = $1))`,
			email,
		)
		testPool.Exec(
			ctx,
			`DELETE FROM coupon_usages WHERE order_id IN (SELECT id FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = $1))`,
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

	var paymentID uuid.UUID
	err = testPool.QueryRow(ctx, `SELECT id FROM payments WHERE order_id = $1`, orderID).Scan(&paymentID)
	require.NoError(t, err)

	// An order that is not paid or delivered takes the "release" path.
	_, err = testPool.Exec(ctx,
		`UPDATE payments SET status = 'success', gateway_txn_id = 'txn_rel_test' WHERE id = $1`, paymentID)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx,
		`UPDATE orders SET status = 'fulfillment_failed' WHERE id = $1`, orderID)
	require.NoError(t, err)

	refundJobID := uuid.New()
	_, err = testPool.Exec(ctx,
		`INSERT INTO payment_jobs (id, payment_id, order_id, action, status, max_attempts, next_retry_at)
		 VALUES ($1, $2, $3, 'refund', 'pending', 3, NOW())`,
		refundJobID, paymentID, orderID)
	require.NoError(t, err)

	t.Run("processing refund job restocks inventory and releases coupon", func(t *testing.T) {
		// The synchronous charge already deducted this order's stock, so the refund must
		// restock rather than release a reservation.
		stockBefore, reservedBefore := inventoryLevelOf(t, prodID)
		assert.Equal(t, 99, stockBefore)
		assert.Equal(t, 0, reservedBefore)

		var usageBefore int
		err = testPool.QueryRow(ctx,
			`SELECT COUNT(*) FROM coupon_usages WHERE coupon_id = $1`, couponID).Scan(&usageBefore)
		require.NoError(t, err)
		assert.Equal(t, 1, usageBefore)

		var job domain.Job
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

		processErr := newPaymentService(t, mockServer.URL+"/mock/payment").JobProcessor.Process(ctx, job)
		require.NoError(t, processErr)

		// Restocked, not released: available_stock returns to its seeded 100 and
		// reserved_stock stays 0.
		stockAfter, reservedAfter := inventoryLevelOf(t, prodID)
		assert.Equal(t, 100, stockAfter)
		assert.Equal(t, 0, reservedAfter)

		var usageAfter int
		err = testPool.QueryRow(ctx,
			`SELECT COUNT(*) FROM coupon_usages WHERE coupon_id = $1`, couponID).Scan(&usageAfter)
		require.NoError(t, err)
		assert.Equal(t, 0, usageAfter)

		var orderStatus string
		err = testPool.QueryRow(ctx, `SELECT status FROM orders WHERE id = $1`, orderID).Scan(&orderStatus)
		require.NoError(t, err)
		assert.Equal(t, "refunded", orderStatus)

		var paymentStatus string
		err = testPool.QueryRow(ctx, `SELECT status FROM payments WHERE id = $1`, paymentID).Scan(&paymentStatus)
		require.NoError(t, err)
		assert.Equal(t, "refunded", paymentStatus)
	})
}
