package http

import (
	"github.com/residwi/go-api-project-template/internal/modules/auth"
	loginhttp "github.com/residwi/go-api-project-template/internal/modules/auth/login/http"
	refreshhttp "github.com/residwi/go-api-project-template/internal/modules/auth/refresh/http"
	registerhttp "github.com/residwi/go-api-project-template/internal/modules/auth/register/http"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

type RouteDeps struct {
	Validator *validator.Validator
	Module    *auth.Module
}

func RegisterRoutes(api *middleware.RouteGroup, deps RouteDeps) {
	registerhttp.New(deps.Module.Register, deps.Validator).RegisterHTTP(api)
	loginhttp.New(deps.Module.Login, deps.Validator).RegisterHTTP(api)
	refreshhttp.New(deps.Module.Refresh, deps.Validator).RegisterHTTP(api)
}
