package revenue

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

func TestReader_ListRevenueByDay(t *testing.T) {
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

		result, err := New(repo).ListRevenueByDay(context.Background(), from, to)

		require.NoError(t, err)
		assert.Len(t, result, 3)
		assert.Equal(t, expected, result)
	})

	t.Run("error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)

		from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(2026, 1, 3, 23, 59, 59, 0, time.UTC)

		dbErr := errors.New("revenue query failed")
		repo.EXPECT().ListRevenueByDay(mock.Anything, from, to).Return(nil, dbErr)

		result, err := New(repo).ListRevenueByDay(context.Background(), from, to)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, dbErr)
	})
}
