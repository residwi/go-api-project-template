package routes

import (
	"github.com/residwi/go-api-project-template/internal/modules/review"
	reviewhttp "github.com/residwi/go-api-project-template/internal/modules/review/adapter/http"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

func Review(api, authed, admin *middleware.RouteGroup, s *review.Service, v *validator.Validator) {
	h := reviewhttp.NewHandler(s, v)
	api.HandleFunc("GET /products/{id}/reviews", h.List)
	authed.HandleFunc("POST /products/{id}/reviews", h.Create)
	admin.HandleFunc("DELETE /reviews/{id}", reviewhttp.NewAdminHandler(s).Delete)
}
