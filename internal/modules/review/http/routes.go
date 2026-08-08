package http

import (
	"github.com/residwi/go-api-project-template/internal/modules/review"
	createhttp "github.com/residwi/go-api-project-template/internal/modules/review/create/http"
	queryhttp "github.com/residwi/go-api-project-template/internal/modules/review/query/http"
	removehttp "github.com/residwi/go-api-project-template/internal/modules/review/remove/http"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

type RouteDeps struct {
	Validator *validator.Validator
	Module    *review.Module
}

func RegisterRoutes(api, authed, admin *middleware.RouteGroup, deps RouteDeps) {
	queryhttp.New(deps.Module.Query).RegisterHTTP(api)
	createhttp.New(deps.Module.Create, deps.Validator).RegisterHTTP(authed)
	removehttp.New(deps.Module.Delete).RegisterHTTP(admin)
}
