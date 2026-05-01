package category_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/features/category"
	mocks "github.com/residwi/go-api-project-template/mocks/category"
)

func TestService_Create(t *testing.T) {
	t.Run("success without parent", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := category.NewService(repo, nil)

		repo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(c *category.Category) bool {
			return c.Name == "Electronics" && c.Slug == "electronics" && c.Active
		})).Run(func(_ context.Context, c *category.Category) {
			c.ID = uuid.New()
			c.CreatedAt = time.Now()
			c.UpdatedAt = time.Now()
		}).Return(nil)

		result, err := svc.Create(context.Background(), category.CreateCategoryRequest{
			Name: "Electronics",
		})

		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, result.ID)
		result.ID = uuid.Nil
		result.CreatedAt = time.Time{}
		result.UpdatedAt = time.Time{}
		assert.Equal(t, &category.Category{
			Name:   "Electronics",
			Slug:   "electronics",
			Active: true,
		}, result)
	})

	t.Run("repo error", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := category.NewService(repo, nil)

		repo.EXPECT().Create(mock.Anything, mock.Anything).Return(core.ErrConflict)

		result, err := svc.Create(context.Background(), category.CreateCategoryRequest{
			Name: "Electronics",
		})

		assert.Nil(t, result)
		assert.ErrorIs(t, err, core.ErrConflict)
	})

	t.Run("sets sort order and active from request", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := category.NewService(repo, nil)

		sortOrder := 5
		active := false
		repo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(c *category.Category) bool {
			return c.Name == "Books" && c.SortOrder == 5 && !c.Active
		})).Run(func(_ context.Context, c *category.Category) {
			c.ID = uuid.New()
			c.CreatedAt = time.Now()
			c.UpdatedAt = time.Now()
		}).Return(nil)

		result, err := svc.Create(context.Background(), category.CreateCategoryRequest{
			Name:      "Books",
			SortOrder: &sortOrder,
			Active:    &active,
		})

		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, result.ID)
		result.ID = uuid.Nil
		result.CreatedAt = time.Time{}
		result.UpdatedAt = time.Time{}
		assert.Equal(t, &category.Category{
			Name:      "Books",
			Slug:      "books",
			SortOrder: 5,
			Active:    false,
		}, result)
	})
}

func TestService_GetBySlug(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := category.NewService(repo, nil)

		expected := &category.Category{
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
		repo := mocks.NewMockRepository(t)
		svc := category.NewService(repo, nil)

		repo.EXPECT().GetBySlug(mock.Anything, "nonexistent").Return(nil, core.ErrNotFound)

		result, err := svc.GetBySlug(context.Background(), "nonexistent")

		assert.Nil(t, result)
		assert.ErrorIs(t, err, core.ErrNotFound)
	})
}

func TestService_List(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := category.NewService(repo, nil)

		expected := []category.Category{
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
		repo := mocks.NewMockRepository(t)
		svc := category.NewService(repo, nil)

		repo.EXPECT().List(mock.Anything).Return(nil, errors.New("db error"))

		result, err := svc.List(context.Background())
		assert.Nil(t, result)
		require.Error(t, err)
	})
}

func TestService_GetByID(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := category.NewService(repo, nil)

		id := uuid.New()
		expected := &category.Category{ID: id, Name: "Electronics", Slug: "electronics"}
		repo.EXPECT().GetByID(mock.Anything, id).Return(expected, nil)

		result, err := svc.GetByID(context.Background(), id)

		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("not found", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := category.NewService(repo, nil)

		id := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, id).Return(nil, core.ErrNotFound)

		result, err := svc.GetByID(context.Background(), id)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, core.ErrNotFound)
	})
}

func TestService_Update(t *testing.T) {
	t.Run("success partial update", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := category.NewService(repo, nil)

		id := uuid.New()
		existing := &category.Category{
			ID:     id,
			Name:   "Electronics",
			Slug:   "electronics",
			Active: true,
		}
		repo.EXPECT().GetByID(mock.Anything, id).Return(existing, nil)
		repo.EXPECT().Update(mock.Anything, mock.MatchedBy(func(c *category.Category) bool {
			return c.Name == "Gadgets" && c.Slug == "gadgets"
		})).Return(nil)

		newName := "Gadgets"
		result, err := svc.Update(context.Background(), id, category.UpdateCategoryRequest{
			Name: &newName,
		})

		require.NoError(t, err)
		result.ID = uuid.Nil
		result.CreatedAt = time.Time{}
		result.UpdatedAt = time.Time{}
		assert.Equal(t, &category.Category{
			Name:   "Gadgets",
			Slug:   "gadgets",
			Active: true,
		}, result)
	})

	t.Run("not found", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := category.NewService(repo, nil)

		id := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, id).Return(nil, core.ErrNotFound)

		newName := "Gadgets"
		result, err := svc.Update(context.Background(), id, category.UpdateCategoryRequest{
			Name: &newName,
		})

		assert.Nil(t, result)
		assert.ErrorIs(t, err, core.ErrNotFound)
	})

	t.Run("update repo error", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := category.NewService(repo, nil)

		id := uuid.New()
		existing := &category.Category{
			ID:     id,
			Name:   "Electronics",
			Slug:   "electronics",
			Active: true,
		}
		repo.EXPECT().GetByID(mock.Anything, id).Return(existing, nil)
		repo.EXPECT().Update(mock.Anything, mock.Anything).Return(core.ErrConflict)

		newName := "Gadgets"
		result, err := svc.Update(context.Background(), id, category.UpdateCategoryRequest{
			Name: &newName,
		})

		assert.Nil(t, result)
		assert.ErrorIs(t, err, core.ErrConflict)
	})

	t.Run("updates all optional fields", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := category.NewService(repo, nil)

		id := uuid.New()
		existing := &category.Category{
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
		result, err := svc.Update(context.Background(), id, category.UpdateCategoryRequest{
			Name:        &newName,
			Description: &newDesc,
			SortOrder:   &newSort,
			Active:      &newActive,
		})

		require.NoError(t, err)
		result.ID = uuid.Nil
		result.CreatedAt = time.Time{}
		result.UpdatedAt = time.Time{}
		assert.Equal(t, &category.Category{
			Name:        "New",
			Slug:        "new",
			Description: &newDesc,
			SortOrder:   10,
			Active:      false,
		}, result)
	})
}

func TestService_Delete(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := category.NewService(repo, nil)

		id := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, id).Return(&category.Category{ID: id}, nil)
		repo.EXPECT().CountPublishedProducts(mock.Anything, id).Return(0, nil)
		repo.EXPECT().Delete(mock.Anything, id).Return(nil)

		err := svc.Delete(context.Background(), id)

		assert.NoError(t, err)
	})

	t.Run("not found", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := category.NewService(repo, nil)

		id := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, id).Return(nil, core.ErrNotFound)

		err := svc.Delete(context.Background(), id)

		assert.ErrorIs(t, err, core.ErrNotFound)
	})

	t.Run("has published products returns ErrBadRequest", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := category.NewService(repo, nil)

		id := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, id).Return(&category.Category{ID: id}, nil)
		repo.EXPECT().CountPublishedProducts(mock.Anything, id).Return(3, nil)

		err := svc.Delete(context.Background(), id)

		assert.ErrorIs(t, err, core.ErrBadRequest)
	})

	t.Run("count published products error", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := category.NewService(repo, nil)

		id := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, id).Return(&category.Category{ID: id}, nil)
		repo.EXPECT().CountPublishedProducts(mock.Anything, id).Return(0, errors.New("db error"))

		err := svc.Delete(context.Background(), id)

		require.Error(t, err)
	})

	t.Run("delete repo error propagates", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := category.NewService(repo, nil)

		id := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, id).Return(&category.Category{ID: id}, nil)
		repo.EXPECT().CountPublishedProducts(mock.Anything, id).Return(0, nil)

		deleteErr := errors.New("database delete failed")
		repo.EXPECT().Delete(mock.Anything, id).Return(deleteErr)

		err := svc.Delete(context.Background(), id)
		assert.ErrorIs(t, err, deleteErr)
	})
}
