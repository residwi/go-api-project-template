package routes

import (
	"github.com/residwi/go-api-project-template/internal/modules/order"
	changestatushttp "github.com/residwi/go-api-project-template/internal/modules/order/usecase/changestatus/http"
	queryhttp "github.com/residwi/go-api-project-template/internal/modules/order/usecase/query/http"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

func Order(
	authed, admin *middleware.RouteGroup,
	m *order.Module,
	v *validator.Validator,
) {
	query := queryhttp.New(m.Query)
	authed.HandleFunc("GET /orders", query.List)
	authed.HandleFunc("GET /orders/{id}", query.Get)

	adminQuery := queryhttp.NewAdmin(m.Query)
	admin.HandleFunc("GET /orders", adminQuery.List)
	admin.HandleFunc("GET /orders/{id}", adminQuery.Get)

	adminStatus := changestatushttp.NewAdmin(m.ChangeStatus, v)
	admin.HandleFunc("PUT /orders/{id}/status", adminStatus.UpdateStatus)
}
