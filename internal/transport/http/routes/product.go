package routes

import (
	"github.com/residwi/go-api-project-template/internal/modules/product"
	createhttp "github.com/residwi/go-api-project-template/internal/modules/product/create/http"
	queryhttp "github.com/residwi/go-api-project-template/internal/modules/product/query/http"
	removehttp "github.com/residwi/go-api-project-template/internal/modules/product/remove/http"
	updatehttp "github.com/residwi/go-api-project-template/internal/modules/product/update/http"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

func Product(api, admin *middleware.RouteGroup, m *product.Module, v *validator.Validator) {
	query := queryhttp.New(m.Query)
	api.HandleFunc("GET /products", query.List)
	api.HandleFunc("GET /products/{slug}", query.GetBySlug)

	adminQuery := queryhttp.NewAdmin(m.Query)
	admin.HandleFunc("GET /products", adminQuery.List)
	admin.HandleFunc("GET /products/{id}", adminQuery.Get)

	admin.HandleFunc("POST /products", createhttp.New(m.Create, v).Create)
	admin.HandleFunc("PUT /products/{id}", updatehttp.New(m.Update, v).Update)
	admin.HandleFunc("DELETE /products/{id}", removehttp.New(m.Delete).Delete)
}
