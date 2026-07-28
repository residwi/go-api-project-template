package http

import (
	"github.com/residwi/go-api-project-template/internal/cart"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

type RouteDeps struct {
	Validator *validator.Validator
	Service   *cart.Service
}

// handler holds the dependencies shared by every endpoint in this package.
// Each endpoint's request/response DTOs and mapper live in their own file
// alongside the method that uses them; only this shared struct lives here.
type handler struct {
	service   *cart.Service
	validator *validator.Validator
}

func RegisterRoutes(authed *middleware.RouteGroup, deps RouteDeps) {
	h := &handler{service: deps.Service, validator: deps.Validator}

	authed.HandleFunc("GET /cart", h.GetCart)
	authed.HandleFunc("POST /cart/items", h.AddItem)
	authed.HandleFunc("PUT /cart/items/{product_id}", h.UpdateItem)
	authed.HandleFunc("DELETE /cart/items/{product_id}", h.RemoveItem)
	authed.HandleFunc("DELETE /cart", h.Clear)
}
