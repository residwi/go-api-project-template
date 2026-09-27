package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/residwi/go-api-project-template/internal/platform/identity"
	"github.com/residwi/go-api-project-template/internal/platform/logger"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

type Authenticator interface {
	Authenticate(ctx context.Context, token string) (identity.Identity, error)
}

func Auth(log *slog.Logger, authenticator Authenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				log.DebugContext(r.Context(), "authentication failed",
					slog.String("reason", "missing_header"), slog.String("path", r.URL.Path))
				response.Unauthorized(w, "missing authorization header")
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
				log.WarnContext(r.Context(), "authentication failed",
					slog.String("reason", "malformed_header"), slog.String("path", r.URL.Path))
				response.Unauthorized(w, "invalid authorization header format")
				return
			}

			id, err := authenticator.Authenticate(r.Context(), parts[1])
			if err != nil {
				log.WarnContext(r.Context(), "authentication failed",
					slog.String("reason", "invalid_token"), slog.String("path", r.URL.Path))
				response.HandleErr(w, err)
				return
			}

			ctx := identity.NewContext(r.Context(), id)
			ctx = logger.WithAttrs(ctx, slog.String("user_id", id.UserID.String()))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
