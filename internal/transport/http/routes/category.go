package routes

import (
	"github.com/residwi/go-api-project-template/internal/modules/category"
	createhttp "github.com/residwi/go-api-project-template/internal/modules/category/create/http"
	queryhttp "github.com/residwi/go-api-project-template/internal/modules/category/query/http"
	removehttp "github.com/residwi/go-api-project-template/internal/modules/category/remove/http"
	updatehttp "github.com/residwi/go-api-project-template/internal/modules/category/update/http"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

func Category(api, admin *middleware.RouteGroup, m *category.Module, v *validator.Validator) {
	query := queryhttp.New(m.Query)
	api.HandleFunc("GET /categories", query.List)
	api.HandleFunc("GET /categories/{slug}", query.GetBySlug)

	admin.HandleFunc("POST /categories", createhttp.New(m.Create, v).Create)
	admin.HandleFunc("PUT /categories/{id}", updatehttp.New(m.Update, v).Update)
	admin.HandleFunc("DELETE /categories/{id}", removehttp.New(m.Delete).Delete)
}
