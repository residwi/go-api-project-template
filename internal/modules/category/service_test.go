package category

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
)

func TestService_Create(t *testing.T) {
	t.Parallel()

	t.Run("success without parent", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		counter := NewMockProductCounter(t)
		svc := NewService(repo, counter)

		repo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(c *Category) bool {
			return c.Name == "Electronics" && c.Slug == "electronics" && c.Active
		})).Run(func(_ context.Context, c *Category) {
			c.ID = uuid.New()
			c.CreatedAt = time.Now()
			c.UpdatedAt = time.Now()
		}).Return(nil)

		result, err := svc.Create(context.Background(), CreateParams{
			Name: "Electronics",
		})

		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, result.ID)
		result.ID = uuid.Nil
		result.CreatedAt = time.Time{}
		result.UpdatedAt = time.Time{}
		assert.Equal(t, &Category{
			Name:   "Electronics",
			Slug:   "electronics",
			Active: true,
		}, result)
	})

	t.Run("repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		counter := NewMockProductCounter(t)
		svc := NewService(repo, counter)

		repo.EXPECT().Create(mock.Anything, mock.Anything).Return(apperror.ErrConflict)

		result, err := svc.Create(context.Background(), CreateParams{
			Name: "Electronics",
		})

		assert.Nil(t, result)
		assert.ErrorIs(t, err, apperror.ErrConflict)
	})

	t.Run("sets sort order and active from request", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		counter := NewMockProductCounter(t)
		svc := NewService(repo, counter)

		sortOrder := 5
		active := false
		repo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(c *Category) bool {
			return c.Name == "Books" && c.SortOrder == 5 && !c.Active
		})).Run(func(_ context.Context, c *Category) {
			c.ID = uuid.New()
			c.CreatedAt = time.Now()
			c.UpdatedAt = time.Now()
		}).Return(nil)

		result, err := svc.Create(context.Background(), CreateParams{
			Name:      "Books",
			SortOrder: &sortOrder,
			Active:    &active,
		})

		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, result.ID)
		result.ID = uuid.Nil
		result.CreatedAt = time.Time{}
		result.UpdatedAt = time.Time{}
		assert.Equal(t, &Category{
			Name:      "Books",
			Slug:      "books",
			SortOrder: 5,
			Active:    false,
		}, result)
	})
}

func TestService_GetBySlug(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		counter := NewMockProductCounter(t)
		svc := NewService(repo, counter)

		expected := &Category{
			ID:   uuid.New(),
			Name: "Electronics",
			Slug: "electronics",
		}
		repo.EXPECT().GetBySlug(mock.Anything, "electronics").Return(expected, nil)

		result, err := svc.GetBySlug(context.Background(), "electronics")

		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		counter := NewMockProductCounter(t)
		svc := NewService(repo, counter)

		repo.EXPECT().GetBySlug(mock.Anything, "nonexistent").Return(nil, apperror.ErrNotFound)

		result, err := svc.GetBySlug(context.Background(), "nonexistent")

		assert.Nil(t, result)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestService_List(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		counter := NewMockProductCounter(t)
		svc := NewService(repo, counter)

		expected := []Category{
			{ID: uuid.New(), Name: "Electronics", Slug: "electronics"},
			{ID: uuid.New(), Name: "Books", Slug: "books"},
		}
		repo.EXPECT().List(mock.Anything).Return(expected, nil)

		result, err := svc.List(context.Background())

		require.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Equal(t, expected, result)
	})

	t.Run("repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		counter := NewMockProductCounter(t)
		svc := NewService(repo, counter)

		repo.EXPECT().List(mock.Anything).Return(nil, errors.New("db error"))

		result, err := svc.List(context.Background())
		assert.Nil(t, result)
		require.Error(t, err)
	})
}

func TestService_GetByID(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		counter := NewMockProductCounter(t)
		svc := NewService(repo, counter)

		id := uuid.New()
		expected := &Category{ID: id, Name: "Electronics", Slug: "electronics"}
		repo.EXPECT().GetByID(mock.Anything, id).Return(expected, nil)

		result, err := svc.GetByID(context.Background(), id)

		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		counter := NewMockProductCounter(t)
		svc := NewService(repo, counter)

		id := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, id).Return(nil, apperror.ErrNotFound)

		result, err := svc.GetByID(context.Background(), id)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestService_Update(t *testing.T) {
	t.Parallel()

	t.Run("success partial update", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		counter := NewMockProductCounter(t)
		svc := NewService(repo, counter)

		id := uuid.New()
		existing := &Category{
			ID:     id,
			Name:   "Electronics",
			Slug:   "electronics",
			Active: true,
		}
		repo.EXPECT().GetByID(mock.Anything, id).Return(existing, nil)
		repo.EXPECT().Update(mock.Anything, mock.MatchedBy(func(c *Category) bool {
			return c.Name == "Gadgets" && c.Slug == "gadgets"
		})).Return(nil)

		newName := "Gadgets"
		result, err := svc.Update(context.Background(), id, UpdateParams{
			Name: &newName,
		})

		require.NoError(t, err)
		result.ID = uuid.Nil
		result.CreatedAt = time.Time{}
		result.UpdatedAt = time.Time{}
		assert.Equal(t, &Category{
			Name:   "Gadgets",
			Slug:   "gadgets",
			Active: true,
		}, result)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		counter := NewMockProductCounter(t)
		svc := NewService(repo, counter)

		id := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, id).Return(nil, apperror.ErrNotFound)

		newName := "Gadgets"
		result, err := svc.Update(context.Background(), id, UpdateParams{
			Name: &newName,
		})

		assert.Nil(t, result)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("update repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		counter := NewMockProductCounter(t)
		svc := NewService(repo, counter)

		id := uuid.New()
		existing := &Category{
			ID:     id,
			Name:   "Electronics",
			Slug:   "electronics",
			Active: true,
		}
		repo.EXPECT().GetByID(mock.Anything, id).Return(existing, nil)
		repo.EXPECT().Update(mock.Anything, mock.Anything).Return(apperror.ErrConflict)

		newName := "Gadgets"
		result, err := svc.Update(context.Background(), id, UpdateParams{
			Name: &newName,
		})

		assert.Nil(t, result)
		assert.ErrorIs(t, err, apperror.ErrConflict)
	})

	t.Run("updates all optional fields", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		counter := NewMockProductCounter(t)
		svc := NewService(repo, counter)

		id := uuid.New()
		existing := &Category{
			ID:     id,
			Name:   "Old",
			Slug:   "old",
			Active: true,
		}
		repo.EXPECT().GetByID(mock.Anything, id).Return(existing, nil)
		repo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil)

		newName := "New"
		newDesc := "A description"
		newSort := 10
		newActive := false
		result, err := svc.Update(context.Background(), id, UpdateParams{
			Name:        &newName,
			Description: &newDesc,
			SortOrder:   &newSort,
			Active:      &newActive,
		})

		require.NoError(t, err)
		result.ID = uuid.Nil
		result.CreatedAt = time.Time{}
		result.UpdatedAt = time.Time{}
		assert.Equal(t, &Category{
			Name:        "New",
			Slug:        "new",
			Description: &newDesc,
			SortOrder:   10,
			Active:      false,
		}, result)
	})
}

func TestService_Delete(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		counter := NewMockProductCounter(t)
		svc := NewService(repo, counter)

		id := uuid.New()
		counter.EXPECT().CountPublished(mock.Anything, id).Return(0, nil)
		repo.EXPECT().Delete(mock.Anything, id).Return(nil)

		err := svc.Delete(context.Background(), id)

		assert.NoError(t, err)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		counter := NewMockProductCounter(t)
		svc := NewService(repo, counter)

		id := uuid.New()
		counter.EXPECT().CountPublished(mock.Anything, id).Return(0, nil)
		repo.EXPECT().Delete(mock.Anything, id).Return(apperror.ErrNotFound)

		err := svc.Delete(context.Background(), id)

		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("has published products returns ErrBadRequest", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		counter := NewMockProductCounter(t)
		svc := NewService(repo, counter)

		id := uuid.New()
		counter.EXPECT().CountPublished(mock.Anything, id).Return(3, nil)

		err := svc.Delete(context.Background(), id)

		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("count published products error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		counter := NewMockProductCounter(t)
		svc := NewService(repo, counter)

		id := uuid.New()
		counter.EXPECT().CountPublished(mock.Anything, id).Return(0, errors.New("db error"))

		err := svc.Delete(context.Background(), id)

		require.Error(t, err)
	})

	t.Run("delete repo error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		counter := NewMockProductCounter(t)
		svc := NewService(repo, counter)

		id := uuid.New()
		counter.EXPECT().CountPublished(mock.Anything, id).Return(0, nil)

		deleteErr := errors.New("database delete failed")
		repo.EXPECT().Delete(mock.Anything, id).Return(deleteErr)

		err := svc.Delete(context.Background(), id)
		assert.ErrorIs(t, err, deleteErr)
	})
}

func TestService_Delete_RefusesCategoryWithPublishedProducts(t *testing.T) {
	t.Parallel()

	repo := NewMockRepository(t)
	counter := NewMockProductCounter(t)
	svc := NewService(repo, counter)

	categoryID := uuid.New()
	counter.EXPECT().CountPublished(mock.Anything, categoryID).Return(3, nil)

	err := svc.Delete(context.Background(), categoryID)
	require.ErrorIs(t, err, apperror.ErrBadRequest)
}

func TestService_ValidateParent(t *testing.T) {
	t.Parallel()

	t.Run("rejects a parent that does not exist", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		counter := NewMockProductCounter(t)
		parentID := uuid.New()
		// A non-existent parentID makes the recursive CTE match zero rows, so
		// AncestorDepthAndCycle reports depth 0 rather than returning ErrNotFound.
		// validateParent never loads the parent via GetByID.
		repo.EXPECT().AncestorDepthAndCycle(mock.Anything, parentID, mock.Anything, 5).
			Return(0, false, nil)
		svc := NewService(repo, counter)

		_, err := svc.Create(context.Background(), CreateParams{
			Name:     "Orphan",
			ParentID: &parentID,
		})

		require.ErrorIs(t, err, apperror.ErrBadRequest)
		assert.ErrorContains(t, err, "parent category not found")
	})

	t.Run("rejects a chain deeper than five", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		counter := NewMockProductCounter(t)
		parentID := uuid.New()
		repo.EXPECT().AncestorDepthAndCycle(mock.Anything, parentID, mock.Anything, 5).
			Return(5, false, nil)
		svc := NewService(repo, counter)

		_, err := svc.Create(context.Background(), CreateParams{
			Name:     "L6",
			ParentID: &parentID,
		})

		require.ErrorIs(t, err, apperror.ErrBadRequest)
		assert.ErrorContains(t, err, "depth exceeds maximum of 5")
	})

	t.Run("rejects a move that the repository reports as circular", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		counter := NewMockProductCounter(t)
		selfID, parentID := uuid.New(), uuid.New()
		// Update loads the category being moved via GetByID(id) before it ever
		// validates the new parent; validateParent itself never calls GetByID.
		repo.EXPECT().GetByID(mock.Anything, selfID).
			Return(&Category{ID: selfID, Name: "A"}, nil)
		repo.EXPECT().AncestorDepthAndCycle(mock.Anything, parentID, selfID, mock.Anything).
			Return(2, true, nil)
		svc := NewService(repo, counter)

		_, err := svc.Update(context.Background(), selfID, UpdateParams{ParentID: &parentID})

		require.ErrorIs(t, err, apperror.ErrBadRequest)
		assert.ErrorContains(t, err, "circular parent reference")
	})

	t.Run("rejects a category set as its own parent", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		counter := NewMockProductCounter(t)
		selfID := uuid.New()
		// Update loads the category via GetByID(id) unconditionally, as its
		// first step, before it ever looks at ParentID - so this call always
		// happens here, regardless of the identity check below.
		repo.EXPECT().GetByID(mock.Anything, selfID).
			Return(&Category{ID: selfID, Name: "A"}, nil)
		svc := NewService(repo, counter)

		_, err := svc.Update(context.Background(), selfID, UpdateParams{ParentID: &selfID})

		require.ErrorIs(t, err, apperror.ErrBadRequest)
		assert.ErrorContains(t, err, "cannot be its own parent")
	})

	t.Run("propagates a repository failure from the depth check", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		counter := NewMockProductCounter(t)
		parentID := uuid.New()
		repo.EXPECT().AncestorDepthAndCycle(mock.Anything, parentID, mock.Anything, 5).
			Return(0, false, errors.New("connection refused"))
		svc := NewService(repo, counter)

		_, err := svc.Create(context.Background(), CreateParams{
			Name:     "Child",
			ParentID: &parentID,
		})

		assert.Error(t, err)
	})

	t.Run("creates a child under a valid parent", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		counter := NewMockProductCounter(t)
		parentID := uuid.New()
		repo.EXPECT().AncestorDepthAndCycle(mock.Anything, parentID, mock.Anything, 5).
			Return(1, false, nil)
		repo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(c *Category) bool {
			return c.Name == "Child" && c.ParentID != nil && *c.ParentID == parentID
		})).Return(nil)
		svc := NewService(repo, counter)

		result, err := svc.Create(context.Background(), CreateParams{
			Name:     "Child",
			ParentID: &parentID,
		})

		require.NoError(t, err)
		assert.Equal(t, &parentID, result.ParentID)
	})

	t.Run("moves a category to a valid new parent", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		counter := NewMockProductCounter(t)
		selfID, parentID := uuid.New(), uuid.New()
		existing := &Category{ID: selfID, Name: "Child", Slug: "child"}
		repo.EXPECT().GetByID(mock.Anything, selfID).Return(existing, nil)
		repo.EXPECT().AncestorDepthAndCycle(mock.Anything, parentID, selfID, mock.Anything).
			Return(1, false, nil)
		repo.EXPECT().Update(mock.Anything, mock.MatchedBy(func(c *Category) bool {
			return c.ID == selfID && c.ParentID != nil && *c.ParentID == parentID
		})).Return(nil)
		svc := NewService(repo, counter)

		result, err := svc.Update(context.Background(), selfID, UpdateParams{ParentID: &parentID})

		require.NoError(t, err)
		assert.Equal(t, &parentID, result.ParentID)
	})
}
