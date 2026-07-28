package http

import (
	"github.com/residwi/go-api-project-template/internal/category"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

type RouteDeps struct {
	Validator *validator.Validator
	Service   *category.Service
}

type publicHandler struct {
	service   *category.Service
	validator *validator.Validator
}

type adminHandler struct {
	service   *category.Service
	validator *validator.Validator
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
