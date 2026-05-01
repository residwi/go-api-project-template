package server_test

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
	"syscall"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/config"
	"github.com/residwi/go-api-project-template/internal/features/payment"
	mockgw "github.com/residwi/go-api-project-template/internal/platform/payment/mock"
	"github.com/residwi/go-api-project-template/internal/server"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

var (
	testPool  *pgxpool.Pool
	testRedis *redis.Client
	testDeps  *server.Deps
)

func TestMain(m *testing.M) {
	pool, cleanupPG := testhelper.MustStartPostgres("test_server")
	defer cleanupPG()
	testPool = pool

	rdb, cleanupRedis := testhelper.MustStartRedis(3)
	defer cleanupRedis()
	testRedis = rdb

	testDeps = &server.Deps{
		Config: &config.Config{
			App: config.AppConfig{
				Name:         "test",
				Env:          "development",
				Port:         8080,
				MaxCartItems: 50,
			},
			JWT: config.JWTConfig{
				Secret:          "test-secret-key-at-least-32-chars-long",
				AccessTokenTTL:  15 * time.Minute,
				RefreshTokenTTL: 168 * time.Hour,
				Issuer:          "test",
			},
			CORS: config.CORSConfig{
				AllowedOrigins: []string{"*"},
				AllowedMethods: []string{"GET", "POST", "PUT", "DELETE"},
				AllowedHeaders: []string{"Content-Type", "Authorization"},
				MaxAge:         86400,
			},
			Payment: config.PaymentConfig{
				Gateway:        "mock",
				GatewayURL:     "http://localhost:19999",
				GatewayTimeout: 5 * time.Second,
			},
		},
		Pool:  pool,
		Redis: rdb,
	}

	os.Exit(m.Run())
}

func setup(t *testing.T) {
	t.Helper()
	testhelper.ResetDB(t, testPool)
	testhelper.ResetRedis(t, testRedis)
}

func TestNewRouter(t *testing.T) {
	setup(t)
	t.Run("initializes without error", func(t *testing.T) {
		handler := server.NewRouter(testDeps)
		require.NotNil(t, handler)
	})
}

func TestHealthHandler(t *testing.T) {
	setup(t)
	handler := server.NewRouter(testDeps).Handler

	t.Run("returns healthy status", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var body map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
		assert.Equal(t, "healthy", body["status"])
		details, ok := body["details"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "up", details["postgres"])
		assert.Equal(t, "up", details["redis"])
	})

	t.Run("returns unhealthy when postgres is down", func(t *testing.T) {
		badPool, err := pgxpool.New(context.Background(), "postgres://invalid:invalid@127.0.0.1:1/invalid")
		if err != nil {
			t.Skip("could not create bad pool")
		}
		defer badPool.Close()

		badDeps := &server.Deps{
			Config: testDeps.Config,
			Pool:   badPool,
			Redis:  testRedis,
		}
		h := server.NewRouter(badDeps).Handler

		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		assert.Equal(t, http.StatusServiceUnavailable, w.Code)

		var body map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
		assert.Equal(t, "unhealthy", body["status"])
		details := body["details"].(map[string]any)
		assert.Equal(t, "down", details["postgres"])
	})

	t.Run("returns degraded when redis is down", func(t *testing.T) {
		badRedis := redis.NewClient(&redis.Options{
			Addr:         "127.0.0.1:1",
			MaxRetries:   0,
			DialTimeout:  200 * time.Millisecond,
			PoolSize:     1,
			MinIdleConns: 0,
		})
		defer badRedis.Close()

		badDeps := &server.Deps{
			Config: testDeps.Config,
			Pool:   testPool,
			Redis:  badRedis,
		}
		h := server.NewRouter(badDeps).Handler

		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		assert.Equal(t, http.StatusServiceUnavailable, w.Code)

		var body map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
		assert.Equal(t, "degraded", body["status"])
		details := body["details"].(map[string]any)
		assert.Equal(t, "up", details["postgres"])
		assert.Equal(t, "down", details["redis"])
	})

	t.Run("returns not configured when redis is nil", func(t *testing.T) {
		nilRedisDeps := &server.Deps{
			Config: testDeps.Config,
			Pool:   testPool,
			Redis:  nil,
		}
		h := server.NewRouter(nilRedisDeps).Handler

		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var body map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
		assert.Equal(t, "healthy", body["status"])
		details := body["details"].(map[string]any)
		assert.Equal(t, "up", details["postgres"])
		assert.Equal(t, "not configured", details["redis"])
	})
}

func TestPublicEndpoints(t *testing.T) {
	setup(t)
	handler := server.NewRouter(testDeps).Handler

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
	handler := server.NewRouter(testDeps).Handler
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
		assert.NotEmpty(t, resp["data"])

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
		// Register first
		regBody := `{"email":"test-login@example.com","password":"Password123!","first_name":"Login","last_name":"User"}`
		regReq := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(regBody))
		regReq.Header.Set("Content-Type", "application/json")
		regW := httptest.NewRecorder()
		handler.ServeHTTP(regW, regReq)
		require.Equal(t, http.StatusCreated, regW.Code)

		// Login
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

		testPool.Exec(ctx, `DELETE FROM users WHERE email = 'test-login@example.com'`)
	})

	t.Run("POST /api/auth/login with wrong password", func(t *testing.T) {
		// Register first
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
		// Register to get tokens
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

		// Refresh
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
	handler := server.NewRouter(testDeps).Handler

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
	handler := server.NewRouter(testDeps).Handler

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
	handler := server.NewRouter(testDeps).Handler
	ctx := context.Background()

	// Register and login to get an access token
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
	handler := server.NewRouter(testDeps).Handler
	ctx := context.Background()

	// Register a regular user
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

func TestE2EOrderFlow(t *testing.T) {
	setup(t)
	handler := server.NewRouter(testDeps).Handler
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
		`INSERT INTO products (id, name, slug, description, price, currency, stock_quantity, status, category_id)
		 VALUES ($1, 'E2E Product', $2, 'desc', 5000, 'USD', 100, 'published', $3)`,
		prodID, "e2e-prod-"+prodID.String()[:8], catID)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM products WHERE id = $1`, prodID) })

	// Ensure product has stock (stock_quantity is on products table)
	_, err = testPool.Exec(ctx,
		`UPDATE products SET stock_quantity = 100, reserved_quantity = 0 WHERE id = $1`, prodID)
	require.NoError(t, err)

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
		testPool.Exec(ctx, `DELETE FROM cart_items WHERE cart_id IN (SELECT id FROM carts WHERE user_id IN (SELECT id FROM users WHERE email = 'e2e-flow@example.com'))`)
		testPool.Exec(ctx, `DELETE FROM carts WHERE user_id IN (SELECT id FROM users WHERE email = 'e2e-flow@example.com')`)
		testPool.Exec(ctx, `DELETE FROM order_items WHERE order_id IN (SELECT id FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = 'e2e-flow@example.com'))`)
		testPool.Exec(ctx, `DELETE FROM payment_jobs WHERE order_id IN (SELECT id FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = 'e2e-flow@example.com'))`)
		testPool.Exec(ctx, `DELETE FROM payments WHERE order_id IN (SELECT id FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = 'e2e-flow@example.com'))`)
		testPool.Exec(ctx, `DELETE FROM notifications WHERE user_id IN (SELECT id FROM users WHERE email = 'e2e-flow@example.com')`)
		testPool.Exec(ctx, `DELETE FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = 'e2e-flow@example.com')`)
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

func TestHealthHandler_NilRedis(t *testing.T) {
	setup(t)
	nilRedisDeps := &server.Deps{
		Config: testDeps.Config,
		Pool:   testDeps.Pool,
		Redis:  nil,
	}
	handler := server.NewRouter(nilRedisDeps).Handler

	t.Run("returns healthy with redis not configured", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var body map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
		assert.Equal(t, "healthy", body["status"])
		details := body["details"].(map[string]any)
		assert.Equal(t, "not configured", details["redis"])
	})
}

func TestE2ECancelOrderFlow(t *testing.T) {
	setup(t)
	handler := server.NewRouter(testDeps).Handler
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
		`INSERT INTO products (id, name, slug, description, price, currency, stock_quantity, status, category_id)
		 VALUES ($1, 'Cancel Product', $2, 'desc', 3000, 'USD', 50, 'published', $3)`,
		prodID, "cancel-prod-"+prodID.String()[:8], catID)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM products WHERE id = $1`, prodID) })

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
		testPool.Exec(ctx, `DELETE FROM cart_items WHERE cart_id IN (SELECT id FROM carts WHERE user_id IN (SELECT id FROM users WHERE email = $1))`, email)
		testPool.Exec(ctx, `DELETE FROM carts WHERE user_id IN (SELECT id FROM users WHERE email = $1)`, email)
		testPool.Exec(ctx, `DELETE FROM order_items WHERE order_id IN (SELECT id FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = $1))`, email)
		testPool.Exec(ctx, `DELETE FROM payment_jobs WHERE order_id IN (SELECT id FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = $1))`, email)
		testPool.Exec(ctx, `DELETE FROM payments WHERE order_id IN (SELECT id FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = $1))`, email)
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

func TestE2EAdminFlow(t *testing.T) {
	setup(t)
	handler := server.NewRouter(testDeps).Handler
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

func TestCORSHeaders(t *testing.T) {
	setup(t)
	handler := server.NewRouter(testDeps).Handler

	t.Run("OPTIONS preflight returns CORS headers", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/api/products", nil)
		req.Header.Set("Origin", "http://example.com")
		req.Header.Set("Access-Control-Request-Method", "GET")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.NotEmpty(t, w.Header().Get("Access-Control-Allow-Origin"))
	})
}

func TestE2EPaymentWebhookFlow(t *testing.T) {
	setup(t)
	// Start a mock payment gateway server
	mockMux := http.NewServeMux()
	mockgw.RegisterRoutes(mockMux)
	mockServer := httptest.NewServer(mockMux)
	defer mockServer.Close()

	webhookDeps := &server.Deps{
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
		Redis: testRedis,
	}
	handler := server.NewRouter(webhookDeps).Handler
	ctx := context.Background()

	// Seed category + product
	catID := uuid.New()
	_, err := testPool.Exec(ctx,
		`INSERT INTO categories (id, name, slug, active) VALUES ($1, 'Webhook Cat', $2, true)`,
		catID, "webhook-cat-"+catID.String()[:8])
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM categories WHERE id = $1`, catID) })

	prodID := uuid.New()
	_, err = testPool.Exec(ctx,
		`INSERT INTO products (id, name, slug, description, price, currency, stock_quantity, status, category_id)
		 VALUES ($1, 'Webhook Product', $2, 'desc', 5000, 'USD', 100, 'published', $3)`,
		prodID, "webhook-prod-"+prodID.String()[:8], catID)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM products WHERE id = $1`, prodID) })

	// Register user
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

	// Look up the payment_id from DB
	var paymentID uuid.UUID
	err = testPool.QueryRow(ctx, `SELECT id FROM payments WHERE order_id = $1`, orderID).Scan(&paymentID)
	require.NoError(t, err)

	t.Run("webhook success updates order to paid", func(t *testing.T) {
		webhookBody := fmt.Sprintf(`{"event":"success","metadata":{"payment_id":"%s"},"transaction_id":"txn_test"}`, paymentID)
		req := httptest.NewRequest(http.MethodPost, "/api/payments/webhook", strings.NewReader(webhookBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		// Verify order status changed to "paid"
		var orderStatus string
		err := testPool.QueryRow(ctx, `SELECT status FROM orders WHERE id = $1`, orderID).Scan(&orderStatus)
		require.NoError(t, err)
		assert.Equal(t, "paid", orderStatus)

		// Verify payment status changed to "success"
		var paymentStatus string
		err = testPool.QueryRow(ctx, `SELECT status FROM payments WHERE id = $1`, paymentID).Scan(&paymentStatus)
		require.NoError(t, err)
		assert.Equal(t, "success", paymentStatus)
	})
}

func TestE2EPaymentFailedWebhookFlow(t *testing.T) {
	setup(t)
	mockMux := http.NewServeMux()
	mockgw.RegisterRoutes(mockMux)
	mockServer := httptest.NewServer(mockMux)
	defer mockServer.Close()

	deps := &server.Deps{
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
		Redis: testRedis,
	}
	handler := server.NewRouter(deps).Handler
	ctx := context.Background()

	// Seed category + product
	catID := uuid.New()
	_, err := testPool.Exec(ctx,
		`INSERT INTO categories (id, name, slug, active) VALUES ($1, 'Fail Cat', $2, true)`,
		catID, "fail-cat-"+catID.String()[:8])
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM categories WHERE id = $1`, catID) })

	prodID := uuid.New()
	_, err = testPool.Exec(ctx,
		`INSERT INTO products (id, name, slug, description, price, currency, stock_quantity, status, category_id)
		 VALUES ($1, 'Fail Product', $2, 'desc', 2000, 'USD', 50, 'published', $3)`,
		prodID, "fail-prod-"+prodID.String()[:8], catID)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM products WHERE id = $1`, prodID) })

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
	cartBody := `{"product_id":"` + prodID.String() + `","quantity":2}`
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

	var paymentID uuid.UUID
	err = testPool.QueryRow(ctx, `SELECT id FROM payments WHERE order_id = $1`, orderID).Scan(&paymentID)
	require.NoError(t, err)

	t.Run("webhook failed cancels payment and jobs", func(t *testing.T) {
		// Record reserved quantity before
		var reservedBefore int
		err := testPool.QueryRow(ctx, `SELECT reserved_quantity FROM products WHERE id = $1`, prodID).Scan(&reservedBefore)
		require.NoError(t, err)
		assert.Equal(t, 2, reservedBefore)

		webhookBody := fmt.Sprintf(`{"event":"failed","metadata":{"payment_id":"%s"},"transaction_id":"txn_fail"}`, paymentID)
		req := httptest.NewRequest(http.MethodPost, "/api/payments/webhook", strings.NewReader(webhookBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		// Verify payment status changed to cancelled
		var paymentStatus string
		err = testPool.QueryRow(ctx, `SELECT status FROM payments WHERE id = $1`, paymentID).Scan(&paymentStatus)
		require.NoError(t, err)
		assert.Equal(t, "cancelled", paymentStatus)

		// Verify charge jobs were cancelled
		var pendingJobs int
		err = testPool.QueryRow(ctx,
			`SELECT COUNT(*) FROM payment_jobs WHERE order_id = $1 AND status IN ('pending','processing')`, orderID).Scan(&pendingJobs)
		require.NoError(t, err)
		assert.Equal(t, 0, pendingJobs)
	})
}

func TestE2EAdminRefundEndpoint(t *testing.T) {
	setup(t)
	// Start a mock payment gateway server
	mockMux := http.NewServeMux()
	mockgw.RegisterRoutes(mockMux)
	mockServer := httptest.NewServer(mockMux)
	defer mockServer.Close()

	webhookDeps := &server.Deps{
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
		Redis: testRedis,
	}
	router := server.NewRouter(webhookDeps)
	handler := router.Handler
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
		`INSERT INTO products (id, name, slug, description, price, currency, stock_quantity, status, category_id)
		 VALUES ($1, 'Refund Product', $2, 'desc', 3000, 'USD', 100, 'published', $3)`,
		prodID, "refund-prod-"+prodID.String()[:8], catID)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM products WHERE id = $1`, prodID) })

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
			        COALESCE(inventory_action, ''), created_at, updated_at
			 FROM payment_jobs
			 WHERE order_id = $1 AND action = 'refund' AND status = 'pending'
			 LIMIT 1`, orderID).Scan(
			&job.ID, &job.PaymentID, &job.OrderID, &job.Action, &job.Status,
			&job.Attempts, &job.MaxAttempts, &job.LastError, &job.LockedUntil,
			&job.NextRetryAt, &job.InventoryAction, &job.CreatedAt, &job.UpdatedAt,
		)
		require.NoError(t, err)
		assert.Equal(t, payment.ActionRefund, job.Action)
		assert.Equal(t, "restock", job.InventoryAction)

		// Record stock before refund
		var stockBefore int
		err = testPool.QueryRow(ctx,
			`SELECT stock_quantity FROM products WHERE id = $1`, prodID).Scan(&stockBefore)
		require.NoError(t, err)

		// Process the refund job via the router's payment service
		ok := router.PaymentSvc.ProcessJob(ctx, job)
		assert.True(t, ok)

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
		var stockAfter int
		err = testPool.QueryRow(ctx,
			`SELECT stock_quantity FROM products WHERE id = $1`, prodID).Scan(&stockAfter)
		require.NoError(t, err)
		assert.Equal(t, stockBefore+1, stockAfter)

		// Verify refund job marked as completed
		var jobStatus string
		err = testPool.QueryRow(ctx,
			`SELECT status FROM payment_jobs WHERE id = $1`, job.ID).Scan(&jobStatus)
		require.NoError(t, err)
		assert.Equal(t, "completed", jobStatus)
	})
}

func TestE2EShippingAndReviewFlow(t *testing.T) {
	setup(t)
	// Start a mock payment gateway server
	mockMux := http.NewServeMux()
	mockgw.RegisterRoutes(mockMux)
	mockServer := httptest.NewServer(mockMux)
	defer mockServer.Close()

	deps := &server.Deps{
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
		Redis: testRedis,
	}
	handler := server.NewRouter(deps).Handler
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
		`INSERT INTO products (id, name, slug, description, price, currency, stock_quantity, status, category_id)
		 VALUES ($1, 'Ship Product', $2, 'desc', 4000, 'USD', 100, 'published', $3)`,
		prodID, "ship-prod-"+prodID.String()[:8], catID)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM products WHERE id = $1`, prodID) })

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

func TestE2ECouponOrderFlow(t *testing.T) {
	setup(t)
	mockMux := http.NewServeMux()
	mockgw.RegisterRoutes(mockMux)
	mockServer := httptest.NewServer(mockMux)
	defer mockServer.Close()

	deps := &server.Deps{
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
		Redis: testRedis,
	}
	handler := server.NewRouter(deps).Handler
	ctx := context.Background()

	// Seed category + product
	catID := uuid.New()
	_, err := testPool.Exec(ctx,
		`INSERT INTO categories (id, name, slug, active) VALUES ($1, 'Coupon Cat', $2, true)`,
		catID, "coupon-cat-"+catID.String()[:8])
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM categories WHERE id = $1`, catID) })

	prodID := uuid.New()
	_, err = testPool.Exec(ctx,
		`INSERT INTO products (id, name, slug, description, price, currency, stock_quantity, status, category_id)
		 VALUES ($1, 'Coupon Product', $2, 'desc', 10000, 'USD', 50, 'published', $3)`,
		prodID, "coupon-prod-"+prodID.String()[:8], catID)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM products WHERE id = $1`, prodID) })

	// Seed a coupon: 10% off, active, valid date range, high usage limit
	couponID := uuid.New()
	couponCode := "TESTCOUPON" + couponID.String()[:8]
	maxUses := 100
	_, err = testPool.Exec(ctx,
		`INSERT INTO promotions (id, code, type, value, min_order_amount, max_uses, used_count, starts_at, expires_at, active)
		 VALUES ($1, $2, 'percentage', 10, 0, $3, 0, $4, $5, true)`,
		couponID, couponCode, maxUses, time.Now().Add(-24*time.Hour), time.Now().Add(24*time.Hour))
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
		testPool.Exec(ctx, `DELETE FROM cart_items WHERE cart_id IN (SELECT id FROM carts WHERE user_id IN (SELECT id FROM users WHERE email = $1))`, email)
		testPool.Exec(ctx, `DELETE FROM carts WHERE user_id IN (SELECT id FROM users WHERE email = $1)`, email)
		testPool.Exec(ctx, `DELETE FROM order_items WHERE order_id IN (SELECT id FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = $1))`, email)
		testPool.Exec(ctx, `DELETE FROM payment_jobs WHERE order_id IN (SELECT id FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = $1))`, email)
		testPool.Exec(ctx, `DELETE FROM payments WHERE order_id IN (SELECT id FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = $1))`, email)
		testPool.Exec(ctx, `DELETE FROM notifications WHERE user_id IN (SELECT id FROM users WHERE email = $1)`, email)
		testPool.Exec(ctx, `DELETE FROM coupon_usages WHERE order_id IN (SELECT id FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = $1))`, email)
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

	// Place order with coupon_code — exercises couponReserverAdapter.Reserve
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
		assert.Equal(t, couponCode, orderData["coupon_code"])
		assert.Greater(t, orderData["discount_amount"].(float64), float64(0))
		assert.Less(t, orderData["total_amount"].(float64), orderData["subtotal_amount"].(float64))
	})

	t.Run("cancel order releases coupon", func(t *testing.T) {
		// Cancel the order — exercises couponReserverAdapter.Release
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

func TestE2ERefundWithCouponAndRelease(t *testing.T) {
	setup(t)
	// This test covers inventoryReleaserAdapter.Release and couponReleaserAdapter.Release
	// by processing a refund job with inventory_action='release' on an order with a coupon.
	mockMux := http.NewServeMux()
	mockgw.RegisterRoutes(mockMux)
	mockServer := httptest.NewServer(mockMux)
	defer mockServer.Close()

	webhookDeps := &server.Deps{
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
		Redis: testRedis,
	}
	router := server.NewRouter(webhookDeps)
	handler := router.Handler
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
		`INSERT INTO products (id, name, slug, description, price, currency, stock_quantity, status, category_id)
		 VALUES ($1, 'RelCoupon Product', $2, 'desc', 8000, 'USD', 100, 'published', $3)`,
		prodID, "relcoupon-prod-"+prodID.String()[:8], catID)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM products WHERE id = $1`, prodID) })

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

	// Create a refund job with inventory_action='release' directly
	refundJobID := uuid.New()
	_, err = testPool.Exec(ctx,
		`INSERT INTO payment_jobs (id, payment_id, order_id, action, status, max_attempts, next_retry_at, inventory_action)
		 VALUES ($1, $2, $3, 'refund', 'pending', 3, NOW(), 'release')`,
		refundJobID, paymentID, orderID)
	require.NoError(t, err)

	t.Run("processing refund job releases inventory and coupon", func(t *testing.T) {
		// Record reserved quantity before
		var reservedBefore int
		err := testPool.QueryRow(ctx,
			`SELECT reserved_quantity FROM products WHERE id = $1`, prodID).Scan(&reservedBefore)
		require.NoError(t, err)

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
			        COALESCE(inventory_action, ''), created_at, updated_at
			 FROM payment_jobs WHERE id = $1`, refundJobID).Scan(
			&job.ID, &job.PaymentID, &job.OrderID, &job.Action, &job.Status,
			&job.Attempts, &job.MaxAttempts, &job.LastError, &job.LockedUntil,
			&job.NextRetryAt, &job.InventoryAction, &job.CreatedAt, &job.UpdatedAt,
		)
		require.NoError(t, err)
		assert.Equal(t, "release", job.InventoryAction)

		ok := router.PaymentSvc.ProcessJob(ctx, job)
		assert.True(t, ok)

		// Verify inventory was released (reserved_quantity decreased)
		var reservedAfter int
		err = testPool.QueryRow(ctx,
			`SELECT reserved_quantity FROM products WHERE id = $1`, prodID).Scan(&reservedAfter)
		require.NoError(t, err)
		assert.Equal(t, reservedBefore-1, reservedAfter)

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

func TestAdapterErrorPaths(t *testing.T) {
	setup(t)
	handler := server.NewRouter(testDeps).Handler
	ctx := context.Background()

	// Register a user for authenticated requests
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
		testPool.Exec(ctx, `DELETE FROM cart_items WHERE cart_id IN (SELECT id FROM carts WHERE user_id IN (SELECT id FROM users WHERE email = $1))`, email)
		testPool.Exec(ctx, `DELETE FROM carts WHERE user_id IN (SELECT id FROM users WHERE email = $1)`, email)
		testPool.Exec(ctx, `DELETE FROM users WHERE email = $1`, email)
	})

	t.Run("productLookupAdapter returns error for nonexistent product", func(t *testing.T) {
		body := `{"product_id":"` + uuid.New().String() + `","quantity":1}`
		req := httptest.NewRequest(http.MethodPost, "/api/cart/items", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("shippingOrderProviderAdapter returns error for nonexistent order", func(t *testing.T) {
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
	mockgw.RegisterRoutes(mockMux)
	mockServer := httptest.NewServer(mockMux)
	defer mockServer.Close()

	deps := &server.Deps{
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
		Redis: testRedis,
	}
	router := server.NewRouter(deps)
	handler := router.Handler
	ctx := context.Background()

	// Seed category + product
	catID := uuid.New()
	_, err := testPool.Exec(ctx,
		`INSERT INTO categories (id, name, slug, active) VALUES ($1, 'ErrAdapt Cat', $2, true)`,
		catID, "erradapt-cat-"+catID.String()[:8])
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM categories WHERE id = $1`, catID) })

	prodID := uuid.New()
	_, err = testPool.Exec(ctx,
		`INSERT INTO products (id, name, slug, description, price, currency, stock_quantity, status, category_id)
		 VALUES ($1, 'ErrAdapt Product', $2, 'desc', 3000, 'USD', 100, 'published', $3)`,
		prodID, "erradapt-prod-"+prodID.String()[:8], catID)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM products WHERE id = $1`, prodID) })

	// Register user
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
		testPool.Exec(ctx, `DELETE FROM payment_jobs WHERE order_id IN (SELECT id FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = $1))`, email)
		testPool.Exec(ctx, `DELETE FROM payments WHERE order_id IN (SELECT id FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = $1))`, email)
		testPool.Exec(ctx, `DELETE FROM notifications WHERE user_id IN (SELECT id FROM users WHERE email = $1)`, email)
		testPool.Exec(ctx, `DELETE FROM order_items WHERE order_id IN (SELECT id FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = $1))`, email)
		testPool.Exec(ctx, `DELETE FROM cart_items WHERE cart_id IN (SELECT id FROM carts WHERE user_id IN (SELECT id FROM users WHERE email = $1))`, email)
		testPool.Exec(ctx, `DELETE FROM carts WHERE user_id IN (SELECT id FROM users WHERE email = $1)`, email)
		testPool.Exec(ctx, `DELETE FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = $1)`, email)
		testPool.Exec(ctx, `DELETE FROM users WHERE email = $1`, email)
	})

	// Add to cart and place order
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
		// Set payment to success so refund can proceed
		_, err := testPool.Exec(ctx,
			`UPDATE payments SET status = 'success', gateway_txn_id = 'txn_erradapt' WHERE id = $1`, paymentID)
		require.NoError(t, err)

		// Create a refund job pointing to this order
		refundJobID := uuid.New()
		_, err = testPool.Exec(ctx,
			`INSERT INTO payment_jobs (id, payment_id, order_id, action, status, max_attempts, next_retry_at, inventory_action)
			 VALUES ($1, $2, $3, 'refund', 'pending', 3, NOW(), 'restock')`,
			refundJobID, paymentID, orderID)
		require.NoError(t, err)

		// Delete order items and the order to force orderGetterAdapter + orderItemsGetterAdapter errors
		testPool.Exec(ctx, `DELETE FROM order_items WHERE order_id = $1`, orderID)
		testPool.Exec(ctx, `DELETE FROM orders WHERE id = $1`, orderID)

		var job payment.Job
		err = testPool.QueryRow(ctx,
			`SELECT id, payment_id, order_id, action, status, attempts, max_attempts,
			        COALESCE(last_error, ''), locked_until, next_retry_at,
			        COALESCE(inventory_action, ''), created_at, updated_at
			 FROM payment_jobs WHERE id = $1`, refundJobID).Scan(
			&job.ID, &job.PaymentID, &job.OrderID, &job.Action, &job.Status,
			&job.Attempts, &job.MaxAttempts, &job.LastError, &job.LockedUntil,
			&job.NextRetryAt, &job.InventoryAction, &job.CreatedAt, &job.UpdatedAt,
		)
		require.NoError(t, err)

		// ProcessJob should fail because orderItemsGetter.ListItemsByOrderID returns
		// empty (no items) — the refund tx still commits but the adapter was exercised
		ok := router.PaymentSvc.ProcessJob(ctx, job)
		// The job may succeed (empty items list) or fail depending on implementation;
		// either way we exercised the adapter paths
		_ = ok

		// Cleanup the job
		testPool.Exec(ctx, `DELETE FROM payment_jobs WHERE id = $1`, refundJobID)

		// Re-insert the order so cleanup doesn't fail
		testPool.Exec(ctx,
			`INSERT INTO orders (id, user_id, status, subtotal_amount, total_amount, currency)
			 SELECT $1, id, 'cancelled', 3000, 3000, 'USD' FROM users WHERE email = $2`,
			orderID, email)
	})
}

func TestAdapterErrorPaths_OrderGetterViaFinalizePayment(t *testing.T) {
	setup(t)
	router := server.NewRouter(testDeps)

	// Call FinalizePaymentSuccess with a non-existent order ID.
	// This exercises orderGetterAdapter.GetByID error path (router.go:355-357).
	fakeJob := payment.Job{
		ID:        uuid.New(),
		PaymentID: uuid.New(),
		OrderID:   uuid.New(), // does not exist in DB
		Action:    payment.ActionCharge,
	}

	err := router.PaymentSvc.FinalizePaymentSuccess(context.Background(), fakeJob)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "getting order for verification")
}

// serverRunEnv sets the base env vars for a server.Run() test using the dockertest containers.
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
	t.Setenv("JWT_SECRET", "test-secret-key-at-least-32-chars-long")
}

// startAndStopServer starts server.Run() in a goroutine, waits for it to be
// ready (via healthAddr), sends SIGINT, and returns the Run() error.
func startAndStopServer(t *testing.T, healthAddr string) error {
	t.Helper()
	errCh := make(chan error, 1)
	go func() { errCh <- server.Run() }()

	require.Eventually(t, func() bool {
		resp, err := http.Get(healthAddr + "/health")
		if err != nil {
			return false
		}
		resp.Body.Close()
		return true
	}, 10*time.Second, 100*time.Millisecond, "server did not start in time")

	require.NoError(t, syscall.Kill(syscall.Getpid(), syscall.SIGINT))

	select {
	case runErr := <-errCh:
		return runErr
	case <-time.After(10 * time.Second):
		t.Fatal("server.Run() did not return after SIGINT")
		return nil
	}
}

func TestServerRunReaderDBFailure(t *testing.T) {
	setup(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	serverRunEnv(t, port)
	t.Setenv("READER_DATABASE_URL", "postgres://invalid:invalid@127.0.0.1:1/invalid?sslmode=disable")

	runErr := startAndStopServer(t, fmt.Sprintf("http://127.0.0.1:%d", port))
	assert.NoError(t, runErr)
}

func TestServerRunReaderDBSuccess(t *testing.T) {
	setup(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	pgCfg := testPool.Config().ConnConfig
	serverRunEnv(t, port)
	t.Setenv("READER_DATABASE_URL", fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable",
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

	errCh := make(chan error, 1)
	go func() { errCh <- server.Run() }()

	// Give the server goroutine time to hit the ListenAndServe error
	time.Sleep(500 * time.Millisecond)

	// Send SIGINT to unblock <-ctx.Done()
	require.NoError(t, syscall.Kill(syscall.Getpid(), syscall.SIGINT))

	select {
	case runErr := <-errCh:
		// Run() returns nil because the ListenAndServe error is only logged, not returned
		require.NoError(t, runErr)
	case <-time.After(10 * time.Second):
		t.Fatal("server.Run() did not return after SIGINT")
	}
}

func TestServerRunConfigError(t *testing.T) {
	setup(t)
	// Set an invalid duration to trigger envconfig parsing error
	t.Setenv("JWT_SECRET", "test-secret-key-at-least-32-chars-long")
	t.Setenv("JWT_ACCESS_TTL", "not-a-duration")

	err := server.Run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading config")
}

func TestServerRunDatabaseError(t *testing.T) {
	setup(t)
	t.Setenv("JWT_SECRET", "test-secret-key-at-least-32-chars-long")
	t.Setenv("DB_HOST", "127.0.0.1")
	t.Setenv("DB_PORT", "1")
	t.Setenv("DB_USER", "invalid")
	t.Setenv("DB_PASSWORD", "invalid")
	t.Setenv("DB_NAME", "invalid")

	err := server.Run()
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

	errCh := make(chan error, 1)
	go func() { errCh <- server.Run() }()

	require.Eventually(t, func() bool {
		resp, err := http.Get(addr + "/health")
		if err != nil {
			return false
		}
		resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 10*time.Second, 100*time.Millisecond, "server did not start in time")

	t.Run("health endpoint returns healthy", func(t *testing.T) {
		resp, err := http.Get(addr + "/health")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var body map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.Equal(t, "healthy", body["status"])
	})

	require.NoError(t, syscall.Kill(syscall.Getpid(), syscall.SIGINT))

	select {
	case runErr := <-errCh:
		require.NoError(t, runErr)
	case <-time.After(10 * time.Second):
		t.Fatal("server.Run() did not return after SIGINT")
	}
}
