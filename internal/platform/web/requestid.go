package web

import (
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/platform/logger"
)

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = uuid.New().String()
		}
		w.Header().Set("X-Request-ID", id)
		ctx := logger.WithAttrs(r.Context(), slog.String("request_id", id))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
