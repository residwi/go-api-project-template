package restore

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

func TestCommand_Restore(t *testing.T) {
	t.Parallel()

	t.Run("releases stock that was only reserved", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		items := map[uuid.UUID]int{uuid.New(): 2}
		repo.EXPECT().ReleaseBatch(mock.Anything, items).Return(nil)

		err := cmd.Restore(context.Background(), items, contract.Reserved)
		require.NoError(t, err)
	})

	t.Run("restocks stock that was deducted", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		items := map[uuid.UUID]int{uuid.New(): 3}
		repo.EXPECT().RestockBatch(mock.Anything, items).Return(nil)

		err := cmd.Restore(context.Background(), items, contract.Deducted)
		require.NoError(t, err)
	})
}

func TestCommand_Release(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		productID := uuid.New()
		expected := &domain.Stock{ProductID: productID, Quantity: 100, Reserved: 5, Available: 95}
		repo.EXPECT().Release(mock.Anything, productID, 5).Return(expected, nil)

		result, err := cmd.Release(context.Background(), productID, 5)

		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		productID := uuid.New()
		repo.EXPECT().Release(mock.Anything, productID, 5).Return(nil, errors.New("cannot release more than reserved"))

		result, err := cmd.Release(context.Background(), productID, 5)

		assert.Nil(t, result)
		assert.Error(t, err)
	})
}
