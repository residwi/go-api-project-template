package promotion

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

func newTestPublicHandler() *publicHandler {
	return &publicHandler{
		service:   &Service{},
		validator: validator.New(),
	}
}

func newTestAdminHandler() *adminHandler {
	return &adminHandler{
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

func TestHandler_Apply(t *testing.T) {
	h := newTestPublicHandler()

	t.Run("missing auth", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/promotions/apply", nil)
		w := httptest.NewRecorder()

		h.Apply(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		success, ok := resp["success"].(bool)
		require.True(t, ok)
		assert.False(t, success)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/promotions/apply", strings.NewReader("{bad"))
		r = setAuthContext(r)
		w := httptest.NewRecorder()

		h.Apply(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("validation error missing fields", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/promotions/apply", strings.NewReader(`{}`))
		r = setAuthContext(r)
		w := httptest.NewRecorder()

		h.Apply(w, r)

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

func TestHandler_AdminCreate(t *testing.T) {
	h := newTestAdminHandler()

	t.Run("invalid JSON", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/promotions", strings.NewReader("{bad"))
		w := httptest.NewRecorder()

		h.Create(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("validation error missing fields", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/promotions", strings.NewReader(`{}`))
		w := httptest.NewRecorder()

		h.Create(w, r)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		errBody, ok := resp["error"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "validation failed", errBody["message"])
	})
}

func TestHandler_AdminUpdate(t *testing.T) {
	h := newTestAdminHandler()

	t.Run("invalid UUID", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPut, "/promotions/bad", nil)
		r.SetPathValue("id", "bad")
		w := httptest.NewRecorder()

		h.Update(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		errBody, ok := resp["error"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, errBody["message"], "invalid id")
	})

	t.Run("invalid JSON", func(t *testing.T) {
		id := uuid.NewString()
		r := httptest.NewRequest(http.MethodPut, "/promotions/"+id, strings.NewReader("{bad"))
		r.SetPathValue("id", id)
		w := httptest.NewRecorder()

		h.Update(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestHandler_AdminDelete(t *testing.T) {
	h := newTestAdminHandler()

	t.Run("invalid UUID", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodDelete, "/promotions/bad", nil)
		r.SetPathValue("id", "bad")
		w := httptest.NewRecorder()

		h.Delete(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		errBody, ok := resp["error"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, errBody["message"], "invalid id")
	})
}
