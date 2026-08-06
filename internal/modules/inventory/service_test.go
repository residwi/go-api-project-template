package inventory

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/modules/inventory/contract"
)

func TestServiceReserveBatchTakesAMap(t *testing.T) {
	t.Parallel()

	productA := uuid.New()
	productB := uuid.New()

	repo := NewMockRepository(t)
	repo.EXPECT().ReserveBatch(mock.Anything, map[uuid.UUID]int{productA: 2, productB: 1}).Return(nil)

	svc := NewService(repo)

	err := svc.ReserveBatch(context.Background(), map[uuid.UUID]int{productA: 2, productB: 1})

	require.NoError(t, err)
}

func TestService_Restore(t *testing.T) {
	t.Parallel()

	t.Run("releases stock that was only reserved", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo)

		items := map[uuid.UUID]int{uuid.New(): 2}
		repo.EXPECT().ReleaseBatch(mock.Anything, items).Return(nil)

		err := svc.Restore(context.Background(), items, contract.Reserved)
		require.NoError(t, err)
	})

	t.Run("restocks stock that was deducted", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo)

		items := map[uuid.UUID]int{uuid.New(): 3}
		repo.EXPECT().RestockBatch(mock.Anything, items).Return(nil)

		err := svc.Restore(context.Background(), items, contract.Deducted)
		require.NoError(t, err)
	})
}

func TestService_Reserve(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo)

		productID := uuid.New()
		expected := &Stock{ProductID: productID, Quantity: 100, Reserved: 10, Available: 90}
		repo.EXPECT().Reserve(mock.Anything, productID, 10).Return(expected, nil)

		result, err := svc.Reserve(context.Background(), productID, 10)

		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo)

		productID := uuid.New()
		repo.EXPECT().Reserve(mock.Anything, productID, 10).Return(nil, errors.New("insufficient stock"))

		result, err := svc.Reserve(context.Background(), productID, 10)

		assert.Nil(t, result)
		assert.Error(t, err)
	})
}

func TestService_Release(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo)

		productID := uuid.New()
		expected := &Stock{ProductID: productID, Quantity: 100, Reserved: 5, Available: 95}
		repo.EXPECT().Release(mock.Anything, productID, 5).Return(expected, nil)

		result, err := svc.Release(context.Background(), productID, 5)

		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo)

		productID := uuid.New()
		repo.EXPECT().Release(mock.Anything, productID, 5).Return(nil, errors.New("cannot release more than reserved"))

		result, err := svc.Release(context.Background(), productID, 5)

		assert.Nil(t, result)
		assert.Error(t, err)
	})
}

func TestService_Deduct(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo)

		productID := uuid.New()
		expected := &Stock{ProductID: productID, Quantity: 90, Reserved: 0, Available: 90}
		repo.EXPECT().Deduct(mock.Anything, productID, 10).Return(expected, nil)

		result, err := svc.Deduct(context.Background(), productID, 10)

		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo)

		productID := uuid.New()
		repo.EXPECT().Deduct(mock.Anything, productID, 10).Return(nil, errors.New("cannot deduct stock"))

		result, err := svc.Deduct(context.Background(), productID, 10)

		assert.Nil(t, result)
		assert.Error(t, err)
	})
}

func TestService_Restock(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo)

		productID := uuid.New()
		expected := &Stock{ProductID: productID, Quantity: 150, Reserved: 5, Available: 145}
		repo.EXPECT().Restock(mock.Anything, productID, 50).Return(expected, nil)

		result, err := svc.Restock(context.Background(), productID, 50)

		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo)

		productID := uuid.New()
		repo.EXPECT().Restock(mock.Anything, productID, 50).Return(nil, errors.New("not found"))

		result, err := svc.Restock(context.Background(), productID, 50)

		assert.Nil(t, result)
		assert.Error(t, err)
	})
}

func TestService_GetStock(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo)

		productID := uuid.New()
		expected := &Stock{ProductID: productID, Quantity: 100, Reserved: 10, Available: 90}
		repo.EXPECT().GetStock(mock.Anything, productID).Return(expected, nil)

		result, err := svc.GetStock(context.Background(), productID)

		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo)

		productID := uuid.New()
		repo.EXPECT().GetStock(mock.Anything, productID).Return(nil, errors.New("not found"))

		result, err := svc.GetStock(context.Background(), productID)

		assert.Nil(t, result)
		assert.Error(t, err)
	})
}

func TestService_AdjustStock(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo)

		productID := uuid.New()
		expected := &Stock{ProductID: productID, Quantity: 200, Reserved: 10, Available: 190}
		repo.EXPECT().AdjustStock(mock.Anything, productID, 200).Return(expected, nil)

		result, err := svc.AdjustStock(context.Background(), productID, 200)

		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo)

		productID := uuid.New()
		repo.EXPECT().
			AdjustStock(mock.Anything, productID, 5).
			Return(nil, errors.New("cannot set stock below reserved quantity"))

		result, err := svc.AdjustStock(context.Background(), productID, 5)

		assert.Nil(t, result)
		assert.Error(t, err)
	})
}

func TestServiceGetAvailabilityDropsReservedCount(t *testing.T) {
	t.Parallel()

	productID := uuid.New()

	repo := NewMockRepository(t)
	repo.EXPECT().GetLevels(mock.Anything, []uuid.UUID{productID}).
		Return(map[uuid.UUID]Stock{productID: {Quantity: 10, Reserved: 3, Available: 7}}, nil)

	svc := NewService(repo)

	got, err := svc.GetAvailability(context.Background(), []uuid.UUID{productID})

	require.NoError(t, err)
	assert.Equal(t, map[uuid.UUID]contract.Availability{productID: {OnHand: 10, Available: 7}}, got)
}

func TestService_GetAvailability_Error(t *testing.T) {
	t.Parallel()

	repo := NewMockRepository(t)
	svc := NewService(repo)

	ids := []uuid.UUID{uuid.New()}
	repo.EXPECT().GetLevels(mock.Anything, ids).Return(nil, errors.New("db error"))

	result, err := svc.GetAvailability(context.Background(), ids)

	assert.Nil(t, result)
	assert.Error(t, err)
}

func TestService_EnsureLevel(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo)

		productID := uuid.New()
		repo.EXPECT().EnsureLevel(mock.Anything, productID).Return(nil)

		err := svc.EnsureLevel(context.Background(), productID)

		require.NoError(t, err)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo)

		productID := uuid.New()
		repo.EXPECT().EnsureLevel(mock.Anything, productID).Return(errors.New("ensuring inventory level"))

		err := svc.EnsureLevel(context.Background(), productID)

		assert.Error(t, err)
	})
}
