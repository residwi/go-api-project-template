package routes

import (
	"log/slog"

	"github.com/residwi/go-api-project-template/internal/modules/payment"
	paymenthttp "github.com/residwi/go-api-project-template/internal/modules/payment/adapter/http"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

func Payment(api, admin *middleware.RouteGroup, s *payment.Service, log *slog.Logger) {
	api.HandleFunc("POST /payments/webhook", paymenthttp.NewHandler(s, log).HandleWebhook)

	adminHandler := paymenthttp.NewAdminHandler(s)
	admin.HandleFunc("GET /payments", adminHandler.List)
	admin.HandleFunc("GET /payments/{id}", adminHandler.Get)
	admin.HandleFunc("POST /payments/{id}/refund", adminHandler.Refund)
}
