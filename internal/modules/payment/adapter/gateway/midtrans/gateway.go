package midtrans

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/residwi/go-api-project-template/internal/modules/payment/adapter/gateway"
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
	return gateway.ChargeResponse{}, errors.New("midtrans: not implemented")
}

func (g *Gateway) Refund(_ context.Context, _ gateway.RefundRequest) (gateway.RefundResponse, error) {
	return gateway.RefundResponse{}, errors.New("midtrans: not implemented")
}
