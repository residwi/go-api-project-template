package review

import (
	"github.com/residwi/go-api-project-template/internal/middleware"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
)

type RouteDeps struct {
	Validator *validator.Validator
	Service   *Service
}

func RegisterRoutes(api *middleware.RouteGroup, authed *middleware.RouteGroup, admin *middleware.RouteGroup, deps RouteDeps) {
	pub := &publicHandler{service: deps.Service, validator: deps.Validator}
	adm := &adminHandler{service: deps.Service}

	api.HandleFunc("GET /products/{id}/reviews", pub.ListByProduct)
	authed.HandleFunc("POST /products/{id}/reviews", pub.Create)
	admin.HandleFunc("DELETE /reviews/{id}", adm.Delete)
}
