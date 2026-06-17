package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/residwi/go-api-project-template/internal/core/response"
)

func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := &recoverWriter{ResponseWriter: w}
		defer func() {
			if err := recover(); err != nil {
				slog.ErrorContext(r.Context(), "panic recovered",
					"error", err,
					"stack", string(debug.Stack()),
					"request_id", GetRequestID(r.Context()),
				)
				// Only emit a 500 if the handler hadn't already started writing the
				// response; otherwise we'd produce a superfluous WriteHeader and a
				// corrupt, double-encoded body.
				if !rw.wrote {
					response.InternalError(rw)
				}
			}
		}()
		next.ServeHTTP(rw, r)
	})
}

// recoverWriter tracks whether the response has been started so Recovery can
// avoid writing a second (corrupting) response after a mid-write panic.
type recoverWriter struct {
	http.ResponseWriter

	wrote bool
}

func (rw *recoverWriter) WriteHeader(code int) {
	rw.wrote = true
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *recoverWriter) Write(b []byte) (int, error) {
	rw.wrote = true
	return rw.ResponseWriter.Write(b)
}
