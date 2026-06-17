package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/residwi/go-api-project-template/internal/config"
)

func CORS(cfg config.CORSConfig) Middleware {
	// Header values are derived from fixed config; compute them once at
	// construction instead of on every request.
	allowOrigins := strings.Join(cfg.AllowedOrigins, ",")
	allowMethods := strings.Join(cfg.AllowedMethods, ",")
	allowHeaders := strings.Join(cfg.AllowedHeaders, ",")
	maxAge := strconv.Itoa(cfg.MaxAge)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", allowOrigins)
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
