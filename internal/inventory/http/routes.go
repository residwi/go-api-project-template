package http

import (
	"github.com/residwi/go-api-project-template/internal/inventory"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

type RouteDeps struct {
	Validator *validator.Validator
	Service   *inventory.Service
}

type handler struct {
	service   *inventory.Service
	validator *validator.Validator
}

func RegisterRoutes(adminGroup *middleware.RouteGroup, deps RouteDeps) {
	h := &handler{service: deps.Service, validator: deps.Validator}

	adminGroup.HandleFunc("GET /inventory/{product_id}", h.GetStock)
	adminGroup.HandleFunc("PUT /inventory/{product_id}/restock", h.Restock)
	adminGroup.HandleFunc("PUT /inventory/{product_id}/adjust", h.Adjust)
}
