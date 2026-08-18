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

// The old cart.UseCase had no dedicated test for LockCart -- GetCartForLock's
// own repository tests were the only coverage. This is new: a bare delegator
// still deserves proof it forwards the id-discarding call and the error
// order.CartLocker's contract promises.
func TestCommand_LockCart(t *testing.T) {
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
