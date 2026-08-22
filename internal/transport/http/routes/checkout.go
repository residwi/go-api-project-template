package routes

import (
	"net/http"

	"github.com/residwi/go-api-project-template/internal/modules/checkout"
	checkouthttp "github.com/residwi/go-api-project-template/internal/modules/checkout/adapter/http"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

func Checkout(
	authed *middleware.RouteGroup,
	s *checkout.Service,
	v *validator.Validator,
	writeLimiter middleware.Middleware,
) {
	place := checkouthttp.NewHandler(s, v)
	authed.Handle("POST /orders", writeLimiter(http.HandlerFunc(place.Place)))

	retry := checkouthttp.NewRetryHandler(s, v)
	authed.Handle("POST /orders/{id}/pay", writeLimiter(http.HandlerFunc(retry.Retry)))
}
