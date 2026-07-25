package database_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/platform/database"
)

func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := fmt.Sprintf("postgres://test:test@localhost:%s/testdb?sslmode=disable", testContainerPort)
	pool, err := pgxpool.New(context.Background(), dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

func TestWithTx(t *testing.T) {
	t.Run("commits on success", func(t *testing.T) {
		pool := newTestPool(t)
		err := database.WithTx(context.Background(), pool, func(ctx context.Context) error {
			_, err := database.DB(ctx, pool).Exec(ctx, `CREATE TEMP TABLE IF NOT EXISTS tx_commit_test (id int)`)
			return err
		})
		require.NoError(t, err)
	})

	t.Run("rolls back on error", func(t *testing.T) {
		pool := newTestPool(t)
		sentinel := errors.New("rollback me")
		err := database.WithTx(context.Background(), pool, func(_ context.Context) error {
			return sentinel
		})
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("returns error when pool is closed", func(t *testing.T) {
		pool := newTestPool(t)
		pool.Close()

		err := database.WithTx(context.Background(), pool, func(_ context.Context) error {
			return nil
		})
		assert.Error(t, err)
	})

	t.Run("nested call reuses existing transaction", func(t *testing.T) {
		pool := newTestPool(t)
		outerCalled, innerCalled := false, false

		err := database.WithTx(context.Background(), pool, func(outerCtx context.Context) error {
			outerCalled = true
			return database.WithTx(outerCtx, pool, func(innerCtx context.Context) error {
				innerCalled = true
				assert.Equal(t, database.DB(outerCtx, pool), database.DB(innerCtx, pool))
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
		result := database.ReadDB(ctx, primary, reader)
		assert.Equal(t, reader, result)
	})

	t.Run("returns primary after recent write even with reader available", func(t *testing.T) {
		ctx := database.WithRecentWrite(context.Background())
		result := database.ReadDB(ctx, primary, reader)
		assert.Equal(t, primary, result)
	})
}

func TestDB(t *testing.T) {
	t.Run("returns transaction when one is in context", func(t *testing.T) {
		pool := newTestPool(t)
		err := database.WithTx(context.Background(), pool, func(txCtx context.Context) error {
			assert.NotNil(t, database.DB(txCtx, pool))
			return nil
		})
		require.NoError(t, err)
	})
}

func TestTxRunner_Run(t *testing.T) {
	pool := newTestPool(t)
	runner := database.NewTxRunner(pool)

	t.Run("commits when fn returns nil", func(t *testing.T) {
		ctx := context.Background()
		email := "txrunner-commit-" + uuid.NewString() + "@example.com"

		err := runner.Run(ctx, func(txCtx context.Context) error {
			_, err := database.DB(txCtx, pool).Exec(txCtx,
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
			if _, err := database.DB(txCtx, pool).Exec(txCtx,
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
				_, err := database.DB(innerCtx, pool).Exec(innerCtx,
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
