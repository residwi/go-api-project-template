package reserve

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/modules/inventory/domain"
)

func TestCommand_ReserveBatch(t *testing.T) {
	t.Parallel()

	productA := uuid.New()
	productB := uuid.New()

	repo := NewMockRepository(t)
	repo.EXPECT().ReserveBatch(mock.Anything, map[uuid.UUID]int{productA: 2, productB: 1}).Return(nil)

	cmd := New(repo)

	err := cmd.ReserveBatch(context.Background(), map[uuid.UUID]int{productA: 2, productB: 1})

	require.NoError(t, err)
}

func TestCommand_Reserve(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		productID := uuid.New()
		expected := &domain.Stock{ProductID: productID, Quantity: 100, Reserved: 10, Available: 90}
		repo.EXPECT().Reserve(mock.Anything, productID, 10).Return(expected, nil)

		result, err := cmd.Reserve(context.Background(), productID, 10)

		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		productID := uuid.New()
		repo.EXPECT().Reserve(mock.Anything, productID, 10).Return(nil, errors.New("insufficient stock"))

		result, err := cmd.Reserve(context.Background(), productID, 10)

		assert.Nil(t, result)
		assert.Error(t, err)
	})
}
