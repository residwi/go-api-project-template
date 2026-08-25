package cache

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

	"github.com/residwi/go-api-project-template/internal/platform/config"
	"github.com/residwi/go-api-project-template/internal/testutil"
)

var testRedisClient *redis.Client

func TestMain(m *testing.M) {
	client, cleanup := testutil.MustStartRedis(0)
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

		client, err := NewRedis(context.Background(), config.Redis{Host: host, Port: port})
		require.NoError(t, err)
		require.NotNil(t, client)
		defer client.Close()

		assert.NoError(t, client.Ping(context.Background()).Err())
	})

	t.Run("connection refused", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		client, err := NewRedis(ctx, config.Redis{Host: "localhost", Port: 1})
		require.Error(t, err)
		assert.Nil(t, client)
		assert.Contains(t, err.Error(), "connecting to redis")
	})
}
