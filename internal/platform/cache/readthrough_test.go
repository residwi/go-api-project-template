package cache

import (
	"context"
	"errors"
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

func TestTake(t *testing.T) {
	t.Run("miss loads and returns the value; a second Take serves it without the loader", func(t *testing.T) {
		rt := NewReadThrough(testRedisClient, testutil.DiscardLogger())
		key := testKey(t)

		got, err := rt.Take(context.Background(), testOptions(), key, func(context.Context) (int, error) {
			return 7, nil
		})
		require.NoError(t, err)
		assert.Equal(t, 7, got)

		var loaded atomic.Bool
		got, err = rt.Take(context.Background(), testOptions(), key, func(context.Context) (int, error) {
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
		_, err := rt.Take(context.Background(), testOptions(), key, func(context.Context) (int, error) {
			loaded.Store(true)
			return 0, nil
		})

		require.ErrorIs(t, err, errs.ErrNotFound)
		assert.False(t, loaded.Load(), "loader must not run on a cached absence")
	})

	t.Run(
		"an errs.ErrNotFound loader writes the placeholder, and PTTL is within the AbsentTTL band and below Options.TTL",
		func(t *testing.T) {
			rt := NewReadThrough(testRedisClient, testutil.DiscardLogger())
			key := testKey(t)
			opts := testOptions()

			_, err := rt.Take(context.Background(), opts, key, func(context.Context) (int, error) {
				return 0, errs.ErrNotFound
			})
			require.ErrorIs(t, err, errs.ErrNotFound)

			ttl, err := testRedisClient.PTTL(context.Background(), key).Result()
			require.NoError(t, err)
			assert.Greater(t, ttl, time.Duration(0))
			assert.LessOrEqual(t, ttl, time.Duration(1.1*float64(opts.AbsentTTL)))
			assert.Less(t, ttl, opts.TTL, "absent TTL must stay below the success TTL")
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
				_, _ = rt.Take(context.Background(), testOptions(), key, func(context.Context) (int, error) {
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
			_, _ = rt.Take(ctx, testOptions(), key, func(context.Context) (int, error) {
				close(started)
				<-release
				return 9, nil
			})
		}()

		<-started

		waiterCtx, waiterCancel := context.WithCancel(context.Background())
		waiterErr := make(chan error, 1)
		go func() {
			_, err := rt.Take(waiterCtx, testOptions(), key, func(context.Context) (int, error) {
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

		_, err := rt.Take(context.Background(), testOptions(), key, func(context.Context) (int, error) {
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
		_, err := rt.Take(context.Background(), testOptions(), key, func(context.Context) (int, error) {
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

		got, err := rt.Take(context.Background(), testOptions(), key, func(context.Context) (int, error) {
			return 3, nil
		})

		require.NoError(t, err)
		assert.Equal(t, 3, got)
	})

	t.Run("panics when Options.TTL, AbsentTTL, LoadTimeout is zero, or Deviation is out of range", func(t *testing.T) {
		rt := NewReadThrough(testRedisClient, testutil.DiscardLogger())
		load := func(context.Context) (int, error) { return 0, nil }

		t.Run("zero TTL", func(t *testing.T) {
			opts := testOptions()
			opts.TTL = 0

			assert.Panics(t, func() { _, _ = rt.Take(context.Background(), opts, testKey(t), load) })
		})

		t.Run("zero AbsentTTL", func(t *testing.T) {
			opts := testOptions()
			opts.AbsentTTL = 0

			assert.Panics(t, func() { _, _ = rt.Take(context.Background(), opts, testKey(t), load) })
		})

		t.Run("zero LoadTimeout", func(t *testing.T) {
			opts := testOptions()
			opts.LoadTimeout = 0

			assert.Panics(t, func() { _, _ = rt.Take(context.Background(), opts, testKey(t), load) })
		})

		t.Run("negative Deviation", func(t *testing.T) {
			opts := testOptions()
			opts.Deviation = -0.1

			assert.Panics(t, func() { _, _ = rt.Take(context.Background(), opts, testKey(t), load) })
		})

		t.Run("Deviation of 1 or more", func(t *testing.T) {
			opts := testOptions()
			opts.Deviation = 1

			assert.Panics(t, func() { _, _ = rt.Take(context.Background(), opts, testKey(t), load) })
		})
	})

	t.Run(
		"a type mismatch between concurrent Take[T] calls on the same key returns an error, not a zero value",
		func(t *testing.T) {
			rt := NewReadThrough(testRedisClient, testutil.DiscardLogger())
			key := testKey(t)

			release := make(chan struct{})
			started := make(chan struct{})

			go func() {
				_, _ = rt.Take(context.Background(), testOptions(), key, func(context.Context) (int, error) {
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
				v, err := rt.Take(context.Background(), testOptions(), key, func(context.Context) (string, error) {
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
		deviation := 0.1

		seen := make(map[time.Duration]struct{})

		var sawBelow, sawAbove bool
		for range 200 {
			d := jitter(base, deviation)
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

func testOptions() Options {
	return Options{
		TTL:         time.Minute,
		AbsentTTL:   10 * time.Second,
		Deviation:   0.1,
		LoadTimeout: time.Second,
	}
}

func testKey(t *testing.T) string {
	t.Helper()

	return "test:cache:" + uuid.NewString()
}
