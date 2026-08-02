package http

import (
	"github.com/residwi/go-api-project-template/internal/modules/promotion"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

type RouteDeps struct {
	Validator *validator.Validator
	Service   *promotion.Service
}

func RegisterRoutes(authed *middleware.RouteGroup, admin *middleware.RouteGroup, deps RouteDeps) {
	pub := &publicHandler{service: deps.Service, validator: deps.Validator}
	adm := &adminHandler{service: deps.Service, validator: deps.Validator}

	authed.HandleFunc("POST /promotions/apply", pub.Apply)

	admin.HandleFunc("POST /promotions", adm.Create)
	admin.HandleFunc("GET /promotions", adm.List)
	admin.HandleFunc("PUT /promotions/{id}", adm.Update)
	admin.HandleFunc("DELETE /promotions/{id}", adm.Delete)
}
