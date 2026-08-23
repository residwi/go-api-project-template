// Package migrations drives goose's Down against a real database. Nothing else
// in the suite rolls a migration back, so a Down script can be wrong from the
// day it is written and nothing notices until someone needs it.
package migrations

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/testutil"
)

// An Up/Down/Up round trip on a scratch database, driven the way `goose down`
// would be in an emergency: the Down must reconstruct the dropped columns for a
// product with an inventory_levels row and one without.
func TestDropProductsStockColumns_DownRoundTrip(t *testing.T) {
	pool, cleanup := testutil.MustStartPostgres("test_db_migrations_stock_columns")
	defer cleanup()
	ctx := context.Background()

	// withLevel has a real inventory_levels row with distinct available/reserved
	// counts, so the reconstructed total can be checked precisely.
	withLevel := uuid.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO products (id, name, slug, description, price, currency, status)
		 VALUES ($1, 'Has Level', $2, 'desc', 1000, 'USD', 'published')`,
		withLevel, "has-level-"+withLevel.String())
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		`INSERT INTO inventory_levels (product_id, available_stock, reserved_stock)
		 VALUES ($1, 7, 3)`, withLevel)
	require.NoError(t, err)

	// levelless has no inventory_levels row at all -- e.g. Create committed but
	// EnsureLevel never ran. This is the case the Down migration must handle
	// explicitly rather than via whatever the ADD COLUMN default happens to be.
	levelless := uuid.New()
	_, err = pool.Exec(ctx,
		`INSERT INTO products (id, name, slug, description, price, currency, status)
		 VALUES ($1, 'Levelless', $2, 'desc', 1000, 'USD', 'draft')`,
		levelless, "levelless-"+levelless.String())
	require.NoError(t, err)

	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()
	require.NoError(t, goose.SetDialect("postgres"))

	// Rolls back only 20260424120018, the migration under test.
	require.NoError(t, goose.DownContext(ctx, db, migrationsDir()))

	var stock, reserved int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT stock_quantity, reserved_quantity FROM products WHERE id = $1`, withLevel,
	).Scan(&stock, &reserved))
	assert.Equal(t, 10, stock, "stock_quantity must be available_stock + reserved_stock")
	assert.Equal(t, 3, reserved)

	require.NoError(t, pool.QueryRow(ctx,
		`SELECT stock_quantity, reserved_quantity FROM products WHERE id = $1`, levelless,
	).Scan(&stock, &reserved))
	assert.Equal(
		t,
		0,
		stock,
		"a product with no inventory_levels row reconstructs as zero stock, not left at whatever ADD COLUMN's default happened to be",
	)
	assert.Equal(t, 0, reserved)

	// Re-applying is the other half of the round trip a real rollback needs.
	require.NoError(t, goose.UpContext(ctx, db, migrationsDir()))

	var columnCount int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM information_schema.columns
		 WHERE table_name = 'products' AND column_name IN ('stock_quantity', 'reserved_quantity')`,
	).Scan(&columnCount))
	assert.Equal(t, 0, columnCount, "re-applying Up must drop the columns again")
}

func migrationsDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Dir(file)
}
