package http

import (
	"github.com/residwi/go-api-project-template/internal/modules/shipping"
	createhttp "github.com/residwi/go-api-project-template/internal/modules/shipping/create/http"
	deliverhttp "github.com/residwi/go-api-project-template/internal/modules/shipping/deliver/http"
	queryhttp "github.com/residwi/go-api-project-template/internal/modules/shipping/query/http"
	updatetrackinghttp "github.com/residwi/go-api-project-template/internal/modules/shipping/updatetracking/http"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

type RouteDeps struct {
	Validator *validator.Validator
	Module    *shipping.Module
}

func RegisterRoutes(authed *middleware.RouteGroup, admin *middleware.RouteGroup, deps RouteDeps) {
	queryhttp.New(deps.Module.Query).RegisterHTTP(authed)
	createhttp.New(deps.Module.Create, deps.Validator).RegisterHTTP(admin)
	updatetrackinghttp.New(deps.Module.UpdateTracking, deps.Validator).RegisterHTTP(admin)
	deliverhttp.New(deps.Module.Deliver).RegisterHTTP(admin)
}
