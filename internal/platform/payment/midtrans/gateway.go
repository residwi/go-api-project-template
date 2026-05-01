package midtrans

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/residwi/go-api-project-template/internal/platform/payment"
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

func (g *Gateway) Charge(_ context.Context, _ payment.ChargeRequest) (payment.ChargeResponse, error) {
	return payment.ChargeResponse{}, errors.New("midtrans: not implemented")
}

func (g *Gateway) Refund(_ context.Context, _ payment.RefundRequest) (payment.RefundResponse, error) {
	return payment.RefundResponse{}, errors.New("midtrans: not implemented")
}
