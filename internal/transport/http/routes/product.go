package routes

import (
	"github.com/residwi/go-api-project-template/internal/modules/product"
	producthttp "github.com/residwi/go-api-project-template/internal/modules/product/adapter/http"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

func Product(api, admin *middleware.RouteGroup, s *product.Service, v *validator.Validator) {
	h := producthttp.NewHandler(s)
	api.HandleFunc("GET /products", h.List)
	api.HandleFunc("GET /products/{slug}", h.GetBySlug)

	adminH := producthttp.NewAdminHandler(s, v)
	admin.HandleFunc("GET /products", adminH.List)
	admin.HandleFunc("GET /products/{id}", adminH.Get)
	admin.HandleFunc("POST /products", adminH.Create)
	admin.HandleFunc("PUT /products/{id}", adminH.Update)
	admin.HandleFunc("DELETE /products/{id}", adminH.Delete)
}
