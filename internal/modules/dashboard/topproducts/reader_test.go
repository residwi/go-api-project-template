package topproducts

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/modules/dashboard/domain"
)

func TestReader_GetTopProducts(t *testing.T) {
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
		repo.EXPECT().GetTopProducts(mock.Anything, 10, from, to).Return(expected, nil)

		result, err := New(repo).GetTopProducts(context.Background(), 10, from, to)

		require.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Equal(t, expected, result)
	})

	t.Run("error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)

		from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC)

		dbErr := errors.New("top products failed")
		repo.EXPECT().GetTopProducts(mock.Anything, 10, from, to).Return(nil, dbErr)

		result, err := New(repo).GetTopProducts(context.Background(), 10, from, to)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, dbErr)
	})
}
