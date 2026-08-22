package routes

import (
	"github.com/residwi/go-api-project-template/internal/modules/wishlist"
	wishlisthttp "github.com/residwi/go-api-project-template/internal/modules/wishlist/adapter/http"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

func Wishlist(authed *middleware.RouteGroup, s *wishlist.Service, v *validator.Validator) {
	h := wishlisthttp.NewHandler(s, v)
	authed.HandleFunc("GET /wishlist", h.List)
	authed.HandleFunc("POST /wishlist/items", h.Add)
	authed.HandleFunc("DELETE /wishlist/items/{product_id}", h.Remove)
}
