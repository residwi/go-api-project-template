package review_test

import (
	"bytes"
	"encoding/json"
	"errors"
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
	"github.com/residwi/go-api-project-template/internal/features/review"
	"github.com/residwi/go-api-project-template/internal/middleware"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	revMocks "github.com/residwi/go-api-project-template/mocks/review"
)

func setupReviewMux(t *testing.T) (*http.ServeMux, *revMocks.MockRepository, *revMocks.MockPurchaseVerifier) {
	repo := revMocks.NewMockRepository(t)
	purchase := revMocks.NewMockPurchaseVerifier(t)
	svc := review.NewService(repo, purchase)
	v := validator.New()

	mux := http.NewServeMux()
	api := middleware.NewRouteGroup(mux, "/api/v1")
	authed := middleware.NewRouteGroup(mux, "/api/v1")
	admin := middleware.NewRouteGroup(mux, "/api/v1/admin")

	review.RegisterRoutes(api, authed, admin, review.RouteDeps{
		Validator: v,
		Service:   svc,
	})

	return mux, repo, purchase
}

func TestHandler_ListByProduct(t *testing.T) {
	t.Run("success with pagination", func(t *testing.T) {
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
	})

	t.Run("invalid product_id", func(t *testing.T) {
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
		mux, repo, _ := setupReviewMux(t)

		productID := uuid.New()
		repo.EXPECT().ListByProduct(mock.Anything, productID, mock.Anything).Return(nil, errors.New("db error"))

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/products/"+productID.String()+"/reviews", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("has more results triggers cursor", func(t *testing.T) {
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
	t.Run("success", func(t *testing.T) {
		mux, repo, purchase := setupReviewMux(t)

		userID := uuid.New()
		productID := uuid.New()
		orderID := uuid.New()

		purchase.EXPECT().HasDeliveredOrder(mock.Anything, userID, productID).Return(true, nil)
		repo.EXPECT().HasUserReviewed(mock.Anything, userID, productID).Return(false, nil)
		repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)

		body, _ := json.Marshal(review.CreateReviewRequest{
			OrderID: orderID,
			Rating:  5,
			Title:   "Great",
			Body:    "Love it",
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
		mux, _, _ := setupReviewMux(t)

		body, _ := json.Marshal(review.CreateReviewRequest{
			OrderID: uuid.New(),
			Rating:  5,
			Title:   "Great",
			Body:    "Love it",
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
		mux, _, purchase := setupReviewMux(t)

		userID := uuid.New()
		productID := uuid.New()
		orderID := uuid.New()

		purchase.EXPECT().HasDeliveredOrder(mock.Anything, userID, productID).Return(false, nil)

		body, _ := json.Marshal(review.CreateReviewRequest{
			OrderID: orderID,
			Rating:  5,
			Title:   "Great",
			Body:    "Love it",
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

func TestHandler_Delete(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mux, repo, _ := setupReviewMux(t)

		reviewID := uuid.New()
		repo.EXPECT().Delete(mock.Anything, reviewID).Return(nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/reviews/"+reviewID.String(), nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("invalid UUID", func(t *testing.T) {
		mux, _, _ := setupReviewMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/reviews/bad", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "invalid id", resp.Error.Message)
	})

	t.Run("service error", func(t *testing.T) {
		mux, repo, _ := setupReviewMux(t)

		reviewID := uuid.New()
		repo.EXPECT().Delete(mock.Anything, reviewID).Return(core.ErrNotFound)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/reviews/"+reviewID.String(), nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}
