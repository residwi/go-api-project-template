package middleware_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/middleware"
)

func TestRateLimit(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	t.Run("nil redis passes through", func(t *testing.T) {
		handler := middleware.RateLimit(nil, 10, time.Minute)(okHandler)

		r := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("allows requests under limit", func(t *testing.T) {
		t.Cleanup(func() {
			testRedis.FlushDB(context.Background())
		})

		const maxRequests = 5
		handler := middleware.RateLimit(testRedis, maxRequests, time.Minute)(okHandler)

		for i := range 3 {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = "10.0.0.1:12345"
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, r)

			require.Equal(t, http.StatusOK, w.Code, "request %d should succeed", i+1)
		}
	})

	t.Run("blocks requests over limit", func(t *testing.T) {
		t.Cleanup(func() {
			testRedis.FlushDB(context.Background())
		})

		const maxRequests = 5
		handler := middleware.RateLimit(testRedis, maxRequests, time.Minute)(okHandler)

		for i := range maxRequests {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = "10.0.0.2:12345"
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, r)

			require.Equal(t, http.StatusOK, w.Code, "request %d should succeed", i+1)
		}

		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = "10.0.0.2:12345"
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, r)

		assert.Equal(t, http.StatusTooManyRequests, w.Code)
	})

	t.Run("redis error allows request through", func(t *testing.T) {
		handler := middleware.RateLimit(testRedis, 10, time.Minute)(okHandler)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel immediately to cause redis error

		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r = r.WithContext(ctx)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("expire error is logged but request still passes", func(t *testing.T) {
		t.Cleanup(func() {
			testRedis.FlushDB(context.Background())
		})

		// Create a new client with a hook that fails EXPIRE commands
		hookedClient := redis.NewClient(testRedis.Options())
		hookedClient.AddHook(expireFailHook{})
		defer hookedClient.Close()

		handler := middleware.RateLimit(hookedClient, 10, time.Minute)(okHandler)

		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = "10.0.0.99:12345"
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("different IPs have separate limits", func(t *testing.T) {
		t.Cleanup(func() {
			testRedis.FlushDB(context.Background())
		})

		const maxRequests = 5
		handler := middleware.RateLimit(testRedis, maxRequests, time.Minute)(okHandler)

		// Exhaust limit for IP1
		for i := range maxRequests {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = "10.0.0.3:12345"
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, r)

			require.Equal(t, http.StatusOK, w.Code, "IP1 request %d should succeed", i+1)
		}

		// IP1 should be blocked
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = "10.0.0.3:12345"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		require.Equal(t, http.StatusTooManyRequests, w.Code, "IP1 should be blocked")

		// IP2 should still pass
		r = httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = "10.0.0.4:12345"
		w = httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		assert.Equal(t, http.StatusOK, w.Code, "IP2 should still be allowed")
	})
}

// expireFailHook is a redis.Hook that makes EXPIRE commands fail.
type expireFailHook struct{}

func (expireFailHook) DialHook(next redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		return next(ctx, network, addr)
	}
}

func (expireFailHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if strings.EqualFold(cmd.Name(), "expire") {
			return errors.New("injected expire error")
		}
		return next(ctx, cmd)
	}
}

func (expireFailHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		return next(ctx, cmds)
	}
}
