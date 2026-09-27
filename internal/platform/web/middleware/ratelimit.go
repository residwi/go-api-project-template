package middleware

import (
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"sync"
	"time"

	"github.com/go-redis/redis_rate/v10"
	"github.com/redis/go-redis/v9"

	"github.com/residwi/go-api-project-template/internal/platform/identity"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

const ipv6RateLimitPrefixBits = 64

const fallbackMaxKeys = 10_000

func RateLimit( //nolint:gocognit // resolves the caller identifier (context IP, else remote addr with the port stripped, masked to its IPv6 /64), then overrides it for an authenticated caller
	log *slog.Logger,
	rdb *redis.Client,
	keyspace string,
	maxRequests, burst int,
	window time.Duration,
) func(http.Handler) http.Handler {
	var limiter *redis_rate.Limiter
	var fallback *fallbackLimiter
	if rdb != nil {
		limiter = redis_rate.NewLimiter(rdb)
		fallback = newFallbackLimiter(maxRequests, window)
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

			key := keyspace + ":" + identifier

			res, err := limiter.Allow(r.Context(), key, redis_rate.Limit{
				Rate:   maxRequests,
				Burst:  burst,
				Period: window,
			})
			if err != nil {
				allowed, saturated := fallback.allow(key, time.Now())
				if saturated {
					log.WarnContext(
						r.Context(),
						"rate limit redis error and fallback saturated, charging new callers to the shared overflow budget",
						slog.String("error", err.Error()),
					)
				}
				if !allowed {
					response.TooManyRequests(w, "rate limit exceeded")

					return
				}
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

type fallbackWindow struct {
	count   int
	resetAt time.Time
}

type fallbackLimiter struct {
	mu          sync.Mutex
	keys        map[string]*fallbackWindow
	overflow    fallbackWindow
	lastReclaim time.Time
	limit       int
	window      time.Duration
}

func newFallbackLimiter(limit int, window time.Duration) *fallbackLimiter {
	return &fallbackLimiter{
		keys:   make(map[string]*fallbackWindow),
		limit:  limit,
		window: window,
	}
}

// allow is the in-process fixed-window fallback consulted only when Redis (the real
// limiter) errors, so a limiter outage still throttles a brute-force flood (CWE-636).
// Once the map holds fallbackMaxKeys live buckets, every untracked caller is charged
// to one shared overflow window capped at the map's own total capacity: saturation
// is exactly the shape of a distributed flood, so it must degrade to a coarse limit,
// not to none. saturated is true only for the first overflow request of each window,
// so the caller can log saturation once instead of once per request.
func (l *fallbackLimiter) allow(key string, now time.Time) (allowed, saturated bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if w, ok := l.keys[key]; ok {
		if now.After(w.resetAt) {
			w.count = 0
			w.resetAt = now.Add(l.window)
		}
		if w.count >= l.limit {
			return false, false
		}
		w.count++

		return true, false
	}

	if len(l.keys) >= fallbackMaxKeys && now.Sub(l.lastReclaim) >= l.window {
		l.reclaimExpired(now)
		l.lastReclaim = now
	}

	if len(l.keys) >= fallbackMaxKeys {
		return l.allowOverflow(now)
	}

	l.keys[key] = &fallbackWindow{count: 1, resetAt: now.Add(l.window)}

	return true, false
}

func (l *fallbackLimiter) allowOverflow(now time.Time) (allowed, saturated bool) {
	if now.After(l.overflow.resetAt) {
		l.overflow = fallbackWindow{resetAt: now.Add(l.window)}
	}
	saturated = l.overflow.count == 0
	if l.overflow.count >= l.limit*fallbackMaxKeys {
		return false, saturated
	}
	l.overflow.count++

	return true, saturated
}

func (l *fallbackLimiter) reclaimExpired(now time.Time) {
	for k, w := range l.keys {
		if now.After(w.resetAt) {
			delete(l.keys, k)
		}
	}
}
