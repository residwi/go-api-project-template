package testhelper_test

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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

// TestMustStartPostgresConcurrentCreateWaitsForMigration is the one test in
// this file that owns and drops its own database, unlike every other caller
// in this suite, which shares a database and never drops it. It has to: the
// invariant under test only engages on the create path, so this test needs
// its name to genuinely not exist yet on every run, which the
// "shared, never dropped" contract can't provide. Drop defensively before and
// after, exactly the WITH (FORCE) pattern the rest of the suite retired --
// safe here specifically because this name has exactly one owner.
//
// Two goroutines then race MustStartPostgres on that name. Each checks its OWN
// pool immediately on return, inside its own goroutine, before any
// [sync.WaitGroup.Wait] -- joining first would hide the bug, since the
// creator's own call does not return until its own migration is done in both
// the broken and fixed code, so waiting for both goroutines to finish always
// waits for the slow migration too, regardless of what the *other*
// goroutine's early return handed back.
func TestMustStartPostgresConcurrentCreateWaitsForMigration(t *testing.T) {
	const name = "test_helper_race"

	probe, probeCleanup := testhelper.MustStartPostgres(name)
	dsn := adminDSN(probe)
	probeCleanup()
	dropDatabase(t, dsn, name)
	t.Cleanup(func() { dropDatabase(t, dsn, name) })

	errs := make([]error, 2)
	cleanups := make([]func(), 2)
	var wg sync.WaitGroup
	wg.Add(2)
	for i := range 2 {
		go func(i int) {
			defer wg.Done()
			pool, cleanup := testhelper.MustStartPostgres(name)
			cleanups[i] = cleanup
			var count int
			errs[i] = pool.QueryRow(context.Background(), `SELECT count(*) FROM categories`).Scan(&count)
		}(i)
	}
	wg.Wait()
	for _, c := range cleanups {
		c()
	}

	for i, err := range errs {
		assert.NoError(t, err, "goroutine %d got a pool with an unmigrated schema", i)
	}
}

// adminDSN reconstructs the maintenance-database DSN from an already-connected
// pool's own config, so this file does not need to know the container's port
// or duplicate testhelper's internal connection details.
func adminDSN(pool *pgxpool.Pool) string {
	cfg := pool.Config().ConnConfig
	hostPort := net.JoinHostPort(cfg.Host, strconv.Itoa(int(cfg.Port)))
	return fmt.Sprintf("postgres://%s:%s@%s/postgres?sslmode=disable", cfg.User, cfg.Password, hostPort)
}

// dropDatabase removes name outright. Only
// TestMustStartPostgresConcurrentCreateWaitsForMigration may call this --
// everywhere else in the suite, seed the rows a subtest asserts on and never
// drop or truncate.
func dropDatabase(t *testing.T, dsn, name string) {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), dsn)
	require.NoError(t, err)
	defer func() { _ = conn.Close(context.Background()) }()
	_, err = conn.Exec(context.Background(), `DROP DATABASE IF EXISTS "`+name+`" WITH (FORCE)`)
	require.NoError(t, err)
}
