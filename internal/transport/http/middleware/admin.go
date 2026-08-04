package middleware

import (
	"net/http"

	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uc, ok := GetUserContext(r.Context())
		if !ok {
			response.Unauthorized(w, "authentication required")
			return
		}

		if uc.Role != "admin" { //nolint:goconst // shared with role fixtures in this package's now in-package tests; a role constant isn't worth it for one comparison
			response.Forbidden(w, "admin access required")
			return
		}

		next.ServeHTTP(w, r)
	})
}
