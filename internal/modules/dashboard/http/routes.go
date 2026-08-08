package http

import (
	"github.com/residwi/go-api-project-template/internal/modules/dashboard"
	revenuehttp "github.com/residwi/go-api-project-template/internal/modules/dashboard/revenue/http"
	summaryhttp "github.com/residwi/go-api-project-template/internal/modules/dashboard/summary/http"
	topproductshttp "github.com/residwi/go-api-project-template/internal/modules/dashboard/topproducts/http"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

type RouteDeps struct {
	Module *dashboard.Module
}

func RegisterRoutes(admin *middleware.RouteGroup, deps RouteDeps) {
	summaryhttp.New(deps.Module.Summary).RegisterHTTP(admin)
	topproductshttp.New(deps.Module.TopProducts).RegisterHTTP(admin)
	revenuehttp.New(deps.Module.Revenue).RegisterHTTP(admin)
}
