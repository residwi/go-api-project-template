package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClientIP(t *testing.T) {
	t.Run("ignores X-Forwarded-For when no proxies are configured", func(t *testing.T) {
		got := captureClientIP(t, nil, func(r *http.Request) {
			r.RemoteAddr = "10.0.0.1:5555"
			r.Header.Set("X-Forwarded-For", "1.2.3.4")
		})

		assert.Equal(t, "10.0.0.1", got)
	})

	t.Run("ignores X-Real-IP when no proxies are configured", func(t *testing.T) {
		got := captureClientIP(t, nil, func(r *http.Request) {
			r.RemoteAddr = "10.0.0.1:5555"
			r.Header.Set("X-Real-IP", "1.2.3.4")
		})

		assert.Equal(t, "10.0.0.1", got)
	})

	t.Run("ignores X-Forwarded-For from an untrusted remote", func(t *testing.T) {
		got := captureClientIP(t, nil, func(r *http.Request) {
			r.RemoteAddr = "203.0.113.7:5555"
			r.Header.Set("X-Forwarded-For", "1.2.3.4")
		})

		assert.Equal(t, "203.0.113.7", got)
	})

	t.Run("walks X-Forwarded-For right to left to the first untrusted entry", func(t *testing.T) {
		got := captureClientIP(t, trustedProxies(), func(r *http.Request) {
			r.RemoteAddr = "10.0.0.1:5555"
			r.Header.Set("X-Forwarded-For", "1.2.3.4, 198.51.100.9, 10.0.0.2")
		})

		assert.Equal(t, "198.51.100.9", got)
	})

	t.Run("falls back to the remote address when every hop is trusted", func(t *testing.T) {
		got := captureClientIP(t, trustedProxies(), func(r *http.Request) {
			r.RemoteAddr = "10.0.0.1:5555"
			r.Header.Set("X-Forwarded-For", "10.0.0.9, 10.0.0.8")
		})

		assert.Equal(t, "10.0.0.1", got)
	})

	t.Run("does not trust private ranges the operator did not list", func(t *testing.T) {
		got := captureClientIP(t, trustedProxies(), func(r *http.Request) {
			r.RemoteAddr = "10.0.0.1:5555"
			r.Header.Set("X-Forwarded-For", "1.2.3.4, 192.168.1.1")
		})

		assert.Equal(t, "192.168.1.1", got)
	})

	t.Run("does not trust loopback unless it is listed", func(t *testing.T) {
		got := captureClientIP(t, trustedProxies(), func(r *http.Request) {
			r.RemoteAddr = "127.0.0.1:5555"
			r.Header.Set("X-Forwarded-For", "1.2.3.4")
		})

		assert.Equal(t, "127.0.0.1", got)
	})

	t.Run("normalises an IPv4-mapped IPv6 client entry", func(t *testing.T) {
		got := captureClientIP(t, trustedProxies(), func(r *http.Request) {
			r.RemoteAddr = "10.0.0.1:5555"
			r.Header.Set("X-Forwarded-For", "::ffff:198.51.100.9")
		})

		assert.Equal(t, "198.51.100.9", got)
	})

	t.Run("matches a trusted prefix written in IPv4-mapped notation", func(t *testing.T) {
		got := captureClientIP(t, trustedProxies(), func(r *http.Request) {
			r.RemoteAddr = "10.0.0.1:5555"
			r.Header.Set("X-Forwarded-For", "198.51.100.9, ::ffff:10.0.0.2")
		})

		assert.Equal(t, "198.51.100.9", got)
	})

	t.Run("matches a trusted prefix through a zoned address", func(t *testing.T) {
		trusted := []netip.Prefix{netip.MustParsePrefix("2001:db8::/32")}

		got := captureClientIP(t, trusted, func(r *http.Request) {
			r.RemoteAddr = "[2001:db8::1]:5555"
			r.Header.Set("X-Forwarded-For", "198.51.100.9, 2001:db8::2%eth0")
		})

		assert.Equal(t, "198.51.100.9", got)
	})

	t.Run("honours an explicitly trusted public CIDR", func(t *testing.T) {
		trusted := []netip.Prefix{netip.MustParsePrefix("203.0.113.0/24")}

		got := captureClientIP(t, trusted, func(r *http.Request) {
			r.RemoteAddr = "203.0.113.7:5555"
			r.Header.Set("X-Forwarded-For", "1.2.3.4, 203.0.113.8")
		})

		assert.Equal(t, "1.2.3.4", got)
	})

	t.Run("flattens repeated X-Forwarded-For header lines", func(t *testing.T) {
		got := captureClientIP(t, trustedProxies(), func(r *http.Request) {
			r.RemoteAddr = "10.0.0.1:5555"
			r.Header.Add("X-Forwarded-For", "10.0.0.2")
			r.Header.Add("X-Forwarded-For", "198.51.100.9")
		})

		assert.Equal(t, "198.51.100.9", got)
	})

	t.Run("uses X-Real-IP when trusted and no X-Forwarded-For is present", func(t *testing.T) {
		got := captureClientIP(t, trustedProxies(), func(r *http.Request) {
			r.RemoteAddr = "10.0.0.1:5555"
			r.Header.Set("X-Real-IP", "198.51.100.9")
		})

		assert.Equal(t, "198.51.100.9", got)
	})

	t.Run("ignores X-Real-IP from an untrusted remote", func(t *testing.T) {
		got := captureClientIP(t, nil, func(r *http.Request) {
			r.RemoteAddr = "203.0.113.7:5555"
			r.Header.Set("X-Real-IP", "1.2.3.4")
		})

		assert.Equal(t, "203.0.113.7", got)
	})

	t.Run("passes an unparseable remote address through", func(t *testing.T) {
		got := captureClientIP(t, nil, func(r *http.Request) {
			r.RemoteAddr = "not-an-address"
		})

		assert.Equal(t, "not-an-address", got)
	})
}

func TestClientIPMisconfiguredProxyWarning(t *testing.T) {
	t.Run(
		"warns once when forwarding headers arrive from an internal peer and no proxies are configured",
		func(t *testing.T) {
			var buf bytes.Buffer
			log := slog.New(slog.NewJSONHandler(&buf, nil))

			handler := ClientIP(log, nil)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
			for range 3 {
				r := httptest.NewRequest(http.MethodGet, "/", nil)
				r.RemoteAddr = "10.0.0.1:5555"
				r.Header.Set("X-Forwarded-For", "1.2.3.4")
				handler.ServeHTTP(httptest.NewRecorder(), r)
			}

			assert.Equal(t, 1, strings.Count(buf.String(), "TRUSTED_PROXIES is empty"))
		},
	)

	t.Run("does not warn for a direct public peer that carries a forwarding header", func(t *testing.T) {
		var buf bytes.Buffer
		log := slog.New(slog.NewJSONHandler(&buf, nil))

		handler := ClientIP(log, nil)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = "203.0.113.7:5555"
		r.Header.Set("X-Forwarded-For", "1.2.3.4")
		handler.ServeHTTP(httptest.NewRecorder(), r)

		assert.Empty(t, buf.String())
	})

	t.Run("does not warn when proxies are configured", func(t *testing.T) {
		var buf bytes.Buffer
		log := slog.New(slog.NewJSONHandler(&buf, nil))

		handler := ClientIP(log, trustedProxies())(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = "10.0.0.1:5555"
		r.Header.Set("X-Forwarded-For", "1.2.3.4")
		handler.ServeHTTP(httptest.NewRecorder(), r)

		assert.Empty(t, buf.String())
	})
}

func captureClientIP(t *testing.T, trusted []netip.Prefix, prepare func(*http.Request)) string {
	t.Helper()

	var got string
	handler := ClientIP(testLogger(), trusted)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got, _ = clientIPFromContext(r.Context())
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	prepare(r)
	handler.ServeHTTP(httptest.NewRecorder(), r)

	return got
}

func trustedProxies() []netip.Prefix {
	return []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
}
