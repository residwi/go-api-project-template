package query

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/category/domain"
)

func TestReader_List(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		r := New(repo)

		expected := []domain.Category{
			{ID: uuid.New(), Name: "Electronics", Slug: "electronics"},
			{ID: uuid.New(), Name: "Books", Slug: "books"},
		}
		repo.EXPECT().List(mock.Anything).Return(expected, nil)

		result, err := r.List(context.Background())

		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		r := New(repo)

		repo.EXPECT().List(mock.Anything).Return(nil, errors.New("db error"))

		result, err := r.List(context.Background())
		assert.Nil(t, result)
		require.Error(t, err)
	})
}

func TestReader_GetBySlug(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		r := New(repo)

		expected := &domain.Category{
			ID:   uuid.New(),
			Name: "Electronics",
			Slug: "electronics",
		}
		repo.EXPECT().GetBySlug(mock.Anything, "electronics").Return(expected, nil)

		result, err := r.GetBySlug(context.Background(), "electronics")

		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		r := New(repo)

		repo.EXPECT().GetBySlug(mock.Anything, "nonexistent").Return(nil, apperror.ErrNotFound)

		result, err := r.GetBySlug(context.Background(), "nonexistent")

		assert.Nil(t, result)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}
