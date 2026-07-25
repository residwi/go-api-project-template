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

// WithTx runs fn inside a transaction that repositories pick up automatically via
// DB(ctx, pool). A transaction already in context is reused, so nested WithTx
// cannot silently open a second connection and break atomicity.
//
// ⚠️ GOROUTINE SAFETY: Never pass a tx-carrying context to a goroutine.
// pgx.Tx is NOT goroutine-safe — concurrent use corrupts state. The worker
// processes jobs on fresh contexts (no tx). Only use tx-context within the
// synchronous call chain of fn.
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

// DB returns the transaction from context if present, otherwise the pool.
// Every repository should use this instead of accessing pool directly.
func DB(ctx context.Context, pool *pgxpool.Pool) DBTX {
	if tx, ok := ctx.Value(txCtxKey{}).(DBTX); ok {
		return tx
	}
	return pool
}

type recentWriteCtxKey struct{}

// ReadDB returns the reader pool if available, otherwise falls back to the primary pool.
// If inside a transaction, always uses the transaction.
// If a recent write occurred in this request, routes to primary (sticky-read-after-write).
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

// WithRecentWrite marks a context as having performed a write operation.
// Subsequent ReadDB calls in this request will route to primary.
func WithRecentWrite(ctx context.Context) context.Context {
	return context.WithValue(ctx, recentWriteCtxKey{}, true)
}

// WithTestTx returns a context with a DBTX set, causing WithTx to skip pool.Begin.
// Use in unit tests where repositories are mocked and no real DB connection exists.
func WithTestTx(ctx context.Context, dbtx DBTX) context.Context {
	return context.WithValue(ctx, txCtxKey{}, dbtx)
}

// TxRunner runs a function inside a transaction. Services depend on this
// instead of *pgxpool.Pool: a pool is a database handle, but a service only
// ever needs atomicity, and the narrower type makes an accidental
// s.pool.Query() in the service layer a compile error.
type TxRunner interface {
	Run(ctx context.Context, fn func(ctx context.Context) error) error
}

type poolTxRunner struct{ pool *pgxpool.Pool }

// NewTxRunner returns a TxRunner backed by pool. Nested calls reuse the
// transaction already in ctx, matching WithTx.
func NewTxRunner(pool *pgxpool.Pool) TxRunner { return &poolTxRunner{pool: pool} }

func (r *poolTxRunner) Run(ctx context.Context, fn func(ctx context.Context) error) error {
	return WithTx(ctx, r.pool, fn)
}
