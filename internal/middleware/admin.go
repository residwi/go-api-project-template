package middleware

import (
	"net/http"

	"github.com/residwi/go-api-project-template/internal/core/response"
)

func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uc, ok := GetUserContext(r.Context())
		if !ok {
			response.Unauthorized(w, "authentication required")
			return
		}

		if uc.Role != "admin" {
			response.Forbidden(w, "admin access required")
			return
		}

		next.ServeHTTP(w, r)
	})
}
