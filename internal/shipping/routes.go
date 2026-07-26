package shipping

import (
	"github.com/residwi/go-api-project-template/internal/middleware"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
)

type RouteDeps struct {
	Validator *validator.Validator
	Service   *Service
	Orders    OrderProvider
}

func RegisterRoutes(authed *middleware.RouteGroup, admin *middleware.RouteGroup, deps RouteDeps) {
	pub := &publicHandler{service: deps.Service, orders: deps.Orders}
	adm := &adminHandler{service: deps.Service, validator: deps.Validator}

	authed.HandleFunc("GET /orders/{id}/shipping", pub.GetShipping)

	admin.HandleFunc("POST /orders/{id}/ship", adm.CreateShipment)
	admin.HandleFunc("PUT /shipments/{id}/tracking", adm.UpdateTracking)
	admin.HandleFunc("POST /shipments/{id}/deliver", adm.MarkDelivered)
}
