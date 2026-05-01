package database_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"

	"github.com/residwi/go-api-project-template/internal/platform/database"
)

type noopDBTX struct{}

func (noopDBTX) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (noopDBTX) Query(context.Context, string, ...any) (pgx.Rows, error) { return nil, nil } //nolint:nilnil // test stub
func (noopDBTX) QueryRow(context.Context, string, ...any) pgx.Row        { return nil }

func TestDBWithoutPool(t *testing.T) {
	t.Run("returns transaction from context", func(t *testing.T) {
		ctx := context.Background()
		fake := noopDBTX{}
		ctx = database.WithTestTx(ctx, fake)

		result := database.DB(ctx, nil)
		assert.Equal(t, fake, result)
	})

	t.Run("returns pool when no transaction in context", func(t *testing.T) {
		ctx := context.Background()
		result := database.DB(ctx, nil)
		assert.Nil(t, result)
	})
}

func TestReadDB(t *testing.T) {
	t.Run("returns transaction from context", func(t *testing.T) {
		ctx := context.Background()
		fake := noopDBTX{}
		ctx = database.WithTestTx(ctx, fake)

		result := database.ReadDB(ctx, nil, nil)
		assert.Equal(t, fake, result)
	})

	t.Run("returns primary after recent write", func(t *testing.T) {
		ctx := context.Background()
		ctx = database.WithRecentWrite(ctx)

		result := database.ReadDB(ctx, nil, nil)
		assert.Nil(t, result)
	})

	t.Run("returns primary when no reader available", func(t *testing.T) {
		ctx := context.Background()

		result := database.ReadDB(ctx, nil, nil)
		assert.Nil(t, result)
	})
}
