package dashboard_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/features/dashboard"
	mocks "github.com/residwi/go-api-project-template/mocks/dashboard"
)

func TestService_GetSalesSummary(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := dashboard.NewService(repo)

		from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC)

		expected := dashboard.SalesSummary{
			TotalOrders:       150,
			TotalRevenue:      5000000,
			AverageOrderValue: 33333.33,
		}
		repo.EXPECT().GetSalesSummary(mock.Anything, from, to).Return(expected, nil)

		result, err := svc.GetSalesSummary(context.Background(), from, to)

		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("error propagates", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := dashboard.NewService(repo)

		from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC)

		dbErr := errors.New("sales summary failed")
		repo.EXPECT().GetSalesSummary(mock.Anything, from, to).Return(dashboard.SalesSummary{}, dbErr)

		result, err := svc.GetSalesSummary(context.Background(), from, to)

		assert.Equal(t, dashboard.SalesSummary{}, result)
		assert.ErrorIs(t, err, dbErr)
	})
}

func TestService_GetSummary(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC)

	t.Run("returns both results on success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := dashboard.NewService(repo)

		expectedSales := dashboard.SalesSummary{
			TotalOrders:       150,
			TotalRevenue:      5000000,
			AverageOrderValue: 33333.33,
		}
		expectedBreakdown := []dashboard.StatusBreakdown{
			{Status: "paid", Count: 50},
			{Status: "shipped", Count: 30},
			{Status: "delivered", Count: 20},
		}
		repo.EXPECT().GetSalesSummary(mock.Anything, from, to).Return(expectedSales, nil)
		repo.EXPECT().GetOrderStatusBreakdown(mock.Anything, from, to).Return(expectedBreakdown, nil)

		sales, breakdown, err := svc.GetSummary(context.Background(), from, to)

		require.NoError(t, err)
		assert.Equal(t, expectedSales, sales)
		assert.Equal(t, expectedBreakdown, breakdown)
	})

	t.Run("sales summary error propagates", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := dashboard.NewService(repo)

		dbErr := errors.New("sales summary failed")
		repo.EXPECT().GetSalesSummary(mock.Anything, from, to).Return(dashboard.SalesSummary{}, dbErr)
		// The sibling query may or may not run before cancellation kicks in; make
		// it optional so the test asserts error handling, not goroutine timing.
		repo.EXPECT().GetOrderStatusBreakdown(mock.Anything, from, to).
			Return(nil, nil).Maybe()

		sales, breakdown, err := svc.GetSummary(context.Background(), from, to)

		assert.ErrorIs(t, err, dbErr)
		assert.Equal(t, dashboard.SalesSummary{}, sales)
		assert.Nil(t, breakdown)
	})

	t.Run("breakdown error propagates", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := dashboard.NewService(repo)

		dbErr := errors.New("breakdown failed")
		repo.EXPECT().GetOrderStatusBreakdown(mock.Anything, from, to).Return(nil, dbErr)
		repo.EXPECT().GetSalesSummary(mock.Anything, from, to).
			Return(dashboard.SalesSummary{}, nil).Maybe()

		sales, breakdown, err := svc.GetSummary(context.Background(), from, to)

		assert.ErrorIs(t, err, dbErr)
		assert.Equal(t, dashboard.SalesSummary{}, sales)
		assert.Nil(t, breakdown)
	})
}

func TestService_GetTopProducts(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := dashboard.NewService(repo)

		from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC)

		expected := []dashboard.TopProduct{
			{ProductID: uuid.New(), Name: "Widget A", TotalSold: 500, Revenue: 2500000},
			{ProductID: uuid.New(), Name: "Widget B", TotalSold: 300, Revenue: 1500000},
		}
		repo.EXPECT().GetTopProducts(mock.Anything, 10, from, to).Return(expected, nil)

		result, err := svc.GetTopProducts(context.Background(), 10, from, to)

		require.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Equal(t, expected, result)
	})

	t.Run("error propagates", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := dashboard.NewService(repo)

		from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC)

		dbErr := errors.New("top products failed")
		repo.EXPECT().GetTopProducts(mock.Anything, 10, from, to).Return(nil, dbErr)

		result, err := svc.GetTopProducts(context.Background(), 10, from, to)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, dbErr)
	})
}

func TestService_GetRevenueByDay(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := dashboard.NewService(repo)

		from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(2026, 1, 3, 23, 59, 59, 0, time.UTC)

		expected := []dashboard.RevenueData{
			{Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Revenue: 100000, OrderCount: 10},
			{Date: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), Revenue: 200000, OrderCount: 20},
			{Date: time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC), Revenue: 150000, OrderCount: 15},
		}
		repo.EXPECT().GetRevenueByDay(mock.Anything, from, to).Return(expected, nil)

		result, err := svc.GetRevenueByDay(context.Background(), from, to)

		require.NoError(t, err)
		assert.Len(t, result, 3)
		assert.Equal(t, expected, result)
	})

	t.Run("error propagates", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := dashboard.NewService(repo)

		from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(2026, 1, 3, 23, 59, 59, 0, time.UTC)

		dbErr := errors.New("revenue query failed")
		repo.EXPECT().GetRevenueByDay(mock.Anything, from, to).Return(nil, dbErr)

		result, err := svc.GetRevenueByDay(context.Background(), from, to)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, dbErr)
	})
}

func TestService_GetOrderStatusBreakdown(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := dashboard.NewService(repo)

		expected := []dashboard.StatusBreakdown{
			{Status: "paid", Count: 50},
			{Status: "shipped", Count: 30},
			{Status: "delivered", Count: 20},
		}
		repo.EXPECT().GetOrderStatusBreakdown(mock.Anything, mock.Anything, mock.Anything).Return(expected, nil)

		result, err := svc.GetOrderStatusBreakdown(context.Background(), time.Time{}, time.Time{})

		require.NoError(t, err)
		assert.Len(t, result, 3)
		assert.Equal(t, expected, result)
	})

	t.Run("error propagates", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := dashboard.NewService(repo)

		dbErr := errors.New("query failed")
		repo.EXPECT().GetOrderStatusBreakdown(mock.Anything, mock.Anything, mock.Anything).Return(nil, dbErr)

		result, err := svc.GetOrderStatusBreakdown(context.Background(), time.Time{}, time.Time{})

		assert.Nil(t, result)
		assert.ErrorIs(t, err, dbErr)
	})
}
