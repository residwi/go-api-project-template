package testhelper_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/testhelper"
)

// Two sequential calls stand in for two test binaries racing on the same
// database name. The old behaviour dropped and recreated on the second call,
// which is what destroyed a sibling package's rows mid-run.
func TestMustStartPostgresIsRepeatableWithoutDroppingData(t *testing.T) {
	poolA, cleanupA := testhelper.MustStartPostgres("test_helper_share")
	t.Cleanup(cleanupA)

	// ON CONFLICT DO NOTHING: the database this test claims is never dropped once
	// created, so a prior run of this same test left its row behind. Without it,
	// the second `go test` invocation against a warm container fails on the
	// slug's UNIQUE constraint instead of exercising the behaviour under test.
	_, err := poolA.Exec(context.Background(),
		`INSERT INTO categories (name, slug) VALUES ('Survivor', 'survivor') ON CONFLICT (slug) DO NOTHING`)
	require.NoError(t, err)

	poolB, cleanupB := testhelper.MustStartPostgres("test_helper_share")
	t.Cleanup(cleanupB)

	var count int
	require.NoError(t, poolB.QueryRow(context.Background(),
		`SELECT count(*) FROM categories WHERE slug = 'survivor'`).Scan(&count))

	assert.Equal(t, 1, count, "second call must attach to the existing database, not recreate it")
}
