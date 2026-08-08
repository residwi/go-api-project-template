package http

import (
	"bytes"
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

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/review/domain"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

func TestHandler_Create(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, cmd := setupCreateMux(t)

		userID := uuid.New()
		productID := uuid.New()
		orderID := uuid.New()

		cmd.EXPECT().Execute(mock.Anything, userID, productID, mock.Anything).Return(&domain.Review{
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

		mux, _ := setupCreateMux(t)

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

		mux, _ := setupCreateMux(t)

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

		mux, _ := setupCreateMux(t)

		productID := uuid.New()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(
			http.MethodPost,
			"/api/v1/products/"+productID.String()+"/reviews",
			bytes.NewReader([]byte("{bad")),
		)
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

		mux, _ := setupCreateMux(t)

		productID := uuid.New()
		body, _ := json.Marshal(map[string]string{})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(
			http.MethodPost,
			"/api/v1/products/"+productID.String()+"/reviews",
			bytes.NewReader(body),
		)
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

		mux, cmd := setupCreateMux(t)

		userID := uuid.New()
		productID := uuid.New()
		orderID := uuid.New()

		cmd.EXPECT().Execute(mock.Anything, userID, productID, mock.Anything).
			Return(nil, apperror.ErrBadRequest)

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

func TestToReviewResponse_OmitsReviewerAndInternalFields(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	orderID := uuid.New()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	got := toReviewResponse(&domain.Review{
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

func setupCreateMux(t *testing.T) (*http.ServeMux, *MockReviewCreator) {
	t.Helper()

	cmd := NewMockReviewCreator(t)
	v := validator.New()

	mux := http.NewServeMux()
	authed := middleware.NewRouteGroup(mux, "/api/v1")
	New(cmd, v).RegisterHTTP(authed)

	return mux, cmd
}
