package middleware

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClientIP(t *testing.T) {
	t.Run("ignores X-Forwarded-For from an untrusted remote", func(t *testing.T) {
		got := captureClientIP(t, nil, func(r *http.Request) {
			r.RemoteAddr = "203.0.113.7:5555"
			r.Header.Set("X-Forwarded-For", "1.2.3.4")
		})

		assert.Equal(t, "203.0.113.7", got)
	})

	t.Run("walks X-Forwarded-For right to left to the first untrusted entry", func(t *testing.T) {
		got := captureClientIP(t, nil, func(r *http.Request) {
			r.RemoteAddr = "10.0.0.1:5555"
			r.Header.Set("X-Forwarded-For", "1.2.3.4, 198.51.100.9, 10.0.0.2")
		})

		assert.Equal(t, "198.51.100.9", got)
	})

	t.Run("falls back to the remote address when every hop is trusted", func(t *testing.T) {
		got := captureClientIP(t, nil, func(r *http.Request) {
			r.RemoteAddr = "10.0.0.1:5555"
			r.Header.Set("X-Forwarded-For", "10.0.0.9, 192.168.1.1")
		})

		assert.Equal(t, "10.0.0.1", got)
	})

	t.Run("normalises an IPv4-mapped IPv6 client entry", func(t *testing.T) {
		got := captureClientIP(t, nil, func(r *http.Request) {
			r.RemoteAddr = "10.0.0.1:5555"
			r.Header.Set("X-Forwarded-For", "::ffff:198.51.100.9")
		})

		assert.Equal(t, "198.51.100.9", got)
	})

	t.Run("matches a trusted prefix written in IPv4-mapped notation", func(t *testing.T) {
		got := captureClientIP(t, nil, func(r *http.Request) {
			r.RemoteAddr = "10.0.0.1:5555"
			r.Header.Set("X-Forwarded-For", "198.51.100.9, ::ffff:10.0.0.2")
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
		got := captureClientIP(t, nil, func(r *http.Request) {
			r.RemoteAddr = "10.0.0.1:5555"
			r.Header.Add("X-Forwarded-For", "198.51.100.9")
			r.Header.Add("X-Forwarded-For", "10.0.0.2")
		})

		assert.Equal(t, "198.51.100.9", got)
	})

	t.Run("uses X-Real-IP when trusted and no X-Forwarded-For is present", func(t *testing.T) {
		got := captureClientIP(t, nil, func(r *http.Request) {
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

func captureClientIP(t *testing.T, trusted []netip.Prefix, prepare func(*http.Request)) string {
	t.Helper()

	var got string
	handler := ClientIP(trusted)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got, _ = clientIPFromContext(r.Context())
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	prepare(r)
	handler.ServeHTTP(httptest.NewRecorder(), r)

	return got
}
