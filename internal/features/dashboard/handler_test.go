package dashboard_test

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

	"github.com/residwi/go-api-project-template/internal/features/dashboard"
	"github.com/residwi/go-api-project-template/internal/middleware"
	mocks "github.com/residwi/go-api-project-template/mocks/dashboard"
)

func setupDashboardMux(t *testing.T) (*http.ServeMux, *mocks.MockRepository) {
	repo := mocks.NewMockRepository(t)
	svc := dashboard.NewService(repo)
	mux := http.NewServeMux()
	admin := middleware.NewRouteGroup(mux, "/api/admin")
	dashboard.RegisterRoutes(admin, dashboard.RouteDeps{Service: svc})
	return mux, repo
}

func TestAdminHandler_Summary(t *testing.T) {
	t.Run("success with from and to params", func(t *testing.T) {
		mux, repo := setupDashboardMux(t)

		from, _ := time.Parse("2006-01-02", "2025-01-01")
		to, _ := time.Parse("2006-01-02", "2025-01-31")
		toEnd := to.Add(24*time.Hour - time.Nanosecond)

		summary := dashboard.SalesSummary{TotalOrders: 10, TotalRevenue: 50000, AverageOrderValue: 5000}
		breakdown := []dashboard.StatusBreakdown{{Status: "paid", Count: 7}, {Status: "shipped", Count: 3}}

		repo.EXPECT().GetSalesSummary(mock.Anything, from, toEnd).Return(summary, nil)
		repo.EXPECT().GetOrderStatusBreakdown(mock.Anything).Return(breakdown, nil)

		r := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/summary?from=2025-01-01&to=2025-01-31", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		success, ok := resp["success"].(bool)
		require.True(t, ok)
		assert.True(t, success)

		dataJSON, err := json.Marshal(resp["data"])
		require.NoError(t, err)
		var got struct {
			Sales struct {
				TotalOrders  float64 `json:"total_orders"`
				TotalRevenue float64 `json:"total_revenue"`
			} `json:"sales"`
			StatusBreakdown []any `json:"status_breakdown"`
		}
		require.NoError(t, json.Unmarshal(dataJSON, &got))
		assert.InDelta(t, float64(10), got.Sales.TotalOrders, 0.001)
		assert.InDelta(t, float64(50000), got.Sales.TotalRevenue, 0.001)
		assert.Len(t, got.StatusBreakdown, 2)
	})

	t.Run("missing from and to params", func(t *testing.T) {
		mux, _ := setupDashboardMux(t)

		r := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/summary", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		success, ok := resp["success"].(bool)
		require.True(t, ok)
		assert.False(t, success)
		errBody, ok := resp["error"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, errBody["message"], "from and to query parameters are required")
	})

	t.Run("missing only from param", func(t *testing.T) {
		mux, _ := setupDashboardMux(t)

		r := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/summary?to=2025-01-31", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Contains(t, resp["error"].(map[string]any)["message"], "from and to query parameters are required")
	})

	t.Run("missing only to param", func(t *testing.T) {
		mux, _ := setupDashboardMux(t)

		r := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/summary?from=2025-01-01", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Contains(t, resp["error"].(map[string]any)["message"], "from and to query parameters are required")
	})

	t.Run("invalid from date format", func(t *testing.T) {
		mux, _ := setupDashboardMux(t)

		r := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/summary?from=bad&to=2025-01-31", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Contains(t, resp["error"].(map[string]any)["message"], "invalid from date format")
	})

	t.Run("invalid to date format", func(t *testing.T) {
		mux, _ := setupDashboardMux(t)

		r := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/summary?from=2025-01-01&to=bad", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Contains(t, resp["error"].(map[string]any)["message"], "invalid to date format")
	})

	t.Run("sales summary service error", func(t *testing.T) {
		mux, repo := setupDashboardMux(t)

		repo.EXPECT().GetSalesSummary(mock.Anything, mock.Anything, mock.Anything).
			Return(dashboard.SalesSummary{}, errors.New("db connection failed"))

		r := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/summary?from=2025-01-01&to=2025-01-31", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("order status breakdown service error", func(t *testing.T) {
		mux, repo := setupDashboardMux(t)

		summary := dashboard.SalesSummary{TotalOrders: 5, TotalRevenue: 25000, AverageOrderValue: 5000}
		repo.EXPECT().GetSalesSummary(mock.Anything, mock.Anything, mock.Anything).Return(summary, nil)
		repo.EXPECT().GetOrderStatusBreakdown(mock.Anything).
			Return(nil, errors.New("db error"))

		r := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/summary?from=2025-01-01&to=2025-01-31", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestAdminHandler_TopProducts(t *testing.T) {
	t.Run("success with from to and limit params", func(t *testing.T) {
		mux, repo := setupDashboardMux(t)

		from, _ := time.Parse("2006-01-02", "2025-01-01")
		to, _ := time.Parse("2006-01-02", "2025-01-31")
		toEnd := to.Add(24*time.Hour - time.Nanosecond)

		products := []dashboard.TopProduct{
			{ProductID: uuid.New(), Name: "Widget", TotalSold: 100, Revenue: 50000},
			{ProductID: uuid.New(), Name: "Gadget", TotalSold: 80, Revenue: 40000},
		}

		repo.EXPECT().GetTopProducts(mock.Anything, 5, from, toEnd).Return(products, nil)

		r := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/top-products?from=2025-01-01&to=2025-01-31&limit=5", nil)
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
	})

	t.Run("success with default limit when not provided", func(t *testing.T) {
		mux, repo := setupDashboardMux(t)

		products := []dashboard.TopProduct{
			{ProductID: uuid.New(), Name: "Widget", TotalSold: 100, Revenue: 50000},
		}

		repo.EXPECT().GetTopProducts(mock.Anything, 10, mock.Anything, mock.Anything).Return(products, nil)

		r := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/top-products?from=2025-01-01&to=2025-01-31", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("invalid limit uses default", func(t *testing.T) {
		mux, repo := setupDashboardMux(t)

		repo.EXPECT().GetTopProducts(mock.Anything, 10, mock.Anything, mock.Anything).
			Return([]dashboard.TopProduct{}, nil)

		r := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/top-products?from=2025-01-01&to=2025-01-31&limit=abc", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("limit exceeding max uses default", func(t *testing.T) {
		mux, repo := setupDashboardMux(t)

		repo.EXPECT().GetTopProducts(mock.Anything, 10, mock.Anything, mock.Anything).
			Return([]dashboard.TopProduct{}, nil)

		r := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/top-products?from=2025-01-01&to=2025-01-31&limit=200", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("limit zero uses default", func(t *testing.T) {
		mux, repo := setupDashboardMux(t)

		repo.EXPECT().GetTopProducts(mock.Anything, 10, mock.Anything, mock.Anything).
			Return([]dashboard.TopProduct{}, nil)

		r := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/top-products?from=2025-01-01&to=2025-01-31&limit=0", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("negative limit uses default", func(t *testing.T) {
		mux, repo := setupDashboardMux(t)

		repo.EXPECT().GetTopProducts(mock.Anything, 10, mock.Anything, mock.Anything).
			Return([]dashboard.TopProduct{}, nil)

		r := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/top-products?from=2025-01-01&to=2025-01-31&limit=-5", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("missing date range", func(t *testing.T) {
		mux, _ := setupDashboardMux(t)

		r := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/top-products", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid from date", func(t *testing.T) {
		mux, _ := setupDashboardMux(t)

		r := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/top-products?from=invalid&to=2025-01-31", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid to date", func(t *testing.T) {
		mux, _ := setupDashboardMux(t)

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
		mux, repo := setupDashboardMux(t)

		repo.EXPECT().GetTopProducts(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil, errors.New("db error"))

		r := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/top-products?from=2025-01-01&to=2025-01-31", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestAdminHandler_Revenue(t *testing.T) {
	t.Run("success with from and to params", func(t *testing.T) {
		mux, repo := setupDashboardMux(t)

		from, _ := time.Parse("2006-01-02", "2025-01-01")
		to, _ := time.Parse("2006-01-02", "2025-01-31")
		toEnd := to.Add(24*time.Hour - time.Nanosecond)

		data := []dashboard.RevenueData{
			{Date: from, Revenue: 10000, OrderCount: 5},
			{Date: from.AddDate(0, 0, 1), Revenue: 15000, OrderCount: 8},
		}

		repo.EXPECT().GetRevenueByDay(mock.Anything, from, toEnd).Return(data, nil)

		r := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/revenue?from=2025-01-01&to=2025-01-31", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		success, ok := resp["success"].(bool)
		require.True(t, ok)
		assert.True(t, success)
		respData, ok := resp["data"].([]any)
		require.True(t, ok)
		assert.Len(t, respData, 2)
	})

	t.Run("missing date range", func(t *testing.T) {
		mux, _ := setupDashboardMux(t)

		r := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/revenue", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid from date", func(t *testing.T) {
		mux, _ := setupDashboardMux(t)

		r := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/revenue?from=bad&to=2025-01-31", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Contains(t, resp["error"].(map[string]any)["message"], "invalid from date format")
	})

	t.Run("invalid to date", func(t *testing.T) {
		mux, _ := setupDashboardMux(t)

		r := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/revenue?from=2025-01-01&to=notadate", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Contains(t, resp["error"].(map[string]any)["message"], "invalid to date format")
	})

	t.Run("service error", func(t *testing.T) {
		mux, repo := setupDashboardMux(t)

		repo.EXPECT().GetRevenueByDay(mock.Anything, mock.Anything, mock.Anything).
			Return(nil, errors.New("db error"))

		r := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/revenue?from=2025-01-01&to=2025-01-31", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}
