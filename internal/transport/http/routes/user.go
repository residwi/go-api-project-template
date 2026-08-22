package routes

import (
	"github.com/residwi/go-api-project-template/internal/modules/user"
	userhttp "github.com/residwi/go-api-project-template/internal/modules/user/adapter/http"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

func User(authed, admin *middleware.RouteGroup, s *user.Service, v *validator.Validator) {
	h := userhttp.NewHandler(s, v)
	authed.HandleFunc("GET /users/me", h.Me)
	authed.HandleFunc("PUT /users/me", h.Update)

	adminH := userhttp.NewAdminHandler(s, v)
	admin.HandleFunc("GET /users", adminH.List)
	admin.HandleFunc("GET /users/{id}", adminH.Get)
	admin.HandleFunc("PUT /users/{id}", adminH.Update)
	admin.HandleFunc("PUT /users/{id}/role", adminH.UpdateRole)
	admin.HandleFunc("DELETE /users/{id}", adminH.Delete)
}
