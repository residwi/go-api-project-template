package middleware

import (
	"fmt"
	"net/http"

	"github.com/residwi/go-api-project-template/internal/platform/identity"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

func Require(denied string, pred func(identity.Identity) bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, ok := identity.FromContext(r.Context())
			if !ok {
				response.Unauthorized(w, "authentication required")
				return
			}

			if !pred(id) {
				response.Forbidden(w, denied)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func RequireRole(role string) func(http.Handler) http.Handler {
	return Require(fmt.Sprintf("%s access required", role), func(id identity.Identity) bool {
		return id.Role == role
	})
}
