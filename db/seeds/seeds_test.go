// Package seeds proves db/seeds/data.sql still applies against a freshly
// migrated database. Nothing else in the suite touches that file, so a dropped
// or renamed column can break it without any test noticing.
package seeds

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/testhelper"
)

func TestDataSQL_Applies(t *testing.T) {
	pool, cleanup := testhelper.MustStartPostgres("test_db_seeds")
	defer cleanup()
	ctx := context.Background()

	seedSQL, err := os.ReadFile(seedFilePath())
	require.NoError(t, err)

	_, err = pool.Exec(ctx, string(seedSQL))
	require.NoError(t, err, "db/seeds/data.sql must apply cleanly against a freshly migrated database")

	// A seeded product must resolve to a real inventory_levels row with the stock
	// the file claims, not merely produce no SQL error.
	var availableStock, reservedStock int
	err = pool.QueryRow(ctx, `
		SELECT il.available_stock, il.reserved_stock
		FROM products p
		JOIN inventory_levels il ON il.product_id = p.id
		WHERE p.slug = 'wireless-headphones'
	`).Scan(&availableStock, &reservedStock)
	require.NoError(t, err, "seeded product must have a matching inventory_levels row")
	assert.Equal(t, 100, availableStock)
	assert.Equal(t, 0, reservedStock)

	// Re-applying must stay a no-op, as running `make seed` twice would.
	_, err = pool.Exec(ctx, string(seedSQL))
	require.NoError(t, err, "re-applying db/seeds/data.sql must also succeed")
}

// Resolved relative to this source file, as testhelper locates the migrations
// directory.
func seedFilePath() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "data.sql")
}
