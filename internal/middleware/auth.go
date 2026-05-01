package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/core/response"
)

type UserContext struct {
	UserID       uuid.UUID
	Email        string
	Role         string
	TokenVersion int
}

type userCtxKey struct{}

func SetUserContext(ctx context.Context, uc UserContext) context.Context {
	return context.WithValue(ctx, userCtxKey{}, uc)
}

func GetUserContext(ctx context.Context) (UserContext, bool) {
	uc, ok := ctx.Value(userCtxKey{}).(UserContext)
	return uc, ok
}

type UserStatusResult struct {
	Active       bool
	TokenVersion int
}

type UserStatusChecker interface {
	CheckStatus(ctx context.Context, userID uuid.UUID) (UserStatusResult, error)
}

type TokenClaims struct {
	UserID       uuid.UUID
	Email        string
	Role         string
	Type         string // "access" or "refresh"
	TokenVersion int
}

type TokenValidator interface {
	ValidateToken(tokenString string) (*TokenClaims, error)
}

func Auth(tokenValidator TokenValidator, userStatus UserStatusChecker) Middleware { //nolint:gocognit
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

			ctx := SetUserContext(r.Context(), UserContext{
				UserID:       claims.UserID,
				Email:        claims.Email,
				Role:         claims.Role,
				TokenVersion: claims.TokenVersion,
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
