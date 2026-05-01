package product

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

	api.HandleFunc("GET /products", pub.List)
	api.HandleFunc("GET /products/{slug}", pub.GetBySlug)

	adminGroup.HandleFunc("POST /products", admin.Create)
	adminGroup.HandleFunc("GET /products", admin.List)
	adminGroup.HandleFunc("GET /products/{id}", admin.Get)
	adminGroup.HandleFunc("PUT /products/{id}", admin.Update)
	adminGroup.HandleFunc("DELETE /products/{id}", admin.Delete)
}
