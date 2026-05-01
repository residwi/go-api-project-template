package cache_test

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/config"
	"github.com/residwi/go-api-project-template/internal/platform/cache"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

var testRedisClient *redis.Client

func TestMain(m *testing.M) {
	client, cleanup := testhelper.MustStartRedis(0)
	defer cleanup()
	testRedisClient = client
	os.Exit(m.Run())
}

func TestNewRedis(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		addr := testRedisClient.Options().Addr
		lastColon := strings.LastIndex(addr, ":")
		host := addr[:lastColon]
		port, _ := strconv.Atoi(addr[lastColon+1:])

		client, err := cache.NewRedis(context.Background(), config.RedisConfig{Host: host, Port: port})
		require.NoError(t, err)
		require.NotNil(t, client)
		defer client.Close()

		assert.NoError(t, client.Ping(context.Background()).Err())
	})

	t.Run("connection refused", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		client, err := cache.NewRedis(ctx, config.RedisConfig{Host: "localhost", Port: 1})
		require.Error(t, err)
		assert.Nil(t, client)
		assert.Contains(t, err.Error(), "connecting to redis")
	})
}
