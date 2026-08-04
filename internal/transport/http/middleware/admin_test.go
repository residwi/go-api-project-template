package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestRequireAdmin(t *testing.T) {
	t.Run("NoUserContext", func(t *testing.T) {
		handler := RequireAdmin(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			t.Fatal("handler should not be called")
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("NonAdminRole", func(t *testing.T) {
		handler := RequireAdmin(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			t.Fatal("handler should not be called")
		}))

		ctx := SetUserContext(
			httptest.NewRequest(http.MethodGet, "/", nil).Context(),
			UserContext{UserID: uuid.New(), Email: "user@example.com", Role: "user"},
		)
		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("AdminRole", func(t *testing.T) {
		called := false
		handler := RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		}))

		ctx := SetUserContext(
			httptest.NewRequest(http.MethodGet, "/", nil).Context(),
			UserContext{UserID: uuid.New(), Email: "admin@example.com", Role: "admin"},
		)
		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.True(t, called)
		assert.Equal(t, http.StatusOK, rec.Code)
	})
}
