package jobqueue

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/testutil"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testutil.MustStartPostgres("test_platform_jobs_insert")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func TestInsertJoinsAnOpenTransaction(t *testing.T) {
	testutil.ResetDB(t, testPool)
	ctx := context.Background()
	db := database.DB{Primary: testPool}

	client, err := NewInsertClient(db)
	require.NoError(t, err)

	runner := database.NewTxRunner(testPool)
	err = runner.Run(ctx, func(txCtx context.Context) error {
		if insertErr := Insert(txCtx, client, db, probeArgs{Value: "rolled-back"}, nil); insertErr != nil {
			return insertErr
		}
		return errors.New("force rollback")
	})
	require.Error(t, err)

	var count int
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT count(*) FROM river_job WHERE kind = 'probe'`).Scan(&count))
	assert.Equal(t, 0, count, "an insert inside a rolled-back transaction must not survive")
}

func TestInsertOutsideATransactionCommits(t *testing.T) {
	testutil.ResetDB(t, testPool)
	ctx := context.Background()
	db := database.DB{Primary: testPool}

	client, err := NewInsertClient(db)
	require.NoError(t, err)

	require.NoError(t, Insert(ctx, client, db, probeArgs{Value: "kept"}, nil))

	var count int
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT count(*) FROM river_job WHERE kind = 'probe'`).Scan(&count))
	assert.Equal(t, 1, count)
}

type probeArgs struct {
	Value string `json:"value"`
}

func (probeArgs) Kind() string { return "probe" }
