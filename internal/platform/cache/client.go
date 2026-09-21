package cache

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
)

func NewRedis(ctx context.Context, opts *redis.Options, logger *slog.Logger) (*redis.Client, error) {
	client := redis.NewClient(opts)

	if err := redisotel.InstrumentTracing(client, redisotel.WithDBStatement(false)); err != nil {
		return nil, fmt.Errorf("instrumenting redis: %w", err)
	}

	if err := client.Ping(ctx).Err(); err != nil {
		logger.WarnContext(ctx, "redis unreachable, returning a client that reconnects on its own",
			slog.String("error", err.Error()))
	}

	return client, nil
}
