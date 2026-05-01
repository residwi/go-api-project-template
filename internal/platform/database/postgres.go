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

	poolCfg.MaxConns = int32(min(cfg.MaxConns, math.MaxInt32)) //nolint:gosec // value capped at MaxInt32
	poolCfg.MinConns = int32(min(cfg.MinConns, math.MaxInt32)) //nolint:gosec // value capped at MaxInt32
	poolCfg.MaxConnLifetime = cfg.MaxConnLifetime
	poolCfg.MaxConnIdleTime = cfg.MaxConnIdleTime

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

func NewReaderPostgres(ctx context.Context, readerURL string) (*pgxpool.Pool, error) {
	if readerURL == "" {
		return nil, core.ErrReaderNotConfigured
	}

	pool, err := pgxpool.New(ctx, readerURL)
	if err != nil {
		return nil, fmt.Errorf("connecting to reader database: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging reader database: %w", err)
	}

	return pool, nil
}
