package remove

import (
	"context"
	"errors"
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
		cmd := New(repo)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()
		cartID := uuid.New()

		repo.EXPECT().GetOrCreate(mock.Anything, userID).Return(cartID, nil)
		repo.EXPECT().RemoveItem(mock.Anything, cartID, productID).Return(nil)

		err := cmd.Execute(ctx, userID, productID)
		require.NoError(t, err)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()
		cartID := uuid.New()

		repo.EXPECT().GetOrCreate(mock.Anything, userID).Return(cartID, nil)
		repo.EXPECT().RemoveItem(mock.Anything, cartID, productID).Return(apperror.ErrNotFound)

		err := cmd.Execute(ctx, userID, productID)
		require.Error(t, err)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("get or create error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		repo.EXPECT().GetOrCreate(mock.Anything, userID).Return(uuid.Nil, errors.New("db error"))

		err := cmd.Execute(ctx, userID, productID)
		require.Error(t, err)
	})
}
