package database

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrimaryDB(t *testing.T) {
	t.Run("returns the primary pool when no transaction in context", func(t *testing.T) {
		pool := newTestPool(t)

		assert.Equal(t, DBTX(pool), PrimaryDB(context.Background(), DB{Primary: pool}))
	})

	t.Run("returns the transaction when one is in context", func(t *testing.T) {
		fake := noopDBTX{}

		assert.Equal(t, fake, PrimaryDB(withTx(context.Background(), fake), DB{}))
	})
}

func TestReplicaDB(t *testing.T) {
	t.Run("returns the primary when no replica is configured", func(t *testing.T) {
		pool := newTestPool(t)

		assert.Equal(t, DBTX(pool), ReplicaDB(context.Background(), DB{Primary: pool}))
	})

	t.Run("returns the replica when one is configured", func(t *testing.T) {
		primary := newTestPool(t)
		replica := newTestPool(t)

		assert.Equal(t, DBTX(replica), ReplicaDB(context.Background(), DB{Primary: primary, Replica: replica}))
	})

	t.Run("returns the transaction when one is in context", func(t *testing.T) {
		fake := noopDBTX{}
		primary := newTestPool(t)
		replica := newTestPool(t)

		assert.Equal(t, fake, ReplicaDB(withTx(context.Background(), fake), DB{Primary: primary, Replica: replica}))
	})
}

func TestWithTx(t *testing.T) {
	t.Run("commits on success", func(t *testing.T) {
		pool := newTestPool(t)
		err := WithTx(context.Background(), pool, func(ctx context.Context) error {
			_, err := PrimaryDB(
				ctx,
				DB{Primary: pool},
			).Exec(ctx, `CREATE TEMP TABLE IF NOT EXISTS tx_commit_test (id int)`)
			return err
		})
		require.NoError(t, err)
	})

	t.Run("rolls back on error", func(t *testing.T) {
		pool := newTestPool(t)
		sentinel := errors.New("rollback me")
		err := WithTx(context.Background(), pool, func(_ context.Context) error {
			return sentinel
		})
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("returns error when pool is closed", func(t *testing.T) {
		pool := newTestPool(t)
		pool.Close()

		err := WithTx(context.Background(), pool, func(_ context.Context) error {
			return nil
		})
		assert.Error(t, err)
	})

	t.Run("nested call reuses existing transaction", func(t *testing.T) {
		pool := newTestPool(t)
		outerCalled, innerCalled := false, false

		err := WithTx(context.Background(), pool, func(outerCtx context.Context) error {
			outerCalled = true
			return WithTx(outerCtx, pool, func(innerCtx context.Context) error {
				innerCalled = true
				assert.Equal(t, PrimaryDB(outerCtx, DB{Primary: pool}), PrimaryDB(innerCtx, DB{Primary: pool}))
				return nil
			})
		})
		require.NoError(t, err)
		assert.True(t, outerCalled)
		assert.True(t, innerCalled)
	})
}

func TestTxRunner_Run(t *testing.T) {
	pool := newTestPool(t)
	runner := NewTxRunner(pool)

	t.Run("commits when fn returns nil", func(t *testing.T) {
		ctx := context.Background()
		email := "txrunner-commit-" + uuid.NewString() + "@example.com"

		err := runner.Run(ctx, func(txCtx context.Context) error {
			_, err := PrimaryDB(txCtx, DB{Primary: pool}).Exec(txCtx,
				`INSERT INTO users (email, password_hash, first_name, last_name)
				 VALUES ($1, 'x', 'Tx', 'Runner')`, email)
			return err
		})
		require.NoError(t, err)

		var count int
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM users WHERE email = $1`, email).Scan(&count))
		assert.Equal(t, 1, count)
	})

	t.Run("rolls back when fn returns an error", func(t *testing.T) {
		ctx := context.Background()
		email := "txrunner-rollback-" + uuid.NewString() + "@example.com"
		sentinel := errors.New("boom")

		err := runner.Run(ctx, func(txCtx context.Context) error {
			if _, err := PrimaryDB(txCtx, DB{Primary: pool}).Exec(txCtx,
				`INSERT INTO users (email, password_hash, first_name, last_name)
				 VALUES ($1, 'x', 'Tx', 'Runner')`, email); err != nil {
				return err
			}
			return sentinel
		})
		require.ErrorIs(t, err, sentinel)

		var count int
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM users WHERE email = $1`, email).Scan(&count))
		assert.Equal(t, 0, count, "insert must not survive a returned error")
	})

	t.Run("nested Run reuses the outer transaction", func(t *testing.T) {
		ctx := context.Background()
		email := "txrunner-nested-" + uuid.NewString() + "@example.com"
		sentinel := errors.New("outer fails")

		err := runner.Run(ctx, func(outerCtx context.Context) error {
			if err := runner.Run(outerCtx, func(innerCtx context.Context) error {
				_, err := PrimaryDB(innerCtx, DB{Primary: pool}).Exec(innerCtx,
					`INSERT INTO users (email, password_hash, first_name, last_name)
					 VALUES ($1, 'x', 'Tx', 'Runner')`, email)
				return err
			}); err != nil {
				return err
			}
			return sentinel
		})
		require.ErrorIs(t, err, sentinel)

		var count int
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM users WHERE email = $1`, email).Scan(&count))
		assert.Equal(t, 0, count,
			"inner Run must join the outer tx, so the outer rollback discards it")
	})
}

type noopDBTX struct{}

func (noopDBTX) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (noopDBTX) Query(
	context.Context,
	string,
	...any,
) (pgx.Rows, error) {
	return nil, nil //nolint:nilnil // test stub
}
func (noopDBTX) QueryRow(context.Context, string, ...any) pgx.Row { return nil }

func withTx(ctx context.Context, dbtx DBTX) context.Context {
	return context.WithValue(ctx, txCtxKey{}, dbtx)
}

func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := fmt.Sprintf("postgres://test:test@localhost:%s/testdb?sslmode=disable", testContainerPort)
	pool, err := pgxpool.New(context.Background(), dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}
