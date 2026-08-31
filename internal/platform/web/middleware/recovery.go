package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

func Recovery(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rw := &recoveryWriter{ResponseWriter: w}
			defer func() {
				if err := recover(); err != nil {
					log.ErrorContext(
						r.Context(), "panic recovered",
						slog.String("panic", fmt.Sprint(err)),
						slog.String("stack", string(debug.Stack())),
					)
					if !rw.wrote {
						response.InternalError(rw)
					}
				}
			}()
			next.ServeHTTP(rw, r)
		})
	}
}

type recoveryWriter struct {
	http.ResponseWriter

	wrote bool
}

func (rw *recoveryWriter) WriteHeader(code int) {
	rw.wrote = true
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *recoveryWriter) Write(b []byte) (int, error) {
	rw.wrote = true
	return rw.ResponseWriter.Write(b)
}
