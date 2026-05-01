package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/residwi/go-api-project-template/internal/core/response"
)

func RateLimit(rdb *redis.Client, maxRequests int, window time.Duration) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if rdb == nil {
				next.ServeHTTP(w, r)
				return
			}

			ip := r.RemoteAddr
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
