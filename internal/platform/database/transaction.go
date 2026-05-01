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

// WithTx begins a transaction, stores it in context, and executes fn.
// All repositories that call DB(ctx, pool) will use the tx automatically.
// If a transaction already exists in context, fn runs inside it (reuse) —
// this prevents nested WithTx from silently opening a second connection
// and breaking atomicity.
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
