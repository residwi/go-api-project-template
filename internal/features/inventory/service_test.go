package inventory

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/features/inventory/domain"
)

func TestService_Adjust(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		s := New(repo)

		productID := uuid.New()
		expected := &domain.Stock{ProductID: productID, Quantity: 200, Reserved: 10, Available: 190}
		repo.EXPECT().AdjustStock(mock.Anything, productID, 200).Return(expected, nil)

		result, err := s.Adjust(context.Background(), productID, 200)

		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		s := New(repo)

		productID := uuid.New()
		repo.EXPECT().
			AdjustStock(mock.Anything, productID, 5).
			Return(nil, errors.New("cannot set stock below reserved quantity"))

		result, err := s.Adjust(context.Background(), productID, 5)

		assert.Nil(t, result)
		assert.Error(t, err)
	})
}

func TestService_Restock(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		s := New(repo)

		productID := uuid.New()
		expected := &domain.Stock{ProductID: productID, Quantity: 150, Reserved: 5, Available: 145}
		repo.EXPECT().Restock(mock.Anything, productID, 50).Return(expected, nil)

		result, err := s.Restock(context.Background(), productID, 50)

		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		s := New(repo)

		productID := uuid.New()
		repo.EXPECT().Restock(mock.Anything, productID, 50).Return(nil, errors.New("not found"))

		result, err := s.Restock(context.Background(), productID, 50)

		assert.Nil(t, result)
		assert.Error(t, err)
	})
}

func TestService_EnsureLevel(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		s := New(repo)

		productID := uuid.New()
		repo.EXPECT().EnsureLevel(mock.Anything, productID).Return(nil)

		err := s.EnsureLevel(context.Background(), productID)

		require.NoError(t, err)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		s := New(repo)

		productID := uuid.New()
		repo.EXPECT().EnsureLevel(mock.Anything, productID).Return(errors.New("ensuring inventory level"))

		err := s.EnsureLevel(context.Background(), productID)

		require.Error(t, err)
	})
}

func TestService_GetStock(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		s := New(repo)

		productID := uuid.New()
		expected := &domain.Stock{ProductID: productID, Quantity: 100, Reserved: 10, Available: 90}
		repo.EXPECT().GetStock(mock.Anything, productID).Return(expected, nil)

		result, err := s.GetStock(context.Background(), productID)

		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		s := New(repo)

		productID := uuid.New()
		repo.EXPECT().GetStock(mock.Anything, productID).Return(nil, errors.New("not found"))

		result, err := s.GetStock(context.Background(), productID)

		assert.Nil(t, result)
		assert.Error(t, err)
	})
}

func TestService_GetAvailability_DropsReservedCount(t *testing.T) {
	t.Parallel()

	productID := uuid.New()

	repo := NewMockRepository(t)
	repo.EXPECT().GetLevels(mock.Anything, []uuid.UUID{productID}).
		Return(map[uuid.UUID]domain.Stock{productID: {Quantity: 10, Reserved: 3, Available: 7}}, nil)

	s := New(repo)

	got, err := s.GetAvailability(context.Background(), []uuid.UUID{productID})

	require.NoError(t, err)
	assert.Equal(t, map[uuid.UUID]Availability{productID: {OnHand: 10, Available: 7}}, got)
}

func TestService_GetAvailability_Error(t *testing.T) {
	t.Parallel()

	repo := NewMockRepository(t)
	s := New(repo)

	ids := []uuid.UUID{uuid.New()}
	repo.EXPECT().GetLevels(mock.Anything, ids).Return(nil, errors.New("db error"))

	result, err := s.GetAvailability(context.Background(), ids)

	assert.Nil(t, result)
	assert.Error(t, err)
}

func TestService_Reserve(t *testing.T) {
	t.Parallel()

	productA := uuid.New()
	productB := uuid.New()

	repo := NewMockRepository(t)
	repo.EXPECT().Reserve(mock.Anything, map[uuid.UUID]int{productA: 2, productB: 1}).Return(nil)

	s := New(repo)

	err := s.Reserve(context.Background(), map[uuid.UUID]int{productA: 2, productB: 1})

	require.NoError(t, err)
}

// Deduct had no service-level test before this flatten -- only its sibling
// Reserve did (deduct/usecase_test.go covered only the now-deleted singular
// Deduct method). Added here for parity now that Deduct is the sole
// surviving deduction path.
func TestService_Deduct(t *testing.T) {
	t.Parallel()

	productA := uuid.New()
	productB := uuid.New()

	repo := NewMockRepository(t)
	repo.EXPECT().Deduct(mock.Anything, map[uuid.UUID]int{productA: 2, productB: 1}).Return(nil)

	s := New(repo)

	err := s.Deduct(context.Background(), map[uuid.UUID]int{productA: 2, productB: 1})

	require.NoError(t, err)
}

func TestService_Restore(t *testing.T) {
	t.Parallel()

	t.Run("releases stock that was only reserved", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		s := New(repo)

		items := map[uuid.UUID]int{uuid.New(): 2}
		repo.EXPECT().ReleaseBatch(mock.Anything, items).Return(nil)

		err := s.Restore(context.Background(), items, StockReserved)
		require.NoError(t, err)
	})

	t.Run("restocks stock that was deducted", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		s := New(repo)

		items := map[uuid.UUID]int{uuid.New(): 3}
		repo.EXPECT().RestockBatch(mock.Anything, items).Return(nil)

		err := s.Restore(context.Background(), items, StockDeducted)
		require.NoError(t, err)
	})
}
