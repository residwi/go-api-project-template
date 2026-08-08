package http

import (
	"github.com/residwi/go-api-project-template/internal/modules/wishlist"
	addhttp "github.com/residwi/go-api-project-template/internal/modules/wishlist/add/http"
	queryhttp "github.com/residwi/go-api-project-template/internal/modules/wishlist/query/http"
	removehttp "github.com/residwi/go-api-project-template/internal/modules/wishlist/remove/http"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

type RouteDeps struct {
	Validator *validator.Validator
	Module    *wishlist.Module
}

func RegisterRoutes(authed *middleware.RouteGroup, deps RouteDeps) {
	queryhttp.New(deps.Module.Query).RegisterHTTP(authed)
	addhttp.New(deps.Module.AddItem, deps.Validator).RegisterHTTP(authed)
	removehttp.New(deps.Module.RemoveItem).RegisterHTTP(authed)
}
