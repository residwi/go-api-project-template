package database

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/platform/config"
)

var ErrReplicaNotConfigured = errors.New("replica database not configured")

func NewPrimaryPostgres(ctx context.Context, cfg config.Database) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("parsing database config: %w", err)
	}

	applyPoolTuning(poolCfg, cfg)

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

func NewReplicaPostgres(ctx context.Context, cfg config.Database) (*pgxpool.Pool, error) {
	if cfg.ReplicaURL == "" {
		return nil, ErrReplicaNotConfigured
	}

	poolCfg, err := pgxpool.ParseConfig(cfg.ReplicaURL)
	if err != nil {
		return nil, fmt.Errorf("parsing replica database config: %w", err)
	}

	applyPoolTuning(poolCfg, cfg)

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

func applyPoolTuning(poolCfg *pgxpool.Config, cfg config.Database) {
	poolCfg.MaxConns = int32(min(cfg.MaxConns, math.MaxInt32)) //nolint:gosec // value capped at MaxInt32
	poolCfg.MinConns = int32(min(cfg.MinConns, math.MaxInt32)) //nolint:gosec // value capped at MaxInt32
	poolCfg.MaxConnLifetime = cfg.MaxConnLifetime
	poolCfg.MaxConnIdleTime = cfg.MaxConnIdleTime
}
