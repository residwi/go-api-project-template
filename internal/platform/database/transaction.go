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

type DB struct {
	Primary *pgxpool.Pool
	Replica *pgxpool.Pool
}

func PrimaryDB(ctx context.Context, db DB) DBTX {
	if tx, ok := ctx.Value(txCtxKey{}).(DBTX); ok {
		return tx
	}
	return db.Primary
}

func ReplicaDB(ctx context.Context, db DB) DBTX {
	if tx, ok := ctx.Value(txCtxKey{}).(DBTX); ok {
		return tx
	}
	if db.Replica != nil {
		return db.Replica
	}
	return db.Primary
}

type TxRunner interface {
	Run(ctx context.Context, fn func(ctx context.Context) error) error
}

type poolTxRunner struct{ pool *pgxpool.Pool }

func NewTxRunner(pool *pgxpool.Pool) TxRunner { return &poolTxRunner{pool: pool} }

func (r *poolTxRunner) Run(ctx context.Context, fn func(ctx context.Context) error) error {
	return WithTx(ctx, r.pool, fn)
}
