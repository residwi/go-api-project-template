package http

import (
	"github.com/residwi/go-api-project-template/internal/modules/shipping"
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

	// Still served by the husk service until tasks 5-7 extract them.
	adm := &adminHandler{service: deps.Service, validator: deps.Validator}
	admin.HandleFunc("POST /orders/{id}/ship", adm.CreateShipment)
	admin.HandleFunc("PUT /shipments/{id}/tracking", adm.UpdateTracking)
	admin.HandleFunc("POST /shipments/{id}/deliver", adm.MarkDelivered)
}
