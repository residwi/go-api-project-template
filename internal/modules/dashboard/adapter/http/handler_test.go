package http

import (
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

	"github.com/residwi/go-api-project-template/internal/modules/dashboard/domain"
	"github.com/residwi/go-api-project-template/internal/platform/web"
)

func TestHandler_Summary(t *testing.T) {
	t.Parallel()

	t.Run("success with from and to params", func(t *testing.T) {
		t.Parallel()

		mux, service := setupMux(t)

		from, _ := time.Parse("2006-01-02", "2025-01-01")
		to, _ := time.Parse("2006-01-02", "2025-01-31")
		toEnd := to.Add(24*time.Hour - time.Nanosecond)

		summary := domain.SalesSummary{TotalOrders: 10, TotalRevenue: 50000, AverageOrderValue: 5000}
		breakdown := []domain.StatusBreakdown{{Status: "paid", Count: 7}, {Status: "shipped", Count: 3}}

		service.EXPECT().GetSummary(mock.Anything, from, toEnd).Return(summary, breakdown, nil)

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
				TotalOrders       float64 `json:"total_orders"`
				TotalRevenue      float64 `json:"total_revenue"`
				AverageOrderValue float64 `json:"average_order_value"`
			} `json:"sales"`
			StatusBreakdown []any `json:"status_breakdown"`
		}
		require.NoError(t, json.Unmarshal(dataJSON, &got))
		assert.InDelta(t, float64(10), got.Sales.TotalOrders, 0.001)
		assert.InDelta(t, float64(50000), got.Sales.TotalRevenue, 0.001)
		assert.InDelta(t, float64(5000), got.Sales.AverageOrderValue, 0.001)
		assert.Len(t, got.StatusBreakdown, 2)

		sb0, ok := got.StatusBreakdown[0].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "paid", sb0["status"])
		assert.InDelta(t, float64(7), sb0["count"].(float64), 0.001)
		sb1, ok := got.StatusBreakdown[1].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "shipped", sb1["status"])
		assert.InDelta(t, float64(3), sb1["count"].(float64), 0.001)
	})

	t.Run("missing from and to params", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupMux(t)

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
		t.Parallel()

		mux, _ := setupMux(t)

		r := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/summary?to=2025-01-31", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Contains(t, resp["error"].(map[string]any)["message"], "from and to query parameters are required")
	})

	t.Run("missing only to param", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupMux(t)

		r := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/summary?from=2025-01-01", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Contains(t, resp["error"].(map[string]any)["message"], "from and to query parameters are required")
	})

	t.Run("invalid from date format", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupMux(t)

		r := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/summary?from=bad&to=2025-01-31", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Contains(t, resp["error"].(map[string]any)["message"], "invalid from date format")
	})

	t.Run("invalid to date format", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupMux(t)

		r := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/summary?from=2025-01-01&to=bad", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Contains(t, resp["error"].(map[string]any)["message"], "invalid to date format")
	})

	t.Run("service error", func(t *testing.T) {
		t.Parallel()

		mux, service := setupMux(t)

		service.EXPECT().GetSummary(mock.Anything, mock.Anything, mock.Anything).
			Return(domain.SalesSummary{}, nil, errors.New("db connection failed"))

		r := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/summary?from=2025-01-01&to=2025-01-31", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestHandler_Revenue(t *testing.T) {
	t.Parallel()

	t.Run("success with from and to params", func(t *testing.T) {
		t.Parallel()

		mux, service := setupMux(t)

		from, _ := time.Parse("2006-01-02", "2025-01-01")
		to, _ := time.Parse("2006-01-02", "2025-01-31")
		toEnd := to.Add(24*time.Hour - time.Nanosecond)

		data := []domain.RevenueData{
			{Date: from, Revenue: 10000, OrderCount: 5},
			{Date: from.AddDate(0, 0, 1), Revenue: 15000, OrderCount: 8},
		}

		service.EXPECT().ListRevenueByDay(mock.Anything, from, toEnd).Return(data, nil)

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

		d0, ok := respData[0].(map[string]any)
		require.True(t, ok)
		assert.InDelta(t, float64(10000), d0["revenue"].(float64), 0.001)
		assert.InDelta(t, float64(5), d0["order_count"].(float64), 0.001)
		d1, ok := respData[1].(map[string]any)
		require.True(t, ok)
		assert.InDelta(t, float64(15000), d1["revenue"].(float64), 0.001)
		assert.InDelta(t, float64(8), d1["order_count"].(float64), 0.001)
	})

	t.Run("missing date range", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupMux(t)

		r := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/revenue", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid from date", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupMux(t)

		r := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/revenue?from=bad&to=2025-01-31", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Contains(t, resp["error"].(map[string]any)["message"], "invalid from date format")
	})

	t.Run("invalid to date", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupMux(t)

		r := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/revenue?from=2025-01-01&to=notadate", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Contains(t, resp["error"].(map[string]any)["message"], "invalid to date format")
	})

	t.Run("service error", func(t *testing.T) {
		t.Parallel()

		mux, service := setupMux(t)

		service.EXPECT().ListRevenueByDay(mock.Anything, mock.Anything, mock.Anything).
			Return(nil, errors.New("db error"))

		r := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/revenue?from=2025-01-01&to=2025-01-31", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestHandler_TopProducts(t *testing.T) {
	t.Parallel()

	t.Run("success with from to and limit params", func(t *testing.T) {
		t.Parallel()

		mux, service := setupMux(t)

		from, _ := time.Parse("2006-01-02", "2025-01-01")
		to, _ := time.Parse("2006-01-02", "2025-01-31")
		toEnd := to.Add(24*time.Hour - time.Nanosecond)

		products := []domain.TopProduct{
			{ProductID: uuid.New(), Name: "Widget", TotalSold: 100, Revenue: 50000},
			{ProductID: uuid.New(), Name: "Gadget", TotalSold: 80, Revenue: 40000},
		}

		service.EXPECT().ListTopProducts(mock.Anything, 5, from, toEnd).Return(products, nil)

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

		mux, service := setupMux(t)

		products := []domain.TopProduct{
			{ProductID: uuid.New(), Name: "Widget", TotalSold: 100, Revenue: 50000},
		}

		service.EXPECT().ListTopProducts(mock.Anything, 10, mock.Anything, mock.Anything).Return(products, nil)

		r := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/top-products?from=2025-01-01&to=2025-01-31", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("invalid limit uses default", func(t *testing.T) {
		t.Parallel()

		mux, service := setupMux(t)

		service.EXPECT().ListTopProducts(mock.Anything, 10, mock.Anything, mock.Anything).
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

		mux, service := setupMux(t)

		service.EXPECT().ListTopProducts(mock.Anything, 10, mock.Anything, mock.Anything).
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

		mux, service := setupMux(t)

		service.EXPECT().ListTopProducts(mock.Anything, 10, mock.Anything, mock.Anything).
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

		mux, service := setupMux(t)

		service.EXPECT().ListTopProducts(mock.Anything, 10, mock.Anything, mock.Anything).
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

		mux, _ := setupMux(t)

		r := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/top-products", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid from date", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupMux(t)

		r := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/top-products?from=invalid&to=2025-01-31", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid to date", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupMux(t)

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

		mux, service := setupMux(t)

		service.EXPECT().ListTopProducts(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil, errors.New("db error"))

		r := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/top-products?from=2025-01-01&to=2025-01-31", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestToSummaryResponse_ExposesExactFieldSet(t *testing.T) {
	t.Parallel()

	got := toSummaryResponse(
		domain.SalesSummary{TotalOrders: 10, TotalRevenue: 50000, AverageOrderValue: 5000},
		[]domain.StatusBreakdown{{Status: "paid", Count: 7}},
	)

	raw, err := json.Marshal(got)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	assert.ElementsMatch(t, []string{"sales", "status_breakdown"}, slices.Collect(maps.Keys(fields)))

	var sales map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(fields["sales"], &sales))
	assert.ElementsMatch(
		t,
		[]string{"total_orders", "total_revenue", "average_order_value"},
		slices.Collect(maps.Keys(sales)),
	)

	var breakdown []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(fields["status_breakdown"], &breakdown))
	require.Len(t, breakdown, 1)
	assert.ElementsMatch(t, []string{"status", "count"}, slices.Collect(maps.Keys(breakdown[0])))
}

func setupMux(t *testing.T) (*http.ServeMux, *MockReporter) {
	service := NewMockReporter(t)
	mux := http.NewServeMux()
	admin := web.NewRouter(mux).Group("/api/admin")

	h := NewHandler(service)
	admin.HandleFunc("GET /dashboard/summary", h.Summary)
	admin.HandleFunc("GET /dashboard/revenue", h.Revenue)
	admin.HandleFunc("GET /dashboard/top-products", h.TopProducts)

	return mux, service
}
