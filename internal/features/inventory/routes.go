package inventory

import (
	"github.com/residwi/go-api-project-template/internal/middleware"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
)

type RouteDeps struct {
	Validator *validator.Validator
	Service   *Service
}

func RegisterRoutes(adminGroup *middleware.RouteGroup, deps RouteDeps) {
	admin := &adminHandler{service: deps.Service, validator: deps.Validator}

	adminGroup.HandleFunc("GET /inventory/{product_id}", admin.GetStock)
	adminGroup.HandleFunc("PUT /inventory/{product_id}/restock", admin.Restock)
	adminGroup.HandleFunc("PUT /inventory/{product_id}/adjust", admin.Adjust)
}
