package middleware

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"sync"

	"github.com/residwi/go-api-project-template/internal/platform/logger"
)

type clientIPKey struct{}

func ClientIP(log *slog.Logger, trusted []netip.Prefix) func(http.Handler) http.Handler {
	all := trusted

	var warnOnce sync.Once

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := resolveClientIP(r, all)

			if len(all) == 0 {
				warnIfProxied(&warnOnce, log, r)
			}

			ctx := context.WithValue(r.Context(), clientIPKey{}, ip)
			ctx = logger.WithAttrs(ctx, slog.String("client_ip", ip))

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func clientIPFromContext(ctx context.Context) (string, bool) {
	ip, ok := ctx.Value(clientIPKey{}).(string)

	return ip, ok
}

func defaultTrustedPrefixes() []netip.Prefix {
	return []netip.Prefix{
		netip.MustParsePrefix("127.0.0.0/8"),
		netip.MustParsePrefix("::1/128"),
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("172.16.0.0/12"),
		netip.MustParsePrefix("192.168.0.0/16"),
		netip.MustParsePrefix("100.64.0.0/10"),
		netip.MustParsePrefix("169.254.0.0/16"),
		netip.MustParsePrefix("fe80::/10"),
		netip.MustParsePrefix("fc00::/7"),
	}
}

func resolveClientIP(r *http.Request, trusted []netip.Prefix) string {
	remote, ok := remoteAddr(r)
	if !ok {
		return r.RemoteAddr
	}

	if !isTrusted(remote, trusted) {
		return remote.String()
	}

	for _, entry := range xffEntriesRightToLeft(r) {
		addr, err := netip.ParseAddr(entry)
		if err != nil {
			continue
		}

		if addr = addr.Unmap().WithZone(""); !isTrusted(addr, trusted) {
			return addr.String()
		}
	}

	if xrip := strings.TrimSpace(r.Header.Get("X-Real-IP")); xrip != "" {
		if addr, err := netip.ParseAddr(xrip); err == nil {
			return addr.Unmap().WithZone("").String()
		}
	}

	return remote.String()
}

func xffEntriesRightToLeft(r *http.Request) []string {
	var entries []string

	for _, value := range r.Header.Values("X-Forwarded-For") {
		for entry := range strings.SplitSeq(value, ",") {
			if entry = strings.TrimSpace(entry); entry != "" {
				entries = append(entries, entry)
			}
		}
	}

	slices.Reverse(entries)

	return entries
}

func isTrusted(addr netip.Addr, trusted []netip.Prefix) bool {
	return slices.ContainsFunc(trusted, func(prefix netip.Prefix) bool {
		return prefix.Contains(addr)
	})
}

func remoteAddr(r *http.Request) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}

	addr, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, false
	}

	return addr.Unmap().WithZone(""), true
}

func warnIfProxied(once *sync.Once, log *slog.Logger, r *http.Request) {
	if r.Header.Get("X-Forwarded-For") == "" && r.Header.Get("X-Real-IP") == "" {
		return
	}

	remote, ok := remoteAddr(r)
	if !ok || !isTrusted(remote, defaultTrustedPrefixes()) {
		return
	}

	once.Do(func() {
		log.WarnContext(
			r.Context(),
			"forwarding headers received from an internal peer but TRUSTED_PROXIES is empty; they are ignored, so every client behind the proxy shares one rate-limit bucket. Set TRUSTED_PROXIES to the proxy CIDR.",
			slog.String("peer", r.RemoteAddr),
		)
	})
}
