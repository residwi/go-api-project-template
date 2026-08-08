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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/modules/dashboard/domain"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

func TestHandler_Summary(t *testing.T) {
	t.Parallel()

	t.Run("success with from and to params", func(t *testing.T) {
		t.Parallel()

		mux, reader := setupSummaryMux(t)

		from, _ := time.Parse("2006-01-02", "2025-01-01")
		to, _ := time.Parse("2006-01-02", "2025-01-31")
		toEnd := to.Add(24*time.Hour - time.Nanosecond)

		summary := domain.SalesSummary{TotalOrders: 10, TotalRevenue: 50000, AverageOrderValue: 5000}
		breakdown := []domain.StatusBreakdown{{Status: "paid", Count: 7}, {Status: "shipped", Count: 3}}

		reader.EXPECT().GetSummary(mock.Anything, from, toEnd).Return(summary, breakdown, nil)

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

		mux, _ := setupSummaryMux(t)

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

		mux, _ := setupSummaryMux(t)

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

		mux, _ := setupSummaryMux(t)

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

		mux, _ := setupSummaryMux(t)

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

		mux, _ := setupSummaryMux(t)

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

		mux, reader := setupSummaryMux(t)

		reader.EXPECT().GetSummary(mock.Anything, mock.Anything, mock.Anything).
			Return(domain.SalesSummary{}, nil, errors.New("db connection failed"))

		r := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/summary?from=2025-01-01&to=2025-01-31", nil)
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

func setupSummaryMux(t *testing.T) (*http.ServeMux, *MockSummaryReader) {
	reader := NewMockSummaryReader(t)
	mux := http.NewServeMux()
	admin := middleware.NewRouteGroup(mux, "/api/admin")
	New(reader).RegisterHTTP(admin)
	return mux, reader
}
