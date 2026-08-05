package database

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DBTX abstracts pgxpool.Pool and pgx.Tx so repositories work with both.
type DBTX interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type txCtxKey struct{}

// WithTx runs fn inside a transaction repositories pick up via DB(ctx, pool). A
// transaction already in context is reused, so a nested WithTx cannot silently
// open a second connection and break atomicity.
//
// Never pass the tx-carrying context to a goroutine: pgx.Tx is not
// goroutine-safe and concurrent use corrupts it. Stay on fn's synchronous
// call chain.
func WithTx(ctx context.Context, pool *pgxpool.Pool, fn func(ctx context.Context) error) error {
	if _, ok := ctx.Value(txCtxKey{}).(DBTX); ok {
		return fn(ctx)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is no-op

	txCtx := context.WithValue(ctx, txCtxKey{}, tx)
	if err := fn(txCtx); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func DB(ctx context.Context, pool *pgxpool.Pool) DBTX {
	if tx, ok := ctx.Value(txCtxKey{}).(DBTX); ok {
		return tx
	}
	return pool
}

type recentWriteCtxKey struct{}

// ReadDB prefers the reader pool, but a transaction or a recent write in this
// request routes to the primary -- sticky read-after-write.
func ReadDB(ctx context.Context, primary *pgxpool.Pool, reader *pgxpool.Pool) DBTX {
	if tx, ok := ctx.Value(txCtxKey{}).(DBTX); ok {
		return tx
	}
	if _, ok := ctx.Value(recentWriteCtxKey{}).(bool); ok {
		return primary
	}
	if reader != nil {
		return reader
	}
	return primary
}

func WithRecentWrite(ctx context.Context) context.Context {
	return context.WithValue(ctx, recentWriteCtxKey{}, true)
}

// TxRunner is what services depend on instead of *pgxpool.Pool: they need
// atomicity, not a database handle, and the narrower type makes an accidental
// s.pool.Query() in the service layer a compile error.
type TxRunner interface {
	Run(ctx context.Context, fn func(ctx context.Context) error) error
}

type poolTxRunner struct{ pool *pgxpool.Pool }

func NewTxRunner(pool *pgxpool.Pool) TxRunner { return &poolTxRunner{pool: pool} }

func (r *poolTxRunner) Run(ctx context.Context, fn func(ctx context.Context) error) error {
	return WithTx(ctx, r.pool, fn)
}
