package jobs

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestQueue_CancelPendingByOrderID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		cmd, repo := newTestQueue(t)

		orderID := uuid.New()

		repo.EXPECT().CancelJobsByOrderID(mock.Anything, orderID).
			Return(nil)

		err := cmd.CancelPendingByOrderID(ctx, orderID)

		require.NoError(t, err)
	})

	t.Run("error propagates", func(t *testing.T) {
		t.Parallel()

		cmd, repo := newTestQueue(t)

		orderID := uuid.New()

		repo.EXPECT().CancelJobsByOrderID(mock.Anything, orderID).
			Return(errors.New("db error"))

		err := cmd.CancelPendingByOrderID(ctx, orderID)

		require.Error(t, err)
		assert.ErrorContains(t, err, "db error")
	})
}

func newTestQueue(t *testing.T) (*Queue, *MockRepository) {
	repo := NewMockRepository(t)
	cmd := New(repo)

	return cmd, repo
}
