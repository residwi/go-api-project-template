package stripe

import (
	"context"
	"errors"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/residwi/go-api-project-template/internal/features/payment"
)

type Gateway struct {
	apiKey     string
	httpClient *http.Client
}

func New(apiKey string, timeout time.Duration) *Gateway {
	return &Gateway{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout:   timeout,
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		},
	}
}

func (g *Gateway) Charge(_ context.Context, _ payment.GatewayChargeRequest) (payment.GatewayChargeResponse, error) {
	return payment.GatewayChargeResponse{}, errors.New("stripe: not implemented")
}

func (g *Gateway) Refund(_ context.Context, _ payment.GatewayRefundRequest) (payment.GatewayRefundResponse, error) {
	return payment.GatewayRefundResponse{}, errors.New("stripe: not implemented")
}
