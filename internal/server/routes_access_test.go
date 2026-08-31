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

	"github.com/residwi/go-api-project-template/internal/testutil"
)

const probeSlug = "route-probe"

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
	handler, mounted := newRouter(
		testAppCfg,
		withPayment(testPaymentCfg),
		testRedis,
		testutil.DiscardLogger(),
		newTestApp(testPaymentCfg),
	)
	token := registerProbeUser(t, handler)
	seedProbeSlug(t)

	require.Len(t, mounted, 65)

	for _, want := range publicRoutes {
		assert.Contains(t, mounted, want, "publicRoutes names a route that is not mounted")
	}

	for _, route := range mounted {
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
