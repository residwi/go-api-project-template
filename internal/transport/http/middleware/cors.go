package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/residwi/go-api-project-template/internal/platform/config"
)

func CORS(cfg config.CORSConfig) Middleware {
	// Header values are derived from fixed config; compute them once at
	// construction instead of on every request.
	allowMethods := strings.Join(cfg.AllowedMethods, ",")
	allowHeaders := strings.Join(cfg.AllowedHeaders, ",")
	maxAge := strconv.Itoa(cfg.MaxAge)

	allowAll := false
	allowed := make(map[string]bool, len(cfg.AllowedOrigins))
	for _, o := range cfg.AllowedOrigins {
		if o == "*" {
			allowAll = true
		}
		allowed[o] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Allow-Origin takes a single origin or "*", never a comma-joined list, so echo
			// the request's own Origin to make a multi-origin config work.
			origin := r.Header.Get("Origin")
			switch {
			case allowAll:
				w.Header().Set("Access-Control-Allow-Origin", "*")
			case origin != "" && allowed[origin]:
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Add("Vary", "Origin")
			case origin == "" && len(cfg.AllowedOrigins) == 1:
				// Non-browser client (no Origin header) with a single allowed origin.
				w.Header().Set("Access-Control-Allow-Origin", cfg.AllowedOrigins[0])
			}
			w.Header().Set("Access-Control-Allow-Methods", allowMethods)
			w.Header().Set("Access-Control-Allow-Headers", allowHeaders)
			w.Header().Set("Access-Control-Max-Age", maxAge)

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
