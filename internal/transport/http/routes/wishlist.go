package routes

import (
	"github.com/residwi/go-api-project-template/internal/modules/wishlist"
	addhttp "github.com/residwi/go-api-project-template/internal/modules/wishlist/add/http"
	queryhttp "github.com/residwi/go-api-project-template/internal/modules/wishlist/query/http"
	removehttp "github.com/residwi/go-api-project-template/internal/modules/wishlist/remove/http"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

func Wishlist(authed *middleware.RouteGroup, m *wishlist.Module, v *validator.Validator) {
	authed.HandleFunc("GET /wishlist", queryhttp.New(m.Query).List)
	authed.HandleFunc("POST /wishlist/items", addhttp.New(m.AddItem, v).Add)
	authed.HandleFunc("DELETE /wishlist/items/{product_id}", removehttp.New(m.RemoveItem).Remove)
}
