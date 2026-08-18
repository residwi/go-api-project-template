package lock

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
)

// Lock discards the cart id GetCartForLock returns and passes through only
// the error, so this is the only place that proves both halves of that:
// a found cart returns nil, and ErrNotFound reaches order.CartLocker's
// caller unchanged.
func TestUseCase_Lock(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		userID := uuid.New()
		repo.EXPECT().GetCartForLock(mock.Anything, userID).Return(uuid.New(), nil)

		err := cmd.Lock(context.Background(), userID)
		require.NoError(t, err)
	})

	t.Run("not found propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		userID := uuid.New()
		repo.EXPECT().GetCartForLock(mock.Anything, userID).Return(uuid.Nil, apperror.ErrNotFound)

		err := cmd.Lock(context.Background(), userID)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}
