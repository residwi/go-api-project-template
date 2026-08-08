package query

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/modules/review/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

func TestReader_ListByProduct(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		reader := New(repo)

		ctx := context.Background()
		productID := uuid.New()
		cursor := paging.CursorPage{Limit: 20}
		expected := []domain.Review{
			{ID: uuid.New(), ProductID: productID, Rating: 5, Title: "Great"},
			{ID: uuid.New(), ProductID: productID, Rating: 4, Title: "Good"},
		}

		repo.EXPECT().ListByProduct(mock.Anything, productID, cursor).Return(expected, nil)

		result, err := reader.ListByProduct(ctx, productID, cursor)
		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		reader := New(repo)

		ctx := context.Background()
		productID := uuid.New()
		cursor := paging.CursorPage{Limit: 20}
		dbErr := errors.New("query failed")

		repo.EXPECT().ListByProduct(mock.Anything, productID, cursor).Return(nil, dbErr)

		result, err := reader.ListByProduct(ctx, productID, cursor)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, dbErr)
	})
}

func TestReader_GetStats(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		reader := New(repo)

		ctx := context.Background()
		productID := uuid.New()
		expected := domain.Stats{AverageRating: 4.5, TotalReviews: 10}

		repo.EXPECT().GetStats(mock.Anything, productID).Return(expected, nil)

		result, err := reader.GetStats(ctx, productID)
		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		reader := New(repo)

		ctx := context.Background()
		productID := uuid.New()
		dbErr := errors.New("stats query failed")

		repo.EXPECT().GetStats(mock.Anything, productID).Return(domain.Stats{}, dbErr)

		result, err := reader.GetStats(ctx, productID)
		assert.Equal(t, domain.Stats{}, result)
		assert.ErrorIs(t, err, dbErr)
	})
}
