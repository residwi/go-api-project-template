package deduct

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

func TestCommand_Deduct(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		productID := uuid.New()
		expected := &domain.Stock{ProductID: productID, Quantity: 90, Reserved: 0, Available: 90}
		repo.EXPECT().Deduct(mock.Anything, productID, 10).Return(expected, nil)

		result, err := cmd.Deduct(context.Background(), productID, 10)

		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		productID := uuid.New()
		repo.EXPECT().Deduct(mock.Anything, productID, 10).Return(nil, errors.New("cannot deduct stock"))

		result, err := cmd.Deduct(context.Background(), productID, 10)

		assert.Nil(t, result)
		assert.Error(t, err)
	})
}
