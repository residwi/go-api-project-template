package middleware

import (
	"net/http"
	"strconv"
	"strings"
)

type CORSOptions struct {
	AllowedOrigins []string
	AllowedMethods []string
	AllowedHeaders []string
	MaxAge         int
}

func CORS(opts CORSOptions) func(http.Handler) http.Handler {
	allowMethods := strings.Join(opts.AllowedMethods, ",")
	allowHeaders := strings.Join(opts.AllowedHeaders, ",")
	maxAge := strconv.Itoa(opts.MaxAge)

	allowAll := false
	allowed := make(map[string]bool, len(opts.AllowedOrigins))
	for _, o := range opts.AllowedOrigins {
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
			case origin == "" && len(opts.AllowedOrigins) == 1:
				w.Header().Set("Access-Control-Allow-Origin", opts.AllowedOrigins[0])
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
