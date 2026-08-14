package register

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestCommand_EnsureLevel(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		productID := uuid.New()
		repo.EXPECT().EnsureLevel(mock.Anything, productID).Return(nil)

		err := cmd.EnsureLevel(context.Background(), productID)

		require.NoError(t, err)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		productID := uuid.New()
		repo.EXPECT().EnsureLevel(mock.Anything, productID).Return(errors.New("ensuring inventory level"))

		err := cmd.EnsureLevel(context.Background(), productID)

		require.Error(t, err)
	})
}
