package http

import (
	"log/slog"

	"github.com/residwi/go-api-project-template/internal/modules/payment"
	queryhttp "github.com/residwi/go-api-project-template/internal/modules/payment/query/http"
	refundhttp "github.com/residwi/go-api-project-template/internal/modules/payment/refund/http"
	webhookhttp "github.com/residwi/go-api-project-template/internal/modules/payment/webhook/http"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

type RouteDeps struct {
	Module *payment.Module
	Logger *slog.Logger
}

// RegisterRoutes mounts every routed slice. charge/ and jobs/ register no
// route: charge is reached only through order's PaymentInitiator port, and
// jobs is drained by the worker and reached by order/cancel's port.
func RegisterRoutes(api, admin *middleware.RouteGroup, deps RouteDeps) {
	webhookhttp.New(deps.Module.Webhook, deps.Logger).RegisterHTTP(api)

	queryhttp.New(deps.Module.Query).RegisterHTTP(admin)
	refundhttp.New(deps.Module.Refund).RegisterHTTP(admin)
}
