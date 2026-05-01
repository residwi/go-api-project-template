package inventory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/features/inventory"
	mocks "github.com/residwi/go-api-project-template/mocks/inventory"
)

func TestService_Reserve(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := inventory.NewService(repo, nil)

		productID := uuid.New()
		expected := &inventory.Stock{ProductID: productID, Quantity: 100, Reserved: 10, Available: 90}
		repo.EXPECT().Reserve(mock.Anything, productID, 10).Return(expected, nil)

		result, err := svc.Reserve(context.Background(), productID, 10)

		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("error", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := inventory.NewService(repo, nil)

		productID := uuid.New()
		repo.EXPECT().Reserve(mock.Anything, productID, 10).Return(nil, errors.New("insufficient stock"))

		result, err := svc.Reserve(context.Background(), productID, 10)

		assert.Nil(t, result)
		assert.Error(t, err)
	})
}

func TestService_Release(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := inventory.NewService(repo, nil)

		productID := uuid.New()
		expected := &inventory.Stock{ProductID: productID, Quantity: 100, Reserved: 5, Available: 95}
		repo.EXPECT().Release(mock.Anything, productID, 5).Return(expected, nil)

		result, err := svc.Release(context.Background(), productID, 5)

		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("error", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := inventory.NewService(repo, nil)

		productID := uuid.New()
		repo.EXPECT().Release(mock.Anything, productID, 5).Return(nil, errors.New("cannot release more than reserved"))

		result, err := svc.Release(context.Background(), productID, 5)

		assert.Nil(t, result)
		assert.Error(t, err)
	})
}

func TestService_Deduct(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := inventory.NewService(repo, nil)

		productID := uuid.New()
		expected := &inventory.Stock{ProductID: productID, Quantity: 90, Reserved: 0, Available: 90}
		repo.EXPECT().Deduct(mock.Anything, productID, 10).Return(expected, nil)

		result, err := svc.Deduct(context.Background(), productID, 10)

		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("error", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := inventory.NewService(repo, nil)

		productID := uuid.New()
		repo.EXPECT().Deduct(mock.Anything, productID, 10).Return(nil, errors.New("cannot deduct stock"))

		result, err := svc.Deduct(context.Background(), productID, 10)

		assert.Nil(t, result)
		assert.Error(t, err)
	})
}

func TestService_Restock(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := inventory.NewService(repo, nil)

		productID := uuid.New()
		expected := &inventory.Stock{ProductID: productID, Quantity: 150, Reserved: 5, Available: 145}
		repo.EXPECT().Restock(mock.Anything, productID, 50).Return(expected, nil)

		result, err := svc.Restock(context.Background(), productID, 50)

		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("error", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := inventory.NewService(repo, nil)

		productID := uuid.New()
		repo.EXPECT().Restock(mock.Anything, productID, 50).Return(nil, errors.New("not found"))

		result, err := svc.Restock(context.Background(), productID, 50)

		assert.Nil(t, result)
		assert.Error(t, err)
	})
}

func TestService_GetStock(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := inventory.NewService(repo, nil)

		productID := uuid.New()
		expected := &inventory.Stock{ProductID: productID, Quantity: 100, Reserved: 10, Available: 90}
		repo.EXPECT().GetStock(mock.Anything, productID).Return(expected, nil)

		result, err := svc.GetStock(context.Background(), productID)

		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("error", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := inventory.NewService(repo, nil)

		productID := uuid.New()
		repo.EXPECT().GetStock(mock.Anything, productID).Return(nil, errors.New("not found"))

		result, err := svc.GetStock(context.Background(), productID)

		assert.Nil(t, result)
		assert.Error(t, err)
	})
}

func TestService_AdjustStock(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := inventory.NewService(repo, nil)

		productID := uuid.New()
		expected := &inventory.Stock{ProductID: productID, Quantity: 200, Reserved: 10, Available: 190}
		repo.EXPECT().AdjustStock(mock.Anything, productID, 200).Return(expected, nil)

		result, err := svc.AdjustStock(context.Background(), productID, 200)

		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("error", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := inventory.NewService(repo, nil)

		productID := uuid.New()
		repo.EXPECT().AdjustStock(mock.Anything, productID, 5).Return(nil, errors.New("cannot set stock below reserved quantity"))

		result, err := svc.AdjustStock(context.Background(), productID, 5)

		assert.Nil(t, result)
		assert.Error(t, err)
	})
}
