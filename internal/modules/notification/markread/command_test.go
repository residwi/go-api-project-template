package markread

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
		cmd := New(repo)

		ctx := context.Background()
		userID := uuid.New()
		id := uuid.New()

		repo.EXPECT().MarkRead(mock.Anything, userID, id).Return(nil)

		err := cmd.Execute(ctx, userID, id)
		require.NoError(t, err)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		ctx := context.Background()
		userID := uuid.New()
		id := uuid.New()

		repo.EXPECT().MarkRead(mock.Anything, userID, id).Return(apperror.ErrNotFound)

		err := cmd.Execute(ctx, userID, id)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}
