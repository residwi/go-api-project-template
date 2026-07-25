package database

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
)

// noopDBTX is a stand-in DBTX for asserting that DB and ReadDB pull a
// transaction out of the context. It lives in an internal test file so the
// context key can be set directly, removing the need for an exported
// test-only helper in production code.
type noopDBTX struct{}

func (noopDBTX) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (noopDBTX) Query(context.Context, string, ...any) (pgx.Rows, error) { return nil, nil } //nolint:nilnil // test stub
func (noopDBTX) QueryRow(context.Context, string, ...any) pgx.Row        { return nil }

func withTx(ctx context.Context, dbtx DBTX) context.Context {
	return context.WithValue(ctx, txCtxKey{}, dbtx)
}

func TestDB_ReturnsTransactionFromContext(t *testing.T) {
	fake := noopDBTX{}
	assert.Equal(t, fake, DB(withTx(context.Background(), fake), nil))
}

func TestReadDB_ReturnsTransactionFromContext(t *testing.T) {
	fake := noopDBTX{}
	assert.Equal(t, fake, ReadDB(withTx(context.Background(), fake), nil, nil))
}
