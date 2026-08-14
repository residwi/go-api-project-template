package routes

import (
	"github.com/residwi/go-api-project-template/internal/modules/shipping"
	createhttp "github.com/residwi/go-api-project-template/internal/modules/shipping/create/http"
	deliverhttp "github.com/residwi/go-api-project-template/internal/modules/shipping/deliver/http"
	queryhttp "github.com/residwi/go-api-project-template/internal/modules/shipping/query/http"
	updatetrackinghttp "github.com/residwi/go-api-project-template/internal/modules/shipping/updatetracking/http"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

func Shipping(authed, admin *middleware.RouteGroup, m *shipping.Module, v *validator.Validator) {
	authed.HandleFunc("GET /orders/{id}/shipping", queryhttp.New(m.Query).Get)

	admin.HandleFunc("POST /orders/{id}/ship", createhttp.New(m.Create, v).Create)
	admin.HandleFunc("PUT /shipments/{id}/tracking", updatetrackinghttp.New(m.UpdateTracking, v).UpdateTracking)
	admin.HandleFunc("POST /shipments/{id}/deliver", deliverhttp.New(m.Deliver).Deliver)
}
