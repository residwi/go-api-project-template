package dashboard

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestService_GetTopProducts(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo)

		from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC)

		expected := []TopProduct{
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
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo)

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
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo)

		from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(2026, 1, 3, 23, 59, 59, 0, time.UTC)

		expected := []RevenueData{
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
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo)

		from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(2026, 1, 3, 23, 59, 59, 0, time.UTC)

		dbErr := errors.New("revenue query failed")
		repo.EXPECT().GetRevenueByDay(mock.Anything, from, to).Return(nil, dbErr)

		result, err := svc.GetRevenueByDay(context.Background(), from, to)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, dbErr)
	})
}
