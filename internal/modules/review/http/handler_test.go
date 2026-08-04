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

	"github.com/residwi/go-api-project-template/internal/modules/review"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
	revMocks "github.com/residwi/go-api-project-template/mocks/review"
)

func TestHandler_ListByProduct(t *testing.T) {
	t.Parallel()

	t.Run("success with pagination", func(t *testing.T) {
		t.Parallel()

		mux, repo, _ := setupReviewMux(t)

		productID := uuid.New()
		now := time.Now()

		repo.EXPECT().ListByProduct(mock.Anything, productID, mock.Anything).Return([]review.Review{
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
		assert.NotContains(t, item, "status", "status is dropped: every review this endpoint can return is already published")
		assert.NotContains(t, item, "user_id", "user_id is dropped to avoid correlating purchases to accounts")

		pagination, ok := data["pagination"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, false, pagination["has_more"])
	})

	t.Run("invalid product_id", func(t *testing.T) {
		t.Parallel()

		mux, _, _ := setupReviewMux(t)

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

		mux, repo, _ := setupReviewMux(t)

		productID := uuid.New()
		repo.EXPECT().ListByProduct(mock.Anything, productID, mock.Anything).Return(nil, errors.New("db error"))

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/products/"+productID.String()+"/reviews", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("has more results triggers cursor", func(t *testing.T) {
		t.Parallel()

		mux, repo, _ := setupReviewMux(t)

		productID := uuid.New()
		now := time.Now()
		reviews := make([]review.Review, 21)
		for i := range reviews {
			reviews[i] = review.Review{
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
		repo.EXPECT().ListByProduct(mock.Anything, productID, mock.Anything).Return(reviews, nil)

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

func TestHandler_Create(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, repo, purchase := setupReviewMux(t)

		userID := uuid.New()
		productID := uuid.New()
		orderID := uuid.New()

		purchase.EXPECT().HasDeliveredOrder(mock.Anything, review.DeliveredPurchase{
			UserID:    userID,
			OrderID:   orderID,
			ProductID: productID,
		}).Return(true, nil)
		repo.EXPECT().HasUserReviewed(mock.Anything, userID, productID).Return(false, nil)
		repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)

		body, _ := json.Marshal(map[string]any{
			"order_id": orderID,
			"rating":   5,
			"title":    "Great",
			"body":     "Love it",
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/products/"+productID.String()+"/reviews", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		ctx := middleware.SetUserContext(r.Context(), middleware.UserContext{
			UserID: userID,
			Email:  "test@example.com",
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

		mux, _, _ := setupReviewMux(t)

		body, _ := json.Marshal(map[string]any{
			"order_id": uuid.New(),
			"rating":   5,
			"title":    "Great",
			"body":     "Love it",
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/products/bad/reviews", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		ctx := middleware.SetUserContext(r.Context(), middleware.UserContext{
			UserID: uuid.New(),
			Email:  "test@example.com",
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

		mux, _, _ := setupReviewMux(t)

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

		mux, _, _ := setupReviewMux(t)

		productID := uuid.New()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/products/"+productID.String()+"/reviews", bytes.NewReader([]byte("{bad")))
		r.Header.Set("Content-Type", "application/json")
		ctx := middleware.SetUserContext(r.Context(), middleware.UserContext{
			UserID: uuid.New(),
			Email:  "test@example.com",
			Role:   "user",
		})
		r = r.WithContext(ctx)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("validation error missing required fields", func(t *testing.T) {
		t.Parallel()

		mux, _, _ := setupReviewMux(t)

		productID := uuid.New()
		body, _ := json.Marshal(map[string]string{})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/products/"+productID.String()+"/reviews", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		ctx := middleware.SetUserContext(r.Context(), middleware.UserContext{
			UserID: uuid.New(),
			Email:  "test@example.com",
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

		mux, _, purchase := setupReviewMux(t)

		userID := uuid.New()
		productID := uuid.New()
		orderID := uuid.New()

		purchase.EXPECT().HasDeliveredOrder(mock.Anything, review.DeliveredPurchase{
			UserID:    userID,
			OrderID:   orderID,
			ProductID: productID,
		}).Return(false, nil)

		body, _ := json.Marshal(map[string]any{
			"order_id": orderID,
			"rating":   5,
			"title":    "Great",
			"body":     "Love it",
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/products/"+productID.String()+"/reviews", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		ctx := middleware.SetUserContext(r.Context(), middleware.UserContext{
			UserID: userID,
			Email:  "test@example.com",
			Role:   "user",
		})
		r = r.WithContext(ctx)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// GET /products/{id}/reviews runs on the unauthenticated `api` route group
// (router.go), so this assertion is the only thing stopping an anonymous
// scraper from reading UserID and correlating purchases to accounts. Status
// and UpdatedAt are dropped too: postgres/repository.go's ListByProduct
// filters WHERE status = 'published', so exposing them would add no
// information.
func TestToReviewResponse_OmitsReviewerAndInternalFields(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	orderID := uuid.New()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	got := toReviewResponse(review.Review{
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
	assert.ElementsMatch(t, []string{"id", "product_id", "rating", "title", "body", "created_at"}, slices.Collect(maps.Keys(fields)),
		"the response must expose exactly these fields")

	assert.NotContains(t, string(raw), userID.String(),
		"a review response naming the reviewer's id lets a scraper correlate purchases to accounts")
	assert.NotContains(t, string(raw), orderID.String(),
		"order_id exists only to verify provenance at creation time; a client has no use for it back")
}

func setupReviewMux(t *testing.T) (*http.ServeMux, *revMocks.MockRepository, *revMocks.MockPurchaseVerifier) {
	repo := revMocks.NewMockRepository(t)
	purchase := revMocks.NewMockPurchaseVerifier(t)
	svc := review.NewService(repo, purchase)
	v := validator.New()

	mux := http.NewServeMux()
	api := middleware.NewRouteGroup(mux, "/api/v1")
	authed := middleware.NewRouteGroup(mux, "/api/v1")
	admin := middleware.NewRouteGroup(mux, "/api/v1/admin")

	RegisterRoutes(api, authed, admin, RouteDeps{
		Validator: v,
		Service:   svc,
	})

	return mux, repo, purchase
}
