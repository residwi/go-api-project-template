package add

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestCommand_Execute(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()
		wishlistID := uuid.New()

		repo.EXPECT().GetOrCreate(mock.Anything, userID).Return(wishlistID, nil)
		repo.EXPECT().AddItem(mock.Anything, wishlistID, productID).Return(nil)

		err := New(repo).Execute(ctx, userID, Params{ProductID: productID})
		require.NoError(t, err)
	})

	t.Run("get or create fails", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		repo.EXPECT().GetOrCreate(mock.Anything, userID).Return(uuid.Nil, assert.AnError)

		err := New(repo).Execute(ctx, userID, Params{ProductID: productID})
		require.Error(t, err)
		assert.ErrorIs(t, err, assert.AnError)
	})

	t.Run("add item repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()
		wishlistID := uuid.New()

		repo.EXPECT().GetOrCreate(mock.Anything, userID).Return(wishlistID, nil)
		repo.EXPECT().AddItem(mock.Anything, wishlistID, productID).Return(assert.AnError)

		err := New(repo).Execute(ctx, userID, Params{ProductID: productID})
		require.Error(t, err)
		assert.ErrorIs(t, err, assert.AnError)
	})
}
