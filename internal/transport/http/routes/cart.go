package routes

import (
	"github.com/residwi/go-api-project-template/internal/modules/cart"
	carthttp "github.com/residwi/go-api-project-template/internal/modules/cart/adapter/http"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

func Cart(authed *middleware.RouteGroup, m *cart.Service, v *validator.Validator) {
	h := carthttp.NewHandler(m, v)
	authed.HandleFunc("GET /cart", h.Get)
	authed.HandleFunc("POST /cart/items", h.Add)
	authed.HandleFunc("PUT /cart/items/{product_id}", h.Update)
	authed.HandleFunc("DELETE /cart/items/{product_id}", h.Remove)
	authed.HandleFunc("DELETE /cart", h.Clear)
}
