package database_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

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
