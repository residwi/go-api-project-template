package http

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRouteSnapshot asserts every route in testdata/routes.golden is still
// mounted, and still on the same middleware group. http.ServeMux exposes no
// route table, so each route is probed instead: an unmounted path falls through
// to Go's default 404, which writes text/plain, while every mounted route
// answers application/json -- including the 401 and 403 the auth middleware
// writes.
func TestRouteSnapshot(t *testing.T) {
	setup(t)
	handler := NewRouter(testDeps, testApp)
	token := registerProbeUser(t, handler)

	for _, r := range loadGoldenRoutes(t) {
		t.Run(r.method+" "+r.path+" is on the "+r.group+" group", func(t *testing.T) {
			anon := probe(handler, r.method, r.path, "")
			require.Contains(t, anon.Header().Get("Content-Type"), "application/json",
				"route is not mounted: mux fell through to the default 404")

			switch r.group {
			case "api":
				assert.NotEqual(t, http.StatusUnauthorized, anon.Code,
					"public route must not require auth")
			case "authed":
				assert.Equal(t, http.StatusUnauthorized, anon.Code)
				authed := probe(handler, r.method, r.path, token)
				assert.NotEqual(t, http.StatusUnauthorized, authed.Code)
				assert.NotEqual(t, http.StatusForbidden, authed.Code,
					"route landed on the admin group")
			case "admin":
				assert.Equal(t, http.StatusUnauthorized, anon.Code)
				authed := probe(handler, r.method, r.path, token)
				assert.Equal(t, http.StatusForbidden, authed.Code,
					"route did not land on the admin group")
			default:
				t.Fatalf("unknown group %q", r.group)
			}
		})
	}
}

type goldenRoute struct {
	method string
	path   string
	group  string
}

// loadGoldenRoutes reads testdata/routes.golden and substitutes a concrete
// value for every wildcard, so the probe reaches the handler rather than the
// mux's pattern parser.
func loadGoldenRoutes(t *testing.T) []goldenRoute {
	t.Helper()

	f, err := os.Open("testdata/routes.golden")
	require.NoError(t, err)
	t.Cleanup(func() { f.Close() })

	replacer := strings.NewReplacer(
		"{id}", "11111111-1111-1111-1111-111111111111",
		"{product_id}", "22222222-2222-2222-2222-222222222222",
		"{slug}", "no-such-slug",
	)

	var routes []goldenRoute
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		require.Len(t, parts, 3, "malformed golden line: %q", line)
		routes = append(routes, goldenRoute{
			method: parts[0],
			path:   replacer.Replace(parts[1]),
			group:  parts[2],
		})
	}
	require.NoError(t, scanner.Err())
	require.Len(t, routes, 64)
	return routes
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

// registerProbeUser mints a non-admin access token through the real register
// route, the same way TestAuthenticatedEndpoints does.
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
