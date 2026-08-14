package create

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/category/domain"
)

func TestCommand_Execute(t *testing.T) {
	t.Parallel()

	t.Run("success without parent", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		repo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(c *domain.Category) bool {
			return c.Name == "Electronics" && c.Slug == "electronics" && c.Active
		})).Run(func(_ context.Context, c *domain.Category) {
			c.ID = uuid.New()
			c.CreatedAt = time.Now()
			c.UpdatedAt = time.Now()
		}).Return(nil)

		result, err := cmd.Execute(context.Background(), Params{
			Name: "Electronics",
		})

		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, result.ID)
		result.ID = uuid.Nil
		result.CreatedAt = time.Time{}
		result.UpdatedAt = time.Time{}
		assert.Equal(t, &domain.Category{
			Name:   "Electronics",
			Slug:   "electronics",
			Active: true,
		}, result)
	})

	t.Run("repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		repo.EXPECT().Create(mock.Anything, mock.Anything).Return(apperror.ErrConflict)

		result, err := cmd.Execute(context.Background(), Params{
			Name: "Electronics",
		})

		assert.Nil(t, result)
		assert.ErrorIs(t, err, apperror.ErrConflict)
	})

	t.Run("sets sort order and active from request", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		sortOrder := 5
		active := false
		repo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(c *domain.Category) bool {
			return c.Name == "Books" && c.SortOrder == 5 && !c.Active
		})).Run(func(_ context.Context, c *domain.Category) {
			c.ID = uuid.New()
			c.CreatedAt = time.Now()
			c.UpdatedAt = time.Now()
		}).Return(nil)

		result, err := cmd.Execute(context.Background(), Params{
			Name:      "Books",
			SortOrder: &sortOrder,
			Active:    &active,
		})

		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, result.ID)
		result.ID = uuid.Nil
		result.CreatedAt = time.Time{}
		result.UpdatedAt = time.Time{}
		assert.Equal(t, &domain.Category{
			Name:      "Books",
			Slug:      "books",
			SortOrder: 5,
			Active:    false,
		}, result)
	})
}

func TestCommand_ValidateParent(t *testing.T) {
	t.Parallel()

	t.Run("rejects a parent that does not exist", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		parentID := uuid.New()
		// A non-existent parentID matches zero rows, so the CTE reports depth 0 rather
		// than ErrNotFound, and validateParent never calls GetByID.
		repo.EXPECT().AncestorDepthAndCycle(mock.Anything, parentID, mock.Anything, 5).
			Return(0, false, nil)
		cmd := New(repo)

		_, err := cmd.Execute(context.Background(), Params{
			Name:     "Orphan",
			ParentID: &parentID,
		})

		require.ErrorIs(t, err, apperror.ErrBadRequest)
		assert.ErrorContains(t, err, "parent category not found")
	})

	t.Run("rejects a chain deeper than five", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		parentID := uuid.New()
		repo.EXPECT().AncestorDepthAndCycle(mock.Anything, parentID, mock.Anything, 5).
			Return(5, false, nil)
		cmd := New(repo)

		_, err := cmd.Execute(context.Background(), Params{
			Name:     "L6",
			ParentID: &parentID,
		})

		require.ErrorIs(t, err, apperror.ErrBadRequest)
		assert.ErrorContains(t, err, "depth exceeds maximum of 5")
	})

	t.Run("propagates a repository failure from the depth check", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		parentID := uuid.New()
		repo.EXPECT().AncestorDepthAndCycle(mock.Anything, parentID, mock.Anything, 5).
			Return(0, false, errors.New("connection refused"))
		cmd := New(repo)

		_, err := cmd.Execute(context.Background(), Params{
			Name:     "Child",
			ParentID: &parentID,
		})

		assert.Error(t, err)
	})

	t.Run("creates a child under a valid parent", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		parentID := uuid.New()
		repo.EXPECT().AncestorDepthAndCycle(mock.Anything, parentID, mock.Anything, 5).
			Return(1, false, nil)
		repo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(c *domain.Category) bool {
			return c.Name == "Child" && c.ParentID != nil && *c.ParentID == parentID
		})).Return(nil)
		cmd := New(repo)

		result, err := cmd.Execute(context.Background(), Params{
			Name:     "Child",
			ParentID: &parentID,
		})

		require.NoError(t, err)
		assert.Equal(t, &parentID, result.ParentID)
	})
}
