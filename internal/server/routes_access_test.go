package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const probeSlug = "route-probe"

var allRoutes = []string{
	"GET\t/health",
	"POST\t/api/auth/register",
	"POST\t/api/auth/login",
	"POST\t/api/auth/refresh",
	"GET\t/api/users/me",
	"PUT\t/api/users/me",
	"GET\t/api/admin/users",
	"GET\t/api/admin/users/{id}",
	"PUT\t/api/admin/users/{id}",
	"PUT\t/api/admin/users/{id}/role",
	"DELETE\t/api/admin/users/{id}",
	"GET\t/api/categories",
	"GET\t/api/categories/{slug}",
	"POST\t/api/admin/categories",
	"PUT\t/api/admin/categories/{id}",
	"DELETE\t/api/admin/categories/{id}",
	"GET\t/api/products",
	"GET\t/api/products/{slug}",
	"GET\t/api/admin/products",
	"GET\t/api/admin/products/{id}",
	"POST\t/api/admin/products",
	"PUT\t/api/admin/products/{id}",
	"DELETE\t/api/admin/products/{id}",
	"GET\t/api/admin/inventory/{product_id}",
	"PUT\t/api/admin/inventory/{product_id}/restock",
	"PUT\t/api/admin/inventory/{product_id}/adjust",
	"GET\t/api/cart",
	"POST\t/api/cart/items",
	"PUT\t/api/cart/items/{product_id}",
	"DELETE\t/api/cart/items/{product_id}",
	"DELETE\t/api/cart",
	"GET\t/api/orders",
	"GET\t/api/orders/{id}",
	"GET\t/api/admin/orders",
	"GET\t/api/admin/orders/{id}",
	"PUT\t/api/admin/orders/{id}/status",
	"POST\t/api/orders",
	"POST\t/api/orders/{id}/pay",
	"POST\t/api/orders/{id}/cancel",
	"POST\t/api/payments/webhook",
	"GET\t/api/admin/payments",
	"GET\t/api/admin/payments/{id}",
	"POST\t/api/admin/payments/{id}/refund",
	"GET\t/api/orders/{id}/shipping",
	"POST\t/api/admin/orders/{id}/ship",
	"PUT\t/api/admin/shipments/{id}/tracking",
	"POST\t/api/admin/shipments/{id}/deliver",
	"GET\t/api/products/{id}/reviews",
	"POST\t/api/products/{id}/reviews",
	"DELETE\t/api/admin/reviews/{id}",
	"POST\t/api/promotions/apply",
	"POST\t/api/admin/promotions",
	"GET\t/api/admin/promotions",
	"PUT\t/api/admin/promotions/{id}",
	"DELETE\t/api/admin/promotions/{id}",
	"GET\t/api/wishlist",
	"POST\t/api/wishlist/items",
	"DELETE\t/api/wishlist/items/{product_id}",
	"GET\t/api/notifications",
	"GET\t/api/notifications/unread-count",
	"PUT\t/api/notifications/{id}/read",
	"PUT\t/api/notifications/read-all",
	"GET\t/api/admin/dashboard/summary",
	"GET\t/api/admin/dashboard/top-products",
	"GET\t/api/admin/dashboard/revenue",
}

var publicRoutes = []string{
	"GET\t/health",
	"POST\t/api/auth/login",
	"POST\t/api/auth/refresh",
	"POST\t/api/auth/register",
	"GET\t/api/categories",
	"GET\t/api/categories/{slug}",
	"POST\t/api/payments/webhook",
	"GET\t/api/products",
	"GET\t/api/products/{id}/reviews",
	"GET\t/api/products/{slug}",
}

var wildcards = strings.NewReplacer(
	"{id}", "11111111-1111-1111-1111-111111111111",
	"{product_id}", "22222222-2222-2222-2222-222222222222",
	"{slug}", probeSlug,
)

func TestRouteAccess(t *testing.T) {
	setup(t)
	handler := newTestRouter(testPaymentCfg)
	token := registerProbeUser(t, handler)
	seedProbeSlug(t)

	for _, want := range publicRoutes {
		assert.Contains(t, allRoutes, want, "publicRoutes names a route absent from allRoutes")
	}

	for _, route := range allRoutes {
		t.Run(route, func(t *testing.T) {
			method, pattern, _ := strings.Cut(route, "\t")
			path := wildcards.Replace(pattern)
			anon := probe(handler, method, path, "")

			switch {
			case slices.Contains(publicRoutes, route):
				assert.NotEqual(t, http.StatusUnauthorized, anon.Code,
					"public route must not require auth")
				assert.NotEqual(t, http.StatusNotFound, anon.Code,
					"public route did not match any mounted pattern")

			case strings.HasPrefix(pattern, "/api/admin/"):
				assert.Equal(t, http.StatusUnauthorized, anon.Code)
				assert.Equal(t, http.StatusForbidden, probe(handler, method, path, token).Code,
					"admin route did not land on the admin group")

			default:
				assert.Equal(t, http.StatusUnauthorized, anon.Code,
					"route is not in publicRoutes but serves anonymous callers")
				authed := probe(handler, method, path, token)
				assert.NotEqual(t, http.StatusUnauthorized, authed.Code,
					"authed route rejected a valid token")
				assert.NotEqual(t, http.StatusForbidden, authed.Code,
					"route landed on the admin group")
			}
		})
	}
}

func probe(handler http.Handler, method, path, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w
}

func registerProbeUser(t *testing.T, handler http.Handler) string {
	t.Helper()

	body := `{"email":"route-probe@example.com","password":"Password123!",` +
		`"first_name":"Route","last_name":"Probe"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	t.Cleanup(func() {
		testPool.Exec(t.Context(), `DELETE FROM users WHERE email = 'route-probe@example.com'`)
	})

	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	data, ok := resp["data"].(map[string]any)
	require.True(t, ok)
	token, ok := data["access_token"].(string)
	require.True(t, ok)
	return token
}

func seedProbeSlug(t *testing.T) {
	t.Helper()

	catID := uuid.New()
	_, err := testPool.Exec(t.Context(),
		`INSERT INTO categories (id, name, slug, active) VALUES ($1, 'Route Probe Category', $2, true)`,
		catID, probeSlug)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(t.Context(), `DELETE FROM categories WHERE id = $1`, catID) })

	prodID := uuid.New()
	_, err = testPool.Exec(t.Context(),
		`INSERT INTO products (id, name, slug, description, price, currency, status)
		 VALUES ($1, 'Route Probe Product', $2, 'seeded for route access probe', 1000, 'USD', 'published')`,
		prodID, probeSlug)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(t.Context(), `DELETE FROM products WHERE id = $1`, prodID) })
}
