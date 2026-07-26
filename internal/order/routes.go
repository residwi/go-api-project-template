package order

import (
	"net/http"

	"github.com/residwi/go-api-project-template/internal/middleware"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
)

type RouteDeps struct {
	Validator *validator.Validator
	Service   *Service
	// WriteLimiter throttles the expensive write endpoints (placement, payment
	// retry); nil leaves them unthrottled (e.g. in handler tests).
	WriteLimiter middleware.Middleware
}

func RegisterRoutes(authed *middleware.RouteGroup, adminGroup *middleware.RouteGroup, deps RouteDeps) {
	pub := &publicHandler{service: deps.Service, validator: deps.Validator}
	admin := &adminHandler{service: deps.Service, validator: deps.Validator}

	// Throttle only the costly write endpoints; listing/get/cancel stay unthrottled.
	limit := func(h http.HandlerFunc) http.Handler {
		if deps.WriteLimiter != nil {
			return deps.WriteLimiter(h)
		}
		return h
	}
	authed.Handle("POST /orders", limit(pub.PlaceOrder))
	authed.HandleFunc("GET /orders", pub.ListOrders)
	authed.HandleFunc("GET /orders/{id}", pub.GetOrder)
	authed.Handle("POST /orders/{id}/pay", limit(pub.RetryPayment))
	authed.HandleFunc("POST /orders/{id}/cancel", pub.CancelOrder)

	adminGroup.HandleFunc("GET /orders", admin.List)
	adminGroup.HandleFunc("GET /orders/{id}", admin.Get)
	adminGroup.HandleFunc("PUT /orders/{id}/status", admin.UpdateStatus)
}
