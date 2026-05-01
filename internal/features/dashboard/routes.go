package dashboard

import (
	"github.com/residwi/go-api-project-template/internal/middleware"
)

type RouteDeps struct {
	Service *Service
}

func RegisterRoutes(admin *middleware.RouteGroup, deps RouteDeps) {
	h := &adminHandler{service: deps.Service}

	admin.HandleFunc("GET /dashboard/summary", h.Summary)
	admin.HandleFunc("GET /dashboard/top-products", h.TopProducts)
	admin.HandleFunc("GET /dashboard/revenue", h.Revenue)
}
