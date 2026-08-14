package clear

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestCommand_Clear(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		ctx := context.Background()
		userID := uuid.New()

		repo.EXPECT().Clear(mock.Anything, userID).Return(nil)

		err := cmd.Clear(ctx, userID)
		require.NoError(t, err)
	})

	t.Run("repo error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		ctx := context.Background()
		userID := uuid.New()

		repo.EXPECT().Clear(mock.Anything, userID).Return(errors.New("db error"))

		err := cmd.Clear(ctx, userID)
		require.Error(t, err)
	})
}
