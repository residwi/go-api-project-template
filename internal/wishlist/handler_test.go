package wishlist

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/middleware"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
)

func newTestHandler() *handler {
	return &handler{
		service:   &Service{},
		validator: validator.New(),
	}
}

func setAuthContext(r *http.Request) *http.Request {
	ctx := middleware.SetUserContext(r.Context(), middleware.UserContext{
		UserID: uuid.New(),
		Email:  "test@example.com",
		Role:   "user",
	})
	return r.WithContext(ctx)
}

func TestHandler_GetWishlist(t *testing.T) {
	h := newTestHandler()

	t.Run("missing auth", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/wishlist", nil)
		w := httptest.NewRecorder()

		h.GetWishlist(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		success, ok := resp["success"].(bool)
		require.True(t, ok)
		assert.False(t, success)
	})
}

func TestHandler_AddItem(t *testing.T) {
	h := newTestHandler()

	t.Run("missing auth", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/wishlist/items", nil)
		w := httptest.NewRecorder()

		h.AddItem(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/wishlist/items", strings.NewReader("{bad"))
		r = setAuthContext(r)
		w := httptest.NewRecorder()

		h.AddItem(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("validation error missing product_id", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/wishlist/items", strings.NewReader(`{}`))
		r = setAuthContext(r)
		w := httptest.NewRecorder()

		h.AddItem(w, r)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		success, ok := resp["success"].(bool)
		require.True(t, ok)
		assert.False(t, success)
		errBody, ok := resp["error"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "validation failed", errBody["message"])
	})
}

func TestHandler_RemoveItem(t *testing.T) {
	h := newTestHandler()

	t.Run("missing auth", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodDelete, "/wishlist/items/"+uuid.NewString(), nil)
		w := httptest.NewRecorder()

		h.RemoveItem(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("invalid product UUID", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodDelete, "/wishlist/items/bad", nil)
		r = setAuthContext(r)
		r.SetPathValue("product_id", "bad")
		w := httptest.NewRecorder()

		h.RemoveItem(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		errBody, ok := resp["error"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, errBody["message"], "invalid product_id")
	})
}
