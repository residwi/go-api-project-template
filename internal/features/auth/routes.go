package auth

import (
	"github.com/residwi/go-api-project-template/internal/middleware"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
)

type RouteDeps struct {
	Validator *validator.Validator
	Service   *Service
}

func RegisterRoutes(api *middleware.RouteGroup, deps RouteDeps) {
	h := NewHandler(deps.Service, deps.Validator)
	api.HandleFunc("POST /auth/register", h.Register)
	api.HandleFunc("POST /auth/login", h.Login)
	api.HandleFunc("POST /auth/refresh", h.RefreshToken)
}
