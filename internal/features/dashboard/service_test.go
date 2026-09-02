package dashboard

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/features/dashboard/domain"
)

func TestService_ListRevenueByDay(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)

		from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(2026, 1, 3, 23, 59, 59, 0, time.UTC)

		expected := []domain.RevenueData{
			{Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Revenue: 100000, OrderCount: 10},
			{Date: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), Revenue: 200000, OrderCount: 20},
			{Date: time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC), Revenue: 150000, OrderCount: 15},
		}
		repo.EXPECT().ListRevenueByDay(mock.Anything, from, to).Return(expected, nil)

		result, err := New(repo).ListRevenueByDay(t.Context(), from, to)

		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)

		from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(2026, 1, 3, 23, 59, 59, 0, time.UTC)

		repo.EXPECT().ListRevenueByDay(mock.Anything, from, to).Return(nil, assert.AnError)

		result, err := New(repo).ListRevenueByDay(t.Context(), from, to)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, assert.AnError)
	})
}

func TestService_GetSummary(t *testing.T) {
	t.Parallel()

	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC)

	t.Run("returns both results on success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)

		expectedSales := domain.SalesSummary{
			TotalOrders:       150,
			TotalRevenue:      5000000,
			AverageOrderValue: 33333.33,
		}
		expectedBreakdown := []domain.StatusBreakdown{
			{Status: "paid", Count: 50},
			{Status: "shipped", Count: 30},
			{Status: "delivered", Count: 20},
		}
		repo.EXPECT().GetSalesSummary(mock.Anything, from, to).Return(expectedSales, nil)
		repo.EXPECT().ListOrderStatusBreakdown(mock.Anything, from, to).Return(expectedBreakdown, nil)

		sales, breakdown, err := New(repo).GetSummary(t.Context(), from, to)

		require.NoError(t, err)
		assert.Equal(t, expectedSales, sales)
		assert.Equal(t, expectedBreakdown, breakdown)
	})

	t.Run("sales summary error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)

		dbErr := assert.AnError
		repo.EXPECT().GetSalesSummary(mock.Anything, from, to).Return(domain.SalesSummary{}, dbErr)
		// The sibling query may or may not run before cancellation kicks in; make
		// it optional so the test asserts error handling, not goroutine timing.
		repo.EXPECT().ListOrderStatusBreakdown(mock.Anything, from, to).
			Return(nil, nil).Maybe()

		sales, breakdown, err := New(repo).GetSummary(t.Context(), from, to)

		require.ErrorIs(t, err, dbErr)
		assert.Equal(t, domain.SalesSummary{}, sales)
		assert.Nil(t, breakdown)
	})

	t.Run("breakdown error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)

		dbErr := assert.AnError
		repo.EXPECT().ListOrderStatusBreakdown(mock.Anything, from, to).Return(nil, dbErr)
		repo.EXPECT().GetSalesSummary(mock.Anything, from, to).
			Return(domain.SalesSummary{}, nil).Maybe()

		sales, breakdown, err := New(repo).GetSummary(t.Context(), from, to)

		require.ErrorIs(t, err, dbErr)
		assert.Equal(t, domain.SalesSummary{}, sales)
		assert.Nil(t, breakdown)
	})
}

func TestService_ListTopProducts(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)

		from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC)

		expected := []domain.TopProduct{
			{ProductID: uuid.New(), Name: "Widget A", TotalSold: 500, Revenue: 2500000},
			{ProductID: uuid.New(), Name: "Widget B", TotalSold: 300, Revenue: 1500000},
		}
		repo.EXPECT().ListTopProducts(mock.Anything, 10, from, to).Return(expected, nil)

		result, err := New(repo).ListTopProducts(t.Context(), 10, from, to)

		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)

		from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC)

		repo.EXPECT().ListTopProducts(mock.Anything, 10, from, to).Return(nil, assert.AnError)

		result, err := New(repo).ListTopProducts(t.Context(), 10, from, to)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, assert.AnError)
	})
}
