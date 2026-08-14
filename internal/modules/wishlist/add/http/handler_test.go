package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

func TestHandler_Add_Success(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, cmd, uc := setupAddMux(t)

		productID := uuid.New()
		cmd.EXPECT().Execute(mock.Anything, uc.UserID, mock.Anything).Return(nil)

		body, _ := json.Marshal(map[string]any{"product_id": productID})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/wishlist/items", bytes.NewReader(body))
		r = withAuth(r, uc)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusCreated, w.Code)
	})
}

func TestHandler_Add_CommandError(t *testing.T) {
	t.Parallel()

	t.Run("get or create fails", func(t *testing.T) {
		t.Parallel()

		mux, cmd, uc := setupAddMux(t)

		productID := uuid.New()
		cmd.EXPECT().Execute(mock.Anything, uc.UserID, mock.Anything).Return(assert.AnError)

		body, _ := json.Marshal(map[string]any{"product_id": productID})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/wishlist/items", bytes.NewReader(body))
		r = withAuth(r, uc)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestHandler_Add(t *testing.T) {
	t.Parallel()

	h := &Handler{validator: validator.New()}

	t.Run("missing auth", func(t *testing.T) {
		t.Parallel()

		r := httptest.NewRequest(http.MethodPost, "/wishlist/items", nil)
		w := httptest.NewRecorder()

		h.Add(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()

		r := httptest.NewRequest(http.MethodPost, "/wishlist/items", strings.NewReader("{bad"))
		r = setAuthContext(r)
		w := httptest.NewRecorder()

		h.Add(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("validation error missing product_id", func(t *testing.T) {
		t.Parallel()

		r := httptest.NewRequest(http.MethodPost, "/wishlist/items", strings.NewReader(`{}`))
		r = setAuthContext(r)
		w := httptest.NewRecorder()

		h.Add(w, r)

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

func setupAddMux(t *testing.T) (*http.ServeMux, *MockItemAdder, middleware.UserContext) {
	cmd := NewMockItemAdder(t)
	v := validator.New()

	mux := http.NewServeMux()
	authed := middleware.NewRouteGroup(mux, "/api/v1")

	New(cmd, v).RegisterHTTP(authed)

	uc := middleware.UserContext{
		UserID: uuid.New(),
		Email:  "test@example.com",
		Role:   "user",
	}

	return mux, cmd, uc
}

func withAuth(r *http.Request, uc middleware.UserContext) *http.Request {
	ctx := middleware.SetUserContext(r.Context(), uc)
	return r.WithContext(ctx)
}

func setAuthContext(r *http.Request) *http.Request {
	ctx := middleware.SetUserContext(r.Context(), middleware.UserContext{
		UserID: uuid.New(),
		Email:  "test@example.com",
		Role:   "user",
	})
	return r.WithContext(ctx)
}
