package http

import (
	"github.com/residwi/go-api-project-template/internal/modules/notification"
	markallreadhttp "github.com/residwi/go-api-project-template/internal/modules/notification/markallread/http"
	markreadhttp "github.com/residwi/go-api-project-template/internal/modules/notification/markread/http"
	queryhttp "github.com/residwi/go-api-project-template/internal/modules/notification/query/http"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

type RouteDeps struct {
	Module *notification.Module
}

func RegisterRoutes(authed *middleware.RouteGroup, deps RouteDeps) {
	queryhttp.New(deps.Module.Query).RegisterHTTP(authed)
	markreadhttp.New(deps.Module.MarkRead).RegisterHTTP(authed)
	markallreadhttp.New(deps.Module.MarkAllRead).RegisterHTTP(authed)
}
