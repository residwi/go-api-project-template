package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequireUser(t *testing.T) {
	t.Run("returns the user when present in context", func(t *testing.T) {
		want := UserContext{UserID: uuid.New(), Email: "a@example.com", Role: "user"}
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r = r.WithContext(SetUserContext(r.Context(), want))
		w := httptest.NewRecorder()

		got, ok := RequireUser(w, r)

		require.True(t, ok)
		assert.Equal(t, want, got)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("writes 401 when no user in context", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()

		_, ok := RequireUser(w, r)

		assert.False(t, ok)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}
