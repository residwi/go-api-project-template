package user_test

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/features/user"
)

func TestService_CheckStatus_Integration(t *testing.T) {
	t.Run("cache miss populates cache from DB", func(t *testing.T) {
		u := seedUser(t)
		ctx := context.Background()
		key := fmt.Sprintf("user:status:%s", u.ID.String())
		t.Cleanup(func() { testRedis.Del(ctx, key) })

		repo := user.NewPostgresRepository(testPool)
		svc := user.NewService(repo, testPool, testRedis)

		result, err := svc.CheckStatus(ctx, u.ID)
		require.NoError(t, err)
		assert.True(t, result.Active)
		assert.Equal(t, u.TokenVersion, result.TokenVersion)

		cached, err := testRedis.HGetAll(ctx, key).Result()
		require.NoError(t, err)
		assert.Equal(t, "1", cached["active"])
		assert.Equal(t, strconv.Itoa(u.TokenVersion), cached["token_version"])
	})

	t.Run("cache hit returns cached data", func(t *testing.T) {
		u := seedUser(t)
		ctx := context.Background()
		key := fmt.Sprintf("user:status:%s", u.ID.String())
		t.Cleanup(func() { testRedis.Del(ctx, key) })

		testRedis.HSet(ctx, key, "active", "1", "token_version", "42")

		repo := user.NewPostgresRepository(testPool)
		svc := user.NewService(repo, testPool, testRedis)

		result, err := svc.CheckStatus(ctx, u.ID)
		require.NoError(t, err)
		assert.True(t, result.Active)
		assert.Equal(t, 42, result.TokenVersion)
	})

	t.Run("redis read error falls back to DB", func(t *testing.T) {
		u := seedUser(t)
		ctx := context.Background()

		// Create a separate Redis client pointing to an invalid address to force errors.
		brokenRedis := redis.NewClient(&redis.Options{
			Addr:         "localhost:1",
			MaxRetries:   0,
			DialTimeout:  200 * time.Millisecond,
			PoolSize:     1,
			MinIdleConns: 0,
		})
		defer brokenRedis.Close()

		repo := user.NewPostgresRepository(testPool)
		svc := user.NewService(repo, testPool, brokenRedis)

		result, err := svc.CheckStatus(ctx, u.ID)
		require.NoError(t, err)
		assert.True(t, result.Active)
		assert.Equal(t, u.TokenVersion, result.TokenVersion)
	})

	t.Run("inactive user returns Active false", func(t *testing.T) {
		u := seedUser(t)
		ctx := context.Background()
		key := fmt.Sprintf("user:status:%s", u.ID.String())
		t.Cleanup(func() { testRedis.Del(ctx, key) })

		_, err := testPool.Exec(ctx, `UPDATE users SET active = false WHERE id = $1`, u.ID)
		require.NoError(t, err)

		repo := user.NewPostgresRepository(testPool)
		svc := user.NewService(repo, testPool, testRedis)

		result, err := svc.CheckStatus(ctx, u.ID)
		require.NoError(t, err)
		assert.False(t, result.Active)

		cached, err := testRedis.HGetAll(ctx, key).Result()
		require.NoError(t, err)
		assert.Equal(t, "0", cached["active"])
	})
}
