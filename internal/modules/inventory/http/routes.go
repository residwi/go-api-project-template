package http

import (
	"github.com/residwi/go-api-project-template/internal/modules/inventory"
	adjusthttp "github.com/residwi/go-api-project-template/internal/modules/inventory/adjust/http"
	queryhttp "github.com/residwi/go-api-project-template/internal/modules/inventory/query/http"
	restockhttp "github.com/residwi/go-api-project-template/internal/modules/inventory/restock/http"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

type RouteDeps struct {
	Validator *validator.Validator
	Module    *inventory.Module
}

func RegisterRoutes(admin *middleware.RouteGroup, deps RouteDeps) {
	queryhttp.New(deps.Module.Query).RegisterHTTP(admin)
	restockhttp.New(deps.Module.Restock, deps.Validator).RegisterHTTP(admin)
	adjusthttp.New(deps.Module.Adjust, deps.Validator).RegisterHTTP(admin)
}
