package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/platform/identity"
)

func TestRequireUser(t *testing.T) {
	t.Run("returns the user when present in context", func(t *testing.T) {
		want := identity.Identity{UserID: uuid.New(), Role: "user"}
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r = r.WithContext(SetIdentity(r.Context(), want))
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
