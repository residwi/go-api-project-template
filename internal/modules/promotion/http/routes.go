package http

import (
	"github.com/residwi/go-api-project-template/internal/modules/promotion"
	applyhttp "github.com/residwi/go-api-project-template/internal/modules/promotion/apply/http"
	createhttp "github.com/residwi/go-api-project-template/internal/modules/promotion/create/http"
	queryhttp "github.com/residwi/go-api-project-template/internal/modules/promotion/query/http"
	removehttp "github.com/residwi/go-api-project-template/internal/modules/promotion/remove/http"
	updatehttp "github.com/residwi/go-api-project-template/internal/modules/promotion/update/http"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

type RouteDeps struct {
	Validator *validator.Validator
	Module    *promotion.Module
}

func RegisterRoutes(authed, admin *middleware.RouteGroup, deps RouteDeps) {
	applyhttp.New(deps.Module.Apply, deps.Validator).RegisterHTTP(authed)

	createhttp.New(deps.Module.Create, deps.Validator).RegisterHTTP(admin)
	queryhttp.New(deps.Module.Query).RegisterHTTP(admin)
	updatehttp.New(deps.Module.Update, deps.Validator).RegisterHTTP(admin)
	removehttp.New(deps.Module.Delete).RegisterHTTP(admin)
}
