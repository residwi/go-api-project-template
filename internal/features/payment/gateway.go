package payment

import "context"

type GatewayChargeRequest struct {
	IdempotencyKey  string
	OrderID         string
	Amount          int64
	Currency        string
	Description     string
	PaymentMethodID string
	ReturnURL       string
	Metadata        map[string]string
}

type GatewayChargeResponse struct {
	TransactionID string
	Status        string
	PaymentURL    string
}

type GatewayRefundRequest struct {
	IdempotencyKey string
	TransactionID  string
	Amount         int64
	Reason         string
}

type GatewayRefundResponse struct {
	RefundID string
	Status   string
}

type Gateway interface {
	Charge(ctx context.Context, req GatewayChargeRequest) (GatewayChargeResponse, error)
	Refund(ctx context.Context, req GatewayRefundRequest) (GatewayRefundResponse, error)
}
