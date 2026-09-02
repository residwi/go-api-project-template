package server

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/residwi/go-api-project-template/internal/features/auth"
	"github.com/residwi/go-api-project-template/internal/platform/logger"
	"github.com/residwi/go-api-project-template/internal/platform/web"
	"github.com/residwi/go-api-project-template/internal/platform/web/middleware"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

type Authenticator interface {
	Authenticate(ctx context.Context, token string) (auth.ClaimsView, error)
}

func authMiddleware(authenticator Authenticator) web.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				response.Unauthorized(w, "missing authorization header")
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
				response.Unauthorized(w, "invalid authorization header format")
				return
			}

			claims, err := authenticator.Authenticate(r.Context(), parts[1])
			if err != nil {
				response.HandleErr(w, err)
				return
			}

			ctx := middleware.SetUserContext(r.Context(), middleware.UserContext{
				UserID:       claims.UserID,
				Email:        claims.Email,
				Role:         claims.Role,
				TokenVersion: claims.TokenVersion,
			})
			ctx = logger.WithAttrs(ctx, slog.String("user_id", claims.UserID.String()))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
