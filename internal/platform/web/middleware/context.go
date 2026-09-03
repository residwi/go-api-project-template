package middleware

import (
	"net/http"

	"github.com/residwi/go-api-project-template/internal/platform/identity"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

func RequireUser(w http.ResponseWriter, r *http.Request) (identity.Identity, bool) {
	id, ok := identity.FromContext(r.Context())
	if !ok {
		response.Unauthorized(w, "authentication required")
		return identity.Identity{}, false
	}
	return id, true
}
