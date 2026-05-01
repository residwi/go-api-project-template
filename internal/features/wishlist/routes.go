package wishlist

import (
	"github.com/residwi/go-api-project-template/internal/middleware"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
)

type RouteDeps struct {
	Validator *validator.Validator
	Service   *Service
}

func RegisterRoutes(authed *middleware.RouteGroup, deps RouteDeps) {
	h := &handler{service: deps.Service, validator: deps.Validator}

	authed.HandleFunc("GET /wishlist", h.GetWishlist)
	authed.HandleFunc("POST /wishlist/items", h.AddItem)
	authed.HandleFunc("DELETE /wishlist/items/{product_id}", h.RemoveItem)
}
