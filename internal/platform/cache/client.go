package cache

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

func NewRedis(ctx context.Context, opts *redis.Options) (*redis.Client, error) {
	client := redis.NewClient(opts)

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("connecting to redis: %w", err)
	}

	return client, nil
}
