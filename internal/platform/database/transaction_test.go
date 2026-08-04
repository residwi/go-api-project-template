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

func TestDBWithoutPool(t *testing.T) {
	t.Run("returns pool when no transaction in context", func(t *testing.T) {
		ctx := context.Background()
		result := DB(ctx, nil)
		assert.Nil(t, result)
	})
}

func TestReadDB(t *testing.T) {
	t.Run("returns primary after recent write", func(t *testing.T) {
		ctx := context.Background()
		ctx = WithRecentWrite(ctx)

		result := ReadDB(ctx, nil, nil)
		assert.Nil(t, result)
	})

	t.Run("returns primary when no reader available", func(t *testing.T) {
		ctx := context.Background()

		result := ReadDB(ctx, nil, nil)
		assert.Nil(t, result)
	})
}

func TestDB_ReturnsTransactionFromContext(t *testing.T) {
	fake := noopDBTX{}
	assert.Equal(t, fake, DB(withTx(context.Background(), fake), nil))
}

func TestReadDB_ReturnsTransactionFromContext(t *testing.T) {
	fake := noopDBTX{}
	assert.Equal(t, fake, ReadDB(withTx(context.Background(), fake), nil, nil))
}

func TestWithTx(t *testing.T) {
	t.Run("commits on success", func(t *testing.T) {
		pool := newTestPool(t)
		err := WithTx(context.Background(), pool, func(ctx context.Context) error {
			_, err := DB(ctx, pool).Exec(ctx, `CREATE TEMP TABLE IF NOT EXISTS tx_commit_test (id int)`)
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
				assert.Equal(t, DB(outerCtx, pool), DB(innerCtx, pool))
				return nil
			})
		})
		require.NoError(t, err)
		assert.True(t, outerCalled)
		assert.True(t, innerCalled)
	})
}

func TestReadDB_WithReaderPool(t *testing.T) {
	primary := newTestPool(t)
	reader := newTestPool(t)

	t.Run("returns reader pool when available and no tx or recent write", func(t *testing.T) {
		ctx := context.Background()
		result := ReadDB(ctx, primary, reader)
		assert.Equal(t, reader, result)
	})

	t.Run("returns primary after recent write even with reader available", func(t *testing.T) {
		ctx := WithRecentWrite(context.Background())
		result := ReadDB(ctx, primary, reader)
		assert.Equal(t, primary, result)
	})
}

func TestDB(t *testing.T) {
	t.Run("returns transaction when one is in context", func(t *testing.T) {
		pool := newTestPool(t)
		err := WithTx(context.Background(), pool, func(txCtx context.Context) error {
			assert.NotNil(t, DB(txCtx, pool))
			return nil
		})
		require.NoError(t, err)
	})
}

func TestTxRunner_Run(t *testing.T) {
	pool := newTestPool(t)
	runner := NewTxRunner(pool)

	t.Run("commits when fn returns nil", func(t *testing.T) {
		ctx := context.Background()
		email := "txrunner-commit-" + uuid.NewString() + "@example.com"

		err := runner.Run(ctx, func(txCtx context.Context) error {
			_, err := DB(txCtx, pool).Exec(txCtx,
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
			if _, err := DB(txCtx, pool).Exec(txCtx,
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
				_, err := DB(innerCtx, pool).Exec(innerCtx,
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

// noopDBTX is a stand-in DBTX for asserting that DB and ReadDB pull a
// transaction out of the context, using txCtxKey directly instead of an
// exported test-only helper in production code.
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
