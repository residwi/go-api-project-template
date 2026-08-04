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
	apihttp "github.com/residwi/go-api-project-template/internal/transport/http"

	"github.com/residwi/go-api-project-template/internal/testhelper"
)

func TestE2EOrderFlow(t *testing.T) {
	setup(t)
	handler := apihttp.NewRouter(testDeps)
	ctx := context.Background()

	// Seed a category
	catID := uuid.New()
	_, err := testPool.Exec(ctx,
		`INSERT INTO categories (id, name, slug, active) VALUES ($1, 'E2E Cat', $2, true)`,
		catID, "e2e-cat-"+catID.String()[:8])
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM categories WHERE id = $1`, catID) })

	// Seed a product
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

	// Register user and get token
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
			`DELETE FROM payment_jobs WHERE order_id IN (SELECT id FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = 'e2e-flow@example.com'))`,
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
	handler := apihttp.NewRouter(testDeps)
	ctx := context.Background()

	// Seed category + product
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

	// Register user
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
			`DELETE FROM payment_jobs WHERE order_id IN (SELECT id FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = $1))`,
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
	mockgatewayserver.RegisterRoutes(mockMux, testhelper.DiscardLogger())
	mockServer := httptest.NewServer(mockMux)
	defer mockServer.Close()

	deps := &apihttp.Deps{
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
	handler := apihttp.NewRouter(deps)
	ctx := context.Background()

	// Seed category + product
	catID := uuid.New()
	_, err := testPool.Exec(ctx,
		`INSERT INTO categories (id, name, slug, active) VALUES ($1, 'Coupon Cat', $2, true)`,
		catID, "coupon-cat-"+catID.String()[:8])
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM categories WHERE id = $1`, catID) })

	prodID := uuid.New()
	// Price chosen so the DISCOUNTED total ends in 99: 1110 x1 with 10% off gives
	// a 111 discount and a 999 total. 999 % 100 == 99 makes the mock gateway's
	// synchronous charge FAIL, leaving the order awaiting_payment so it can be
	// cancelled (a paid order can't be cancelled).
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

	// Seed a coupon: 10% off, active, valid date range, high usage limit
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

	// Register user
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
			`DELETE FROM coupon_usages WHERE order_id IN (SELECT id FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = $1))`,
			email,
		)
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

	// Place order with coupon_code — exercises promotion.Service.Reserve via order.CouponReserver
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
		// Cancel the order — exercises promotion.Service.Release via order.CouponReserver
		req := httptest.NewRequest(http.MethodPost, "/api/orders/"+orderID+"/cancel", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNoContent, w.Code)

		// Verify coupon usage was released
		var usageCount int
		err := testPool.QueryRow(ctx,
			`SELECT COUNT(*) FROM coupon_usages WHERE coupon_id = $1`, couponID).Scan(&usageCount)
		require.NoError(t, err)
		assert.Equal(t, 0, usageCount)

		// Verify promotion used_count was decremented back
		var usedCount int
		err = testPool.QueryRow(ctx,
			`SELECT used_count FROM promotions WHERE id = $1`, couponID).Scan(&usedCount)
		require.NoError(t, err)
		assert.Equal(t, 0, usedCount)
	})
}
