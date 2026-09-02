package http

import (
	"bytes"
	"encoding/json"
	"errors"
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

	"github.com/residwi/go-api-project-template/internal/features/review/domain"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/identity"
	"github.com/residwi/go-api-project-template/internal/platform/web"
	"github.com/residwi/go-api-project-template/internal/platform/web/middleware"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

func TestHandler_Create(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, service := setupMux(t)

		userID := uuid.New()
		productID := uuid.New()
		orderID := uuid.New()

		service.EXPECT().Create(mock.Anything, userID, productID, orderID, 5, "Great", "Love it").
			Return(&domain.Review{
				ID:        uuid.New(),
				UserID:    userID,
				ProductID: productID,
				OrderID:   orderID,
				Rating:    5,
				Title:     "Great",
				Body:      "Love it",
				Status:    "published",
			}, nil)

		body, _ := json.Marshal(map[string]any{
			"order_id": orderID,
			"rating":   5,
			"title":    "Great",
			"body":     "Love it",
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(
			http.MethodPost,
			"/api/v1/products/"+productID.String()+"/reviews",
			bytes.NewReader(body),
		)
		r.Header.Set("Content-Type", "application/json")
		ctx := middleware.SetIdentity(r.Context(), identity.Identity{
			UserID: userID,
			Role:   "user",
		})
		r = r.WithContext(ctx)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusCreated, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)

		dataJSON, err := json.Marshal(resp.Data)
		require.NoError(t, err)
		var got struct {
			Title  string  `json:"title"`
			Rating float64 `json:"rating"`
		}
		require.NoError(t, json.Unmarshal(dataJSON, &got))
		assert.Equal(t, struct {
			Title  string  `json:"title"`
			Rating float64 `json:"rating"`
		}{Title: "Great", Rating: 5}, got)
	})

	t.Run("invalid product_id", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupMux(t)

		body, _ := json.Marshal(map[string]any{
			"order_id": uuid.New(),
			"rating":   5,
			"title":    "Great",
			"body":     "Love it",
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/products/bad/reviews", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		ctx := middleware.SetIdentity(r.Context(), identity.Identity{
			UserID: uuid.New(),
			Role:   "user",
		})
		r = r.WithContext(ctx)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, "invalid id", resp.Error.Message)
	})

	t.Run("missing auth", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupMux(t)

		productID := uuid.New()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/products/"+productID.String()+"/reviews", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupMux(t)

		productID := uuid.New()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(
			http.MethodPost,
			"/api/v1/products/"+productID.String()+"/reviews",
			bytes.NewReader([]byte("{bad")),
		)
		r.Header.Set("Content-Type", "application/json")
		ctx := middleware.SetIdentity(r.Context(), identity.Identity{
			UserID: uuid.New(),
			Role:   "user",
		})
		r = r.WithContext(ctx)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("validation error missing required fields", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupMux(t)

		productID := uuid.New()
		body, _ := json.Marshal(map[string]string{})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(
			http.MethodPost,
			"/api/v1/products/"+productID.String()+"/reviews",
			bytes.NewReader(body),
		)
		r.Header.Set("Content-Type", "application/json")
		ctx := middleware.SetIdentity(r.Context(), identity.Identity{
			UserID: uuid.New(),
			Role:   "user",
		})
		r = r.WithContext(ctx)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "validation failed", resp.Error.Message)
	})

	t.Run("service error", func(t *testing.T) {
		t.Parallel()

		mux, service := setupMux(t)

		userID := uuid.New()
		productID := uuid.New()
		orderID := uuid.New()

		service.EXPECT().Create(mock.Anything, userID, productID, orderID, 5, "Great", "Love it").
			Return(nil, errs.ErrBadRequest)

		body, _ := json.Marshal(map[string]any{
			"order_id": orderID,
			"rating":   5,
			"title":    "Great",
			"body":     "Love it",
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(
			http.MethodPost,
			"/api/v1/products/"+productID.String()+"/reviews",
			bytes.NewReader(body),
		)
		r.Header.Set("Content-Type", "application/json")
		ctx := middleware.SetIdentity(r.Context(), identity.Identity{
			UserID: userID,
			Role:   "user",
		})
		r = r.WithContext(ctx)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestHandler_ListByProduct(t *testing.T) {
	t.Parallel()

	t.Run("success with pagination", func(t *testing.T) {
		t.Parallel()

		mux, service := setupMux(t)

		productID := uuid.New()
		now := time.Now()

		service.EXPECT().ListByProduct(mock.Anything, productID, mock.Anything).Return([]domain.Review{
			{
				ID:        uuid.New(),
				UserID:    uuid.New(),
				ProductID: productID,
				OrderID:   uuid.New(),
				Rating:    5,
				Title:     "Great product",
				Body:      "Love it",
				Status:    "published",
				CreatedAt: now,
				UpdatedAt: now,
			},
		}, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/products/"+productID.String()+"/reviews?limit=10", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)

		data, ok := resp.Data.(map[string]any)
		require.True(t, ok)
		items, ok := data["items"].([]any)
		require.True(t, ok)
		assert.Len(t, items, 1)

		item := items[0].(map[string]any)
		assert.InDelta(t, float64(5), item["rating"], 0.0001)
		assert.Equal(t, "Great product", item["title"])
		assert.Equal(t, "Love it", item["body"])
		assert.NotContains(
			t,
			item,
			"status",
			"status is dropped: every review this endpoint can return is already published",
		)
		assert.NotContains(t, item, "user_id", "user_id is dropped to avoid correlating purchases to accounts")

		pagination, ok := data["pagination"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, false, pagination["has_more"])
	})

	t.Run("invalid product_id", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/products/bad/reviews", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "invalid id", resp.Error.Message)
	})

	t.Run("service error", func(t *testing.T) {
		t.Parallel()

		mux, service := setupMux(t)

		productID := uuid.New()
		service.EXPECT().ListByProduct(mock.Anything, productID, mock.Anything).Return(nil, errors.New("db error"))

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/products/"+productID.String()+"/reviews", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("has more results triggers cursor", func(t *testing.T) {
		t.Parallel()

		mux, service := setupMux(t)

		productID := uuid.New()
		now := time.Now()
		reviews := make([]domain.Review, 21)
		for i := range reviews {
			reviews[i] = domain.Review{
				ID:        uuid.New(),
				UserID:    uuid.New(),
				ProductID: productID,
				OrderID:   uuid.New(),
				Rating:    5,
				Title:     "Great",
				Status:    "published",
				CreatedAt: now.Add(-time.Duration(i) * time.Minute),
				UpdatedAt: now,
			}
		}
		service.EXPECT().ListByProduct(mock.Anything, productID, mock.Anything).Return(reviews, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/products/"+productID.String()+"/reviews", nil)

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

func TestToReviewResponse_OmitsReviewerAndInternalFields(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	orderID := uuid.New()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	got := toReviewResponse(domain.Review{
		ID:        uuid.New(),
		UserID:    userID,
		ProductID: uuid.New(),
		OrderID:   orderID,
		Rating:    5,
		Title:     "Great",
		Body:      "Love it",
		Status:    "published",
		CreatedAt: now,
		UpdatedAt: now,
	})

	raw, err := json.Marshal(got)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	assert.ElementsMatch(
		t,
		[]string{"id", "product_id", "rating", "title", "body", "created_at"},
		slices.Collect(maps.Keys(fields)),
		"the response must expose exactly these fields",
	)

	assert.NotContains(t, string(raw), userID.String(),
		"a review response naming the reviewer's id lets a scraper correlate purchases to accounts")
	assert.NotContains(t, string(raw), orderID.String(),
		"order_id exists only to verify provenance at creation time; a client has no use for it back")
}

func setupMux(t *testing.T) (*http.ServeMux, *MockReviewManager) {
	t.Helper()

	service := NewMockReviewManager(t)

	mux := http.NewServeMux()
	group := web.NewRouter(mux).Group("/api/v1")
	h := NewHandler(service)
	group.HandleFunc("GET /products/{id}/reviews", h.List)
	group.HandleFunc("POST /products/{id}/reviews", h.Create)

	return mux, service
}
