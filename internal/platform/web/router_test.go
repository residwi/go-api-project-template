package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRouterGroup(t *testing.T) {
	t.Parallel()

	t.Run("nested groups compose their prefixes", func(t *testing.T) {
		t.Parallel()

		mux := http.NewServeMux()
		router := NewRouter(mux)
		admin := router.Group("/api").Group("/admin")
		admin.HandleFunc("GET /users", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		})

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/admin/users", nil))

		assert.Equal(t, http.StatusTeapot, w.Code)
	})

	t.Run("a derived group runs the parent middleware outside its own", func(t *testing.T) {
		t.Parallel()

		var order []string
		mark := func(name string) Middleware {
			return func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					order = append(order, name)
					next.ServeHTTP(w, r)
				})
			}
		}

		mux := http.NewServeMux()
		parent := NewRouter(mux).Group("/api", mark("parent"))
		child := parent.Group("/admin", mark("child"))
		child.HandleFunc("GET /x", func(_ http.ResponseWriter, _ *http.Request) {
			order = append(order, "handler")
		})

		mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/admin/x", nil))

		assert.Equal(t, []string{"parent", "child", "handler"}, order)
	})

	t.Run("sibling groups do not inherit each other's middleware", func(t *testing.T) {
		t.Parallel()

		var seen []string
		mark := func(name string) Middleware {
			return func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					seen = append(seen, name)
					next.ServeHTTP(w, r)
				})
			}
		}

		mux := http.NewServeMux()
		router := NewRouter(mux)
		left := router.Group("/left", mark("left"))
		right := router.Group("/right", mark("right"))
		left.HandleFunc("GET /x", func(http.ResponseWriter, *http.Request) {})
		right.HandleFunc("GET /x", func(http.ResponseWriter, *http.Request) {})

		mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/right/x", nil))

		assert.Equal(t, []string{"right"}, seen)
	})

	t.Run("the root group applies no middleware", func(t *testing.T) {
		t.Parallel()

		mux := http.NewServeMux()
		router := NewRouter(mux)
		router.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))

		assert.Equal(t, http.StatusNoContent, w.Code)
	})
}

func TestRouterRoutes(t *testing.T) {
	t.Parallel()

	t.Run("records every route mounted through the root and its derived groups", func(t *testing.T) {
		t.Parallel()

		noop := func(http.ResponseWriter, *http.Request) {}

		router := NewRouter(http.NewServeMux())
		router.HandleFunc("GET /health", noop)
		api := router.Group("/api")
		api.HandleFunc("GET /products", noop)
		admin := api.Group("/admin")
		admin.HandleFunc("DELETE /products/{id}", noop)

		assert.Equal(t, []string{
			"GET\t/health",
			"GET\t/api/products",
			"DELETE\t/api/admin/products/{id}",
		}, router.Routes())
	})

	t.Run("a derived group reports the whole table, not only its own routes", func(t *testing.T) {
		t.Parallel()

		noop := func(http.ResponseWriter, *http.Request) {}

		router := NewRouter(http.NewServeMux())
		api := router.Group("/api")
		api.HandleFunc("GET /a", noop)
		admin := api.Group("/admin")
		admin.HandleFunc("GET /b", noop)

		assert.Equal(t, router.Routes(), admin.Routes())
	})

	t.Run("the returned slice is a copy", func(t *testing.T) {
		t.Parallel()

		router := NewRouter(http.NewServeMux())
		router.HandleFunc("GET /a", func(http.ResponseWriter, *http.Request) {})

		got := router.Routes()
		require.Len(t, got, 1)
		got[0] = "mutated"

		assert.Equal(t, []string{"GET\t/a"}, router.Routes())
	})

	t.Run("Handle registers the same pattern HandleFunc does", func(t *testing.T) {
		t.Parallel()

		router := NewRouter(http.NewServeMux())
		router.Group("/api").Handle("POST /orders", http.NotFoundHandler())

		assert.Equal(t, []string{"POST\t/api/orders"}, router.Routes())
	})
}
