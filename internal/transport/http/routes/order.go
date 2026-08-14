package routes

import (
	"net/http"

	"github.com/residwi/go-api-project-template/internal/modules/order"
	cancelhttp "github.com/residwi/go-api-project-template/internal/modules/order/cancel/http"
	changestatushttp "github.com/residwi/go-api-project-template/internal/modules/order/changestatus/http"
	placehttp "github.com/residwi/go-api-project-template/internal/modules/order/place/http"
	queryhttp "github.com/residwi/go-api-project-template/internal/modules/order/query/http"
	retrypaymenthttp "github.com/residwi/go-api-project-template/internal/modules/order/retrypayment/http"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

func Order(
	authed, admin *middleware.RouteGroup,
	m *order.Module,
	v *validator.Validator,
	writeLimiter middleware.Middleware,
) {
	place := placehttp.New(m.Place, v)
	authed.Handle("POST /orders", writeLimiter(http.HandlerFunc(place.Place)))

	retry := retrypaymenthttp.New(m.RetryPayment, v)
	authed.Handle("POST /orders/{id}/pay", writeLimiter(http.HandlerFunc(retry.Retry)))

	query := queryhttp.New(m.Query)
	authed.HandleFunc("GET /orders", query.List)
	authed.HandleFunc("GET /orders/{id}", query.Get)

	authed.HandleFunc("POST /orders/{id}/cancel", cancelhttp.New(m.Cancel).Cancel)

	adminQuery := queryhttp.NewAdmin(m.Query)
	admin.HandleFunc("GET /orders", adminQuery.List)
	admin.HandleFunc("GET /orders/{id}", adminQuery.Get)

	adminStatus := changestatushttp.NewAdmin(m.ChangeStatus, v)
	admin.HandleFunc("PUT /orders/{id}/status", adminStatus.UpdateStatus)
}
