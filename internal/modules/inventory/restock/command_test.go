package restock

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

func TestCommand_Execute(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		productID := uuid.New()
		expected := &domain.Stock{ProductID: productID, Quantity: 150, Reserved: 5, Available: 145}
		repo.EXPECT().Restock(mock.Anything, productID, 50).Return(expected, nil)

		result, err := cmd.Execute(context.Background(), productID, 50)

		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		productID := uuid.New()
		repo.EXPECT().Restock(mock.Anything, productID, 50).Return(nil, errors.New("not found"))

		result, err := cmd.Execute(context.Background(), productID, 50)

		assert.Nil(t, result)
		assert.Error(t, err)
	})
}
