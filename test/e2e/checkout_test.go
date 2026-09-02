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

func TestE2EOrderFlow(t *testing.T) {
	setup(t)
	handler := newTestRouter(testPaymentCfg)
	ctx := context.Background()

	catID := uuid.New()
	_, err := testPool.Exec(ctx,
		`INSERT INTO categories (id, name, slug, active) VALUES ($1, 'E2E Cat', $2, true)`,
		catID, "e2e-cat-"+catID.String()[:8])
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM categories WHERE id = $1`, catID) })

	prodID := uuid.New()
	_, err = testPool.Exec(ctx,
		`INSERT INTO products (id, name, slug, description, price, currency, status, category_id)
		 VALUES ($1, 'E2E Product', $2, 'desc', 5000, 'USD', 'published', $3)`,
		prodID, "e2e-prod-"+prodID.String()[:8], catID)
	require.NoError(t, err)
	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM inventory_levels WHERE product_id = $1`, prodID)
		testPool.Exec(ctx, `DELETE FROM products WHERE id = $1`, prodID)
	})

	// Ensure product has stock: inventory reserves against inventory_levels, the
	// products table no longer carries a stock column at all.
	seedInventoryLevel(t, prodID, 100, 0)

	regBody := `{"email":"e2e-flow@example.com","password":"Password123!","first_name":"E2E","last_name":"User"}`
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
			`DELETE FROM cart_items WHERE cart_id IN (SELECT id FROM carts WHERE user_id IN (SELECT id FROM users WHERE email = 'e2e-flow@example.com'))`,
		)
		testPool.Exec(
			ctx,
			`DELETE FROM carts WHERE user_id IN (SELECT id FROM users WHERE email = 'e2e-flow@example.com')`,
		)
		testPool.Exec(
			ctx,
			`DELETE FROM order_items WHERE order_id IN (SELECT id FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = 'e2e-flow@example.com'))`,
		)
		testPool.Exec(
			ctx,
			`DELETE FROM payments WHERE order_id IN (SELECT id FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = 'e2e-flow@example.com'))`,
		)
		testPool.Exec(
			ctx,
			`DELETE FROM notifications WHERE user_id IN (SELECT id FROM users WHERE email = 'e2e-flow@example.com')`,
		)
		testPool.Exec(
			ctx,
			`DELETE FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = 'e2e-flow@example.com')`,
		)
		testPool.Exec(ctx, `DELETE FROM users WHERE email = 'e2e-flow@example.com'`)
	})

	t.Run("add item to cart", func(t *testing.T) {
		body := `{"product_id":"` + prodID.String() + `","quantity":2}`
		req := httptest.NewRequest(http.MethodPost, "/api/cart/items", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusCreated, w.Code)
	})

	t.Run("get cart", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/cart", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("place order", func(t *testing.T) {
		body := `{"payment_method_id":"pm_test_123"}`
		req := httptest.NewRequest(http.MethodPost, "/api/orders", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Idempotency-Key", uuid.New().String())
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusCreated, w.Code)
	})

	t.Run("list orders", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/orders", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestE2ECancelOrderFlow(t *testing.T) {
	setup(t)
	handler := newTestRouter(testPaymentCfg)
	ctx := context.Background()

	catID := uuid.New()
	_, err := testPool.Exec(ctx,
		`INSERT INTO categories (id, name, slug, active) VALUES ($1, 'Cancel Cat', $2, true)`,
		catID, "cancel-cat-"+catID.String()[:8])
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM categories WHERE id = $1`, catID) })

	prodID := uuid.New()
	_, err = testPool.Exec(ctx,
		`INSERT INTO products (id, name, slug, description, price, currency, status, category_id)
		 VALUES ($1, 'Cancel Product', $2, 'desc', 3000, 'USD', 'published', $3)`,
		prodID, "cancel-prod-"+prodID.String()[:8], catID)
	require.NoError(t, err)
	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM inventory_levels WHERE product_id = $1`, prodID)
		testPool.Exec(ctx, `DELETE FROM products WHERE id = $1`, prodID)
	})
	seedInventoryLevel(t, prodID, 50, 0)

	email := "cancel-flow@example.com"
	regBody := `{"email":"` + email + `","password":"Password123!","first_name":"Cancel","last_name":"User"}`
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
	data := orderResp["data"].(map[string]any)
	orderID := data["order"].(map[string]any)["id"].(string)

	t.Run("cancel order releases inventory and cancels payment jobs", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/orders/"+orderID+"/cancel", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNoContent, w.Code)
	})
}

func TestE2ECouponOrderFlow(t *testing.T) {
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
		`INSERT INTO categories (id, name, slug, active) VALUES ($1, 'Coupon Cat', $2, true)`,
		catID, "coupon-cat-"+catID.String()[:8])
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM categories WHERE id = $1`, catID) })

	prodID := uuid.New()
	// 1110 less 10% is 999, and 99 mod 100 makes the mock gateway decline: the order
	// stays awaiting_payment, which is the only state it can be cancelled from.
	_, err = testPool.Exec(ctx,
		`INSERT INTO products (id, name, slug, description, price, currency, status, category_id)
		 VALUES ($1, 'Coupon Product', $2, 'desc', 1110, 'USD', 'published', $3)`,
		prodID, "coupon-prod-"+prodID.String()[:8], catID)
	require.NoError(t, err)
	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM inventory_levels WHERE product_id = $1`, prodID)
		testPool.Exec(ctx, `DELETE FROM products WHERE id = $1`, prodID)
	})
	seedInventoryLevel(t, prodID, 50, 0)

	couponID := uuid.New()
	couponCode := "TESTCOUPON" + couponID.String()[:8]
	maxUses := 100
	_, err = testPool.Exec(
		ctx,
		`INSERT INTO promotions (id, code, type, value, min_order_amount, max_uses, used_count, starts_at, expires_at, active)
		 VALUES ($1, $2, 'percentage', 10, 0, $3, 0, $4, $5, true)`,
		couponID,
		couponCode,
		maxUses,
		time.Now().Add(-24*time.Hour),
		time.Now().Add(24*time.Hour),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM coupon_usages WHERE coupon_id = $1`, couponID)
		testPool.Exec(ctx, `DELETE FROM promotions WHERE id = $1`, couponID)
	})

	email := "coupon-flow@example.com"
	regBody := `{"email":"` + email + `","password":"Password123!","first_name":"Coupon","last_name":"User"}`
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
		testPool.Exec(
			ctx,
			`DELETE FROM coupon_usages WHERE order_id IN (SELECT id FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = $1))`,
			email,
		)
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

	// coupon_code drives promotion.Service.Reserve through place.CouponReserver.
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
	data := orderResp["data"].(map[string]any)
	orderData := data["order"].(map[string]any)
	orderID := orderData["id"].(string)

	t.Run("order has coupon applied with discount", func(t *testing.T) {
		// Product price 1110 x1, 10% coupon -> discount 111, total 999.
		assert.Equal(t, couponCode, orderData["coupon_code"])
		assert.InDelta(t, float64(1110), orderData["subtotal_amount"], 0.0001)
		assert.InDelta(t, float64(111), orderData["discount_amount"], 0.0001)
		assert.InDelta(t, float64(999), orderData["total_amount"], 0.0001)
	})

	t.Run("cancel order releases coupon", func(t *testing.T) {
		// Cancelling drives promotion.Service.Release through the same port.
		req := httptest.NewRequest(http.MethodPost, "/api/orders/"+orderID+"/cancel", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNoContent, w.Code)

		var usageCount int
		err := testPool.QueryRow(ctx,
			`SELECT COUNT(*) FROM coupon_usages WHERE coupon_id = $1`, couponID).Scan(&usageCount)
		require.NoError(t, err)
		assert.Equal(t, 0, usageCount)

		var usedCount int
		err = testPool.QueryRow(ctx,
			`SELECT used_count FROM promotions WHERE id = $1`, couponID).Scan(&usedCount)
		require.NoError(t, err)
		assert.Equal(t, 0, usedCount)
	})
}

// TestE2ERetryPayment is the endpoint's first e2e coverage. Nothing else in
// this suite ever drove POST /api/orders/{id}/pay through the real router,
// which is exactly how a refactor moved it from order to checkout, wired it
// to order's Snapshot projection, and left it 404-ing for every real
// caller while every mock-backed unit test stayed green.
func TestE2ERetryPayment(t *testing.T) {
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

	ownerID, ownerToken := registerE2EUser(t, handler, "retry-owner@example.com")
	_, otherToken := registerE2EUser(t, handler, "retry-other@example.com")

	// Seeded directly rather than through POST /api/orders: against this
	// working gateway, place's own payment leg would succeed synchronously and
	// the order would never sit in awaiting_payment for retry to act on.
	orderID := seedAwaitingPaymentOrder(t, ownerID)
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM payments WHERE order_id = $1`, orderID) })

	body := `{"payment_method_id":"pm_test_retry"}`

	t.Run("a different user retrying someone else's order gets 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/orders/"+orderID.String()+"/pay", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+otherToken)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("the owner retries payment and it succeeds", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/orders/"+orderID.String()+"/pay", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+ownerToken)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		assert.Equal(t, "paid", orderStatusOf(t, orderID))
	})
}

// registerE2EUser drives POST /api/auth/register through the real handler and
// returns the new user's id alongside its access token. Every other e2e test
// only ever reads its own token back; this test needs a second user's id to
// seed an order it does not own.
func registerE2EUser(t *testing.T, handler http.Handler, email string) (uuid.UUID, string) {
	t.Helper()

	body := `{"email":"` + email + `","password":"Password123!","first_name":"E2E","last_name":"User"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	data := resp["data"].(map[string]any)
	user := data["user"].(map[string]any)

	id, err := uuid.Parse(user["id"].(string))
	require.NoError(t, err)

	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM users WHERE email = $1`, email) })

	return id, data["access_token"].(string)
}
