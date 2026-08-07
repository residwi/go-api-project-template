package http

import (
	"github.com/residwi/go-api-project-template/internal/modules/shipping"
	createhttp "github.com/residwi/go-api-project-template/internal/modules/shipping/create/http"
	queryhttp "github.com/residwi/go-api-project-template/internal/modules/shipping/query/http"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

type RouteDeps struct {
	Validator *validator.Validator
	Service   *shipping.Service
	Module    *shipping.Module
}

func RegisterRoutes(authed *middleware.RouteGroup, admin *middleware.RouteGroup, deps RouteDeps) {
	queryhttp.New(deps.Module.Query).RegisterHTTP(authed)
	createhttp.New(deps.Module.Create, deps.Validator).RegisterHTTP(admin)

	// Still served by the husk service until tasks 6-7 extract them.
	adm := &adminHandler{service: deps.Service, validator: deps.Validator}
	admin.HandleFunc("PUT /shipments/{id}/tracking", adm.UpdateTracking)
	admin.HandleFunc("POST /shipments/{id}/deliver", adm.MarkDelivered)
}
