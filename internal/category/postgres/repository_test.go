package postgres_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/category"
	"github.com/residwi/go-api-project-template/internal/category/postgres"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testhelper.MustStartPostgres("test_features_category")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func setup(t *testing.T) {
	t.Helper()
	testhelper.ResetDB(t, testPool)
}

func seedCategory(t *testing.T) *category.Category {
	t.Helper()
	repo := postgres.New(testPool)
	desc := "Test description"
	cat := &category.Category{
		Name:        "Category-" + uuid.New().String()[:8],
		Slug:        "slug-" + uuid.New().String(),
		Description: &desc,
		SortOrder:   0,
		Active:      true,
	}
	require.NoError(t, repo.Create(context.Background(), cat))
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM categories WHERE id = $1`, cat.ID)
	})
	return cat
}

func TestPostgresRepository_Create(t *testing.T) {
	t.Run("creates category", func(t *testing.T) {
		setup(t)
		repo := postgres.New(testPool)
		desc := "A description"
		cat := &category.Category{
			Name:        "New Category",
			Slug:        "new-category-" + uuid.New().String(),
			Description: &desc,
			SortOrder:   1,
			Active:      true,
		}

		err := repo.Create(context.Background(), cat)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, cat.ID)
		assert.False(t, cat.CreatedAt.IsZero())
		t.Cleanup(func() {
			testPool.Exec(context.Background(), `DELETE FROM categories WHERE id = $1`, cat.ID)
		})
	})

	t.Run("returns conflict on duplicate slug", func(t *testing.T) {
		setup(t)
		existing := seedCategory(t)
		repo := postgres.New(testPool)

		dup := &category.Category{
			Name:      "Duplicate",
			Slug:      existing.Slug,
			SortOrder: 0,
			Active:    true,
		}
		err := repo.Create(context.Background(), dup)
		assert.ErrorIs(t, err, apperror.ErrConflict)
	})
}

func TestPostgresRepository_GetByID(t *testing.T) {
	t.Run("returns category", func(t *testing.T) {
		setup(t)
		cat := seedCategory(t)
		repo := postgres.New(testPool)

		got, err := repo.GetByID(context.Background(), cat.ID)
		require.NoError(t, err)
		assert.Equal(t, cat.ID, got.ID)
		assert.Equal(t, cat.Name, got.Name)
		assert.Equal(t, cat.Slug, got.Slug)
	})

	t.Run("returns not found", func(t *testing.T) {
		setup(t)
		repo := postgres.New(testPool)

		_, err := repo.GetByID(context.Background(), uuid.New())
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestPostgresRepository_GetBySlug(t *testing.T) {
	t.Run("returns category by slug", func(t *testing.T) {
		setup(t)
		cat := seedCategory(t)
		repo := postgres.New(testPool)

		got, err := repo.GetBySlug(context.Background(), cat.Slug)
		require.NoError(t, err)
		assert.Equal(t, cat.ID, got.ID)
		assert.Equal(t, cat.Slug, got.Slug)
	})

	t.Run("returns not found", func(t *testing.T) {
		setup(t)
		repo := postgres.New(testPool)

		_, err := repo.GetBySlug(context.Background(), "nonexistent-slug")
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestPostgresRepository_Update(t *testing.T) {
	t.Run("updates category fields", func(t *testing.T) {
		setup(t)
		cat := seedCategory(t)
		repo := postgres.New(testPool)

		cat.Name = "Updated Name"
		cat.Active = false
		err := repo.Update(context.Background(), cat)
		require.NoError(t, err)

		got, err := repo.GetByID(context.Background(), cat.ID)
		require.NoError(t, err)
		assert.Equal(t, "Updated Name", got.Name)
		assert.False(t, got.Active)
	})

	t.Run("returns not found", func(t *testing.T) {
		setup(t)
		repo := postgres.New(testPool)

		cat := &category.Category{
			ID:        uuid.New(),
			Name:      "Ghost",
			Slug:      "ghost-slug-" + uuid.New().String(),
			SortOrder: 0,
			Active:    true,
		}
		err := repo.Update(context.Background(), cat)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("returns conflict on duplicate slug", func(t *testing.T) {
		setup(t)
		cat1 := seedCategory(t)
		cat2 := seedCategory(t)
		repo := postgres.New(testPool)

		cat2.Slug = cat1.Slug
		err := repo.Update(context.Background(), cat2)
		assert.ErrorIs(t, err, apperror.ErrConflict)
	})
}

func TestPostgresRepository_Delete(t *testing.T) {
	t.Run("deletes category", func(t *testing.T) {
		setup(t)
		cat := seedCategory(t)
		repo := postgres.New(testPool)

		err := repo.Delete(context.Background(), cat.ID)
		require.NoError(t, err)

		_, err = repo.GetByID(context.Background(), cat.ID)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("returns not found", func(t *testing.T) {
		setup(t)
		repo := postgres.New(testPool)

		err := repo.Delete(context.Background(), uuid.New())
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestPostgresRepository_List(t *testing.T) {
	t.Run("returns all categories", func(t *testing.T) {
		setup(t)
		seedCategory(t)
		seedCategory(t)
		repo := postgres.New(testPool)

		categories, err := repo.List(context.Background())
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(categories), 2)
	})
}

func TestPostgresRepository_CancelledContext(t *testing.T) {
	repo := postgres.New(testPool)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	t.Run("Create returns error on cancelled context", func(t *testing.T) {
		setup(t)
		cat := &category.Category{
			Name:      "Cancelled",
			Slug:      "cancelled-" + uuid.New().String(),
			SortOrder: 0,
			Active:    true,
		}
		err := repo.Create(ctx, cat)
		require.Error(t, err)
		assert.NotErrorIs(t, err, apperror.ErrConflict)
	})

	t.Run("GetByID returns error on cancelled context", func(t *testing.T) {
		setup(t)
		_, err := repo.GetByID(ctx, uuid.New())
		require.Error(t, err)
		assert.NotErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("GetBySlug returns error on cancelled context", func(t *testing.T) {
		setup(t)
		_, err := repo.GetBySlug(ctx, "some-slug")
		require.Error(t, err)
		assert.NotErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("Update returns error on cancelled context", func(t *testing.T) {
		setup(t)
		cat := &category.Category{
			ID:        uuid.New(),
			Name:      "Test",
			Slug:      "test-" + uuid.New().String(),
			SortOrder: 0,
			Active:    true,
		}
		err := repo.Update(ctx, cat)
		require.Error(t, err)
		require.NotErrorIs(t, err, apperror.ErrNotFound)
		assert.NotErrorIs(t, err, apperror.ErrConflict)
	})

	t.Run("Delete returns error on cancelled context", func(t *testing.T) {
		setup(t)
		err := repo.Delete(ctx, uuid.New())
		require.Error(t, err)
		assert.NotErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("List returns error on cancelled context", func(t *testing.T) {
		setup(t)
		_, err := repo.List(ctx)
		assert.Error(t, err)
	})
}
