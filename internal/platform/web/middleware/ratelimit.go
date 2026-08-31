package middleware

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

func RateLimit(
	log *slog.Logger,
	rdb *redis.Client,
	maxRequests int,
	window time.Duration,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if rdb == nil || maxRequests <= 0 || window <= 0 {
				next.ServeHTTP(w, r)
				return
			}

			identifier := clientIP(r)
			if uc, ok := GetUserContext(r.Context()); ok {
				identifier = "user:" + uc.UserID.String()
			}
			bucket := time.Now().Unix() / int64(window.Seconds())
			key := fmt.Sprintf("rl:%s:%d", identifier, bucket)

			count, err := rdb.Incr(r.Context(), key).Result()
			if err != nil {
				log.WarnContext(
					r.Context(),
					"rate limit redis error, allowing request",
					slog.String("error", err.Error()),
				)
				next.ServeHTTP(w, r)
				return
			}

			if count == 1 {
				if err := rdb.Expire(r.Context(), key, window+5*time.Second).Err(); err != nil {
					log.WarnContext(r.Context(), "rate limit expire error", slog.String("error", err.Error()))
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

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
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
