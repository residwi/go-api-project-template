package http

import (
	"github.com/residwi/go-api-project-template/internal/modules/order"
	cancelhttp "github.com/residwi/go-api-project-template/internal/modules/order/cancel/http"
	changestatushttp "github.com/residwi/go-api-project-template/internal/modules/order/changestatus/http"
	placehttp "github.com/residwi/go-api-project-template/internal/modules/order/place/http"
	queryhttp "github.com/residwi/go-api-project-template/internal/modules/order/query/http"
	retrypaymenthttp "github.com/residwi/go-api-project-template/internal/modules/order/retrypayment/http"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

type RouteDeps struct {
	Validator    *validator.Validator
	Module       *order.Module
	WriteLimiter middleware.Middleware
}

func RegisterRoutes(authed, admin *middleware.RouteGroup, deps RouteDeps) {
	placehttp.New(deps.Module.Place, deps.Validator).RegisterHTTP(authed, deps.WriteLimiter)
	queryhttp.New(deps.Module.Query).RegisterHTTP(authed)
	retrypaymenthttp.New(deps.Module.RetryPayment, deps.Validator).RegisterHTTP(authed, deps.WriteLimiter)
	cancelhttp.New(deps.Module.Cancel).RegisterHTTP(authed)

	queryhttp.NewAdmin(deps.Module.Query).RegisterHTTP(admin)
	changestatushttp.NewAdmin(deps.Module.ChangeStatus, deps.Validator).RegisterHTTP(admin)
}
