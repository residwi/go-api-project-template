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

func TestPostgresRepository_Create(t *testing.T) {
	t.Run("creates product", func(t *testing.T) {
		repo := New(testPool)
		desc := "A description"
		p := &domain.Product{
			Name:        "New Product",
			Slug:        "new-product-" + uuid.New().String(),
			Description: &desc,
			Price:       money.New(1000, "USD"),
			Status:      domain.StatusPublished,
		}

		err := repo.Create(context.Background(), p)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, p.ID)
		assert.False(t, p.CreatedAt.IsZero())
		t.Cleanup(func() {
			testPool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, p.ID)
		})
	})

	// create writes price and compare_at_price as separate params under the
	// price's own currency (never independently), so this checks the row landed
	// with the right amounts and currency, not the read-side scanner -- that nil
	// branch is update's, exercised in update/postgres against its own GetByID.
	t.Run("round-trips both amounts under the row's single currency", func(t *testing.T) {
		repo := New(testPool)
		compareAt := money.New(2999, "EUR")
		p := &domain.Product{
			Name:           "Denominated Product",
			Slug:           "denominated-" + uuid.New().String(),
			Price:          money.New(1999, "EUR"),
			CompareAtPrice: &compareAt,
			Status:         domain.StatusPublished,
		}
		require.NoError(t, repo.Create(context.Background(), p))
		t.Cleanup(func() {
			testPool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, p.ID)
		})

		var gotPrice, gotCompareAt int64
		var gotCurrency string
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT price, compare_at_price, currency FROM products WHERE id = $1`, p.ID,
		).Scan(&gotPrice, &gotCompareAt, &gotCurrency))
		assert.Equal(t, int64(1999), gotPrice)
		assert.Equal(t, int64(2999), gotCompareAt)
		assert.Equal(t, "EUR", gotCurrency)
	})

	t.Run("returns conflict on duplicate slug", func(t *testing.T) {
		existing := seedProduct(t)
		repo := New(testPool)

		dup := &domain.Product{
			Name:   "Duplicate",
			Slug:   existing.Slug,
			Price:  money.New(500, "USD"),
			Status: domain.StatusDraft,
		}
		err := repo.Create(context.Background(), dup)
		assert.ErrorIs(t, err, apperror.ErrConflict)
	})
}

func TestPostgresRepository_CancelledContext(t *testing.T) {
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	repo := New(testPool)

	t.Run("Create", func(t *testing.T) {
		p := &domain.Product{
			Name:   "X",
			Slug:   "x-" + uuid.New().String(),
			Price:  money.New(100, "USD"),
			Status: domain.StatusDraft,
		}
		err := repo.Create(cancelledCtx, p)
		assert.Error(t, err)
	})
}

// seedProduct inserts a product with a fresh id and cleans it up on test
// completion -- this package never truncates a table it shares with every
// other product slice.
func seedProduct(t *testing.T) *domain.Product {
	t.Helper()
	id := uuid.New()
	slug := "slug-" + id.String()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO products (id, name, slug, description, price, currency)
		 VALUES ($1, 'Product', $2, 'desc', 1000, 'USD')`,
		id, slug,
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, id)
	})
	return &domain.Product{ID: id, Slug: slug}
}
