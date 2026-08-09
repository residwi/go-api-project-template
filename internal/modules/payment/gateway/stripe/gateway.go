package stripe

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/residwi/go-api-project-template/internal/modules/payment/gateway"
)

type Gateway struct {
	apiKey     string
	httpClient *http.Client
}

func New(apiKey string, timeout time.Duration) *Gateway {
	return &Gateway{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: timeout},
	}
}

func (g *Gateway) Charge(_ context.Context, _ gateway.ChargeRequest) (gateway.ChargeResponse, error) {
	return gateway.ChargeResponse{}, errors.New("stripe: not implemented")
}

func (g *Gateway) Refund(_ context.Context, _ gateway.RefundRequest) (gateway.RefundResponse, error) {
	return gateway.RefundResponse{}, errors.New("stripe: not implemented")
}
