package middleware

import (
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"time"

	"github.com/go-redis/redis_rate/v10"
	"github.com/redis/go-redis/v9"

	"github.com/residwi/go-api-project-template/internal/platform/identity"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

const ipv6RateLimitPrefixBits = 64

func RateLimit( //nolint:gocognit // resolves the caller identifier (context IP, else remote addr with the port stripped, masked to its IPv6 /64), then overrides it for an authenticated caller
	log *slog.Logger,
	rdb *redis.Client,
	maxRequests, burst int,
	window time.Duration,
) func(http.Handler) http.Handler {
	var limiter *redis_rate.Limiter
	if rdb != nil {
		limiter = redis_rate.NewLimiter(rdb)
	}

	burst = min(burst, maxRequests)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if limiter == nil || maxRequests <= 0 || burst <= 0 || window <= 0 {
				next.ServeHTTP(w, r)
				return
			}

			identifier, ok := clientIPFromContext(r.Context())
			if !ok {
				identifier = r.RemoteAddr
				if host, _, err := net.SplitHostPort(identifier); err == nil {
					identifier = host
				}
			}
			if addr, err := netip.ParseAddr(identifier); err == nil && addr.Is6() && !addr.Is4In6() {
				if prefix, err := addr.Prefix(ipv6RateLimitPrefixBits); err == nil {
					identifier = prefix.String()
				}
			}
			if id, ok := identity.FromContext(r.Context()); ok {
				identifier = "user:" + id.UserID.String()
			}

			res, err := limiter.Allow(r.Context(), identifier, redis_rate.Limit{
				Rate:   maxRequests,
				Burst:  burst,
				Period: window,
			})
			if err != nil {
				log.WarnContext(
					r.Context(),
					"rate limit redis error, allowing request",
					slog.String("error", err.Error()),
				)
				next.ServeHTTP(w, r)

				return
			}

			w.Header().Set("X-Ratelimit-Limit", strconv.Itoa(maxRequests))
			w.Header().Set("X-Ratelimit-Remaining", strconv.Itoa(res.Remaining))
			w.Header().Set(
				"X-Ratelimit-Reset",
				strconv.FormatInt(time.Now().Add(res.ResetAfter).Unix(), 10),
			)

			if res.Allowed == 0 {
				w.Header().Set(
					"Retry-After",
					strconv.Itoa(int(math.Ceil(res.RetryAfter.Seconds()))),
				)
				response.TooManyRequests(w, "rate limit exceeded")

				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
