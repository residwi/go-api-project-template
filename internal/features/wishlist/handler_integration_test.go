package wishlist_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/core/response"
	"github.com/residwi/go-api-project-template/internal/features/wishlist"
	"github.com/residwi/go-api-project-template/internal/middleware"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	wishMocks "github.com/residwi/go-api-project-template/mocks/wishlist"
)

func setupWishlistMux(t *testing.T) (*http.ServeMux, *wishMocks.MockRepository, middleware.UserContext) {
	repo := wishMocks.NewMockRepository(t)
	svc := wishlist.NewService(repo, nil)
	v := validator.New()

	mux := http.NewServeMux()
	authed := middleware.NewRouteGroup(mux, "/api/v1")

	wishlist.RegisterRoutes(authed, wishlist.RouteDeps{
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

func TestHandler_GetWishlist_Success(t *testing.T) {
	t.Run("success with items", func(t *testing.T) {
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
	t.Run("repo error", func(t *testing.T) {
		mux, repo, uc := setupWishlistMux(t)

		repo.EXPECT().GetItems(mock.Anything, uc.UserID, mock.Anything).Return(nil, assert.AnError)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/wishlist", nil)
		r = withAuth(r, uc)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestHandler_AddItem_Success(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mux, repo, uc := setupWishlistMux(t)

		productID := uuid.New()
		wishlistID := uuid.New()
		repo.EXPECT().GetOrCreate(mock.Anything, uc.UserID).Return(wishlistID, nil)
		repo.EXPECT().AddItem(mock.Anything, wishlistID, productID).Return(nil)

		body, _ := json.Marshal(wishlist.AddItemRequest{ProductID: productID})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/wishlist/items", bytes.NewReader(body))
		r = withAuth(r, uc)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusCreated, w.Code)
	})
}

func TestHandler_AddItem_ServiceError(t *testing.T) {
	t.Run("get or create fails", func(t *testing.T) {
		mux, repo, uc := setupWishlistMux(t)

		productID := uuid.New()
		repo.EXPECT().GetOrCreate(mock.Anything, uc.UserID).Return(uuid.Nil, assert.AnError)

		body, _ := json.Marshal(wishlist.AddItemRequest{ProductID: productID})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/wishlist/items", bytes.NewReader(body))
		r = withAuth(r, uc)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestHandler_RemoveItem_Success(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mux, repo, uc := setupWishlistMux(t)

		productID := uuid.New()
		wishlistID := uuid.New()
		repo.EXPECT().GetOrCreate(mock.Anything, uc.UserID).Return(wishlistID, nil)
		repo.EXPECT().RemoveItem(mock.Anything, wishlistID, productID).Return(nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/wishlist/items/"+productID.String(), nil)
		r = withAuth(r, uc)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})
}

func TestHandler_RemoveItem_ServiceError(t *testing.T) {
	t.Run("not found", func(t *testing.T) {
		mux, repo, uc := setupWishlistMux(t)

		productID := uuid.New()
		wishlistID := uuid.New()
		repo.EXPECT().GetOrCreate(mock.Anything, uc.UserID).Return(wishlistID, nil)
		repo.EXPECT().RemoveItem(mock.Anything, wishlistID, productID).Return(core.ErrNotFound)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/wishlist/items/"+productID.String(), nil)
		r = withAuth(r, uc)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestHandler_GetWishlist_Pagination(t *testing.T) {
	t.Run("has more results triggers cursor", func(t *testing.T) {
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
