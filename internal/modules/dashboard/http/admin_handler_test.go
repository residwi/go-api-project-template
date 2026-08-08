package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/modules/dashboard"
	"github.com/residwi/go-api-project-template/internal/modules/dashboard/summary"
	"github.com/residwi/go-api-project-template/internal/modules/dashboard/topproducts"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

func TestAdminHandler_Revenue(t *testing.T) {
	t.Parallel()

	t.Run("success with from and to params", func(t *testing.T) {
		t.Parallel()

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

		mux, _ := setupDashboardMux(t)

		r := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/revenue", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid from date", func(t *testing.T) {
		t.Parallel()

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
		t.Parallel()

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
		t.Parallel()

		mux, repo := setupDashboardMux(t)

		repo.EXPECT().GetRevenueByDay(mock.Anything, mock.Anything, mock.Anything).
			Return(nil, errors.New("db error"))

		r := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/revenue?from=2025-01-01&to=2025-01-31", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

// setupDashboardMux wires the still-standing husk service for the routes this
// file tests, plus a real (non-nil) *dashboard.Module so RegisterRoutes has
// something to dereference for the already-extracted summary and
// top-products routes -- these tests never hit them, and nothing calls a mock
// method with no expectation set.
func setupDashboardMux(t *testing.T) (*http.ServeMux, *MockRepository) {
	repo := NewMockRepository(t)
	svc := dashboard.NewService(repo)
	mod := &dashboard.Module{Summary: summary.New(repo), TopProducts: topproducts.New(repo)}
	mux := http.NewServeMux()
	admin := middleware.NewRouteGroup(mux, "/api/admin")
	RegisterRoutes(admin, RouteDeps{Service: svc, Module: mod})
	return mux, repo
}
