package wiring

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/product"
	productMocks "github.com/residwi/go-api-project-template/mocks/product"
)

func TestProductLookupAdapter_GetByIDs(t *testing.T) {
	t.Run("maps a batch in one call and carries Status through", func(t *testing.T) {
		repo := productMocks.NewMockRepository(t)
		inv := productMocks.NewMockInventoryReader(t)
		reg := productMocks.NewMockInventoryRegistrar(t)
		productSvc := product.NewService(repo, inv, reg)
		adapter := &productLookupAdapter{svc: productSvc}

		liveID, archivedID := uuid.New(), uuid.New()
		ids := []uuid.UUID{liveID, archivedID}
		repo.EXPECT().GetByIDsIncludingDeleted(mock.Anything, ids).
			Return([]product.Product{
				{ID: liveID, Name: "Widget", Price: 1500, Currency: "USD", Status: product.StatusPublished},
				{ID: archivedID, Name: "Gone", Price: 900, Currency: "USD", Status: product.StatusArchived},
			}, nil)
		inv.EXPECT().GetAvailability(mock.Anything, ids).
			Return(map[uuid.UUID]product.Availability{
				liveID: {OnHand: 10, Available: 7},
			}, nil)

		result, err := adapter.GetByIDs(context.Background(), ids)
		require.NoError(t, err)
		require.Len(t, result, 2)
		assert.Equal(t, 7, result[liveID].Available, "Available must come from Availability.Available")
		assert.Equal(t, product.StatusArchived, result[archivedID].Status, "archived products must still come back, carrying Status")
	})

	t.Run("flags a soft-deleted product unavailable instead of passing its stale status through", func(t *testing.T) {
		// product.Delete only sets deleted_at -- it never touches status -- so a
		// withdrawn product's row still reads status='published'. GetByIDs must not
		// forward that stale status verbatim, or a cart line (and the order guard
		// downstream) would see a perfectly sellable-looking product.
		repo := productMocks.NewMockRepository(t)
		inv := productMocks.NewMockInventoryReader(t)
		reg := productMocks.NewMockInventoryRegistrar(t)
		productSvc := product.NewService(repo, inv, reg)
		adapter := &productLookupAdapter{svc: productSvc}

		deletedID := uuid.New()
		ids := []uuid.UUID{deletedID}
		deletedAt := time.Now()
		repo.EXPECT().GetByIDsIncludingDeleted(mock.Anything, ids).
			Return([]product.Product{
				{
					ID: deletedID, Name: "Withdrawn Widget", Price: 1200, Currency: "USD",
					Status: product.StatusPublished, DeletedAt: &deletedAt,
				},
			}, nil)
		inv.EXPECT().GetAvailability(mock.Anything, ids).
			Return(map[uuid.UUID]product.Availability{}, nil)

		result, err := adapter.GetByIDs(context.Background(), ids)
		require.NoError(t, err)
		require.Len(t, result, 1)
		assert.Equal(t, "unavailable", result[deletedID].Status,
			"a soft-deleted product must be flagged unavailable, not carry its stale published status")
	})

	t.Run("propagates a service error", func(t *testing.T) {
		repo := productMocks.NewMockRepository(t)
		inv := productMocks.NewMockInventoryReader(t)
		reg := productMocks.NewMockInventoryRegistrar(t)
		productSvc := product.NewService(repo, inv, reg)
		adapter := &productLookupAdapter{svc: productSvc}

		ids := []uuid.UUID{uuid.New()}
		repo.EXPECT().GetByIDsIncludingDeleted(mock.Anything, ids).Return(nil, errors.New("db error"))

		result, err := adapter.GetByIDs(context.Background(), ids)
		assert.Nil(t, result)
		assert.Error(t, err)
	})
}
