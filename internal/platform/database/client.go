package database

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrReplicaNotConfigured = errors.New("replica database not configured")

type PostgresOptions struct {
	DSN             string
	MaxConns        int
	MinConns        int
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
}

func NewPrimaryPostgres(ctx context.Context, opts PostgresOptions) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(opts.DSN)
	if err != nil {
		return nil, fmt.Errorf("parsing database config: %w", err)
	}

	applyPoolTuning(poolCfg, opts)

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("connecting to database: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	return pool, nil
}

func NewReplicaPostgres(ctx context.Context, opts PostgresOptions) (*pgxpool.Pool, error) {
	if opts.DSN == "" {
		return nil, ErrReplicaNotConfigured
	}

	poolCfg, err := pgxpool.ParseConfig(opts.DSN)
	if err != nil {
		return nil, fmt.Errorf("parsing replica database config: %w", err)
	}

	applyPoolTuning(poolCfg, opts)

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("connecting to replica database: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging replica database: %w", err)
	}

	return pool, nil
}

func applyPoolTuning(poolCfg *pgxpool.Config, opts PostgresOptions) {
	poolCfg.MaxConns = int32(min(opts.MaxConns, math.MaxInt32)) //nolint:gosec // value capped at MaxInt32
	poolCfg.MinConns = int32(min(opts.MinConns, math.MaxInt32)) //nolint:gosec // value capped at MaxInt32
	poolCfg.MaxConnLifetime = opts.MaxConnLifetime
	poolCfg.MaxConnIdleTime = opts.MaxConnIdleTime
}
