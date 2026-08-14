package http

import (
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/modules/wishlist/domain"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

func TestHandler_List_Success(t *testing.T) {
	t.Parallel()

	t.Run("success with items", func(t *testing.T) {
		t.Parallel()

		mux, reader, uc := setupQueryMux(t)

		items := []domain.Item{
			{ID: uuid.New(), ProductID: uuid.New()},
		}
		reader.EXPECT().ListItemsForUser(mock.Anything, uc.UserID, mock.Anything).Return(items, nil)

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

func TestHandler_List_ReaderError(t *testing.T) {
	t.Parallel()

	t.Run("repo error", func(t *testing.T) {
		t.Parallel()

		mux, reader, uc := setupQueryMux(t)

		reader.EXPECT().ListItemsForUser(mock.Anything, uc.UserID, mock.Anything).Return(nil, assert.AnError)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/wishlist", nil)
		r = withAuth(r, uc)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestHandler_List(t *testing.T) {
	t.Parallel()

	h := &Handler{}

	t.Run("missing auth", func(t *testing.T) {
		t.Parallel()

		r := httptest.NewRequest(http.MethodGet, "/wishlist", nil)
		w := httptest.NewRecorder()

		h.List(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		success, ok := resp["success"].(bool)
		require.True(t, ok)
		assert.False(t, success)
	})
}

func TestHandler_List_Pagination(t *testing.T) {
	t.Parallel()

	t.Run("has more results triggers cursor", func(t *testing.T) {
		t.Parallel()

		mux, reader, uc := setupQueryMux(t)

		now := time.Now()
		items := make([]domain.Item, 21)
		for i := range items {
			items[i] = domain.Item{
				ID:        uuid.New(),
				ProductID: uuid.New(),
				CreatedAt: now.Add(-time.Duration(i) * time.Minute),
			}
		}
		reader.EXPECT().ListItemsForUser(mock.Anything, uc.UserID, mock.Anything).Return(items, nil)

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

func setupQueryMux(t *testing.T) (*http.ServeMux, *MockItemReader, middleware.UserContext) {
	reader := NewMockItemReader(t)

	mux := http.NewServeMux()
	authed := middleware.NewRouteGroup(mux, "/api/v1")

	authed.HandleFunc("GET /wishlist", New(reader).List)

	uc := middleware.UserContext{
		UserID: uuid.New(),
		Email:  "test@example.com",
		Role:   "user",
	}

	return mux, reader, uc
}

func withAuth(r *http.Request, uc middleware.UserContext) *http.Request {
	ctx := middleware.SetUserContext(r.Context(), uc)
	return r.WithContext(ctx)
}
