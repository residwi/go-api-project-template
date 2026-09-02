package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mockgatewayserver "github.com/residwi/go-api-project-template/cmd/mockgateway/mockserver"
	"github.com/residwi/go-api-project-template/internal/app"
	"github.com/residwi/go-api-project-template/internal/config"
	"github.com/residwi/go-api-project-template/internal/features/auth"
	"github.com/residwi/go-api-project-template/internal/features/cart"
	"github.com/residwi/go-api-project-template/internal/features/payment"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/testutil"
)

var (
	testPool  *pgxpool.Pool
	testRedis *redis.Client
	testApp   *app.Services

	// Fixed across every App this file builds, so a token minted by one is still
	// valid against another -- the only config every existing call site actually
	// varies is Payment's gateway URL, pointed at a local httptest mock server.
	testAppCfg = &config.Settings{
		App: config.App{
			Name: "test",
			Env:  "development",
			Port: 8080,
		},
		CORS: config.CORS{
			AllowedOrigins: []string{"*"},
			AllowedMethods: []string{"GET", "POST", "PUT", "DELETE"},
			AllowedHeaders: []string{"Content-Type", "Authorization"},
			MaxAge:         86400,
		},
	}
	testPaymentCfg = payment.Config{
		Gateway:        "mock",
		GatewayURL:     "http://localhost:19999",
		GatewayTimeout: 5 * time.Second,
	}
	testModCfg = app.Config{
		Auth: auth.Config{
			Secret:          "test-secret-key-at-least-32-chars-long",
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: 168 * time.Hour,
			Issuer:          "test",
		},
		Cart:    cart.Config{MaxItems: 50},
		Payment: testPaymentCfg,
	}
)

func TestMain(m *testing.M) {
	pool, cleanupPG := testutil.MustStartPostgres("test_server")
	defer cleanupPG()
	testPool = pool

	rdb, cleanupRedis := testutil.MustStartRedis(3)
	defer cleanupRedis()
	testRedis = rdb

	testApp = newTestApp(testPaymentCfg)

	os.Exit(m.Run())
}

func TestNewRouter(t *testing.T) {
	setup(t)
	t.Run("initializes without error", func(t *testing.T) {
		handler := newTestRouter(testPaymentCfg)
		require.NotNil(t, handler)
	})
}

func TestHealthHandler(t *testing.T) {
	setup(t)
	handler := newTestRouter(testPaymentCfg)

	t.Run("reports healthy once the process is serving", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var body map[string]string
		require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
		assert.Equal(t, map[string]string{"status": "healthy"}, body)
	})
}

func TestPublicEndpoints(t *testing.T) {
	setup(t)
	handler := newTestRouter(testPaymentCfg)

	t.Run("GET /api/categories returns list", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/categories", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("GET /api/products returns list", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/products", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("GET /api/products/{slug} returns 404 for nonexistent slug", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/products/nonexistent-slug", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("GET /api/categories/{slug} returns 404 for nonexistent slug", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/categories/nonexistent-slug", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("GET /api/products/{id}/reviews returns list for random product", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/products/"+uuid.New().String()+"/reviews", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestAuthEndpoints(t *testing.T) {
	setup(t)
	handler := newTestRouter(testPaymentCfg)
	ctx := context.Background()

	t.Run("POST /api/auth/register creates user", func(t *testing.T) {
		body := `{"email":"test-router@example.com","password":"Password123!","first_name":"Test","last_name":"User"}`
		req := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)

		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		data, ok := resp["data"].(map[string]any)
		require.True(t, ok)
		assert.NotEmpty(t, data["access_token"])
		assert.NotEmpty(t, data["refresh_token"])
		user, ok := data["user"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "test-router@example.com", user["email"])
		assert.Equal(t, "Test", user["first_name"])
		assert.Equal(t, "User", user["last_name"])
		assert.Equal(t, "user", user["role"])

		testPool.Exec(ctx, `DELETE FROM users WHERE email = 'test-router@example.com'`)
	})

	t.Run("POST /api/auth/register rejects invalid payload", func(t *testing.T) {
		body := `{"email":"bad"}`
		req := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	})

	t.Run("POST /api/auth/login with valid credentials", func(t *testing.T) {
		regBody := `{"email":"test-login@example.com","password":"Password123!","first_name":"Login","last_name":"User"}`
		regReq := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(regBody))
		regReq.Header.Set("Content-Type", "application/json")
		regW := httptest.NewRecorder()
		handler.ServeHTTP(regW, regReq)
		require.Equal(t, http.StatusCreated, regW.Code)

		loginBody := `{"email":"test-login@example.com","password":"Password123!"}`
		loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(loginBody))
		loginReq.Header.Set("Content-Type", "application/json")
		loginW := httptest.NewRecorder()
		handler.ServeHTTP(loginW, loginReq)

		assert.Equal(t, http.StatusOK, loginW.Code)

		var resp map[string]any
		require.NoError(t, json.NewDecoder(loginW.Body).Decode(&resp))
		data, ok := resp["data"].(map[string]any)
		require.True(t, ok)
		assert.NotEmpty(t, data["access_token"])
		assert.NotEmpty(t, data["refresh_token"])
		user, ok := data["user"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "test-login@example.com", user["email"])
		assert.Equal(t, "Login", user["first_name"])
		assert.Equal(t, "User", user["last_name"])
		assert.Equal(t, "user", user["role"])

		testPool.Exec(ctx, `DELETE FROM users WHERE email = 'test-login@example.com'`)
	})

	t.Run("POST /api/auth/login with wrong password", func(t *testing.T) {
		regBody := `{"email":"test-wrongpw@example.com","password":"Password123!","first_name":"Wrong","last_name":"Pw"}`
		regReq := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(regBody))
		regReq.Header.Set("Content-Type", "application/json")
		regW := httptest.NewRecorder()
		handler.ServeHTTP(regW, regReq)
		require.Equal(t, http.StatusCreated, regW.Code)

		loginBody := `{"email":"test-wrongpw@example.com","password":"WrongPassword!"}`
		loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(loginBody))
		loginReq.Header.Set("Content-Type", "application/json")
		loginW := httptest.NewRecorder()
		handler.ServeHTTP(loginW, loginReq)

		assert.Equal(t, http.StatusUnauthorized, loginW.Code)

		testPool.Exec(ctx, `DELETE FROM users WHERE email = 'test-wrongpw@example.com'`)
	})

	t.Run("POST /api/auth/refresh with valid token", func(t *testing.T) {
		regBody := `{"email":"test-refresh@example.com","password":"Password123!","first_name":"Refresh","last_name":"User"}`
		regReq := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(regBody))
		regReq.Header.Set("Content-Type", "application/json")
		regW := httptest.NewRecorder()
		handler.ServeHTTP(regW, regReq)
		require.Equal(t, http.StatusCreated, regW.Code)

		var regResp map[string]any
		require.NoError(t, json.NewDecoder(regW.Body).Decode(&regResp))
		data := regResp["data"].(map[string]any)
		refreshToken := data["refresh_token"].(string)

		refreshBody := `{"refresh_token":"` + refreshToken + `"}`
		refreshReq := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", strings.NewReader(refreshBody))
		refreshReq.Header.Set("Content-Type", "application/json")
		refreshW := httptest.NewRecorder()
		handler.ServeHTTP(refreshW, refreshReq)

		assert.Equal(t, http.StatusOK, refreshW.Code)

		testPool.Exec(ctx, `DELETE FROM users WHERE email = 'test-refresh@example.com'`)
	})
}

func TestProtectedEndpointsRequireAuth(t *testing.T) {
	setup(t)
	handler := newTestRouter(testPaymentCfg)

	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/cart"},
		{http.MethodGet, "/api/orders"},
		{http.MethodGet, "/api/users/me"},
		{http.MethodGet, "/api/wishlist"},
		{http.MethodGet, "/api/notifications"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path+" requires auth", func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			assert.Equal(t, http.StatusUnauthorized, w.Code)
		})
	}
}

func TestAdminEndpointsRequireAuth(t *testing.T) {
	setup(t)
	handler := newTestRouter(testPaymentCfg)

	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/admin/users"},
		{http.MethodGet, "/api/admin/orders"},
		{http.MethodGet, "/api/admin/products"},
		{http.MethodGet, "/api/admin/payments"},
		{http.MethodGet, "/api/admin/promotions"},
		{http.MethodGet, "/api/admin/dashboard/summary"},
		{http.MethodPost, "/api/admin/categories"},
		{http.MethodGet, "/api/admin/inventory/" + uuid.New().String()},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path+" requires auth", func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			assert.Equal(t, http.StatusUnauthorized, w.Code)
		})
	}
}

func TestAuthenticatedEndpoints(t *testing.T) {
	setup(t)
	handler := newTestRouter(testPaymentCfg)
	ctx := context.Background()

	regBody := `{"email":"test-authed@example.com","password":"Password123!","first_name":"Authed","last_name":"User"}`
	regReq := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(regBody))
	regReq.Header.Set("Content-Type", "application/json")
	regW := httptest.NewRecorder()
	handler.ServeHTTP(regW, regReq)
	require.Equal(t, http.StatusCreated, regW.Code)

	var regResp map[string]any
	require.NoError(t, json.NewDecoder(regW.Body).Decode(&regResp))
	data := regResp["data"].(map[string]any)
	accessToken := data["access_token"].(string)

	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM users WHERE email = 'test-authed@example.com'`)
	})

	t.Run("GET /api/users/me returns profile", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/users/me", nil)
		req.Header.Set("Authorization", "Bearer "+accessToken)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("GET /api/cart returns empty cart", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/cart", nil)
		req.Header.Set("Authorization", "Bearer "+accessToken)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("GET /api/orders returns empty list", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/orders", nil)
		req.Header.Set("Authorization", "Bearer "+accessToken)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("GET /api/wishlist returns empty list", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/wishlist", nil)
		req.Header.Set("Authorization", "Bearer "+accessToken)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("GET /api/notifications returns empty list", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/notifications", nil)
		req.Header.Set("Authorization", "Bearer "+accessToken)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("GET /api/notifications/unread-count returns count", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/notifications/unread-count", nil)
		req.Header.Set("Authorization", "Bearer "+accessToken)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestAdminEndpointsRequireAdminRole(t *testing.T) {
	setup(t)
	handler := newTestRouter(testPaymentCfg)
	ctx := context.Background()

	regBody := `{"email":"test-nonadmin@example.com","password":"Password123!","first_name":"Regular","last_name":"User"}`
	regReq := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(regBody))
	regReq.Header.Set("Content-Type", "application/json")
	regW := httptest.NewRecorder()
	handler.ServeHTTP(regW, regReq)
	require.Equal(t, http.StatusCreated, regW.Code)

	var regResp map[string]any
	require.NoError(t, json.NewDecoder(regW.Body).Decode(&regResp))
	data := regResp["data"].(map[string]any)
	accessToken := data["access_token"].(string)

	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM users WHERE email = 'test-nonadmin@example.com'`)
	})

	t.Run("GET /api/admin/users forbidden for regular user", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
		req.Header.Set("Authorization", "Bearer "+accessToken)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("GET /api/admin/orders forbidden for regular user", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/admin/orders", nil)
		req.Header.Set("Authorization", "Bearer "+accessToken)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("GET /api/admin/dashboard/summary forbidden for regular user", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/summary", nil)
		req.Header.Set("Authorization", "Bearer "+accessToken)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})
}

func TestNewRouterWithNilCache(t *testing.T) {
	setup(t)

	t.Run("builds and serves when no redis is configured", func(t *testing.T) {
		handler := NewRouter(
			testAppCfg, testModCfg,
			nil, testutil.DiscardLogger(),
			testApp,
		)

		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestCORSHeaders(t *testing.T) {
	setup(t)
	handler := newTestRouter(testPaymentCfg)

	t.Run("OPTIONS preflight returns CORS headers", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/api/products", nil)
		req.Header.Set("Origin", "http://example.com")
		req.Header.Set("Access-Control-Request-Method", "GET")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.NotEmpty(t, w.Header().Get("Access-Control-Allow-Origin"))
	})
}

func TestAdapterErrorPaths(t *testing.T) {
	setup(t)
	handler := newTestRouter(testPaymentCfg)
	ctx := context.Background()

	email := "adapter-err@example.com"
	regBody := `{"email":"` + email + `","password":"Password123!","first_name":"Err","last_name":"User"}`
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
		testPool.Exec(ctx, `DELETE FROM users WHERE email = $1`, email)
	})

	t.Run("cart add-item returns error for nonexistent product", func(t *testing.T) {
		body := `{"product_id":"` + uuid.New().String() + `","quantity":1}`
		req := httptest.NewRequest(http.MethodPost, "/api/cart/items", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("shipping query slice returns error for nonexistent order", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/orders/"+uuid.New().String()+"/shipping", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestAdapterErrorPaths_PaymentJobWithDeletedOrder(t *testing.T) {
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
		`INSERT INTO categories (id, name, slug, active) VALUES ($1, 'ErrAdapt Cat', $2, true)`,
		catID, "erradapt-cat-"+catID.String()[:8])
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM categories WHERE id = $1`, catID) })

	prodID := uuid.New()
	_, err = testPool.Exec(ctx,
		`INSERT INTO products (id, name, slug, description, price, currency, status, category_id)
		 VALUES ($1, 'ErrAdapt Product', $2, 'desc', 3000, 'USD', 'published', $3)`,
		prodID, "erradapt-prod-"+prodID.String()[:8], catID)
	require.NoError(t, err)
	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM inventory_levels WHERE product_id = $1`, prodID)
		testPool.Exec(ctx, `DELETE FROM products WHERE id = $1`, prodID)
	})
	seedInventoryLevel(t, prodID, 100, 0)

	email := "erradapt-flow@example.com"
	regBody := `{"email":"` + email + `","password":"Password123!","first_name":"ErrAdapt","last_name":"User"}`
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

	t.Run("orderGetterAdapter error when order deleted during refund", func(t *testing.T) {
		_, err := testPool.Exec(ctx,
			`UPDATE payments SET status = 'success', gateway_txn_id = 'txn_erradapt' WHERE id = $1`, paymentID)
		require.NoError(t, err)

		// Deleting both drives orderGetterAdapter and orderItemsGetterAdapter down their
		// error paths.
		testPool.Exec(ctx, `DELETE FROM order_items WHERE order_id = $1`, orderID)
		testPool.Exec(ctx, `DELETE FROM orders WHERE id = $1`, orderID)

		svc := newPaymentServiceForTest(t, mockServer.URL+"/mock/payment")

		// The outcome is not asserted: this exists to drive the order-facing adapters
		// with an order whose items are gone.
		_ = svc.SettleRefund(ctx, paymentID, uuid.MustParse(orderID))

		testPool.Exec(ctx,
			`INSERT INTO orders (id, user_id, status, subtotal_amount, total_amount, currency)
			 SELECT $1, id, 'cancelled', 3000, 3000, 'USD' FROM users WHERE email = $2`,
			orderID, email)
	})
}

func TestAdapterErrorPaths_OrderGetterViaFinalizePayment(t *testing.T) {
	setup(t)

	// A missing order drives orderGetterAdapter.GetByID down its error path.
	paymentID := uuid.New()
	orderID := uuid.New() // does not exist in DB

	// GatewayURL is the placeholder from TestMain (never a real listener):
	// FinalizeSuccess fails inside orderGetterAdapter.GetByID before it
	// ever reaches the gateway, so no URL here is actually dialled.
	err := newPaymentServiceForTest(
		t,
		testPaymentCfg.GatewayURL,
	).FinalizeSuccess(context.Background(), paymentID, orderID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "getting order for verification")
}

func TestServerRunReplicaDBFailure(t *testing.T) {
	setup(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	serverRunEnv(t, port)
	t.Setenv("REPLICA_DATABASE_URL", "postgres://invalid:invalid@127.0.0.1:1/invalid?sslmode=disable")

	runErr := startAndStopServer(t, fmt.Sprintf("http://127.0.0.1:%d", port))
	assert.NoError(t, runErr)
}

func TestServerRunReplicaDBSuccess(t *testing.T) {
	setup(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	pgCfg := testPool.Config().ConnConfig
	serverRunEnv(t, port)
	t.Setenv("REPLICA_DATABASE_URL", fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable",
		pgCfg.User, pgCfg.Password, net.JoinHostPort(pgCfg.Host, strconv.Itoa(int(pgCfg.Port))), pgCfg.Database))

	runErr := startAndStopServer(t, fmt.Sprintf("http://127.0.0.1:%d", port))
	assert.NoError(t, runErr)
}

func TestServerRunRedisFailure(t *testing.T) {
	setup(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	serverRunEnv(t, port)
	t.Setenv("REDIS_HOST", "127.0.0.1")
	t.Setenv("REDIS_PORT", "1")

	runErr := startAndStopServer(t, fmt.Sprintf("http://127.0.0.1:%d", port))
	assert.NoError(t, runErr)
}

func TestServerRunListenError(t *testing.T) {
	setup(t)
	// Bind on all interfaces (":port") to match srv.Addr = ":<port>"
	ln, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	defer ln.Close()

	serverRunEnv(t, port)

	ctx := t.Context()

	errCh := make(chan error, 1)
	go func() { errCh <- RunContext(ctx) }()

	// No sleep and no cancel: the port is already bound, so ListenAndServe fails on
	// its own and waiting on that error is what makes this deterministic. Cancelling
	// first would send RunContext down its graceful-shutdown path, returning nil
	// without ever attempting the listen.
	select {
	case runErr := <-errCh:
		require.Error(t, runErr)
		require.ErrorContains(t, runErr, "address already in use",
			"the returned error must name the bind failure, not some later shutdown error")
	case <-time.After(30 * time.Second):
		t.Fatal("RunContext did not return the listen error for an already-bound port")
	}
}

func TestServerRunConfigError(t *testing.T) {
	setup(t)
	t.Setenv("JWT_SECRET", "test-secret-key-at-least-32-chars-long")
	// JWT_ACCESS_TTL is auth's own tag now (task 8), so a bad value fails inside
	// auth.LoadConfig rather than a central config.Load.
	t.Setenv("JWT_ACCESS_TTL", "not-a-duration")

	err := Run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading auth config")
}

func TestServerRunDatabaseError(t *testing.T) {
	setup(t)
	t.Setenv("JWT_SECRET", "test-secret-key-at-least-32-chars-long")
	t.Setenv("DB_HOST", "127.0.0.1")
	t.Setenv("DB_PORT", "1")
	t.Setenv("DB_USER", "invalid")
	t.Setenv("DB_PASSWORD", "invalid")
	t.Setenv("DB_NAME", "invalid")

	err := Run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connecting to database")
}

func TestServerRun(t *testing.T) {
	setup(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	serverRunEnv(t, port)

	addr := fmt.Sprintf("http://127.0.0.1:%d", port)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- RunContext(ctx) }()

	require.Eventually(t, func() bool {
		resp, err := http.Get(addr + "/health")
		if err != nil {
			return false
		}
		resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 30*time.Second, 100*time.Millisecond, "server did not start in time")

	t.Run("health endpoint returns healthy", func(t *testing.T) {
		resp, err := http.Get(addr + "/health")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var body map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.Equal(t, "healthy", body["status"])
	})

	cancel()

	select {
	case runErr := <-errCh:
		require.NoError(t, runErr)
	case <-time.After(30 * time.Second):
		t.Fatal("RunContext did not return after its context was cancelled")
	}
}

func setup(t *testing.T) {
	t.Helper()
	testutil.ResetDB(t, testPool)
	testutil.ResetRedis(t, testRedis)
}

// newPaymentServiceForTest wires a whole App against a custom gateway URL (a
// local httptest mock server) and hands back the payment service.
// test/e2e carries its own newPaymentService in testmain_test.go; the two are
// separate because each package builds its App from its own test fixtures.
func newPaymentServiceForTest(t *testing.T, gatewayURL string) *payment.Service {
	t.Helper()

	return newTestApp(payment.Config{
		Gateway:        "mock",
		GatewayURL:     gatewayURL,
		GatewayTimeout: 5 * time.Second,
	}).Payments
}

// Reserve and Deduct need a row to update, and these tests insert
// products with raw SQL, bypassing the EnsureLevel in product.Service.Create.
//
// test/e2e/testmain_test.go carries its own copy. Keep them in step.
func seedInventoryLevel(t *testing.T, productID uuid.UUID, available, reserved int) {
	t.Helper()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO inventory_levels (product_id, available_stock, reserved_stock)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (product_id) DO UPDATE
		 SET available_stock = EXCLUDED.available_stock,
		     reserved_stock  = EXCLUDED.reserved_stock`,
		productID, available, reserved)
	require.NoError(t, err)
}

func serverRunEnv(t *testing.T, port int) {
	t.Helper()
	pgCfg := testPool.Config().ConnConfig
	redisAddr := testRedis.Options().Addr

	t.Setenv("APP_PORT", strconv.Itoa(port))
	t.Setenv("APP_ENV", "development")
	t.Setenv("APP_SHUTDOWN_TIMEOUT", "2s")
	t.Setenv("DB_HOST", pgCfg.Host)
	t.Setenv("DB_PORT", strconv.FormatUint(uint64(pgCfg.Port), 10))
	t.Setenv("DB_USER", pgCfg.User)
	t.Setenv("DB_PASSWORD", pgCfg.Password)
	t.Setenv("DB_NAME", pgCfg.Database)
	t.Setenv("DB_SSLMODE", "disable")
	t.Setenv("REDIS_HOST", strings.Split(redisAddr, ":")[0])
	t.Setenv("REDIS_PORT", strings.Split(redisAddr, ":")[1])
	// REDIS_DB would otherwise default to 0, the index internal/platform/cache owns
	// and flushes, racing this package's own index 3.
	t.Setenv("REDIS_DB", "3")
	t.Setenv("JWT_SECRET", "test-secret-key-at-least-32-chars-long")
}

// Cancels a context rather than signalling the test process's own PID: a SIGINT
// fires every handler in the binary, and if the readiness wait failed it was
// never sent at all, leaking the server goroutine for the rest of the run.
func startAndStopServer(t *testing.T, healthAddr string) error {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- RunContext(ctx) }()

	require.Eventually(t, func() bool {
		resp, err := http.Get(healthAddr + "/health")
		if err != nil {
			return false
		}
		resp.Body.Close()
		return true
	}, 30*time.Second, 100*time.Millisecond, "server did not start in time")

	cancel()

	select {
	case runErr := <-errCh:
		return runErr
	case <-time.After(30 * time.Second):
		t.Fatal("RunContext did not return after its context was cancelled")
		return nil
	}
}

// newTestApp wires a app.Services against testPool/testRedis for a given
// payment config. TestMain uses it for testApp; tests that need a different
// payment gateway URL (a local httptest mock server) build their own
// payment.Config and call this instead.
// withPayment returns the shared module config with only Payment replaced --
// the one field any call site varies.
func withPayment(paymentCfg payment.Config) app.Config {
	cfg := testModCfg
	cfg.Payment = paymentCfg
	return cfg
}

func newTestApp(paymentCfg payment.Config) *app.Services {
	deps, err := app.New(
		withPayment(paymentCfg),
		database.DB{Primary: testPool},
		testRedis,
		testutil.DiscardLogger(),
	)
	if err != nil {
		panic(err)
	}
	return deps
}

// newTestRouter builds a router for a given payment config against a
// freshly wired app -- the common case, where nothing but Payment varies
// between call sites.
func newTestRouter(paymentCfg payment.Config) http.Handler {
	return NewRouter(
		testAppCfg, withPayment(paymentCfg),
		testRedis, testutil.DiscardLogger(),
		newTestApp(paymentCfg),
	)
}
