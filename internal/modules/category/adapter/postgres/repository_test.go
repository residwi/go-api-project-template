package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/category/domain"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/testutil"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testutil.MustStartPostgres("test_category")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func TestPostgresRepository_Create(t *testing.T) {
	t.Run("creates category", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})
		desc := "A description"
		cat := &domain.Category{
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
		existing := seedCategory(t, nil)
		repo := New(database.DB{Primary: testPool})

		dup := &domain.Category{
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
		cat := seedCategory(t, nil)
		repo := New(database.DB{Primary: testPool})

		got, err := repo.GetByID(context.Background(), cat.ID)
		require.NoError(t, err)
		assert.Equal(t, cat.ID, got.ID)
		assert.Equal(t, cat.Name, got.Name)
		assert.Equal(t, cat.Slug, got.Slug)
	})

	t.Run("returns not found", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})

		_, err := repo.GetByID(context.Background(), uuid.New())
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestPostgresRepository_GetBySlug(t *testing.T) {
	t.Run("returns category by slug", func(t *testing.T) {
		cat := seedCategory(t, nil)
		repo := New(database.DB{Primary: testPool})

		got, err := repo.GetBySlug(context.Background(), cat.Slug)
		require.NoError(t, err)
		assert.Equal(t, cat.ID, got.ID)
		assert.Equal(t, cat.Slug, got.Slug)
	})

	t.Run("returns not found", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})

		_, err := repo.GetBySlug(context.Background(), "nonexistent-slug")
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestPostgresRepository_List(t *testing.T) {
	t.Run("returns all categories", func(t *testing.T) {
		seedCategory(t, nil)
		seedCategory(t, nil)
		repo := New(database.DB{Primary: testPool})

		categories, err := repo.List(context.Background())
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(categories), 2)
	})
}

func TestPostgresRepository_Update(t *testing.T) {
	t.Run("updates category fields", func(t *testing.T) {
		cat := seedCategory(t, nil)
		repo := New(database.DB{Primary: testPool})

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
		repo := New(database.DB{Primary: testPool})

		cat := &domain.Category{
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
		cat1 := seedCategory(t, nil)
		cat2 := seedCategory(t, nil)
		repo := New(database.DB{Primary: testPool})

		cat2.Slug = cat1.Slug
		err := repo.Update(context.Background(), cat2)
		assert.ErrorIs(t, err, apperror.ErrConflict)
	})
}

func TestPostgresRepository_Delete(t *testing.T) {
	t.Run("deletes category", func(t *testing.T) {
		cat := seedCategory(t, nil)
		repo := New(database.DB{Primary: testPool})

		err := repo.Delete(context.Background(), cat.ID)
		require.NoError(t, err)

		var count int
		err = testPool.QueryRow(context.Background(),
			`SELECT COUNT(*) FROM categories WHERE id = $1`, cat.ID).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("returns not found", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})

		err := repo.Delete(context.Background(), uuid.New())
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestPostgresRepository_AncestorDepthAndCycle(t *testing.T) {
	ctx := context.Background()

	t.Run("reports depth one for a root parent", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})
		root := seedCategory(t, nil)

		depth, formsCycle, err := repo.AncestorDepthAndCycle(ctx, root.ID, uuid.New(), 5)

		require.NoError(t, err)
		assert.Equal(t, 1, depth)
		assert.False(t, formsCycle)
	})

	t.Run("counts every ancestor in a five-deep chain", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})
		l1 := seedCategory(t, nil)
		l2 := seedCategory(t, &l1.ID)
		l3 := seedCategory(t, &l2.ID)
		l4 := seedCategory(t, &l3.ID)
		l5 := seedCategory(t, &l4.ID)

		depth, formsCycle, err := repo.AncestorDepthAndCycle(ctx, l5.ID, uuid.New(), 10)

		require.NoError(t, err)
		assert.Equal(t, 5, depth)
		assert.False(t, formsCycle)
	})

	t.Run("flags a cycle when selfID is among the prospective parent's ancestors", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})
		catA := seedCategory(t, nil)
		catB := seedCategory(t, &catA.ID)
		catC := seedCategory(t, &catB.ID)

		// Re-parenting A under C would close the loop A -> B -> C -> A.
		_, formsCycle, err := repo.AncestorDepthAndCycle(ctx, catC.ID, catA.ID, 10)

		require.NoError(t, err)
		assert.True(t, formsCycle)
	})
}

func TestPostgresRepository_CancelledContext(t *testing.T) {
	repo := New(database.DB{Primary: testPool})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	t.Run("Create returns error on cancelled context", func(t *testing.T) {
		cat := &domain.Category{
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
		_, err := repo.GetByID(ctx, uuid.New())
		require.Error(t, err)
		assert.NotErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("GetBySlug returns error on cancelled context", func(t *testing.T) {
		_, err := repo.GetBySlug(ctx, "some-slug")
		require.Error(t, err)
		assert.NotErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("List returns error on cancelled context", func(t *testing.T) {
		_, err := repo.List(ctx)
		assert.Error(t, err)
	})

	t.Run("Update returns error on cancelled context", func(t *testing.T) {
		cat := &domain.Category{
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
		err := repo.Delete(ctx, uuid.New())
		require.Error(t, err)
		assert.NotErrorIs(t, err, apperror.ErrNotFound)
	})
}

func seedCategory(t *testing.T, parentID *uuid.UUID) *domain.Category {
	t.Helper()
	id := uuid.New()
	desc := "Test description"
	cat := &domain.Category{
		ID:          id,
		Name:        "Category-" + id.String()[:8],
		Slug:        "slug-" + id.String(),
		Description: &desc,
		ParentID:    parentID,
		SortOrder:   0,
		Active:      true,
	}
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO categories (id, name, slug, description, parent_id, sort_order, active)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		cat.ID, cat.Name, cat.Slug, cat.Description, cat.ParentID, cat.SortOrder, cat.Active,
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM categories WHERE id = $1`, cat.ID)
	})
	return cat
}
