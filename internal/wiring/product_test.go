package wiring

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/inventory"
	invMocks "github.com/residwi/go-api-project-template/mocks/inventory"
)

func TestInventoryReaderAdapter_GetAvailability(t *testing.T) {
	t.Run("maps OnHand and Available without transposing them", func(t *testing.T) {
		repo := invMocks.NewMockRepository(t)
		invSvc := inventory.NewService(repo)
		adapter := &inventoryReaderAdapter{svc: invSvc}

		// 10 on hand, 4 reserved, 6 sellable -- distinct values so a swap of
		// OnHand/Available in the adapter's mapping fails this assertion.
		id := uuid.New()
		ids := []uuid.UUID{id}
		repo.EXPECT().GetLevels(mock.Anything, ids).
			Return(map[uuid.UUID]inventory.Stock{
				id: {ProductID: id, Quantity: 10, Reserved: 4, Available: 6},
			}, nil)

		result, err := adapter.GetAvailability(context.Background(), ids)
		require.NoError(t, err)
		require.Contains(t, result, id)
		assert.Equal(t, 10, result[id].OnHand, "OnHand must come from Stock.Quantity")
		assert.Equal(t, 6, result[id].Available, "Available must come from Stock.Available, not Quantity")
	})

	t.Run("propagates a repository error", func(t *testing.T) {
		repo := invMocks.NewMockRepository(t)
		invSvc := inventory.NewService(repo)
		adapter := &inventoryReaderAdapter{svc: invSvc}

		ids := []uuid.UUID{uuid.New()}
		repo.EXPECT().GetLevels(mock.Anything, ids).Return(nil, errors.New("db error"))

		result, err := adapter.GetAvailability(context.Background(), ids)
		assert.Nil(t, result)
		assert.Error(t, err)
	})
}
