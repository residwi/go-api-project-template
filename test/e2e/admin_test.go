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
)

func TestE2EAdminFlow(t *testing.T) {
	setup(t)
	handler := apihttp.NewRouter(testDeps)
	ctx := context.Background()

	// Register and promote to admin
	email := "admin-e2e@example.com"
	regBody := `{"email":"` + email + `","password":"Password123!","first_name":"Admin","last_name":"User"}`
	regReq := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(regBody))
	regReq.Header.Set("Content-Type", "application/json")
	regW := httptest.NewRecorder()
	handler.ServeHTTP(regW, regReq)
	require.Equal(t, http.StatusCreated, regW.Code)

	var regResp map[string]any
	require.NoError(t, json.NewDecoder(regW.Body).Decode(&regResp))
	token := regResp["data"].(map[string]any)["access_token"].(string)

	// Promote to admin directly in DB
	_, err := testPool.Exec(ctx, `UPDATE users SET role = 'admin' WHERE email = $1`, email)
	require.NoError(t, err)

	// Re-login to get fresh token with admin role
	loginBody := `{"email":"` + email + `","password":"Password123!"}`
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginW := httptest.NewRecorder()
	handler.ServeHTTP(loginW, loginReq)
	require.Equal(t, http.StatusOK, loginW.Code)

	var loginResp map[string]any
	require.NoError(t, json.NewDecoder(loginW.Body).Decode(&loginResp))
	token = loginResp["data"].(map[string]any)["access_token"].(string)

	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM users WHERE email = $1`, email)
	})

	t.Run("admin can list users", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("admin can list orders", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/admin/orders", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("admin can view dashboard summary", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/summary?from=2024-01-01&to=2025-12-31", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("admin can list payments", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/admin/payments", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("admin can list promotions", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/admin/promotions", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestE2EShippingAndReviewFlow(t *testing.T) {
	setup(t)
	// Start a mock payment gateway server
	mockMux := http.NewServeMux()
	mockgatewayserver.RegisterRoutes(mockMux)
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
		Pool:  testPool,
		Cache: testRedis,
	}
	handler := apihttp.NewRouter(deps)
	ctx := context.Background()

	// Seed category + product
	catID := uuid.New()
	_, err := testPool.Exec(ctx,
		`INSERT INTO categories (id, name, slug, active) VALUES ($1, 'Ship Cat', $2, true)`,
		catID, "ship-cat-"+catID.String()[:8])
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM categories WHERE id = $1`, catID) })

	prodID := uuid.New()
	_, err = testPool.Exec(ctx,
		`INSERT INTO products (id, name, slug, description, price, currency, status, category_id)
		 VALUES ($1, 'Ship Product', $2, 'desc', 4000, 'USD', 'published', $3)`,
		prodID, "ship-prod-"+prodID.String()[:8], catID)
	require.NoError(t, err)
	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM inventory_levels WHERE product_id = $1`, prodID)
		testPool.Exec(ctx, `DELETE FROM products WHERE id = $1`, prodID)
	})
	seedInventoryLevel(t, prodID, 100, 0)

	// Register user
	email := "shipping-flow@example.com"
	regBody := `{"email":"` + email + `","password":"Password123!","first_name":"Ship","last_name":"User"}`
	regReq := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(regBody))
	regReq.Header.Set("Content-Type", "application/json")
	regW := httptest.NewRecorder()
	handler.ServeHTTP(regW, regReq)
	require.Equal(t, http.StatusCreated, regW.Code)

	var regResp map[string]any
	require.NoError(t, json.NewDecoder(regW.Body).Decode(&regResp))
	token := regResp["data"].(map[string]any)["access_token"].(string)

	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM reviews WHERE user_id IN (SELECT id FROM users WHERE email = $1)`, email)
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

	// Webhook to mark as paid
	var paymentID uuid.UUID
	err = testPool.QueryRow(ctx, `SELECT id FROM payments WHERE order_id = $1`, orderID).Scan(&paymentID)
	require.NoError(t, err)

	webhookBody := fmt.Sprintf(`{"event":"success","metadata":{"payment_id":"%s"},"transaction_id":"txn_ship"}`, paymentID)
	whReq := httptest.NewRequest(http.MethodPost, "/api/payments/webhook", strings.NewReader(webhookBody))
	whReq.Header.Set("Content-Type", "application/json")
	whW := httptest.NewRecorder()
	handler.ServeHTTP(whW, whReq)
	require.Equal(t, http.StatusOK, whW.Code)

	// Promote to admin and re-login
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

	t.Run("admin creates shipment for paid order", func(t *testing.T) {
		body := `{"tracking_number":"TRACK123","carrier":"FedEx"}`
		req := httptest.NewRequest(http.MethodPost, "/api/admin/orders/"+orderID+"/ship", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+adminToken)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusCreated, w.Code)

		// Verify order status changed to shipped
		var orderStatus string
		err := testPool.QueryRow(ctx, `SELECT status FROM orders WHERE id = $1`, orderID).Scan(&orderStatus)
		require.NoError(t, err)
		assert.Equal(t, "shipped", orderStatus)
	})

	t.Run("admin delivers order", func(t *testing.T) {
		var shipmentID uuid.UUID
		err := testPool.QueryRow(ctx, `SELECT id FROM shipments WHERE order_id = $1`, orderID).Scan(&shipmentID)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/admin/shipments/"+shipmentID.String()+"/deliver", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		// Verify order is delivered
		var orderStatus string
		err = testPool.QueryRow(ctx, `SELECT status FROM orders WHERE id = $1`, orderID).Scan(&orderStatus)
		require.NoError(t, err)
		assert.Equal(t, "delivered", orderStatus)
	})

	t.Run("user can review purchased product after delivery", func(t *testing.T) {
		body := `{"order_id":"` + orderID + `","rating":5,"title":"Great","body":"Great product!"}`
		req := httptest.NewRequest(http.MethodPost, "/api/products/"+prodID.String()+"/reviews", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+adminToken)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusCreated, w.Code)
	})

	t.Run("user can view shipping for their order", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/orders/"+orderID+"/shipping", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}
