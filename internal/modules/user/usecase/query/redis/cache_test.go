package redis

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/modules/user/usecase/query"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

// This package owns Redis DB index 6; see the registry in internal/testhelper.
var testRedis *goredis.Client

func TestMain(m *testing.M) {
	rdb, cleanup := testhelper.MustStartRedis(6)
	defer cleanup()
	testRedis = rdb
	os.Exit(m.Run())
}

func TestCache_RoundTrip(t *testing.T) {
	t.Run("returns what Put stored", func(t *testing.T) {
		ctx := context.Background()
		c := New(testRedis)
		id := uuid.New()
		t.Cleanup(func() { c.Invalidate(ctx, id) })

		require.NoError(t, c.Put(ctx, id, query.StatusSnapshot{Active: true, TokenVersion: 42}, time.Minute))

		got, found, err := c.Get(ctx, id)

		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, query.StatusSnapshot{Active: true, TokenVersion: 42}, got)
	})

	t.Run("round-trips an inactive user", func(t *testing.T) {
		ctx := context.Background()
		c := New(testRedis)
		id := uuid.New()
		t.Cleanup(func() { c.Invalidate(ctx, id) })

		require.NoError(t, c.Put(ctx, id, query.StatusSnapshot{Active: false, TokenVersion: 3}, time.Minute))

		got, found, err := c.Get(ctx, id)

		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, query.StatusSnapshot{Active: false, TokenVersion: 3}, got)
	})
}

func TestCache_Get(t *testing.T) {
	t.Run("reports found=false for a user never cached", func(t *testing.T) {
		got, found, err := New(testRedis).Get(context.Background(), uuid.New())

		require.NoError(t, err)
		assert.False(t, found)
		assert.Equal(t, query.StatusSnapshot{}, got)
	})

	t.Run("reports found=false for a hash missing the active field", func(t *testing.T) {
		ctx := context.Background()
		id := uuid.New()
		t.Cleanup(func() { testRedis.Del(ctx, statusKey(id)) })

		// Write a partial hash directly, bypassing Put, to simulate the state
		// Get must not trust: token_version present, active missing.
		require.NoError(t, testRedis.HSet(ctx, statusKey(id), "token_version", "1").Err())

		got, found, err := New(testRedis).Get(ctx, id)

		require.NoError(t, err)
		assert.False(t, found)
		assert.Equal(t, query.StatusSnapshot{}, got)
	})
}

func TestCache_Put(t *testing.T) {
	t.Run("sets the ttl in the same command that writes the fields", func(t *testing.T) {
		ctx := context.Background()
		c := New(testRedis)
		id := uuid.New()
		t.Cleanup(func() { c.Invalidate(ctx, id) })

		require.NoError(t, c.Put(ctx, id, query.StatusSnapshot{Active: true, TokenVersion: 1}, 30*time.Second))

		// HSETEX applies the expiry per field, so read it back with HTTL.
		ttls, err := testRedis.HTTL(ctx, statusKey(id), "active", "token_version").Result()
		require.NoError(t, err)
		require.Len(t, ttls, 2)
		for _, ttl := range ttls {
			assert.Positive(t, ttl, "every field must carry the expiry")
			assert.LessOrEqual(t, ttl, int64(30))
		}
	})
}

func TestCache_Invalidate(t *testing.T) {
	t.Run("removes the entry so a later Get misses", func(t *testing.T) {
		ctx := context.Background()
		c := New(testRedis)
		id := uuid.New()
		require.NoError(t, c.Put(ctx, id, query.StatusSnapshot{Active: true, TokenVersion: 1}, time.Minute))

		require.NoError(t, c.Invalidate(ctx, id))

		_, found, err := c.Get(ctx, id)
		require.NoError(t, err)
		assert.False(t, found)
	})

	t.Run("succeeds for a user never cached", func(t *testing.T) {
		assert.NoError(t, New(testRedis).Invalidate(context.Background(), uuid.New()))
	})
}
