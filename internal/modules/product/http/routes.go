package http

import (
	"github.com/residwi/go-api-project-template/internal/modules/product"
	createhttp "github.com/residwi/go-api-project-template/internal/modules/product/create/http"
	queryhttp "github.com/residwi/go-api-project-template/internal/modules/product/query/http"
	removehttp "github.com/residwi/go-api-project-template/internal/modules/product/remove/http"
	updatehttp "github.com/residwi/go-api-project-template/internal/modules/product/update/http"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

type RouteDeps struct {
	Validator *validator.Validator
	Module    *product.Module
}

func RegisterRoutes(api, admin *middleware.RouteGroup, deps RouteDeps) {
	queryhttp.New(deps.Module.Query).RegisterHTTP(api)
	queryhttp.NewAdmin(deps.Module.Query).RegisterHTTP(admin)

	createhttp.New(deps.Module.Create, deps.Validator).RegisterHTTP(admin)
	updatehttp.New(deps.Module.Update, deps.Validator).RegisterHTTP(admin)
	removehttp.New(deps.Module.Delete).RegisterHTTP(admin)
}
