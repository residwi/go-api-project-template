package order

import (
	"github.com/residwi/go-api-project-template/internal/middleware"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
)

type RouteDeps struct {
	Validator *validator.Validator
	Service   *Service
}

func RegisterRoutes(authed *middleware.RouteGroup, adminGroup *middleware.RouteGroup, deps RouteDeps) {
	pub := &publicHandler{service: deps.Service, validator: deps.Validator}
	admin := &adminHandler{service: deps.Service, validator: deps.Validator}

	authed.HandleFunc("POST /orders", pub.PlaceOrder)
	authed.HandleFunc("GET /orders", pub.ListOrders)
	authed.HandleFunc("GET /orders/{id}", pub.GetOrder)
	authed.HandleFunc("POST /orders/{id}/pay", pub.RetryPayment)
	authed.HandleFunc("POST /orders/{id}/cancel", pub.CancelOrder)

	adminGroup.HandleFunc("GET /orders", admin.List)
	adminGroup.HandleFunc("GET /orders/{id}", admin.Get)
	adminGroup.HandleFunc("PUT /orders/{id}/status", admin.UpdateStatus)
}
