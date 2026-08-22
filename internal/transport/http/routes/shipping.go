package routes

import (
	"github.com/residwi/go-api-project-template/internal/modules/shipping"
	shippinghttp "github.com/residwi/go-api-project-template/internal/modules/shipping/adapter/http"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

func Shipping(authed, admin *middleware.RouteGroup, s *shipping.Service, v *validator.Validator) {
	authed.HandleFunc("GET /orders/{id}/shipping", shippinghttp.NewHandler(s).Get)

	adminH := shippinghttp.NewAdminHandler(s, v)
	admin.HandleFunc("POST /orders/{id}/ship", adminH.Create)
	admin.HandleFunc("PUT /shipments/{id}/tracking", adminH.UpdateTracking)
	admin.HandleFunc("POST /shipments/{id}/deliver", adminH.Deliver)
}
