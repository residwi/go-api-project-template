package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/residwi/go-api-project-template/internal/platform/identity"
)

func TestRequireRole(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})

	t.Run("passes a caller holding the required role", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r = r.WithContext(identity.NewContext(r.Context(), identity.Identity{UserID: uuid.New(), Role: "admin"}))

		RequireRole("admin")(next).ServeHTTP(w, r)

		assert.Equal(t, http.StatusTeapot, w.Code)
	})

	t.Run("rejects a caller holding a different role with 403", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r = r.WithContext(identity.NewContext(r.Context(), identity.Identity{UserID: uuid.New(), Role: "user"}))

		RequireRole("admin")(next).ServeHTTP(w, r)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("rejects an anonymous caller with 401", func(t *testing.T) {
		w := httptest.NewRecorder()

		RequireRole("admin")(next).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("gates on the role it was given, not on admin", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r = r.WithContext(identity.NewContext(r.Context(), identity.Identity{UserID: uuid.New(), Role: "auditor"}))

		RequireRole("auditor")(next).ServeHTTP(w, r)

		assert.Equal(t, http.StatusTeapot, w.Code)
	})
}
