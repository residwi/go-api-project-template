package user

import (
	"github.com/residwi/go-api-project-template/internal/middleware"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
)

type RouteDeps struct {
	Validator *validator.Validator
	Service   *Service
}

func RegisterRoutes(authed *middleware.RouteGroup, adminGroup *middleware.RouteGroup, deps RouteDeps) {
	pub := &publicHandler{service: deps.Service, validator: deps.Validator}
	admin := &adminHandler{service: deps.Service, validator: deps.Validator}

	authed.HandleFunc("GET /users/me", pub.Me)
	authed.HandleFunc("PUT /users/me", pub.UpdateProfile)

	adminGroup.HandleFunc("GET /users", admin.List)
	adminGroup.HandleFunc("GET /users/{id}", admin.Get)
	adminGroup.HandleFunc("PUT /users/{id}", admin.Update)
	adminGroup.HandleFunc("PUT /users/{id}/role", admin.UpdateRole)
	adminGroup.HandleFunc("DELETE /users/{id}", admin.Delete)
}
