package middleware

import (
	"context"
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
		handler := RateLimit(testLogger(), nil, 10, 2, time.Minute)(okHandler)

		assert.Equal(t, http.StatusOK, doRateLimited(handler, "10.1.0.1:1111").Code)
	})

	t.Run("zero burst disables the limiter", func(t *testing.T) {
		handler := RateLimit(testLogger(), testRedis, 10, 0, time.Minute)(okHandler)

		for range 5 {
			assert.Equal(t, http.StatusOK, doRateLimited(handler, "10.1.0.2:1111").Code)
		}
	})

	t.Run("allows the burst arriving at once", func(t *testing.T) {
		t.Cleanup(func() { testRedis.FlushDB(context.Background()) })

		handler := RateLimit(testLogger(), testRedis, 10, 2, time.Second)(okHandler)

		for i := range 2 {
			require.Equal(t, http.StatusOK, doRateLimited(handler, "10.1.0.3:1111").Code,
				"request %d should pass", i+1)
		}
	})

	t.Run("rejects the request past the burst", func(t *testing.T) {
		t.Cleanup(func() { testRedis.FlushDB(context.Background()) })

		handler := RateLimit(testLogger(), testRedis, 10, 2, time.Second)(okHandler)

		for range 2 {
			require.Equal(t, http.StatusOK, doRateLimited(handler, "10.1.0.4:1111").Code)
		}

		assert.Equal(t, http.StatusTooManyRequests, doRateLimited(handler, "10.1.0.4:1111").Code)
	})

	t.Run("recovers after one refill interval", func(t *testing.T) {
		t.Cleanup(func() { testRedis.FlushDB(context.Background()) })

		// Rate 10 over a 1s period is one token every 100ms.
		handler := RateLimit(testLogger(), testRedis, 10, 2, time.Second)(okHandler)

		for range 2 {
			require.Equal(t, http.StatusOK, doRateLimited(handler, "10.1.0.5:1111").Code)
		}
		require.Equal(t, http.StatusTooManyRequests, doRateLimited(handler, "10.1.0.5:1111").Code)

		time.Sleep(150 * time.Millisecond)

		assert.Equal(t, http.StatusOK, doRateLimited(handler, "10.1.0.5:1111").Code)
	})

	t.Run("sets Retry-After on a rejection", func(t *testing.T) {
		t.Cleanup(func() { testRedis.FlushDB(context.Background()) })

		handler := RateLimit(testLogger(), testRedis, 10, 1, time.Second)(okHandler)
		require.Equal(t, http.StatusOK, doRateLimited(handler, "10.1.0.6:1111").Code)

		w := doRateLimited(handler, "10.1.0.6:1111")

		require.Equal(t, http.StatusTooManyRequests, w.Code)
		assert.Equal(t, "1", w.Header().Get("Retry-After"))
	})

	t.Run("reports the budget on an allowed request", func(t *testing.T) {
		t.Cleanup(func() { testRedis.FlushDB(context.Background()) })

		handler := RateLimit(testLogger(), testRedis, 10, 3, time.Second)(okHandler)

		w := doRateLimited(handler, "10.1.0.7:1111")

		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "10", w.Header().Get("X-RateLimit-Limit"))
		assert.NotEmpty(t, w.Header().Get("X-RateLimit-Remaining"))
		assert.NotEmpty(t, w.Header().Get("X-RateLimit-Reset"))
	})

	t.Run("reports the budget on a rejected request", func(t *testing.T) {
		t.Cleanup(func() { testRedis.FlushDB(context.Background()) })

		handler := RateLimit(testLogger(), testRedis, 10, 1, time.Second)(okHandler)
		require.Equal(t, http.StatusOK, doRateLimited(handler, "10.1.0.8:1111").Code)

		w := doRateLimited(handler, "10.1.0.8:1111")

		require.Equal(t, http.StatusTooManyRequests, w.Code)
		assert.Equal(t, "10", w.Header().Get("X-RateLimit-Limit"))
		assert.Equal(t, "0", w.Header().Get("X-RateLimit-Remaining"))
	})

	t.Run("keys an authenticated caller on the user id, not the address", func(t *testing.T) {
		t.Cleanup(func() { testRedis.FlushDB(context.Background()) })

		handler := RateLimit(testLogger(), testRedis, 10, 1, time.Second)(okHandler)
		caller := identity.Identity{UserID: uuid.New(), Role: "user"}

		require.Equal(t, http.StatusOK, doRateLimitedAs(handler, "10.1.0.9:1111", caller).Code)

		w := doRateLimitedAs(handler, "10.1.0.10:2222", caller)

		assert.Equal(t, http.StatusTooManyRequests, w.Code)
	})

	t.Run("different addresses have separate budgets", func(t *testing.T) {
		t.Cleanup(func() { testRedis.FlushDB(context.Background()) })

		handler := RateLimit(testLogger(), testRedis, 10, 1, time.Second)(okHandler)

		require.Equal(t, http.StatusOK, doRateLimited(handler, "10.1.0.11:1111").Code)
		require.Equal(t, http.StatusTooManyRequests, doRateLimited(handler, "10.1.0.11:1111").Code)

		assert.Equal(t, http.StatusOK, doRateLimited(handler, "10.1.0.12:1111").Code)
	})

	t.Run("redis error allows request through", func(t *testing.T) {
		handler := RateLimit(testLogger(), testRedis, 10, 2, time.Minute)(okHandler)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = "10.1.0.13:1111"
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, r.WithContext(ctx))

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func doRateLimited(handler http.Handler, remoteAddr string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = remoteAddr
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

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
