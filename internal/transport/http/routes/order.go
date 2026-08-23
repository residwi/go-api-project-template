package routes

import (
	"github.com/residwi/go-api-project-template/internal/modules/order"
	orderhttp "github.com/residwi/go-api-project-template/internal/modules/order/adapter/http"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

func Order(
	authed, admin *middleware.RouteGroup,
	s *order.Service,
	v *validator.Validator,
) {
	handler := orderhttp.NewHandler(s)
	authed.HandleFunc("GET /orders", handler.List)
	authed.HandleFunc("GET /orders/{id}", handler.Get)

	adminHandler := orderhttp.NewAdminHandler(s, v)
	admin.HandleFunc("GET /orders", adminHandler.List)
	admin.HandleFunc("GET /orders/{id}", adminHandler.Get)
	admin.HandleFunc("PUT /orders/{id}/status", adminHandler.UpdateStatus)
}
