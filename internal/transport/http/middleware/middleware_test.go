package middleware

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"

	"github.com/residwi/go-api-project-template/internal/testhelper"
)

var testRedis *redis.Client

func TestMain(m *testing.M) {
	rdb, cleanup := testhelper.MustStartRedis(1)
	defer cleanup()
	testRedis = rdb
	os.Exit(m.Run())
}

// testLogger is what the middleware under test log into. Nothing asserts on the
// output, and logger.Setup no longer installs a package default, so a discard
// handler is the whole requirement.
func testLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func TestChain_AppliesMiddlewareInCorrectOrder(t *testing.T) {
	var order []string

	mw1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "mw1-before")
			next.ServeHTTP(w, r)
			order = append(order, "mw1-after")
		})
	}

	mw2 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "mw2-before")
			next.ServeHTTP(w, r)
			order = append(order, "mw2-after")
		})
	}

	chain := Chain(mw1, mw2)
	handler := chain(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		order = append(order, "handler")
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	expected := []string{"mw1-before", "mw2-before", "handler", "mw2-after", "mw1-after"}
	assert.Equal(t, expected, order)
}

func TestNewRouteGroup_NoMiddlewareRegistersRoute(t *testing.T) {
	mux := http.NewServeMux()
	group := NewRouteGroup(mux, "/api")

	called := false
	group.Handle("GET /health", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestNewRouteGroup_WithMiddlewareWrapsHandler(t *testing.T) {
	mux := http.NewServeMux()

	mwCalled := false
	mw := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mwCalled = true
			next.ServeHTTP(w, r)
		})
	}

	group := NewRouteGroup(mux, "/api", mw)

	handlerCalled := false
	group.Handle("GET /test", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.True(t, mwCalled)
	assert.True(t, handlerCalled)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRouteGroup_HandleFuncDelegatesToHandle(t *testing.T) {
	mux := http.NewServeMux()
	group := NewRouteGroup(mux, "/api")

	called := false
	group.HandleFunc("POST /items", func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusCreated)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/items", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.True(t, called)
	assert.Equal(t, http.StatusCreated, rec.Code)
}
