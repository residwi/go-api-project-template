package markallread

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
		cmd := New(repo)

		ctx := context.Background()
		userID := uuid.New()

		repo.EXPECT().MarkAllRead(mock.Anything, userID).Return(nil)

		err := cmd.Execute(ctx, userID)
		require.NoError(t, err)
	})

	t.Run("repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		ctx := context.Background()
		userID := uuid.New()

		repo.EXPECT().MarkAllRead(mock.Anything, userID).Return(assert.AnError)

		err := cmd.Execute(ctx, userID)
		assert.ErrorIs(t, err, assert.AnError)
	})
}
