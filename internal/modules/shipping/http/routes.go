package http

import (
	"github.com/residwi/go-api-project-template/internal/modules/shipping"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

type RouteDeps struct {
	Validator *validator.Validator
	Service   *shipping.Service
	Orders    shipping.OrderProvider
}

func RegisterRoutes(authed *middleware.RouteGroup, admin *middleware.RouteGroup, deps RouteDeps) {
	pub := &handler{service: deps.Service, orders: deps.Orders}
	adm := &adminHandler{service: deps.Service, validator: deps.Validator}

	authed.HandleFunc("GET /orders/{id}/shipping", pub.GetShipping)

	admin.HandleFunc("POST /orders/{id}/ship", adm.CreateShipment)
	admin.HandleFunc("PUT /shipments/{id}/tracking", adm.UpdateTracking)
	admin.HandleFunc("POST /shipments/{id}/deliver", adm.MarkDelivered)
}
