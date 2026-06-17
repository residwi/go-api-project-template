package middleware

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/residwi/go-api-project-template/internal/core/response"
)

// clientIP resolves the per-client identifier for rate limiting. It strips the
// ephemeral source port from RemoteAddr (otherwise every TCP connection gets a
// distinct bucket and the limit is bypassed) and honours X-Forwarded-For /
// X-Real-IP. NOTE: the forwarded headers are only trustworthy when the service
// runs behind a proxy that sets them; expose this app directly and a client can
// spoof them.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Leftmost entry is the original client when set by a trusted proxy.
		first, _, _ := strings.Cut(xff, ",")
		return strings.TrimSpace(first)
	}
	if xrip := strings.TrimSpace(r.Header.Get("X-Real-IP")); xrip != "" {
		return xrip
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func RateLimit(rdb *redis.Client, maxRequests int, window time.Duration) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Disabled (no Redis) or misconfigured (non-positive limit/window):
			// fail open rather than panic on the bucket division below.
			if rdb == nil || maxRequests <= 0 || window <= 0 {
				next.ServeHTTP(w, r)
				return
			}

			ip := clientIP(r)
			bucket := time.Now().Unix() / int64(window.Seconds())
			key := fmt.Sprintf("rl:%s:%d", ip, bucket)

			count, err := rdb.Incr(r.Context(), key).Result()
			if err != nil {
				slog.WarnContext(r.Context(), "rate limit redis error, allowing request", "error", err)
				next.ServeHTTP(w, r)
				return
			}

			if count == 1 {
				if err := rdb.Expire(r.Context(), key, window+5*time.Second).Err(); err != nil {
					slog.WarnContext(r.Context(), "rate limit expire error", "error", err)
				}
			}

			if count > int64(maxRequests) {
				response.TooManyRequests(w, "rate limit exceeded")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
