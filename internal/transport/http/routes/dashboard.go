package routes

import (
	"github.com/residwi/go-api-project-template/internal/modules/dashboard"
	revenuehttp "github.com/residwi/go-api-project-template/internal/modules/dashboard/revenue/http"
	summaryhttp "github.com/residwi/go-api-project-template/internal/modules/dashboard/summary/http"
	topproductshttp "github.com/residwi/go-api-project-template/internal/modules/dashboard/topproducts/http"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

func Dashboard(admin *middleware.RouteGroup, m *dashboard.Module) {
	admin.HandleFunc("GET /dashboard/summary", summaryhttp.New(m.Summary).Summary)
	admin.HandleFunc("GET /dashboard/top-products", topproductshttp.New(m.TopProducts).TopProducts)
	admin.HandleFunc("GET /dashboard/revenue", revenuehttp.New(m.Revenue).Revenue)
}
