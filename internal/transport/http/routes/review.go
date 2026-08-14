package routes

import (
	"github.com/residwi/go-api-project-template/internal/modules/review"
	createhttp "github.com/residwi/go-api-project-template/internal/modules/review/usecase/create/http"
	queryhttp "github.com/residwi/go-api-project-template/internal/modules/review/usecase/query/http"
	removehttp "github.com/residwi/go-api-project-template/internal/modules/review/usecase/remove/http"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

func Review(api, authed, admin *middleware.RouteGroup, m *review.Module, v *validator.Validator) {
	api.HandleFunc("GET /products/{id}/reviews", queryhttp.New(m.Query).List)
	authed.HandleFunc("POST /products/{id}/reviews", createhttp.New(m.Create, v).Create)
	admin.HandleFunc("DELETE /reviews/{id}", removehttp.New(m.Delete).Delete)
}
