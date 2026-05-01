package payment

import (
	"github.com/residwi/go-api-project-template/internal/middleware"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
)

type RouteDeps struct {
	Validator *validator.Validator
	Service   *Service
}

func RegisterRoutes(api *middleware.RouteGroup, admin *middleware.RouteGroup, deps RouteDeps) {
	wh := &webhookHandler{service: deps.Service}
	adm := &adminHandler{service: deps.Service, validator: deps.Validator}

	api.HandleFunc("POST /payments/webhook", wh.HandleWebhook)

	admin.HandleFunc("GET /payments", adm.List)
	admin.HandleFunc("GET /payments/{id}", adm.Get)
	admin.HandleFunc("POST /payments/{id}/refund", adm.Refund)
}
