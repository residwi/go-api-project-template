package http

import (
	"github.com/residwi/go-api-project-template/internal/modules/dashboard"
	summaryhttp "github.com/residwi/go-api-project-template/internal/modules/dashboard/summary/http"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

type RouteDeps struct {
	Service *dashboard.Service
	Module  *dashboard.Module
}

func RegisterRoutes(admin *middleware.RouteGroup, deps RouteDeps) {
	summaryhttp.New(deps.Module.Summary).RegisterHTTP(admin)

	// Still served by the husk service until the topproducts and revenue slices
	// extract them.
	h := &adminHandler{service: deps.Service}
	admin.HandleFunc("GET /dashboard/top-products", h.TopProducts)
	admin.HandleFunc("GET /dashboard/revenue", h.Revenue)
}
