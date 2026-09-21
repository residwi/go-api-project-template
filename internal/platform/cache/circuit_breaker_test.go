package cache

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/testutil"
)

func TestCircuitBreakerAllow(t *testing.T) {
	t.Run("a fresh breaker allows", func(t *testing.T) {
		assert.NoError(t, NewCircuitBreaker().Allow())
	})

	t.Run("threshold consecutive failures open it", func(t *testing.T) {
		b := NewCircuitBreaker()

		for range circuitThreshold - 1 {
			b.ReportResult(errors.New("dial tcp: connect: connection refused"))
			require.NoError(t, b.Allow(), "must stay closed below the threshold")
		}

		b.ReportResult(errors.New("dial tcp: connect: connection refused"))
		assert.ErrorIs(t, b.Allow(), ErrCircuitOpen)
	})

	t.Run("a success resets the counter, so the next streak starts from zero", func(t *testing.T) {
		b := NewCircuitBreaker()

		for range circuitThreshold - 1 {
			b.ReportResult(errors.New("connection refused"))
		}
		b.ReportResult(nil)

		for range circuitThreshold - 1 {
			b.ReportResult(errors.New("connection refused"))
		}
		assert.NoError(t, b.Allow(), "the success must have cleared the earlier streak")
	})

	t.Run("redis.Nil counts as a success, not a failure", func(t *testing.T) {
		b := NewCircuitBreaker()

		for range circuitThreshold * 2 {
			b.ReportResult(redis.Nil)
		}

		assert.NoError(t, b.Allow())
	})

	t.Run("an error the server authored proves it is reachable and never opens the breaker", func(t *testing.T) {
		b := NewCircuitBreaker()

		for range circuitThreshold * 2 {
			err := testRedisClient.Do(context.Background(), "SET").Err()
			require.Error(t, err)
			b.ReportResult(err)
		}

		assert.NoError(t, b.Allow(), "a server-authored reply is proof Redis answered, like MISCONF or OOM")
	})

	t.Run("a cancelled caller neither opens the breaker nor clears the streak", func(t *testing.T) {
		b := NewCircuitBreaker()

		for range circuitThreshold - 1 {
			b.ReportResult(errors.New("connection refused"))
		}
		b.ReportResult(context.Canceled)
		require.NoError(t, b.Allow(), "a cancelled caller must not open the breaker")

		b.ReportResult(errors.New("connection refused"))
		assert.ErrorIs(t, b.Allow(), ErrCircuitOpen, "the streak must have survived the cancellation")
	})

	t.Run("a timed-out command counts as a failure", func(t *testing.T) {
		b := NewCircuitBreaker()

		for range circuitThreshold {
			b.ReportResult(context.DeadlineExceeded)
		}

		assert.ErrorIs(t, b.Allow(), ErrCircuitOpen)
	})
}

func TestCircuitBreakerCooldown(t *testing.T) {
	t.Run("a success that is not the probe cannot end the cooldown", func(t *testing.T) {
		b := NewCircuitBreaker()

		for range circuitThreshold {
			b.ReportResult(errors.New("connection refused"))
		}
		require.ErrorIs(t, b.Allow(), ErrCircuitOpen)

		b.ReportResult(nil)

		assert.ErrorIs(t, b.Allow(), ErrCircuitOpen,
			"a command allowed before the trip can answer after it, and must not release the herd")
	})

	t.Run("a failed probe holds the breaker open against any later success", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			b := NewCircuitBreaker()

			for range circuitThreshold {
				b.ReportResult(errors.New("connection refused"))
			}
			time.Sleep(circuitCooldown + time.Second)

			require.NoError(t, b.Allow(), "the cooldown must let a probe through")
			b.ReportResult(errors.New("connection refused"))
			require.ErrorIs(t, b.Allow(), ErrCircuitOpen)

			b.ReportResult(nil)

			assert.ErrorIs(t, b.Allow(), ErrCircuitOpen,
				"the probe already failed, so a straggler success must not close it")
		})
	})

	t.Run("the expired cooldown releases one probe, not the whole herd", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			b := NewCircuitBreaker()

			for range circuitThreshold {
				b.ReportResult(errors.New("connection refused"))
			}
			time.Sleep(circuitCooldown + time.Second)

			require.NoError(t, b.Allow(), "the first caller after the cooldown is the probe")
			require.ErrorIs(t, b.Allow(), ErrCircuitOpen, "callers behind the probe must still be refused")
			assert.ErrorIs(t, b.Allow(), ErrCircuitOpen)
		})
	})

	t.Run("a successful probe after the cooldown restores the full threshold", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			b := NewCircuitBreaker()

			for range circuitThreshold {
				b.ReportResult(errors.New("connection refused"))
			}
			time.Sleep(circuitCooldown + time.Second)

			require.NoError(t, b.Allow())
			b.ReportResult(nil)

			for range circuitThreshold - 1 {
				b.ReportResult(errors.New("connection refused"))
			}

			assert.NoError(t, b.Allow(), "a tripped breaker must not come back as a one-failure breaker")
		})
	})
}

func TestCircuitBreakerShortCircuitsARealClient(t *testing.T) {
	broken := redis.NewClient(&redis.Options{
		Addr:        "localhost:1",
		MaxRetries:  -1,
		DialTimeout: 200 * time.Millisecond,
		Limiter:     NewCircuitBreaker(),
	})
	t.Cleanup(func() { _ = broken.Close() })

	for range circuitThreshold {
		require.Error(t, broken.Get(context.Background(), "probe").Err())
	}

	start := time.Now()
	err := broken.Get(context.Background(), "probe").Err()
	elapsed := time.Since(start)

	require.ErrorIs(t, err, ErrCircuitOpen, "go-redis must consult the Limiter before it dials")
	assert.Less(t, elapsed, 50*time.Millisecond, "an open breaker must not pay the dial timeout")

	got, takeErr := NewReadThrough(broken, testutil.DiscardLogger()).
		Take(context.Background(), testKey(t), testTTL, func(context.Context) (int, error) {
			return 42, nil
		})

	require.NoError(t, takeErr)
	assert.Equal(t, 42, got, "an open breaker must still serve the loader")
}
