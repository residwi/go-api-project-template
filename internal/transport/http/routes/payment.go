package routes

import (
	"log/slog"

	"github.com/residwi/go-api-project-template/internal/modules/payment"
	queryhttp "github.com/residwi/go-api-project-template/internal/modules/payment/usecase/query/http"
	refundhttp "github.com/residwi/go-api-project-template/internal/modules/payment/usecase/refund/http"
	webhookhttp "github.com/residwi/go-api-project-template/internal/modules/payment/usecase/webhook/http"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

func Payment(api, admin *middleware.RouteGroup, m *payment.Module, log *slog.Logger) {
	api.HandleFunc("POST /payments/webhook", webhookhttp.New(m.Webhook, log).HandleWebhook)

	query := queryhttp.New(m.Query)
	admin.HandleFunc("GET /payments", query.List)
	admin.HandleFunc("GET /payments/{id}", query.Get)

	admin.HandleFunc("POST /payments/{id}/refund", refundhttp.New(m.Refund).Refund)
}
