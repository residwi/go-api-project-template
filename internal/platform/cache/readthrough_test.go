package cache

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/testutil"
)

const testTTL = time.Minute

func TestTake(t *testing.T) {
	t.Run("miss loads and returns the value; a second Take serves it without the loader", func(t *testing.T) {
		rt := NewReadThrough(testRedisClient, testutil.DiscardLogger())
		key := testKey(t)

		got, err := rt.Take(context.Background(), key, testTTL, func(context.Context) (int, error) {
			return 7, nil
		})
		require.NoError(t, err)
		assert.Equal(t, 7, got)

		var loaded atomic.Bool
		got, err = rt.Take(context.Background(), key, testTTL, func(context.Context) (int, error) {
			loaded.Store(true)
			return 0, nil
		})
		require.NoError(t, err)
		assert.Equal(t, 7, got)
		assert.False(t, loaded.Load(), "second Take must be served from the cache without calling the loader")
	})

	t.Run("a cached absence returns errs.ErrNotFound without calling the loader", func(t *testing.T) {
		rt := NewReadThrough(testRedisClient, testutil.DiscardLogger())
		key := testKey(t)
		require.NoError(t, testRedisClient.Set(context.Background(), key, absentPlaceholder, time.Minute).Err())

		var loaded atomic.Bool
		_, err := rt.Take(context.Background(), key, testTTL, func(context.Context) (int, error) {
			loaded.Store(true)
			return 0, nil
		})

		require.ErrorIs(t, err, errs.ErrNotFound)
		assert.False(t, loaded.Load(), "loader must not run on a cached absence")
	})

	t.Run(
		"an errs.ErrNotFound loader writes the placeholder, and PTTL is within the absentTTL band and below the ttl",
		func(t *testing.T) {
			rt := NewReadThrough(testRedisClient, testutil.DiscardLogger())
			key := testKey(t)

			_, err := rt.Take(context.Background(), key, testTTL, func(context.Context) (int, error) {
				return 0, errs.ErrNotFound
			})
			require.ErrorIs(t, err, errs.ErrNotFound)

			ttl, err := testRedisClient.PTTL(context.Background(), key).Result()
			require.NoError(t, err)
			assert.Greater(t, ttl, time.Duration(0))
			assert.LessOrEqual(t, ttl, time.Duration(1.1*float64(absentTTL)))
			assert.Less(t, ttl, testTTL, "absent TTL must stay below the success TTL")
		},
	)

	t.Run("N concurrent callers with a blocking loader call it once", func(t *testing.T) {
		rt := NewReadThrough(testRedisClient, testutil.DiscardLogger())
		key := testKey(t)

		var loads atomic.Int64
		release := make(chan struct{})

		var wg sync.WaitGroup
		for range 20 {
			wg.Go(func() {
				_, _ = rt.Take(context.Background(), key, testTTL, func(context.Context) (int, error) {
					loads.Add(1)
					<-release
					return 5, nil
				})
			})
		}

		assert.Eventually(t, func() bool { return loads.Load() == 1 }, time.Second, time.Millisecond)
		close(release)
		wg.Wait()

		assert.Equal(t, int64(1), loads.Load())
	})

	t.Run("a cancelled caller returns context.Canceled while the shared load still writes through", func(t *testing.T) {
		rt := NewReadThrough(testRedisClient, testutil.DiscardLogger())
		key := testKey(t)

		release := make(chan struct{})
		started := make(chan struct{})
		done := make(chan struct{})
		ctx, cancel := context.WithCancel(context.Background())

		go func() {
			defer close(done)
			_, _ = rt.Take(ctx, key, testTTL, func(context.Context) (int, error) {
				close(started)
				<-release
				return 9, nil
			})
		}()

		<-started

		waiterCtx, waiterCancel := context.WithCancel(context.Background())
		waiterErr := make(chan error, 1)
		go func() {
			_, err := rt.Take(waiterCtx, key, testTTL, func(context.Context) (int, error) {
				return 0, errors.New("second loader must not run")
			})
			waiterErr <- err
		}()

		waiterCancel()
		require.ErrorIs(t, <-waiterErr, context.Canceled)

		cancel()
		close(release)
		<-done

		assert.Eventually(t, func() bool {
			raw, err := testRedisClient.Get(context.Background(), key).Result()
			return err == nil && raw == `9`
		}, time.Second, time.Millisecond, "the shared load must still write through")
	})

	t.Run("recovers a panicking loader into an error instead of crashing", func(t *testing.T) {
		rt := NewReadThrough(testRedisClient, testutil.DiscardLogger())
		key := testKey(t)

		_, err := rt.Take(context.Background(), key, testTTL, func(context.Context) (int, error) {
			panic("boom")
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "boom")
	})

	t.Run("a corrupt entry is DELeted and reloaded", func(t *testing.T) {
		rt := NewReadThrough(testRedisClient, testutil.DiscardLogger())
		key := testKey(t)
		require.NoError(t, testRedisClient.Set(context.Background(), key, "not-json", time.Minute).Err())

		var loaded atomic.Bool
		_, err := rt.Take(context.Background(), key, testTTL, func(context.Context) (int, error) {
			loaded.Store(true)
			return 0, errs.ErrNotFound
		})

		require.ErrorIs(t, err, errs.ErrNotFound)
		assert.True(t, loaded.Load(), "a corrupt entry must fall through to the loader")

		raw, err := testRedisClient.Get(context.Background(), key).Result()
		require.NoError(t, err)
		assert.Equal(t, absentPlaceholder, raw, "the corrupt entry must be deleted before the placeholder is written")
	})

	t.Run("a Redis error falls through to the loader", func(t *testing.T) {
		broken := redis.NewClient(&redis.Options{
			Addr:        "localhost:1",
			MaxRetries:  -1,
			DialTimeout: 200 * time.Millisecond,
		})
		t.Cleanup(func() { _ = broken.Close() })

		rt := NewReadThrough(broken, testutil.DiscardLogger())
		key := testKey(t)

		got, err := rt.Take(context.Background(), key, testTTL, func(context.Context) (int, error) {
			return 3, nil
		})

		require.NoError(t, err)
		assert.Equal(t, 3, got)
	})

	t.Run("a ttl of 0 returns the loaded value and writes nothing", func(t *testing.T) {
		rt := NewReadThrough(testRedisClient, testutil.DiscardLogger())
		key := testKey(t)

		got, err := rt.Take(context.Background(), key, 0, func(context.Context) (int, error) {
			return 11, nil
		})
		require.NoError(t, err)
		assert.Equal(t, 11, got)

		_, err = testRedisClient.Get(context.Background(), key).Result()
		assert.ErrorIs(t, err, redis.Nil, "a ttl of 0 must not write the entry")
	})

	t.Run("a ttl of 0 still bounds the loader", func(t *testing.T) {
		rt := NewReadThrough(testRedisClient, testutil.DiscardLogger())

		_, err := rt.Take(context.Background(), testKey(t), 0, func(ctx context.Context) (int, error) {
			deadline, ok := ctx.Deadline()
			assert.True(t, ok, "the loader context must carry a deadline")
			assert.WithinDuration(t, time.Now().Add(loadTimeout), deadline, 200*time.Millisecond)

			return 1, nil
		})

		require.NoError(t, err)
	})

	t.Run(
		"a type mismatch between concurrent Take[T] calls on the same key returns an error, not a zero value",
		func(t *testing.T) {
			rt := NewReadThrough(testRedisClient, testutil.DiscardLogger())
			key := testKey(t)

			release := make(chan struct{})
			started := make(chan struct{})

			go func() {
				_, _ = rt.Take(context.Background(), key, testTTL, func(context.Context) (int, error) {
					close(started)
					<-release

					return 42, nil
				})
			}()

			<-started

			type takeResult struct {
				val string
				err error
			}

			ready := make(chan struct{})
			followerCh := make(chan takeResult, 1)
			go func() {
				close(ready)
				v, err := rt.Take(context.Background(), key, testTTL, func(context.Context) (string, error) {
					return "unreachable", nil
				})
				followerCh <- takeResult{val: v, err: err}
			}()

			<-ready
			close(release)

			got := <-followerCh
			require.Error(t, got.err)
			assert.Empty(t, got.val)
		},
	)
}

func TestJitter(t *testing.T) {
	t.Run("varies within the deviation band across repeated calls", func(t *testing.T) {
		base := time.Minute

		seen := make(map[time.Duration]struct{})

		var sawBelow, sawAbove bool
		for range 200 {
			d := jitter(base)
			assert.GreaterOrEqual(t, d, time.Duration(0.9*float64(base)))
			assert.LessOrEqual(t, d, time.Duration(1.1*float64(base)))
			seen[d] = struct{}{}

			if d < base {
				sawBelow = true
			}
			if d > base {
				sawAbove = true
			}
		}

		assert.Greater(t, len(seen), 1, "jitter must vary the duration across calls")
		assert.True(t, sawBelow, "jitter must be able to shorten the TTL")
		assert.True(t, sawAbove, "jitter must be able to lengthen the TTL")
	})
}

func TestTakeFallsBackWhenRedisAcceptsButNeverAnswers(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	done := make(chan struct{})
	t.Cleanup(func() { close(done) })

	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		<-done
		_ = conn.Close()
	}()

	silent := redis.NewClient(&redis.Options{
		Addr:        listener.Addr().String(),
		MaxRetries:  -1,
		ReadTimeout: 200 * time.Millisecond,
	})
	t.Cleanup(func() { _ = silent.Close() })

	start := time.Now()
	got, takeErr := NewReadThrough(silent, testutil.DiscardLogger()).
		Take(context.Background(), testKey(t), testTTL, func(context.Context) (int, error) {
			return 9, nil
		})
	elapsed := time.Since(start)

	require.NoError(t, takeErr)
	assert.Equal(t, 9, got)
	assert.Less(t, elapsed, 2*time.Second,
		"go-redis ignores the command context while it handshakes, so REDIS_READ_TIMEOUT is the only bound here")
}

func testKey(t *testing.T) string {
	t.Helper()

	return "test:cache:" + uuid.NewString()
}
