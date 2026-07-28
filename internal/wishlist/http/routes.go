package http

import (
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/wishlist"
)

type RouteDeps struct {
	Validator *validator.Validator
	Service   *wishlist.Service
}

// handler holds the dependencies shared by every endpoint in this package.
// Each endpoint's request/response DTOs and mapper live in their own file
// alongside the method that uses them; only this shared struct lives here.
type handler struct {
	service   *wishlist.Service
	validator *validator.Validator
}

func RegisterRoutes(authed *middleware.RouteGroup, deps RouteDeps) {
	h := &handler{service: deps.Service, validator: deps.Validator}

	authed.HandleFunc("GET /wishlist", h.GetWishlist)
	authed.HandleFunc("POST /wishlist/items", h.AddItem)
	authed.HandleFunc("DELETE /wishlist/items/{product_id}", h.RemoveItem)
}
