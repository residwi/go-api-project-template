package midtrans

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/residwi/go-api-project-template/internal/features/payment"
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

func (g *Gateway) Charge(_ context.Context, _ payment.GatewayChargeRequest) (payment.GatewayChargeResponse, error) {
	return payment.GatewayChargeResponse{}, errors.New("midtrans: not implemented")
}

func (g *Gateway) Refund(_ context.Context, _ payment.GatewayRefundRequest) (payment.GatewayRefundResponse, error) {
	return payment.GatewayRefundResponse{}, errors.New("midtrans: not implemented")
}
