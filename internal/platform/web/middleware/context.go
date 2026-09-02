package middleware

import (
	"context"
	"net/http"

	"github.com/residwi/go-api-project-template/internal/platform/identity"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

type identityCtxKey struct{}

func SetIdentity(ctx context.Context, id identity.Identity) context.Context {
	return context.WithValue(ctx, identityCtxKey{}, id)
}

func GetIdentity(ctx context.Context) (identity.Identity, bool) {
	id, ok := ctx.Value(identityCtxKey{}).(identity.Identity)
	return id, ok
}

func RequireUser(w http.ResponseWriter, r *http.Request) (identity.Identity, bool) {
	id, ok := GetIdentity(r.Context())
	if !ok {
		response.Unauthorized(w, "authentication required")
		return identity.Identity{}, false
	}
	return id, true
}
