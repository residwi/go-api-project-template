package database

import (
	"context"
	"fmt"
	"math"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/config"
	"github.com/residwi/go-api-project-template/internal/core"
)

func NewPostgres(ctx context.Context, cfg config.DatabaseConfig) (*pgxpool.Pool, error) {
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

func NewReaderPostgres(ctx context.Context, cfg config.DatabaseConfig) (*pgxpool.Pool, error) {
	if cfg.ReaderURL == "" {
		return nil, core.ErrReaderNotConfigured
	}

	poolCfg, err := pgxpool.ParseConfig(cfg.ReaderURL)
	if err != nil {
		return nil, fmt.Errorf("parsing reader database config: %w", err)
	}

	applyPoolTuning(poolCfg, cfg)

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("connecting to reader database: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging reader database: %w", err)
	}

	return pool, nil
}

// applyPoolTuning applies the configured connection-pool limits so both the
// primary and reader pools are bounded consistently instead of running on
// pgx defaults.
func applyPoolTuning(poolCfg *pgxpool.Config, cfg config.DatabaseConfig) {
	poolCfg.MaxConns = int32(min(cfg.MaxConns, math.MaxInt32)) //nolint:gosec // value capped at MaxInt32
	poolCfg.MinConns = int32(min(cfg.MinConns, math.MaxInt32)) //nolint:gosec // value capped at MaxInt32
	poolCfg.MaxConnLifetime = cfg.MaxConnLifetime
	poolCfg.MaxConnIdleTime = cfg.MaxConnIdleTime
}
