package adjust

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
		expected := &domain.Stock{ProductID: productID, Quantity: 200, Reserved: 10, Available: 190}
		repo.EXPECT().AdjustStock(mock.Anything, productID, 200).Return(expected, nil)

		result, err := cmd.Execute(context.Background(), productID, 200)

		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		productID := uuid.New()
		repo.EXPECT().
			AdjustStock(mock.Anything, productID, 5).
			Return(nil, errors.New("cannot set stock below reserved quantity"))

		result, err := cmd.Execute(context.Background(), productID, 5)

		assert.Nil(t, result)
		assert.Error(t, err)
	})
}
