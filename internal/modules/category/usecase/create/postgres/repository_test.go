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
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testhelper.MustStartPostgres("test_category")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func TestPostgresRepository_Create(t *testing.T) {
	t.Run("creates category", func(t *testing.T) {
		repo := New(testPool)
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
		repo := New(testPool)

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

func TestPostgresRepository_CancelledContext(t *testing.T) {
	repo := New(testPool)
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
}

func TestPostgresRepository_AncestorDepthAndCycle(t *testing.T) {
	ctx := context.Background()

	t.Run("reports depth one for a root parent", func(t *testing.T) {
		repo := New(testPool)
		root := seedCategory(t, nil)

		depth, formsCycle, err := repo.AncestorDepthAndCycle(ctx, root.ID, uuid.New(), 5)

		require.NoError(t, err)
		assert.Equal(t, 1, depth)
		assert.False(t, formsCycle)
	})

	t.Run("counts every ancestor in a five-deep chain", func(t *testing.T) {
		repo := New(testPool)
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
		repo := New(testPool)
		catA := seedCategory(t, nil)
		catB := seedCategory(t, &catA.ID)
		catC := seedCategory(t, &catB.ID)

		// Re-parenting A under C would close the loop A -> B -> C -> A.
		_, formsCycle, err := repo.AncestorDepthAndCycle(ctx, catC.ID, catA.ID, 10)

		require.NoError(t, err)
		assert.True(t, formsCycle)
	})
}

func seedCategory(t *testing.T, parentID *uuid.UUID) *domain.Category {
	t.Helper()
	repo := New(testPool)
	cat := &domain.Category{
		Name:      "Cat-" + uuid.New().String()[:8],
		Slug:      "slug-" + uuid.New().String(),
		ParentID:  parentID,
		SortOrder: 0,
		Active:    true,
	}
	require.NoError(t, repo.Create(context.Background(), cat))
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM categories WHERE id = $1`, cat.ID) })
	return cat
}
