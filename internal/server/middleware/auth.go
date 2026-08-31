package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/auth"
	"github.com/residwi/go-api-project-template/internal/modules/user"
	"github.com/residwi/go-api-project-template/internal/platform/logger"
	"github.com/residwi/go-api-project-template/internal/platform/web"
	webmw "github.com/residwi/go-api-project-template/internal/platform/web/middleware"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

type UserStatusChecker interface {
	CheckStatus(ctx context.Context, userID uuid.UUID) (user.AccountStatus, error)
}

type TokenValidator interface {
	ValidateToken(tokenString string) (auth.ClaimsView, error)
}

//nolint:gocognit // token parse + claims validation + fail-open status-check branches are inherently branchy
func Auth(
	tokenValidator TokenValidator,
	userStatus UserStatusChecker,
) web.Middleware {
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

			claims, err := tokenValidator.ValidateToken(parts[1])
			if err != nil {
				response.Unauthorized(w, "invalid token")
				return
			}

			if claims.Type != "access" {
				response.Unauthorized(w, "invalid token type")
				return
			}

			status, err := userStatus.CheckStatus(r.Context(), claims.UserID)
			if err != nil {
				response.InternalError(w)
				return
			}

			if !status.Active {
				response.Unauthorized(w, "account is deactivated")
				return
			}

			if status.TokenVersion != claims.TokenVersion {
				response.Unauthorized(w, "token has been revoked")
				return
			}

			ctx := webmw.SetUserContext(r.Context(), webmw.UserContext{
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
