package routes

import (
	"github.com/residwi/go-api-project-template/internal/modules/auth"
	authhttp "github.com/residwi/go-api-project-template/internal/modules/auth/adapter/http"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

func Auth(api *middleware.RouteGroup, s *auth.Service, v *validator.Validator) {
	h := authhttp.NewHandler(s, v)
	api.HandleFunc("POST /auth/register", h.Register)
	api.HandleFunc("POST /auth/login", h.Login)
	api.HandleFunc("POST /auth/refresh", h.Refresh)
}
