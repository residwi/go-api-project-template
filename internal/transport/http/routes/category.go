package routes

import (
	"github.com/residwi/go-api-project-template/internal/modules/category"
	categoryhttp "github.com/residwi/go-api-project-template/internal/modules/category/adapter/http"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

func Category(api, admin *middleware.RouteGroup, s *category.Service, v *validator.Validator) {
	h := categoryhttp.NewHandler(s)
	api.HandleFunc("GET /categories", h.List)
	api.HandleFunc("GET /categories/{slug}", h.GetBySlug)

	adminH := categoryhttp.NewAdminHandler(s, v)
	admin.HandleFunc("POST /categories", adminH.Create)
	admin.HandleFunc("PUT /categories/{id}", adminH.Update)
	admin.HandleFunc("DELETE /categories/{id}", adminH.Delete)
}
