package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/platform/web/response"
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

func RequireUser(w http.ResponseWriter, r *http.Request) (UserContext, bool) {
	uc, ok := GetUserContext(r.Context())
	if !ok {
		response.Unauthorized(w, "authentication required")
		return UserContext{}, false
	}
	return uc, true
}
