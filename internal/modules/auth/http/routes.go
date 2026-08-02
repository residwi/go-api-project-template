package http

import (
	"github.com/residwi/go-api-project-template/internal/modules/auth"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

type RouteDeps struct {
	Validator *validator.Validator
	Service   *auth.Service
}

func RegisterRoutes(api *middleware.RouteGroup, deps RouteDeps) {
	h := &handler{service: deps.Service, validator: deps.Validator}
	api.HandleFunc("POST /auth/register", h.Register)
	api.HandleFunc("POST /auth/login", h.Login)
	api.HandleFunc("POST /auth/refresh", h.RefreshToken)
}
