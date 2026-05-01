package category

import (
	"github.com/residwi/go-api-project-template/internal/middleware"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
)

type RouteDeps struct {
	Validator *validator.Validator
	Service   *Service
}

func RegisterRoutes(api *middleware.RouteGroup, adminGroup *middleware.RouteGroup, deps RouteDeps) {
	pub := &publicHandler{service: deps.Service, validator: deps.Validator}
	admin := &adminHandler{service: deps.Service, validator: deps.Validator}

	api.HandleFunc("GET /categories", pub.List)
	api.HandleFunc("GET /categories/{slug}", pub.GetBySlug)

	adminGroup.HandleFunc("POST /categories", admin.Create)
	adminGroup.HandleFunc("PUT /categories/{id}", admin.Update)
	adminGroup.HandleFunc("DELETE /categories/{id}", admin.Delete)
}
