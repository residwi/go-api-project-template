package summary

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/modules/dashboard/domain"
)

func TestReader_GetSummary(t *testing.T) {
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

		sales, breakdown, err := New(repo).GetSummary(context.Background(), from, to)

		require.NoError(t, err)
		assert.Equal(t, expectedSales, sales)
		assert.Equal(t, expectedBreakdown, breakdown)
	})

	t.Run("sales summary error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)

		dbErr := errors.New("sales summary failed")
		repo.EXPECT().GetSalesSummary(mock.Anything, from, to).Return(domain.SalesSummary{}, dbErr)
		// The sibling query may or may not run before cancellation kicks in; make
		// it optional so the test asserts error handling, not goroutine timing.
		repo.EXPECT().ListOrderStatusBreakdown(mock.Anything, from, to).
			Return(nil, nil).Maybe()

		sales, breakdown, err := New(repo).GetSummary(context.Background(), from, to)

		require.ErrorIs(t, err, dbErr)
		assert.Equal(t, domain.SalesSummary{}, sales)
		assert.Nil(t, breakdown)
	})

	t.Run("breakdown error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)

		dbErr := errors.New("breakdown failed")
		repo.EXPECT().ListOrderStatusBreakdown(mock.Anything, from, to).Return(nil, dbErr)
		repo.EXPECT().GetSalesSummary(mock.Anything, from, to).
			Return(domain.SalesSummary{}, nil).Maybe()

		sales, breakdown, err := New(repo).GetSummary(context.Background(), from, to)

		require.ErrorIs(t, err, dbErr)
		assert.Equal(t, domain.SalesSummary{}, sales)
		assert.Nil(t, breakdown)
	})
}
