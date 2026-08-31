package middleware

import (
	"fmt"
	"net/http"

	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

func RequireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			uc, ok := GetUserContext(r.Context())
			if !ok {
				response.Unauthorized(w, "authentication required")
				return
			}

			if uc.Role != role {
				response.Forbidden(w, fmt.Sprintf("%s access required", role))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
