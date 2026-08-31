package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/residwi/go-api-project-template/internal/platform/config"
)

func CORS(cfg config.CORS) func(http.Handler) http.Handler {
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
			origin := r.Header.Get("Origin")
			switch {
			case allowAll:
				w.Header().Set("Access-Control-Allow-Origin", "*")
			case origin != "" && allowed[origin]:
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Add("Vary", "Origin")
			case origin == "" && len(cfg.AllowedOrigins) == 1:
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
