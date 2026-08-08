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

func TestPostgresRepository_GetBySlug(t *testing.T) {
	t.Run("returns category by slug", func(t *testing.T) {
		cat := seedCategory(t)
		repo := New(testPool)

		got, err := repo.GetBySlug(context.Background(), cat.Slug)
		require.NoError(t, err)
		assert.Equal(t, cat.ID, got.ID)
		assert.Equal(t, cat.Slug, got.Slug)
	})

	t.Run("returns not found", func(t *testing.T) {
		repo := New(testPool)

		_, err := repo.GetBySlug(context.Background(), "nonexistent-slug")
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestPostgresRepository_List(t *testing.T) {
	t.Run("returns all categories", func(t *testing.T) {
		seedCategory(t)
		seedCategory(t)
		repo := New(testPool)

		categories, err := repo.List(context.Background())
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(categories), 2)
	})
}

func TestPostgresRepository_CancelledContext(t *testing.T) {
	repo := New(testPool)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	t.Run("GetBySlug returns error on cancelled context", func(t *testing.T) {
		_, err := repo.GetBySlug(ctx, "some-slug")
		require.Error(t, err)
		assert.NotErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("List returns error on cancelled context", func(t *testing.T) {
		_, err := repo.List(ctx)
		assert.Error(t, err)
	})
}

func seedCategory(t *testing.T) *domain.Category {
	t.Helper()
	id := uuid.New()
	desc := "Test description"
	cat := &domain.Category{
		ID:          id,
		Name:        "Category-" + id.String()[:8],
		Slug:        "slug-" + id.String(),
		Description: &desc,
		SortOrder:   0,
		Active:      true,
	}
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO categories (id, name, slug, description, sort_order, active)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		cat.ID, cat.Name, cat.Slug, cat.Description, cat.SortOrder, cat.Active,
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM categories WHERE id = $1`, cat.ID)
	})
	return cat
}
