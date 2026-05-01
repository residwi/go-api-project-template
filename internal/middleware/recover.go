package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/residwi/go-api-project-template/internal/core/response"
)

func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				slog.ErrorContext(r.Context(), "panic recovered",
					"error", err,
					"stack", string(debug.Stack()),
					"request_id", GetRequestID(r.Context()),
				)
				response.InternalError(w)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
