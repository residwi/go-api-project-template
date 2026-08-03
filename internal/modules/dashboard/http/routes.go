package http

import (
	"github.com/residwi/go-api-project-template/internal/modules/dashboard"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

type RouteDeps struct {
	Service *dashboard.Service
}

func RegisterRoutes(admin *middleware.RouteGroup, deps RouteDeps) {
	h := &adminHandler{service: deps.Service}

	admin.HandleFunc("GET /dashboard/summary", h.Summary)
	admin.HandleFunc("GET /dashboard/top-products", h.TopProducts)
	admin.HandleFunc("GET /dashboard/revenue", h.Revenue)
}
