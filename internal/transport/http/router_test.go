package http_test

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
	"github.com/residwi/go-api-project-template/internal/bootstrap"
	"github.com/residwi/go-api-project-template/internal/config"
	cartpg "github.com/residwi/go-api-project-template/internal/modules/cart/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/inventory"
	inventorypg "github.com/residwi/go-api-project-template/internal/modules/inventory/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/notification"
	notificationpg "github.com/residwi/go-api-project-template/internal/modules/notification/postgres"
	orderpg "github.com/residwi/go-api-project-template/internal/modules/order/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/payment"
	mockgateway "github.com/residwi/go-api-project-template/internal/modules/payment/mock"
	paymentpg "github.com/residwi/go-api-project-template/internal/modules/payment/postgres"
	productpg "github.com/residwi/go-api-project-template/internal/modules/product/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/promotion"
	promotionpg "github.com/residwi/go-api-project-template/internal/modules/promotion/postgres"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/testhelper"
	apihttp "github.com/residwi/go-api-project-template/internal/transport/http"
)

var (
	testPool  *pgxpool.Pool
	testRedis *redis.Client
	testDeps  *apihttp.Deps
)

func TestMain(m *testing.M) {
	pool, cleanupPG := testhelper.MustStartPostgres("test_server")
	defer cleanupPG()
	testPool = pool

	rdb, cleanupRedis := testhelper.MustStartRedis(3)
	defer cleanupRedis()
	testRedis = rdb

	testDeps = &apihttp.Deps{
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
		Cache: rdb,
	}

	os.Exit(m.Run())
}

func setup(t *testing.T) {
	t.Helper()
	testhelper.ResetDB(t, testPool)
	testhelper.ResetRedis(t, testRedis)
}

// newPaymentServiceForTest composes a payment service the way cmd/worker does.
// test/e2e carries its own copy in testmain_test.go; keep the two in step.
func newPaymentServiceForTest(t *testing.T, gatewayURL string) *payment.Service {
	t.Helper()

	txRunner := database.NewTxRunner(testPool)
	inventorySvc := inventory.NewService(inventorypg.New(testPool))
	productSvc := bootstrap.NewProductService(productpg.New(testPool), inventorySvc)
	cartSvc := bootstrap.NewCartService(cartpg.New(testPool), txRunner, productSvc, 50)
	promotionSvc := promotion.NewService(promotionpg.New(testPool), txRunner)
	notificationSvc := notification.NewService(notificationpg.New(testPool))

	orderSvc := bootstrap.NewOrderService(
		orderpg.New(testPool), txRunner, cartSvc, inventorySvc, promotionSvc, notificationSvc,
	)
	gw := mockgateway.New(gatewayURL, 5*time.Second)
	paymentSvc := bootstrap.NewPaymentService(
		paymentpg.New(testPool), txRunner, gw, orderSvc, inventorySvc, promotionSvc,
	)
	bootstrap.SetOrderPaymentDeps(orderSvc, paymentSvc)

	return paymentSvc
}

// seedInventoryLevel gives a product an inventory_levels row so ReserveBatch/
// DeductBatch have something to update. product.Service.Create does register a
// new product with inventory (via EnsureLevel), but the row it writes is zeroed,
// and the tests here insert products with raw SQL anyway, bypassing Create
// entirely -- so there is no row at all. Either way the stock is seeded here.
//
// test/e2e/testmain_test.go carries its own copy for the saga flows. Keep the
// two in step.
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

func TestNewRouter(t *testing.T) {
	setup(t)
	t.Run("initializes without error", func(t *testing.T) {
		handler := apihttp.NewRouter(testDeps)
		require.NotNil(t, handler)
	})
}

func TestHealthHandler(t *testing.T) {
	setup(t)
	handler := apihttp.NewRouter(testDeps).Handler

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

		badDeps := &apihttp.Deps{
			Config: testDeps.Config,
			Pool:   badPool,
			Cache:  testRedis,
		}
		h := apihttp.NewRouter(badDeps).Handler

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

		badDeps := &apihttp.Deps{
			Config: testDeps.Config,
			Pool:   testPool,
			Cache:  badRedis,
		}
		h := apihttp.NewRouter(badDeps).Handler

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
		nilRedisDeps := &apihttp.Deps{
			Config: testDeps.Config,
			Pool:   testPool,
			Cache:  nil,
		}
		h := apihttp.NewRouter(nilRedisDeps).Handler

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
	handler := apihttp.NewRouter(testDeps).Handler

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
	handler := apihttp.NewRouter(testDeps).Handler
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
		user, ok := data["user"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "test-login@example.com", user["email"])
		assert.Equal(t, "Login", user["first_name"])
		assert.Equal(t, "User", user["last_name"])
		assert.Equal(t, "user", user["role"])

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
	handler := apihttp.NewRouter(testDeps).Handler

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
	handler := apihttp.NewRouter(testDeps).Handler

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
	handler := apihttp.NewRouter(testDeps).Handler
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
	handler := apihttp.NewRouter(testDeps).Handler
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

func TestHealthHandler_NilRedis(t *testing.T) {
	setup(t)
	nilRedisDeps := &apihttp.Deps{
		Config: testDeps.Config,
		Pool:   testDeps.Pool,
		Cache:  nil,
	}
	handler := apihttp.NewRouter(nilRedisDeps).Handler

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

func TestCORSHeaders(t *testing.T) {
	setup(t)
	handler := apihttp.NewRouter(testDeps).Handler

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
	handler := apihttp.NewRouter(testDeps).Handler
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
	router := apihttp.NewRouter(deps)
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
		`INSERT INTO products (id, name, slug, description, price, currency, status, category_id)
		 VALUES ($1, 'ErrAdapt Product', $2, 'desc', 3000, 'USD', 'published', $3)`,
		prodID, "erradapt-prod-"+prodID.String()[:8], catID)
	require.NoError(t, err)
	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM inventory_levels WHERE product_id = $1`, prodID)
		testPool.Exec(ctx, `DELETE FROM products WHERE id = $1`, prodID)
	})
	seedInventoryLevel(t, prodID, 100, 0)

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
			`INSERT INTO payment_jobs (id, payment_id, order_id, action, status, max_attempts, next_retry_at)
			 VALUES ($1, $2, $3, 'refund', 'pending', 3, NOW())`,
			refundJobID, paymentID, orderID)
		require.NoError(t, err)

		// Delete order items and the order to force orderGetterAdapter + orderItemsGetterAdapter errors
		testPool.Exec(ctx, `DELETE FROM order_items WHERE order_id = $1`, orderID)
		testPool.Exec(ctx, `DELETE FROM orders WHERE id = $1`, orderID)

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

		// The outcome is not asserted: this exists to drive the order-facing adapters
		// with an order whose items are gone.
		_ = newPaymentServiceForTest(t, mockServer.URL+"/mock/payment").Process(ctx, job)

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

	// A missing order drives orderGetterAdapter.GetByID down its error path.
	fakeJob := payment.Job{
		ID:        uuid.New(),
		PaymentID: uuid.New(),
		OrderID:   uuid.New(), // does not exist in DB
		Action:    payment.ActionCharge,
	}

	err := newPaymentServiceForTest(t, testDeps.Config.Payment.GatewayURL).FinalizePaymentSuccess(context.Background(), fakeJob)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "getting order for verification")
}

// serverRunEnv sets the base env vars for a apihttp.Run() test using the dockertest containers.
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

// startAndStopServer starts apihttp.RunContext in a goroutine, waits for it to
// be ready (via healthAddr), cancels the context to shut it down, and returns
// the RunContext error.
//
// Cancelling a context rather than signalling the test process's own PID is the
// point. The old SIGINT went to the *whole test process*, so it fired every
// signal handler in the binary, not just this server's -- and if the readiness
// wait below failed, the signal was never sent at all and the server goroutine
// leaked for the rest of the package's run. The deferred cancel now guarantees
// shutdown on that path too.
func startAndStopServer(t *testing.T, healthAddr string) error {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- apihttp.RunContext(ctx) }()

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
		t.Fatal("apihttp.RunContext did not return after its context was cancelled")
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- apihttp.RunContext(ctx) }()

	// Give the server goroutine time to hit the ListenAndServe error
	time.Sleep(500 * time.Millisecond)

	// Cancel to unblock <-ctx.Done()
	cancel()

	select {
	case runErr := <-errCh:
		// RunContext returns nil because the ListenAndServe error is only logged, not returned
		require.NoError(t, runErr)
	case <-time.After(30 * time.Second):
		t.Fatal("apihttp.RunContext did not return after its context was cancelled")
	}
}

func TestServerRunConfigError(t *testing.T) {
	setup(t)
	// Set an invalid duration to trigger envconfig parsing error
	t.Setenv("JWT_SECRET", "test-secret-key-at-least-32-chars-long")
	t.Setenv("JWT_ACCESS_TTL", "not-a-duration")

	err := apihttp.Run()
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

	err := apihttp.Run()
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
	go func() { errCh <- apihttp.RunContext(ctx) }()

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
		t.Fatal("apihttp.RunContext did not return after its context was cancelled")
	}
}
