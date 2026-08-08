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
		counter := NewMockProductCounter(t)
		cmd := New(repo, counter)

		id := uuid.New()
		counter.EXPECT().CountPublished(mock.Anything, id).Return(0, nil)
		repo.EXPECT().Delete(mock.Anything, id).Return(nil)

		err := cmd.Execute(context.Background(), id)

		assert.NoError(t, err)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		counter := NewMockProductCounter(t)
		cmd := New(repo, counter)

		id := uuid.New()
		counter.EXPECT().CountPublished(mock.Anything, id).Return(0, nil)
		repo.EXPECT().Delete(mock.Anything, id).Return(apperror.ErrNotFound)

		err := cmd.Execute(context.Background(), id)

		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("has published products returns ErrBadRequest", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		counter := NewMockProductCounter(t)
		cmd := New(repo, counter)

		id := uuid.New()
		counter.EXPECT().CountPublished(mock.Anything, id).Return(3, nil)

		err := cmd.Execute(context.Background(), id)

		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("count published products error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		counter := NewMockProductCounter(t)
		cmd := New(repo, counter)

		id := uuid.New()
		counter.EXPECT().CountPublished(mock.Anything, id).Return(0, errors.New("db error"))

		err := cmd.Execute(context.Background(), id)

		require.Error(t, err)
	})

	t.Run("delete repo error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		counter := NewMockProductCounter(t)
		cmd := New(repo, counter)

		id := uuid.New()
		counter.EXPECT().CountPublished(mock.Anything, id).Return(0, nil)

		deleteErr := errors.New("database delete failed")
		repo.EXPECT().Delete(mock.Anything, id).Return(deleteErr)

		err := cmd.Execute(context.Background(), id)
		assert.ErrorIs(t, err, deleteErr)
	})
}

// TestCommand_Execute_RefusesCategoryWithPublishedProducts pins the guard on
// its own: the products.category_id foreign key would also stop this delete,
// but this guard exists to give the caller a useful message first, and that
// message is behaviour a constraint violation alone would not produce.
func TestCommand_Execute_RefusesCategoryWithPublishedProducts(t *testing.T) {
	t.Parallel()

	repo := NewMockRepository(t)
	counter := NewMockProductCounter(t)
	cmd := New(repo, counter)

	categoryID := uuid.New()
	counter.EXPECT().CountPublished(mock.Anything, categoryID).Return(3, nil)

	err := cmd.Execute(context.Background(), categoryID)
	require.ErrorIs(t, err, apperror.ErrBadRequest)
}
