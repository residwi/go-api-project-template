package http

import (
	"bytes"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/features/wishlist/domain"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/identity"
	"github.com/residwi/go-api-project-template/internal/platform/web"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

func TestHandler_Add(t *testing.T) {
	t.Parallel()

	t.Run("missing auth", func(t *testing.T) {
		t.Parallel()

		mux, _, _ := setupMux(t)

		r := httptest.NewRequest(http.MethodPost, "/api/v1/wishlist/items", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()

		mux, _, uc := setupMux(t)

		r := httptest.NewRequest(http.MethodPost, "/api/v1/wishlist/items", strings.NewReader("{bad"))
		r = withAuth(r, uc)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("validation error missing product_id", func(t *testing.T) {
		t.Parallel()

		mux, _, uc := setupMux(t)

		r := httptest.NewRequest(http.MethodPost, "/api/v1/wishlist/items", strings.NewReader(`{}`))
		r = withAuth(r, uc)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "validation failed", resp.Error.Message)
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, service, uc := setupMux(t)

		productID := uuid.New()
		service.EXPECT().Add(mock.Anything, uc.UserID, productID).Return(nil)

		body, _ := json.Marshal(map[string]any{"product_id": productID})
		r := httptest.NewRequest(http.MethodPost, "/api/v1/wishlist/items", bytes.NewReader(body))
		r = withAuth(r, uc)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusCreated, w.Code)
	})

	t.Run("service error", func(t *testing.T) {
		t.Parallel()

		mux, service, uc := setupMux(t)

		productID := uuid.New()
		service.EXPECT().Add(mock.Anything, uc.UserID, productID).Return(assert.AnError)

		body, _ := json.Marshal(map[string]any{"product_id": productID})
		r := httptest.NewRequest(http.MethodPost, "/api/v1/wishlist/items", bytes.NewReader(body))
		r = withAuth(r, uc)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestHandler_List(t *testing.T) {
	t.Parallel()

	t.Run("missing auth", func(t *testing.T) {
		t.Parallel()

		mux, _, _ := setupMux(t)

		r := httptest.NewRequest(http.MethodGet, "/api/v1/wishlist", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		success, ok := resp["success"].(bool)
		require.True(t, ok)
		assert.False(t, success)
	})

	t.Run("success with items", func(t *testing.T) {
		t.Parallel()

		mux, service, uc := setupMux(t)

		items := []domain.Item{{ID: uuid.New(), ProductID: uuid.New()}}
		service.EXPECT().List(mock.Anything, uc.UserID, mock.Anything).Return(items, nil)

		r := httptest.NewRequest(http.MethodGet, "/api/v1/wishlist", nil)
		r = withAuth(r, uc)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)
	})

	t.Run("has more results triggers cursor", func(t *testing.T) {
		t.Parallel()

		mux, service, uc := setupMux(t)

		now := time.Now()
		items := make([]domain.Item, 21)
		for i := range items {
			items[i] = domain.Item{
				ID:        uuid.New(),
				ProductID: uuid.New(),
				CreatedAt: now.Add(-time.Duration(i) * time.Minute),
			}
		}
		service.EXPECT().List(mock.Anything, uc.UserID, mock.Anything).Return(items, nil)

		r := httptest.NewRequest(http.MethodGet, "/api/v1/wishlist", nil)
		r = withAuth(r, uc)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)

		data, ok := resp.Data.(map[string]any)
		require.True(t, ok)
		pagination, ok := data["pagination"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, true, pagination["has_more"])
		assert.NotEmpty(t, pagination["next_cursor"])
	})

	t.Run("repo error", func(t *testing.T) {
		t.Parallel()

		mux, service, uc := setupMux(t)

		service.EXPECT().List(mock.Anything, uc.UserID, mock.Anything).Return(nil, assert.AnError)

		r := httptest.NewRequest(http.MethodGet, "/api/v1/wishlist", nil)
		r = withAuth(r, uc)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestHandler_Remove(t *testing.T) {
	t.Parallel()

	t.Run("missing auth", func(t *testing.T) {
		t.Parallel()

		mux, _, _ := setupMux(t)

		r := httptest.NewRequest(http.MethodDelete, "/api/v1/wishlist/items/"+uuid.NewString(), nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("invalid product UUID", func(t *testing.T) {
		t.Parallel()

		mux, _, uc := setupMux(t)

		r := httptest.NewRequest(http.MethodDelete, "/api/v1/wishlist/items/bad", nil)
		r = withAuth(r, uc)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		errBody, ok := resp["error"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, errBody["message"], "invalid product_id")
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, service, uc := setupMux(t)

		productID := uuid.New()
		service.EXPECT().Remove(mock.Anything, uc.UserID, productID).Return(nil)

		r := httptest.NewRequest(http.MethodDelete, "/api/v1/wishlist/items/"+productID.String(), nil)
		r = withAuth(r, uc)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		mux, service, uc := setupMux(t)

		productID := uuid.New()
		service.EXPECT().Remove(mock.Anything, uc.UserID, productID).Return(errs.ErrNotFound)

		r := httptest.NewRequest(http.MethodDelete, "/api/v1/wishlist/items/"+productID.String(), nil)
		r = withAuth(r, uc)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestToItemResponse_OmitsInternalFields(t *testing.T) {
	t.Parallel()

	itemID, productID, listID := uuid.New(), uuid.New(), uuid.New()
	created := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)

	got := toItemResponse(domain.Item{
		ID:         itemID,
		WishlistID: listID,
		ProductID:  productID,
		CreatedAt:  created,
	})

	raw, err := json.Marshal(got)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))

	assert.ElementsMatch(t, []string{"id", "product_id", "created_at"}, slices.Collect(maps.Keys(fields)),
		"the response must expose exactly these fields")
	assert.NotContains(t, string(raw), listID.String(),
		"wishlist_id is an internal join key and must not be serialised")
}

func setupMux(t *testing.T) (*http.ServeMux, *MockWishlistManager, identity.Identity) {
	service := NewMockWishlistManager(t)

	mux := http.NewServeMux()
	authed := web.NewRouter(mux).Group("/api/v1")

	h := NewHandler(service)
	authed.HandleFunc("GET /wishlist", h.List)
	authed.HandleFunc("POST /wishlist/items", h.Add)
	authed.HandleFunc("DELETE /wishlist/items/{product_id}", h.Remove)

	uc := identity.Identity{
		UserID: uuid.New(),
		Role:   "user",
	}

	return mux, service, uc
}

func withAuth(r *http.Request, uc identity.Identity) *http.Request {
	ctx := identity.NewContext(r.Context(), uc)
	return r.WithContext(ctx)
}
