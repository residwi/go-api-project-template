package http

import (
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

	"github.com/residwi/go-api-project-template/internal/modules/dashboard/domain"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

func TestHandler_TopProducts(t *testing.T) {
	t.Parallel()

	t.Run("success with from to and limit params", func(t *testing.T) {
		t.Parallel()

		mux, reader := setupTopProductsMux(t)

		from, _ := time.Parse("2006-01-02", "2025-01-01")
		to, _ := time.Parse("2006-01-02", "2025-01-31")
		toEnd := to.Add(24*time.Hour - time.Nanosecond)

		products := []domain.TopProduct{
			{ProductID: uuid.New(), Name: "Widget", TotalSold: 100, Revenue: 50000},
			{ProductID: uuid.New(), Name: "Gadget", TotalSold: 80, Revenue: 40000},
		}

		reader.EXPECT().ListTopProducts(mock.Anything, 5, from, toEnd).Return(products, nil)

		r := httptest.NewRequest(
			http.MethodGet,
			"/api/admin/dashboard/top-products?from=2025-01-01&to=2025-01-31&limit=5",
			nil,
		)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		success, ok := resp["success"].(bool)
		require.True(t, ok)
		assert.True(t, success)
		data, ok := resp["data"].([]any)
		require.True(t, ok)
		assert.Len(t, data, 2)

		p0, ok := data[0].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "Widget", p0["name"])
		assert.InDelta(t, float64(100), p0["total_sold"].(float64), 0.001)
		assert.InDelta(t, float64(50000), p0["revenue"].(float64), 0.001)
		p1, ok := data[1].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "Gadget", p1["name"])
		assert.InDelta(t, float64(80), p1["total_sold"].(float64), 0.001)
		assert.InDelta(t, float64(40000), p1["revenue"].(float64), 0.001)
	})

	t.Run("success with default limit when not provided", func(t *testing.T) {
		t.Parallel()

		mux, reader := setupTopProductsMux(t)

		products := []domain.TopProduct{
			{ProductID: uuid.New(), Name: "Widget", TotalSold: 100, Revenue: 50000},
		}

		reader.EXPECT().ListTopProducts(mock.Anything, 10, mock.Anything, mock.Anything).Return(products, nil)

		r := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/top-products?from=2025-01-01&to=2025-01-31", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("invalid limit uses default", func(t *testing.T) {
		t.Parallel()

		mux, reader := setupTopProductsMux(t)

		reader.EXPECT().ListTopProducts(mock.Anything, 10, mock.Anything, mock.Anything).
			Return([]domain.TopProduct{}, nil)

		r := httptest.NewRequest(
			http.MethodGet,
			"/api/admin/dashboard/top-products?from=2025-01-01&to=2025-01-31&limit=abc",
			nil,
		)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("limit exceeding max uses default", func(t *testing.T) {
		t.Parallel()

		mux, reader := setupTopProductsMux(t)

		reader.EXPECT().ListTopProducts(mock.Anything, 10, mock.Anything, mock.Anything).
			Return([]domain.TopProduct{}, nil)

		r := httptest.NewRequest(
			http.MethodGet,
			"/api/admin/dashboard/top-products?from=2025-01-01&to=2025-01-31&limit=200",
			nil,
		)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("limit zero uses default", func(t *testing.T) {
		t.Parallel()

		mux, reader := setupTopProductsMux(t)

		reader.EXPECT().ListTopProducts(mock.Anything, 10, mock.Anything, mock.Anything).
			Return([]domain.TopProduct{}, nil)

		r := httptest.NewRequest(
			http.MethodGet,
			"/api/admin/dashboard/top-products?from=2025-01-01&to=2025-01-31&limit=0",
			nil,
		)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("negative limit uses default", func(t *testing.T) {
		t.Parallel()

		mux, reader := setupTopProductsMux(t)

		reader.EXPECT().ListTopProducts(mock.Anything, 10, mock.Anything, mock.Anything).
			Return([]domain.TopProduct{}, nil)

		r := httptest.NewRequest(
			http.MethodGet,
			"/api/admin/dashboard/top-products?from=2025-01-01&to=2025-01-31&limit=-5",
			nil,
		)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("missing date range", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupTopProductsMux(t)

		r := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/top-products", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid from date", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupTopProductsMux(t)

		r := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/top-products?from=invalid&to=2025-01-31", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid to date", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupTopProductsMux(t)

		r := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/top-products?from=2025-01-01&to=bad", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		errBody, ok := resp["error"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, errBody["message"], "invalid to date format")
	})

	t.Run("service error", func(t *testing.T) {
		t.Parallel()

		mux, reader := setupTopProductsMux(t)

		reader.EXPECT().ListTopProducts(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil, errors.New("db error"))

		r := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/top-products?from=2025-01-01&to=2025-01-31", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func setupTopProductsMux(t *testing.T) (*http.ServeMux, *MockTopProductsReader) {
	reader := NewMockTopProductsReader(t)
	mux := http.NewServeMux()
	admin := middleware.NewRouteGroup(mux, "/api/admin")
	admin.HandleFunc("GET /dashboard/top-products", New(reader).TopProducts)
	return mux, reader
}
