package routes

import (
	"github.com/residwi/go-api-project-template/internal/modules/cart"
	addhttp "github.com/residwi/go-api-project-template/internal/modules/cart/usecase/add/http"
	clearhttp "github.com/residwi/go-api-project-template/internal/modules/cart/usecase/clear/http"
	queryhttp "github.com/residwi/go-api-project-template/internal/modules/cart/usecase/query/http"
	removehttp "github.com/residwi/go-api-project-template/internal/modules/cart/usecase/remove/http"
	updatequantityhttp "github.com/residwi/go-api-project-template/internal/modules/cart/usecase/updatequantity/http"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

func Cart(authed *middleware.RouteGroup, m *cart.Module, v *validator.Validator) {
	authed.HandleFunc("GET /cart", queryhttp.New(m.Query).Get)
	authed.HandleFunc("POST /cart/items", addhttp.New(m.Add, v).Add)
	authed.HandleFunc("PUT /cart/items/{product_id}", updatequantityhttp.New(m.UpdateQuantity, v).Update)
	authed.HandleFunc("DELETE /cart/items/{product_id}", removehttp.New(m.Remove).Remove)
	authed.HandleFunc("DELETE /cart", clearhttp.New(m.ClearCart).Clear)
}
