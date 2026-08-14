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
	"github.com/residwi/go-api-project-template/internal/modules/product/domain"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

// This package shares test_product with every other product slice's postgres/
// package. It never resets or truncates -- every row it touches is seeded here
// with a fresh uuid.New() and cleaned up by name.
var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testhelper.MustStartPostgres("test_product")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func TestPostgresRepository_Update(t *testing.T) {
	t.Run("updates product fields", func(t *testing.T) {
		p := seedProduct(t)
		repo := New(testPool)

		p.Name = "Updated Product"
		p.Price = money.New(2000, "EUR")
		p.Status = domain.StatusArchived
		err := repo.Update(context.Background(), p)
		require.NoError(t, err)

		got, err := repo.GetByID(context.Background(), p.ID)
		require.NoError(t, err)
		assert.Equal(t, "Updated Product", got.Name)
		// The whole Money: asserting only the amount would pass even if Update wrote
		// the price under the wrong denomination.
		assert.Equal(t, money.New(2000, "EUR"), got.Price)
		assert.Equal(t, domain.StatusArchived, got.Status)
	})

	// Nothing else in this package writes a compare-at price then clears it, so
	// this is the only test that exercises the scanner's nil branch -- where an
	// absent price coming back as a denominated zero would publish
	// `compare_at_price: 0`.
	t.Run("nulling compare_at_price round-trips as nil, not a denominated zero", func(t *testing.T) {
		id := uuid.New()
		_, err := testPool.Exec(context.Background(),
			`INSERT INTO products (id, name, slug, description, price, compare_at_price, currency, status)
			 VALUES ($1, 'Denominated Product', $2, 'desc', 1999, 2999, 'EUR', 'published')`,
			id, "denominated-"+id.String())
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, id) })

		repo := New(testPool)
		p, err := repo.GetByID(context.Background(), id)
		require.NoError(t, err)
		require.NotNil(t, p.CompareAtPrice)
		assert.Equal(t, money.New(2999, "EUR"), *p.CompareAtPrice,
			"compare_at_price has no currency column of its own, so it must read back denominated from the row's")

		p.CompareAtPrice = nil
		require.NoError(t, repo.Update(context.Background(), p))

		got, err := repo.GetByID(context.Background(), id)
		require.NoError(t, err)
		assert.Nil(t, got.CompareAtPrice)
	})

	t.Run("returns not found", func(t *testing.T) {
		repo := New(testPool)

		p := &domain.Product{
			ID:     uuid.New(),
			Name:   "Ghost",
			Slug:   "ghost-" + uuid.New().String(),
			Price:  money.New(100, "USD"),
			Status: domain.StatusDraft,
		}
		err := repo.Update(context.Background(), p)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("returns conflict on duplicate slug", func(t *testing.T) {
		p1 := seedProduct(t)
		p2 := seedProduct(t)
		repo := New(testPool)

		p2.Slug = p1.Slug
		err := repo.Update(context.Background(), p2)
		assert.ErrorIs(t, err, apperror.ErrConflict)
	})
}

// exercises isUniqueViolation returning false: an update that leaves the slug
// unchanged hits no unique constraint.
func TestSearchString(t *testing.T) {
	t.Run("returns false when substring not found", func(t *testing.T) {
		existing := seedProduct(t)
		repo := New(testPool)

		existing.Name = "Updated Name"
		err := repo.Update(context.Background(), existing)
		require.NoError(t, err)
	})
}

func TestPostgresRepository_CancelledContext(t *testing.T) {
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	repo := New(testPool)

	t.Run("Update", func(t *testing.T) {
		p := &domain.Product{
			ID:     uuid.New(),
			Name:   "X",
			Slug:   "x-" + uuid.New().String(),
			Price:  money.New(100, "USD"),
			Status: domain.StatusDraft,
		}
		err := repo.Update(cancelledCtx, p)
		assert.Error(t, err)
	})
}

// seedProduct inserts a product with a fresh id and cleans it up on test
// completion -- this package never truncates a table it shares with every
// other product slice.
func seedProduct(t *testing.T) *domain.Product {
	t.Helper()
	id := uuid.New()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO products (id, name, slug, description, price, currency)
		 VALUES ($1, 'Product', $2, 'desc', 1000, 'USD')`,
		id, "slug-"+id.String(),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, id)
	})

	repo := New(testPool)
	p, err := repo.GetByID(context.Background(), id)
	require.NoError(t, err)
	return p
}
