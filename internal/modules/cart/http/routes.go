package http

import (
	"github.com/residwi/go-api-project-template/internal/modules/cart"
	addhttp "github.com/residwi/go-api-project-template/internal/modules/cart/add/http"
	emptyhttp "github.com/residwi/go-api-project-template/internal/modules/cart/empty/http"
	queryhttp "github.com/residwi/go-api-project-template/internal/modules/cart/query/http"
	removehttp "github.com/residwi/go-api-project-template/internal/modules/cart/remove/http"
	updatequantityhttp "github.com/residwi/go-api-project-template/internal/modules/cart/updatequantity/http"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

type RouteDeps struct {
	Validator *validator.Validator
	Module    *cart.Module
}

// RegisterRoutes mounts every routed slice. lock/ registers no route:
// order/place is its only caller.
func RegisterRoutes(authed *middleware.RouteGroup, deps RouteDeps) {
	queryhttp.New(deps.Module.Query).RegisterHTTP(authed)
	addhttp.New(deps.Module.Add, deps.Validator).RegisterHTTP(authed)
	updatequantityhttp.New(deps.Module.UpdateQuantity, deps.Validator).RegisterHTTP(authed)
	removehttp.New(deps.Module.Remove).RegisterHTTP(authed)
	emptyhttp.New(deps.Module.Empty).RegisterHTTP(authed)
}
