package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/platform/identity"
)

func TestRateLimit(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	t.Run("nil redis passes through", func(t *testing.T) {
		handler := RateLimit(testLogger(), nil, "test", 10, 2, time.Minute)(okHandler)

		assert.Equal(t, http.StatusOK, doRateLimited(handler, "10.1.0.1:1111").Code)
	})

	t.Run("zero burst disables the limiter", func(t *testing.T) {
		handler := RateLimit(testLogger(), testRedis, "test", 10, 0, time.Minute)(okHandler)

		for range 5 {
			assert.Equal(t, http.StatusOK, doRateLimited(handler, "10.1.0.2:1111").Code)
		}
	})

	t.Run("allows the burst arriving at once", func(t *testing.T) {
		t.Cleanup(func() { testRedis.FlushDB(context.Background()) })

		handler := RateLimit(testLogger(), testRedis, "test", 10, 2, time.Second)(okHandler)

		for i := range 2 {
			require.Equal(t, http.StatusOK, doRateLimited(handler, "10.1.0.3:1111").Code,
				"request %d should pass", i+1)
		}
	})

	t.Run("rejects the request past the burst", func(t *testing.T) {
		t.Cleanup(func() { testRedis.FlushDB(context.Background()) })

		handler := RateLimit(testLogger(), testRedis, "test", 10, 2, time.Second)(okHandler)

		for range 2 {
			require.Equal(t, http.StatusOK, doRateLimited(handler, "10.1.0.4:1111").Code)
		}

		assert.Equal(t, http.StatusTooManyRequests, doRateLimited(handler, "10.1.0.4:1111").Code)
	})

	t.Run("admits exactly the auth burst of three", func(t *testing.T) {
		t.Cleanup(func() { testRedis.FlushDB(context.Background()) })

		handler := RateLimit(testLogger(), testRedis, "test", 10, 3, time.Second)(okHandler)

		for i := range 3 {
			require.Equal(t, http.StatusOK, doRateLimited(handler, "10.1.0.14:1111").Code,
				"request %d should pass", i+1)
		}

		assert.Equal(t, http.StatusTooManyRequests, doRateLimited(handler, "10.1.0.14:1111").Code)
	})

	t.Run("recovers after one refill interval", func(t *testing.T) {
		t.Cleanup(func() { testRedis.FlushDB(context.Background()) })

		// Rate 10 over a 1s period is one token every 100ms.
		handler := RateLimit(testLogger(), testRedis, "test", 10, 2, time.Second)(okHandler)

		for range 2 {
			require.Equal(t, http.StatusOK, doRateLimited(handler, "10.1.0.5:1111").Code)
		}
		require.Equal(t, http.StatusTooManyRequests, doRateLimited(handler, "10.1.0.5:1111").Code)

		time.Sleep(150 * time.Millisecond)

		assert.Equal(t, http.StatusOK, doRateLimited(handler, "10.1.0.5:1111").Code)
	})

	t.Run("sets Retry-After on a rejection", func(t *testing.T) {
		t.Cleanup(func() { testRedis.FlushDB(context.Background()) })

		handler := RateLimit(testLogger(), testRedis, "test", 10, 1, time.Second)(okHandler)
		require.Equal(t, http.StatusOK, doRateLimited(handler, "10.1.0.6:1111").Code)

		w := doRateLimited(handler, "10.1.0.6:1111")

		require.Equal(t, http.StatusTooManyRequests, w.Code)
		assert.Equal(t, "1", w.Header().Get("Retry-After"))
	})

	t.Run("reports the budget on an allowed request", func(t *testing.T) {
		t.Cleanup(func() { testRedis.FlushDB(context.Background()) })

		handler := RateLimit(testLogger(), testRedis, "test", 10, 3, time.Second)(okHandler)

		w := doRateLimited(handler, "10.1.0.7:1111")

		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "10", w.Header().Get("X-Ratelimit-Limit"))
		// An artifact of this test's 1s period: redis_rate's emission interval
		// is then 0.1, and the inexact float cancels to burst-2. The shipped
		// 1m period gives exactly 6.0 and reports correctly.
		assert.Equal(t, "1", w.Header().Get("X-Ratelimit-Remaining"))
		assert.NotEmpty(t, w.Header().Get("X-Ratelimit-Reset"))
	})

	t.Run("reports the budget on a rejected request", func(t *testing.T) {
		t.Cleanup(func() { testRedis.FlushDB(context.Background()) })

		handler := RateLimit(testLogger(), testRedis, "test", 10, 1, time.Second)(okHandler)
		require.Equal(t, http.StatusOK, doRateLimited(handler, "10.1.0.8:1111").Code)

		w := doRateLimited(handler, "10.1.0.8:1111")

		require.Equal(t, http.StatusTooManyRequests, w.Code)
		assert.Equal(t, "10", w.Header().Get("X-Ratelimit-Limit"))
		assert.Equal(t, "0", w.Header().Get("X-Ratelimit-Remaining"))
	})

	t.Run("keys an authenticated caller on the user id, not the address", func(t *testing.T) {
		t.Cleanup(func() { testRedis.FlushDB(context.Background()) })

		handler := RateLimit(testLogger(), testRedis, "test", 10, 1, time.Second)(okHandler)
		caller := identity.Identity{UserID: uuid.New(), Role: "user"}

		require.Equal(t, http.StatusOK, doRateLimitedAs(handler, "10.1.0.9:1111", caller).Code)

		w := doRateLimitedAs(handler, "10.1.0.10:2222", caller)

		assert.Equal(t, http.StatusTooManyRequests, w.Code)
	})

	t.Run("different addresses have separate budgets", func(t *testing.T) {
		t.Cleanup(func() { testRedis.FlushDB(context.Background()) })

		handler := RateLimit(testLogger(), testRedis, "test", 10, 1, time.Second)(okHandler)

		require.Equal(t, http.StatusOK, doRateLimited(handler, "10.1.0.11:1111").Code)
		require.Equal(t, http.StatusTooManyRequests, doRateLimited(handler, "10.1.0.11:1111").Code)

		assert.Equal(t, http.StatusOK, doRateLimited(handler, "10.1.0.12:1111").Code)
	})

	t.Run("keys IPv6 callers by their 64 prefix", func(t *testing.T) {
		t.Cleanup(func() { testRedis.FlushDB(context.Background()) })

		handler := RateLimit(testLogger(), testRedis, "test", 10, 1, time.Second)(okHandler)

		require.Equal(t, http.StatusOK, doRateLimited(handler, "[2001:db8:1::1]:1111").Code)
		assert.Equal(t, http.StatusTooManyRequests, doRateLimited(handler, "[2001:db8:1::9999]:2222").Code,
			"a different suffix in the same /64 must share the bucket")

		assert.Equal(t, http.StatusOK, doRateLimited(handler, "[2001:db8:2::1]:3333").Code,
			"a different /64 must get its own bucket")
	})

	t.Run("redis error allows request through", func(t *testing.T) {
		handler := RateLimit(testLogger(), testRedis, "test", 10, 2, time.Minute)(okHandler)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = "10.1.0.13:1111"
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, r.WithContext(ctx))

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("throttles a flooding client while redis is down", func(t *testing.T) {
		handler := RateLimit(testLogger(), testRedis, "test", 2, 2, time.Minute)(okHandler)

		require.Equal(t, http.StatusOK, doRateLimitedDown(handler, "10.2.0.1:1111").Code)
		require.Equal(t, http.StatusOK, doRateLimitedDown(handler, "10.2.0.1:1111").Code)

		assert.Equal(t, http.StatusTooManyRequests, doRateLimitedDown(handler, "10.2.0.1:1111").Code,
			"the in-process fallback must brute-force-throttle when the redis limiter errors")
	})
}

func TestFallbackLimiter(t *testing.T) {
	fill := func(fb *fallbackLimiter, now time.Time) {
		for i := range fallbackMaxKeys {
			fb.allow(fmt.Sprintf("client-%d", i), now)
		}
	}

	t.Run("charges newcomers to the shared overflow budget when full", func(t *testing.T) {
		fb := newFallbackLimiter(5, time.Minute)
		now := time.Now()
		fill(fb, now)
		require.Len(t, fb.keys, fallbackMaxKeys)

		first, firstSaturated := fb.allow("newcomer-1", now)
		second, secondSaturated := fb.allow("newcomer-2", now)

		assert.True(t, first)
		assert.True(t, firstSaturated, "the first overflow of a window reports saturation")
		assert.True(t, second)
		assert.False(t, secondSaturated, "saturation is reported once per window, not per request")
		assert.Len(t, fb.keys, fallbackMaxKeys)
	})

	t.Run("denies new callers once the overflow budget is spent", func(t *testing.T) {
		fb := newFallbackLimiter(1, time.Minute)
		now := time.Now()
		fill(fb, now)

		for i := range fallbackMaxKeys {
			allowed, _ := fb.allow(fmt.Sprintf("flood-%d", i), now)
			require.True(t, allowed)
		}
		allowed, _ := fb.allow("one-too-many", now)

		assert.False(t, allowed)
	})

	t.Run("reclaims an expired slot rather than overflowing", func(t *testing.T) {
		fb := newFallbackLimiter(5, time.Minute)
		now := time.Now()
		fill(fb, now)
		fb.keys["client-0"].resetAt = now.Add(-time.Second)

		allowed, saturated := fb.allow("newcomer", now)

		assert.True(t, allowed)
		assert.False(t, saturated)
		assert.Contains(t, fb.keys, "newcomer")
		assert.Len(t, fb.keys, fallbackMaxKeys)
	})

	t.Run("scans for expired slots at most once per window", func(t *testing.T) {
		fb := newFallbackLimiter(5, time.Minute)
		now := time.Now()
		fill(fb, now)
		fb.allow("triggers-the-first-scan", now)

		fb.keys["client-0"].resetAt = now.Add(-time.Second)
		fb.allow("inside-the-window", now.Add(time.Second))
		assert.NotContains(t, fb.keys, "inside-the-window", "no second scan within one window")
		assert.Contains(t, fb.keys, "client-0")

		fb.allow("after-the-window", now.Add(time.Minute+time.Second))
		assert.Contains(t, fb.keys, "after-the-window", "a new window permits a scan")
	})

	t.Run("recovers a tracked key after its window", func(t *testing.T) {
		fb := newFallbackLimiter(1, time.Minute)
		now := time.Now()

		allowed, _ := fb.allow("client", now)
		require.True(t, allowed)
		denied, _ := fb.allow("client", now)
		require.False(t, denied)

		recovered, _ := fb.allow("client", now.Add(2*time.Minute))
		assert.True(t, recovered)
	})
}

func doRateLimited(handler http.Handler, remoteAddr string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = remoteAddr
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	return w
}

func doRateLimitedDown(handler http.Handler, remoteAddr string) *httptest.ResponseRecorder {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = remoteAddr
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r.WithContext(ctx))

	return w
}

func doRateLimitedAs(
	handler http.Handler,
	remoteAddr string,
	caller identity.Identity,
) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = remoteAddr
	r = r.WithContext(identity.NewContext(r.Context(), caller))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	return w
}
