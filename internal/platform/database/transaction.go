package database

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DBTX interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type txCtxKey struct{}

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

type TxRunner interface {
	Run(ctx context.Context, fn func(ctx context.Context) error) error
}

type poolTxRunner struct{ pool *pgxpool.Pool }

func NewTxRunner(pool *pgxpool.Pool) TxRunner { return &poolTxRunner{pool: pool} }

func (r *poolTxRunner) Run(ctx context.Context, fn func(ctx context.Context) error) error {
	return WithTx(ctx, r.pool, fn)
}
