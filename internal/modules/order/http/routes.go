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
	Validator *validator.Validator
	Module    *order.Module
	// nil leaves the write endpoints unthrottled, as the handler tests do.
	WriteLimiter middleware.Middleware
}

// RegisterRoutes mounts every routed slice. transition/ and expire/ and
// recoverstale/ register no route: transition/ is reached only through the
// other slices' ports, and expire/recoverstale are worker-tick sweeps.
func RegisterRoutes(authed, admin *middleware.RouteGroup, deps RouteDeps) {
	placehttp.New(deps.Module.Place, deps.Validator).RegisterHTTP(authed, deps.WriteLimiter)
	queryhttp.New(deps.Module.Query).RegisterHTTP(authed)
	retrypaymenthttp.New(deps.Module.RetryPayment, deps.Validator).RegisterHTTP(authed, deps.WriteLimiter)
	cancelhttp.New(deps.Module.Cancel).RegisterHTTP(authed)

	queryhttp.NewAdmin(deps.Module.Query).RegisterHTTP(admin)
	changestatushttp.NewAdmin(deps.Module.ChangeStatus, deps.Validator).RegisterHTTP(admin)
}
