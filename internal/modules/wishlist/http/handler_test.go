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

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/wishlist"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
	wishMocks "github.com/residwi/go-api-project-template/mocks/wishlist"
)

func TestHandler_GetWishlist_Success(t *testing.T) {
	t.Parallel()

	t.Run("success with items", func(t *testing.T) {
		t.Parallel()

		mux, repo, uc := setupWishlistMux(t)

		items := []wishlist.Item{
			{ID: uuid.New(), ProductID: uuid.New()},
		}
		repo.EXPECT().GetItems(mock.Anything, uc.UserID, mock.Anything).Return(items, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/wishlist", nil)
		r = withAuth(r, uc)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)
	})
}

func TestHandler_GetWishlist_ServiceError(t *testing.T) {
	t.Parallel()

	t.Run("repo error", func(t *testing.T) {
		t.Parallel()

		mux, repo, uc := setupWishlistMux(t)

		repo.EXPECT().GetItems(mock.Anything, uc.UserID, mock.Anything).Return(nil, assert.AnError)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/wishlist", nil)
		r = withAuth(r, uc)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestHandler_GetWishlist(t *testing.T) {
	t.Parallel()

	h := newTestHandler()

	t.Run("missing auth", func(t *testing.T) {
		t.Parallel()

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

func TestHandler_AddItem_Success(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, repo, uc := setupWishlistMux(t)

		productID := uuid.New()
		wishlistID := uuid.New()
		repo.EXPECT().GetOrCreate(mock.Anything, uc.UserID).Return(wishlistID, nil)
		repo.EXPECT().AddItem(mock.Anything, wishlistID, productID).Return(nil)

		body, _ := json.Marshal(map[string]any{"product_id": productID})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/wishlist/items", bytes.NewReader(body))
		r = withAuth(r, uc)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusCreated, w.Code)
	})
}

func TestHandler_AddItem_ServiceError(t *testing.T) {
	t.Parallel()

	t.Run("get or create fails", func(t *testing.T) {
		t.Parallel()

		mux, repo, uc := setupWishlistMux(t)

		productID := uuid.New()
		repo.EXPECT().GetOrCreate(mock.Anything, uc.UserID).Return(uuid.Nil, assert.AnError)

		body, _ := json.Marshal(map[string]any{"product_id": productID})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/wishlist/items", bytes.NewReader(body))
		r = withAuth(r, uc)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestHandler_AddItem(t *testing.T) {
	t.Parallel()

	h := newTestHandler()

	t.Run("missing auth", func(t *testing.T) {
		t.Parallel()

		r := httptest.NewRequest(http.MethodPost, "/wishlist/items", nil)
		w := httptest.NewRecorder()

		h.AddItem(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()

		r := httptest.NewRequest(http.MethodPost, "/wishlist/items", strings.NewReader("{bad"))
		r = setAuthContext(r)
		w := httptest.NewRecorder()

		h.AddItem(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("validation error missing product_id", func(t *testing.T) {
		t.Parallel()

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

func TestHandler_RemoveItem_Success(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, repo, uc := setupWishlistMux(t)

		productID := uuid.New()
		repo.EXPECT().RemoveItem(mock.Anything, uc.UserID, productID).Return(nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/wishlist/items/"+productID.String(), nil)
		r = withAuth(r, uc)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})
}

func TestHandler_RemoveItem_ServiceError(t *testing.T) {
	t.Parallel()

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		mux, repo, uc := setupWishlistMux(t)

		productID := uuid.New()
		repo.EXPECT().RemoveItem(mock.Anything, uc.UserID, productID).Return(apperror.ErrNotFound)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/wishlist/items/"+productID.String(), nil)
		r = withAuth(r, uc)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestHandler_RemoveItem(t *testing.T) {
	t.Parallel()

	h := newTestHandler()

	t.Run("missing auth", func(t *testing.T) {
		t.Parallel()

		r := httptest.NewRequest(http.MethodDelete, "/wishlist/items/"+uuid.NewString(), nil)
		w := httptest.NewRecorder()

		h.RemoveItem(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("invalid product UUID", func(t *testing.T) {
		t.Parallel()

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

func TestHandler_GetWishlist_Pagination(t *testing.T) {
	t.Parallel()

	t.Run("has more results triggers cursor", func(t *testing.T) {
		t.Parallel()

		mux, repo, uc := setupWishlistMux(t)

		now := time.Now()
		items := make([]wishlist.Item, 21)
		for i := range items {
			items[i] = wishlist.Item{
				ID:        uuid.New(),
				ProductID: uuid.New(),
				CreatedAt: now.Add(-time.Duration(i) * time.Minute),
			}
		}
		repo.EXPECT().GetItems(mock.Anything, uc.UserID, mock.Anything).Return(items, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/wishlist", nil)
		r = withAuth(r, uc)

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
}

// wishlist.Item (model.go) carries no json tags at all, so WishlistID would
// serialize under its own name if ever marshaled directly -- toItemResponse's
// explicit field list is the only thing keeping it off the wire. No other
// test in this file decodes an item's JSON shape.
func TestToItemResponse_OmitsInternalFields(t *testing.T) {
	t.Parallel()

	itemID, productID, listID := uuid.New(), uuid.New(), uuid.New()
	created := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)

	got := toItemResponse(wishlist.Item{
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

func setupWishlistMux(t *testing.T) (*http.ServeMux, *wishMocks.MockRepository, middleware.UserContext) {
	repo := wishMocks.NewMockRepository(t)
	svc := wishlist.NewService(repo)
	v := validator.New()

	mux := http.NewServeMux()
	authed := middleware.NewRouteGroup(mux, "/api/v1")

	RegisterRoutes(authed, RouteDeps{
		Validator: v,
		Service:   svc,
	})

	uc := middleware.UserContext{
		UserID: uuid.New(),
		Email:  "test@example.com",
		Role:   "user",
	}

	return mux, repo, uc
}

func withAuth(r *http.Request, uc middleware.UserContext) *http.Request {
	ctx := middleware.SetUserContext(r.Context(), uc)
	return r.WithContext(ctx)
}

func newTestHandler() *handler {
	return &handler{
		service:   &wishlist.Service{},
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
