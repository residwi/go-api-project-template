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

	"github.com/residwi/go-api-project-template/internal/modules/category/domain"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
)

func TestService_Create(t *testing.T) {
	t.Parallel()

	t.Run("success without parent", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var products ProductCounter
		svc := New(repo, products)

		repo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(c *domain.Category) bool {
			return c.Name == "Electronics" && c.Slug == "electronics" && c.Active
		})).Run(func(_ context.Context, c *domain.Category) {
			c.ID = uuid.New()
			c.CreatedAt = time.Now()
			c.UpdatedAt = time.Now()
		}).Return(nil)

		result, err := svc.Create(t.Context(), "Electronics", nil, nil, nil, nil)

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
		var products ProductCounter
		svc := New(repo, products)

		repo.EXPECT().Create(mock.Anything, mock.Anything).Return(errs.ErrConflict)

		result, err := svc.Create(t.Context(), "Electronics", nil, nil, nil, nil)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, errs.ErrConflict)
	})

	t.Run("sets sort order and active from request", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var products ProductCounter
		svc := New(repo, products)

		sortOrder := 5
		active := false
		repo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(c *domain.Category) bool {
			return c.Name == "Books" && c.SortOrder == 5 && !c.Active
		})).Run(func(_ context.Context, c *domain.Category) {
			c.ID = uuid.New()
			c.CreatedAt = time.Now()
			c.UpdatedAt = time.Now()
		}).Return(nil)

		result, err := svc.Create(t.Context(), "Books", nil, nil, &sortOrder, &active)

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

func TestService_Create_ValidatesParent(t *testing.T) {
	t.Parallel()

	t.Run("rejects a parent that does not exist", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		parentID := uuid.New()
		// A non-existent parentID matches zero rows, so the CTE reports depth 0 rather
		// than ErrNotFound, and validateParent never calls GetByID.
		repo.EXPECT().AncestorDepthAndCycle(mock.Anything, parentID, mock.Anything, 5).
			Return(0, false, nil)
		var products ProductCounter
		svc := New(repo, products)

		_, err := svc.Create(t.Context(), "Orphan", nil, &parentID, nil, nil)

		require.ErrorIs(t, err, errs.ErrBadRequest)
		assert.ErrorContains(t, err, "parent category not found")
	})

	t.Run("rejects a chain deeper than five", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		parentID := uuid.New()
		repo.EXPECT().AncestorDepthAndCycle(mock.Anything, parentID, mock.Anything, 5).
			Return(5, false, nil)
		var products ProductCounter
		svc := New(repo, products)

		_, err := svc.Create(t.Context(), "L6", nil, &parentID, nil, nil)

		require.ErrorIs(t, err, errs.ErrBadRequest)
		assert.ErrorContains(t, err, "depth exceeds maximum of 5")
	})

	t.Run("propagates a repository failure from the depth check", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		parentID := uuid.New()
		repo.EXPECT().AncestorDepthAndCycle(mock.Anything, parentID, mock.Anything, 5).
			Return(0, false, errors.New("connection refused"))
		var products ProductCounter
		svc := New(repo, products)

		_, err := svc.Create(t.Context(), "Child", nil, &parentID, nil, nil)

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
		var products ProductCounter
		svc := New(repo, products)

		result, err := svc.Create(t.Context(), "Child", nil, &parentID, nil, nil)

		require.NoError(t, err)
		assert.Equal(t, &parentID, result.ParentID)
	})
}

func TestService_Update(t *testing.T) {
	t.Parallel()

	t.Run("success partial update", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var products ProductCounter
		svc := New(repo, products)

		id := uuid.New()
		existing := &domain.Category{
			ID:     id,
			Name:   "Electronics",
			Slug:   "electronics",
			Active: true,
		}
		repo.EXPECT().GetByID(mock.Anything, id).Return(existing, nil)
		repo.EXPECT().Update(mock.Anything, mock.MatchedBy(func(c *domain.Category) bool {
			return c.Name == "Gadgets" && c.Slug == "gadgets"
		})).Return(nil)

		newName := "Gadgets"
		result, err := svc.Update(t.Context(), id, &newName, nil, nil, nil, nil)

		require.NoError(t, err)
		result.ID = uuid.Nil
		result.CreatedAt = time.Time{}
		result.UpdatedAt = time.Time{}
		assert.Equal(t, &domain.Category{
			Name:   "Gadgets",
			Slug:   "gadgets",
			Active: true,
		}, result)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var products ProductCounter
		svc := New(repo, products)

		id := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, id).Return(nil, errs.ErrNotFound)

		newName := "Gadgets"
		result, err := svc.Update(t.Context(), id, &newName, nil, nil, nil, nil)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})

	t.Run("update repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var products ProductCounter
		svc := New(repo, products)

		id := uuid.New()
		existing := &domain.Category{
			ID:     id,
			Name:   "Electronics",
			Slug:   "electronics",
			Active: true,
		}
		repo.EXPECT().GetByID(mock.Anything, id).Return(existing, nil)
		repo.EXPECT().Update(mock.Anything, mock.Anything).Return(errs.ErrConflict)

		newName := "Gadgets"
		result, err := svc.Update(t.Context(), id, &newName, nil, nil, nil, nil)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, errs.ErrConflict)
	})

	t.Run("updates all optional fields", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var products ProductCounter
		svc := New(repo, products)

		id := uuid.New()
		existing := &domain.Category{
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
		result, err := svc.Update(t.Context(), id, &newName, &newDesc, nil, &newSort, &newActive)

		require.NoError(t, err)
		result.ID = uuid.Nil
		result.CreatedAt = time.Time{}
		result.UpdatedAt = time.Time{}
		assert.Equal(t, &domain.Category{
			Name:        "New",
			Slug:        "new",
			Description: &newDesc,
			SortOrder:   10,
			Active:      false,
		}, result)
	})
}

func TestService_Update_ValidatesParent(t *testing.T) {
	t.Parallel()

	t.Run("rejects a move that the repository reports as circular", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var products ProductCounter
		svc := New(repo, products)

		selfID, parentID := uuid.New(), uuid.New()
		// Update loads the category being moved via GetByID(id) before it ever
		// validates the new parent; validateParent itself never calls GetByID.
		repo.EXPECT().GetByID(mock.Anything, selfID).
			Return(&domain.Category{ID: selfID, Name: "A"}, nil)
		repo.EXPECT().AncestorDepthAndCycle(mock.Anything, parentID, selfID, mock.Anything).
			Return(2, true, nil)

		_, err := svc.Update(t.Context(), selfID, nil, nil, &parentID, nil, nil)

		require.ErrorIs(t, err, errs.ErrBadRequest)
		assert.ErrorContains(t, err, "circular parent reference")
	})

	t.Run("rejects a category set as its own parent", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var products ProductCounter
		svc := New(repo, products)

		selfID := uuid.New()
		// Update calls GetByID(id) first, unconditionally, whatever the check below does.
		repo.EXPECT().GetByID(mock.Anything, selfID).
			Return(&domain.Category{ID: selfID, Name: "A"}, nil)

		_, err := svc.Update(t.Context(), selfID, nil, nil, &selfID, nil, nil)

		require.ErrorIs(t, err, errs.ErrBadRequest)
		assert.ErrorContains(t, err, "cannot be its own parent")
	})

	t.Run("moves a category to a valid new parent", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var products ProductCounter
		svc := New(repo, products)

		selfID, parentID := uuid.New(), uuid.New()
		existing := &domain.Category{ID: selfID, Name: "Child", Slug: "child"}
		repo.EXPECT().GetByID(mock.Anything, selfID).Return(existing, nil)
		repo.EXPECT().AncestorDepthAndCycle(mock.Anything, parentID, selfID, mock.Anything).
			Return(1, false, nil)
		repo.EXPECT().Update(mock.Anything, mock.MatchedBy(func(c *domain.Category) bool {
			return c.ID == selfID && c.ParentID != nil && *c.ParentID == parentID
		})).Return(nil)

		result, err := svc.Update(t.Context(), selfID, nil, nil, &parentID, nil, nil)

		require.NoError(t, err)
		assert.Equal(t, &parentID, result.ParentID)
	})
}

func TestService_Delete(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		counter := NewMockProductCounter(t)
		svc := New(repo, counter)

		id := uuid.New()
		counter.EXPECT().CountPublished(mock.Anything, id).Return(0, nil)
		repo.EXPECT().Delete(mock.Anything, id).Return(nil)

		err := svc.Delete(t.Context(), id)

		assert.NoError(t, err)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		counter := NewMockProductCounter(t)
		svc := New(repo, counter)

		id := uuid.New()
		counter.EXPECT().CountPublished(mock.Anything, id).Return(0, nil)
		repo.EXPECT().Delete(mock.Anything, id).Return(errs.ErrNotFound)

		err := svc.Delete(t.Context(), id)

		assert.ErrorIs(t, err, errs.ErrNotFound)
	})

	t.Run("has published products returns ErrBadRequest", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		counter := NewMockProductCounter(t)
		svc := New(repo, counter)

		id := uuid.New()
		counter.EXPECT().CountPublished(mock.Anything, id).Return(3, nil)

		err := svc.Delete(t.Context(), id)

		assert.ErrorIs(t, err, errs.ErrBadRequest)
	})

	t.Run("count published products error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		counter := NewMockProductCounter(t)
		svc := New(repo, counter)

		id := uuid.New()
		counter.EXPECT().CountPublished(mock.Anything, id).Return(0, errors.New("db error"))

		err := svc.Delete(t.Context(), id)

		require.Error(t, err)
	})

	t.Run("delete repo error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		counter := NewMockProductCounter(t)
		svc := New(repo, counter)

		id := uuid.New()
		counter.EXPECT().CountPublished(mock.Anything, id).Return(0, nil)

		deleteErr := errors.New("database delete failed")
		repo.EXPECT().Delete(mock.Anything, id).Return(deleteErr)

		err := svc.Delete(t.Context(), id)
		assert.ErrorIs(t, err, deleteErr)
	})
}

// TestService_Delete_RefusesCategoryWithPublishedProducts pins the guard on
// its own: the products.category_id foreign key would also stop this delete,
// but this guard exists to give the caller a useful message first, and that
// message is behaviour a constraint violation alone would not produce.
func TestService_Delete_RefusesCategoryWithPublishedProducts(t *testing.T) {
	t.Parallel()

	repo := NewMockRepository(t)
	counter := NewMockProductCounter(t)
	svc := New(repo, counter)

	categoryID := uuid.New()
	counter.EXPECT().CountPublished(mock.Anything, categoryID).Return(3, nil)

	err := svc.Delete(t.Context(), categoryID)
	require.ErrorIs(t, err, errs.ErrBadRequest)
}

func TestService_List(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var products ProductCounter
		svc := New(repo, products)

		expected := []domain.Category{
			{ID: uuid.New(), Name: "Electronics", Slug: "electronics"},
			{ID: uuid.New(), Name: "Books", Slug: "books"},
		}
		repo.EXPECT().List(mock.Anything).Return(expected, nil)

		result, err := svc.List(t.Context())

		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var products ProductCounter
		svc := New(repo, products)

		repo.EXPECT().List(mock.Anything).Return(nil, errors.New("db error"))

		result, err := svc.List(t.Context())
		assert.Nil(t, result)
		require.Error(t, err)
	})
}

func TestService_GetBySlug(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var products ProductCounter
		svc := New(repo, products)

		expected := &domain.Category{
			ID:   uuid.New(),
			Name: "Electronics",
			Slug: "electronics",
		}
		repo.EXPECT().GetBySlug(mock.Anything, "electronics").Return(expected, nil)

		result, err := svc.GetBySlug(t.Context(), "electronics")

		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var products ProductCounter
		svc := New(repo, products)

		repo.EXPECT().GetBySlug(mock.Anything, "nonexistent").Return(nil, errs.ErrNotFound)

		result, err := svc.GetBySlug(t.Context(), "nonexistent")

		assert.Nil(t, result)
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})
}
