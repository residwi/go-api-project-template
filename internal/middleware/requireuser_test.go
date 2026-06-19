package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/middleware"
)

func TestRequireUser(t *testing.T) {
	t.Run("returns the user when present in context", func(t *testing.T) {
		want := middleware.UserContext{UserID: uuid.New(), Email: "a@example.com", Role: "user"}
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r = r.WithContext(middleware.SetUserContext(r.Context(), want))
		w := httptest.NewRecorder()

		got, ok := middleware.RequireUser(w, r)

		require.True(t, ok)
		assert.Equal(t, want, got)
		assert.Equal(t, http.StatusOK, w.Code) // nothing written when authenticated
	})

	t.Run("writes 401 when no user in context", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()

		_, ok := middleware.RequireUser(w, r)

		assert.False(t, ok)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}
