package remove

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
)

func TestCommand_Execute(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		repo.EXPECT().RemoveItem(mock.Anything, userID, productID).Return(nil)

		err := New(repo).Execute(ctx, userID, productID)
		require.NoError(t, err)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		repo.EXPECT().RemoveItem(mock.Anything, userID, productID).Return(apperror.ErrNotFound)

		err := New(repo).Execute(ctx, userID, productID)
		require.Error(t, err)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("repo error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		repo.EXPECT().RemoveItem(mock.Anything, userID, productID).Return(assert.AnError)

		err := New(repo).Execute(ctx, userID, productID)
		require.Error(t, err)
		assert.ErrorIs(t, err, assert.AnError)
	})
}
