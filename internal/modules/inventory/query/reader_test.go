package query

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/modules/inventory/contract"
	"github.com/residwi/go-api-project-template/internal/modules/inventory/domain"
)

func TestReader_GetStock(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		reader := New(repo)

		productID := uuid.New()
		expected := &domain.Stock{ProductID: productID, Quantity: 100, Reserved: 10, Available: 90}
		repo.EXPECT().GetStock(mock.Anything, productID).Return(expected, nil)

		result, err := reader.GetStock(context.Background(), productID)

		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		reader := New(repo)

		productID := uuid.New()
		repo.EXPECT().GetStock(mock.Anything, productID).Return(nil, errors.New("not found"))

		result, err := reader.GetStock(context.Background(), productID)

		assert.Nil(t, result)
		assert.Error(t, err)
	})
}

func TestReaderGetAvailabilityDropsReservedCount(t *testing.T) {
	t.Parallel()

	productID := uuid.New()

	repo := NewMockRepository(t)
	repo.EXPECT().GetLevels(mock.Anything, []uuid.UUID{productID}).
		Return(map[uuid.UUID]domain.Stock{productID: {Quantity: 10, Reserved: 3, Available: 7}}, nil)

	reader := New(repo)

	got, err := reader.GetAvailability(context.Background(), []uuid.UUID{productID})

	require.NoError(t, err)
	assert.Equal(t, map[uuid.UUID]contract.Availability{productID: {OnHand: 10, Available: 7}}, got)
}

func TestReader_GetAvailability_Error(t *testing.T) {
	t.Parallel()

	repo := NewMockRepository(t)
	reader := New(repo)

	ids := []uuid.UUID{uuid.New()}
	repo.EXPECT().GetLevels(mock.Anything, ids).Return(nil, errors.New("db error"))

	result, err := reader.GetAvailability(context.Background(), ids)

	assert.Nil(t, result)
	assert.Error(t, err)
}
