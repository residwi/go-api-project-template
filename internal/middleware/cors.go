package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/residwi/go-api-project-template/internal/config"
)

func CORS(cfg config.CORSConfig) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", strings.Join(cfg.AllowedOrigins, ","))
			w.Header().Set("Access-Control-Allow-Methods", strings.Join(cfg.AllowedMethods, ","))
			w.Header().Set("Access-Control-Allow-Headers", strings.Join(cfg.AllowedHeaders, ","))
			w.Header().Set("Access-Control-Max-Age", strconv.Itoa(cfg.MaxAge))

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
