package routes

import (
	"github.com/residwi/go-api-project-template/internal/modules/promotion"
	applyhttp "github.com/residwi/go-api-project-template/internal/modules/promotion/apply/http"
	createhttp "github.com/residwi/go-api-project-template/internal/modules/promotion/create/http"
	queryhttp "github.com/residwi/go-api-project-template/internal/modules/promotion/query/http"
	removehttp "github.com/residwi/go-api-project-template/internal/modules/promotion/remove/http"
	updatehttp "github.com/residwi/go-api-project-template/internal/modules/promotion/update/http"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

func Promotion(authed, admin *middleware.RouteGroup, m *promotion.Module, v *validator.Validator) {
	authed.HandleFunc("POST /promotions/apply", applyhttp.New(m.Apply, v).Apply)

	admin.HandleFunc("POST /promotions", createhttp.New(m.Create, v).Create)
	admin.HandleFunc("GET /promotions", queryhttp.New(m.Query).List)
	admin.HandleFunc("PUT /promotions/{id}", updatehttp.New(m.Update, v).Update)
	admin.HandleFunc("DELETE /promotions/{id}", removehttp.New(m.Delete).Delete)
}
