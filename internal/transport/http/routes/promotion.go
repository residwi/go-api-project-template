package routes

import (
	"github.com/residwi/go-api-project-template/internal/modules/promotion"
	promotionhttp "github.com/residwi/go-api-project-template/internal/modules/promotion/adapter/http"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

func Promotion(authed, admin *middleware.RouteGroup, s *promotion.Service, v *validator.Validator) {
	h := promotionhttp.NewHandler(s, v)
	authed.HandleFunc("POST /promotions/apply", h.Apply)

	adminH := promotionhttp.NewAdminHandler(s, v)
	admin.HandleFunc("POST /promotions", adminH.Create)
	admin.HandleFunc("GET /promotions", adminH.List)
	admin.HandleFunc("PUT /promotions/{id}", adminH.Update)
	admin.HandleFunc("DELETE /promotions/{id}", adminH.Delete)
}
