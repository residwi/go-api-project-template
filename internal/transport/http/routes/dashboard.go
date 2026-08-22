package routes

import (
	"github.com/residwi/go-api-project-template/internal/modules/dashboard"
	dashboardhttp "github.com/residwi/go-api-project-template/internal/modules/dashboard/adapter/http"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

func Dashboard(admin *middleware.RouteGroup, s *dashboard.Service) {
	h := dashboardhttp.NewHandler(s)
	admin.HandleFunc("GET /dashboard/summary", h.Summary)
	admin.HandleFunc("GET /dashboard/top-products", h.TopProducts)
	admin.HandleFunc("GET /dashboard/revenue", h.Revenue)
}
