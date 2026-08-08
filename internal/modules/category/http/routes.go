package http

import (
	"github.com/residwi/go-api-project-template/internal/modules/category"
	createhttp "github.com/residwi/go-api-project-template/internal/modules/category/create/http"
	queryhttp "github.com/residwi/go-api-project-template/internal/modules/category/query/http"
	removehttp "github.com/residwi/go-api-project-template/internal/modules/category/remove/http"
	updatehttp "github.com/residwi/go-api-project-template/internal/modules/category/update/http"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

type RouteDeps struct {
	Validator *validator.Validator
	Module    *category.Module
}

func RegisterRoutes(api, admin *middleware.RouteGroup, deps RouteDeps) {
	queryhttp.New(deps.Module.Query).RegisterHTTP(api)
	createhttp.New(deps.Module.Create, deps.Validator).RegisterHTTP(admin)
	updatehttp.New(deps.Module.Update, deps.Validator).RegisterHTTP(admin)
	removehttp.New(deps.Module.Delete).RegisterHTTP(admin)
}
